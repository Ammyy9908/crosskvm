package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/crosskvm/crosskvm/internal/control"
	"github.com/crosskvm/crosskvm/internal/discovery"
	"github.com/crosskvm/crosskvm/internal/input"
	"github.com/crosskvm/crosskvm/internal/protocol"
	"github.com/crosskvm/crosskvm/internal/transport"
)

var (
	ErrAlreadyConnected = errors.New("daemon: already connected to a peer")
	ErrPeerNotFound     = errors.New("daemon: peer not found")
	ErrNoActiveConn     = errors.New("daemon: no active peer connection")
)

// DaemonService coordinates the full CrossKVM backend for the desktop UI.
type DaemonService struct {
	clipboardConn      atomic.Pointer[transport.Conn]
	imageClipboardConn atomic.Pointer[transport.Conn]
	fileTransferConn   atomic.Pointer[transport.Conn]
	mu                 sync.RWMutex
	logger             *log.Logger
	localDev           *discovery.LocalDevice
	backend            input.InputBackend
	router             *control.Router
	stateMgr           *control.StateManager
	connMgr            *control.ConnectionManager
	peerStore          *discovery.PeerStore
	discService        *discovery.DiscoveryService
	server             *transport.Server
	activeConn         atomic.Pointer[transport.Conn]
	seqCounter         atomic.Uint64
	inputError         string
	captureMu          sync.Mutex
	captureReady       atomic.Bool
	currentPeer        *PeerInfo
	kvmActive          atomic.Bool
	edgeThresh         int
	listenPort         int

	eventsCh chan input.InputEvent
	emitFunc func(event IPCEvent)
	ctx      context.Context
	cancel   context.CancelFunc
	closed   atomic.Bool
	stopCh   chan struct{}
}

// NewDaemonService creates and initializes the daemon service.
func NewDaemonService(listenPort int, logger *log.Logger) (*DaemonService, error) {
	if logger == nil {
		logger = log.New(os.Stdout, "[CrossKVM-Daemon] ", log.LstdFlags|log.Lmsgprefix)
	}

	backend, err := input.NewBackend()
	if err != nil {
		return nil, fmt.Errorf("failed to create input backend: %w", err)
	}

	bounds := backend.ScreenBounds()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		bounds = input.ScreenBounds{Width: 1920, Height: 1080}
	}

	if listenPort <= 0 {
		listenPort = 4545
	}

	localDev, err := discovery.GetLocalDevice(listenPort, bounds.Width, bounds.Height, protocol.CurrentProtocolVersion)
	if err != nil {
		logger.Printf("[WARN] Failed to load local device ID: %v", err)
	}

	if os.Getenv("CROSSKVM_DESKTOP_CLIPBOARD") == "1" {
		localDev.Capabilities = append(localDev.Capabilities, clipboardCapability, fileTransferCapability)
	}

	stateMgr := control.NewStateManager()
	cfg := control.RouterConfig{
		PeerSide:         control.PeerSideRight,
		EdgeThreshold:    3,
		HysteresisOffset: 20,
	}
	router := control.NewRouterWithConfig(stateMgr, backend, cfg, logger)
	connMgr := control.NewConnectionManager(router, router.Metrics(), control.DefaultConnectionConfig(), logger)

	peerStore := discovery.NewPeerStore()
	discService := discovery.NewDiscoveryService(*localDev, peerStore, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())

	ds := &DaemonService{
		logger:      logger,
		localDev:    localDev,
		backend:     backend,
		router:      router,
		stateMgr:    stateMgr,
		connMgr:     connMgr,
		peerStore:   peerStore,
		discService: discService,
		listenPort:  listenPort,
		edgeThresh:  3,
		eventsCh:    make(chan input.InputEvent, 1024),
		ctx:         ctx,
		cancel:      cancel,
		stopCh:      make(chan struct{}),
	}

	return ds, nil
}

// SetEmitter registers the IPC broadcast handler.
func (ds *DaemonService) SetEmitter(emit func(event IPCEvent)) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.emitFunc = emit
}

func (ds *DaemonService) emit(evt IPCEvent) {
	ds.mu.RLock()
	fn := ds.emitFunc
	ds.mu.RUnlock()
	if fn != nil {
		fn(evt)
	}
}

// Start initiates background listeners, discovery, and event loops.
func (ds *DaemonService) Start() error {
	// Start physical input capture (warn if pending permission, will retry on StartKVM)
	if err := ds.ensureCapture(); err != nil {
		ds.logger.Printf("[WARN] Physical input capture notice on startup (will retry on StartKVM): %v", err)
	}

	// Permission may be granted after launch. Retry until capture starts instead
	// of leaving the startup denial displayed indefinitely.
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for !ds.captureReady.Load() {
			select {
			case <-ds.ctx.Done():
				return
			case <-ticker.C:
				_ = ds.ensureCapture()
			}
		}
	}()

	// Capture routing loop
	go func() {
		for {
			select {
			case <-ds.ctx.Done():
				return
			case ev, ok := <-ds.eventsCh:
				if !ok {
					return
				}
				if ds.kvmActive.Load() {
					_ = ds.router.RouteLocalEvent(ev)
				}
			}
		}
	}()

	// Wire state change notifications
	ds.stateMgr.OnStateChange(ds.notifyControlState)

	// Wire peer layout changes received from remote peer
	ds.router.OnPeerSideChange(func(side control.PeerSide) {
		ds.emit(IPCEvent{
			Event: "peer_side_changed",
			Data:  map[string]string{"side": string(side)},
		})
	})

	// Wire connection state notifications
	ds.connMgr.OnStateChange(func(oldState, newState control.ConnState) {
		switch newState {
		case control.ConnStateConnected:
			ds.mu.RLock()
			cp := ds.currentPeer
			side := string(ds.router.PeerSide())
			ds.mu.RUnlock()
			if cp != nil {
				ds.emit(IPCEvent{
					Event: "connected",
					Data: ConnectedEvent{
						Peer: *cp,
						Side: side,
					},
				})
			}
		case control.ConnStateDisconnected:
			ds.emit(IPCEvent{
				Event: "disconnected",
				Data: DisconnectedEvent{
					Reason: "Peer disconnected",
				},
			})
		case control.ConnStateReconnecting:
			ds.emit(IPCEvent{
				Event: "reconnecting",
				Data:  ReconnectingEvent{Attempt: 1},
			})
		}
	})

	// Wire discovery notifications
	ds.discService.OnPeerDiscovered(func(p discovery.Peer) {
		info := FromDiscoveryPeer(p)
		ds.emit(IPCEvent{
			Event: "peer_discovered",
			Data:  info,
		})
	})

	// Start UDP discovery
	if err := ds.discService.Start(ds.ctx); err != nil {
		ds.logger.Printf("[DISCOVERY] Background discovery notice: %v", err)
	}

	// Start inbound TCP KVM listener
	serverHandler := transport.ServerFuncHandler{
		ConnectFunc: func(conn *transport.Conn) {
			ds.handleInboundConnect(conn)
		},
		MessageFunc: func(conn *transport.Conn, msg protocol.Message) {
			ds.handleMessage(conn, msg)
		},
		DisconnectFunc: func(conn *transport.Conn, err error) {
			ds.handleDisconnect(conn, err)
		},
	}

	listenAddr := fmt.Sprintf(":%d", ds.listenPort)
	ds.server = transport.NewServer(listenAddr, serverHandler)
	go func() {
		_ = ds.server.ListenAndServe(ds.ctx)
	}()

	// Periodic metrics push ticker (every 500ms)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ds.ctx.Done():
				return
			case <-ticker.C:
				if ds.connMgr.State() == control.ConnStateConnected {
					snap := ds.GetMetrics()
					ds.emit(IPCEvent{
						Event: "latency_updated",
						Data:  snap,
					})
				}
			}
		}
	}()

	return nil
}

func (ds *DaemonService) handleInboundConnect(conn *transport.Conn) {
	ds.logger.Printf("Inbound peer connected from %s", conn.RemoteAddr().String())
	ds.clipboardConn.Store(nil)
	ds.imageClipboardConn.Store(nil)
	ds.fileTransferConn.Store(nil)
	ds.activeConn.Store(conn)
	ds.router.SetConnection(conn)
	ds.connMgr.RecordActivity()
	ds.connMgr.SetState(control.ConnStateConnecting)

	bounds := ds.backend.ScreenBounds()
	ackMsg, err := protocol.NewHandshakeAckMessageWithDevice(
		ds.seqCounter.Add(1),
		true,
		ds.localDev.ID,
		"kvm daemon listener ready",
		bounds.Width,
		bounds.Height,
		ds.localDev.Capabilities,
	)
	if err == nil {
		_ = conn.Send(ackMsg)
	}
	ds.connMgr.StartHeartbeat(conn)
}

func (ds *DaemonService) handleMessage(conn *transport.Conn, msg protocol.Message) {
	ds.connMgr.RecordActivity()
	if msg.Type == protocol.MessageTypeFileTransfer {
		ds.receiveFileTransfer(conn, msg)
		return
	}
	if msg.Type == protocol.MessageTypeClipboardImage {
		ds.receiveClipboardImage(conn, msg)
		return
	}
	if msg.Type == protocol.MessageTypeClipboardText {
		ds.receiveClipboard(conn, msg)
		return
	}

	if msg.Type == protocol.MessageTypeHandshake {
		var payload protocol.HandshakePayload
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			if err := protocol.ValidateHandshake(payload); err != nil {
				ds.logger.Printf("[SECURITY] Inbound handshake rejected: %v", err)
				ack, _ := protocol.NewHandshakeAckMessage(ds.seqCounter.Add(1), false, err.Error(), 0, 0)
				_ = conn.Send(ack)
				_ = conn.Close()
				return
			}

			ds.router.SetRemoteBounds(input.ScreenBounds{
				Width:  payload.ScreenWidth,
				Height: payload.ScreenHeight,
			})

			remoteHost := conn.RemoteAddr().String()
			hostIP := remoteHost
			if h, _, err := net.SplitHostPort(remoteHost); err == nil {
				hostIP = h
			}
			ds.peerStore.AddOrUpdate(discovery.Peer{
				DeviceID:        payload.DeviceID,
				DeviceName:      payload.PeerName,
				OS:              payload.OS,
				Arch:            payload.Arch,
				Port:            4545,
				Address:         fmt.Sprintf("%s:%d", hostIP, 4545),
				IP:              hostIP,
				ScreenWidth:     payload.ScreenWidth,
				ScreenHeight:    payload.ScreenHeight,
				ProtocolVersion: protocol.CurrentProtocolVersion,
				LastSeen:        time.Now(),
			})

			peerInfo := PeerInfo{
				ID:           payload.DeviceID,
				Name:         payload.PeerName,
				OS:           payload.OS,
				Arch:         payload.Arch,
				Address:      fmt.Sprintf("%s:%d", hostIP, 4545),
				IP:           hostIP,
				Port:         4545,
				ScreenWidth:  payload.ScreenWidth,
				ScreenHeight: payload.ScreenHeight,
				Online:       true,
			}

			ds.mu.Lock()
			ds.currentPeer = &peerInfo
			ds.mu.Unlock()

			ds.connMgr.SetState(control.ConnStateConnected)
			ds.kvmActive.Store(ds.ensureCapture() == nil)
			ds.negotiateClipboard(conn, payload.Capabilities)
		}
	} else if msg.Type == protocol.MessageTypeHandshakeAck {
		var payload protocol.HandshakeAckPayload
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			if payload.Success {
				ds.router.SetRemoteBounds(input.ScreenBounds{
					Width:  payload.ScreenWidth,
					Height: payload.ScreenHeight,
				})
				if payload.DeviceID != "" {
					remoteHost := conn.RemoteAddr().String()
					hostIP := remoteHost
					if h, _, err := net.SplitHostPort(remoteHost); err == nil {
						hostIP = h
					}
					ds.peerStore.AddOrUpdate(discovery.Peer{
						DeviceID:        payload.DeviceID,
						DeviceName:      payload.DeviceID,
						Port:            4545,
						Address:         fmt.Sprintf("%s:%d", hostIP, 4545),
						IP:              hostIP,
						ScreenWidth:     payload.ScreenWidth,
						ScreenHeight:    payload.ScreenHeight,
						ProtocolVersion: protocol.CurrentProtocolVersion,
						LastSeen:        time.Now(),
					})

					ds.mu.Lock()
					if ds.currentPeer == nil || ds.currentPeer.ID == "" {
						if p, ok := ds.peerStore.FindByNameOrID(payload.DeviceID); ok {
							info := FromDiscoveryPeer(p)
							ds.currentPeer = &info
						} else {
							ds.currentPeer = &PeerInfo{
								ID:           payload.DeviceID,
								Name:         payload.DeviceID,
								Address:      fmt.Sprintf("%s:%d", hostIP, 4545),
								IP:           hostIP,
								Port:         4545,
								ScreenWidth:  payload.ScreenWidth,
								ScreenHeight: payload.ScreenHeight,
								Online:       true,
							}
						}
					}
					ds.mu.Unlock()
				}
				ds.connMgr.SetState(control.ConnStateConnected)
				ds.kvmActive.Store(ds.ensureCapture() == nil)
				ds.negotiateClipboard(conn, payload.Capabilities)
			}
		}
	}

	if err := ds.router.HandleRemoteMessage(msg); err != nil {
		ds.reportInputError(err)
	}
}

func (ds *DaemonService) handleDisconnect(conn *transport.Conn, err error) {
	ds.logger.Printf("Peer %s disconnected: %v", conn.RemoteAddr().String(), err)
	ds.clipboardConn.Store(nil)
	ds.imageClipboardConn.Store(nil)
	ds.fileTransferConn.Store(nil)
	ds.activeConn.Store(nil)
	ds.kvmActive.Store(false)
	ds.router.HandleDisconnect(err)
	ds.connMgr.SetState(control.ConnStateDisconnected)

	ds.mu.Lock()
	ds.currentPeer = nil
	ds.mu.Unlock()
}

// GetLocalInfo returns machine specs and identity.
func (ds *DaemonService) GetLocalInfo() LocalInfoResponse {
	bounds := ds.backend.ScreenBounds()
	return LocalInfoResponse{
		ID:              ds.localDev.ID,
		Name:            ds.localDev.Name,
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		ScreenWidth:     bounds.Width,
		ScreenHeight:    bounds.Height,
		ProtocolVersion: protocol.CurrentProtocolVersion,
		Port:            ds.listenPort,
	}
}

// GetPeers returns all discovered peers.
func (ds *DaemonService) GetPeers() []PeerInfo {
	raw := ds.peerStore.List()
	var out []PeerInfo
	for _, p := range raw {
		out = append(out, FromDiscoveryPeer(p))
	}
	return out
}

// Rescan triggers an active discovery probe and beacon broadcast.
func (ds *DaemonService) Rescan() {
	if ds.discService != nil {
		ds.discService.BroadcastProbe()
		ds.discService.BroadcastBeacon()
	}
}

// GetStatus returns current connection and KVM status.
func (ds *DaemonService) GetStatus() StatusResponse {
	ds.mu.RLock()
	cp := ds.currentPeer
	inputError := ds.inputError
	side := string(ds.router.PeerSide())
	ds.mu.RUnlock()

	return StatusResponse{
		ConnectionState:         string(ds.connMgr.State()),
		KVMActive:               ds.kvmActive.Load(),
		ClipboardAvailable:      ds.clipboardConn.Load() != nil,
		ImageClipboardAvailable: ds.imageClipboardConn.Load() != nil,
		FileTransferAvailable:   ds.fileTransferConn.Load() != nil,
		InputError:              inputError,
		ControlState:            string(ds.stateMgr.Current()),
		PeerSide:                side,
		CurrentPeer:             cp,
		LocalBounds:             ds.router.LocalBounds(),
		RemoteBounds:            ds.router.RemoteBounds(),
	}
}

// GetMetrics returns real-time latency snapshot.
func (ds *DaemonService) GetMetrics() control.MetricsSnapshot {
	m := ds.router.Metrics()
	if m == nil {
		return control.MetricsSnapshot{}
	}
	snap := m.Snapshot()
	q := ds.router.EventQueue()
	if q != nil {
		snap.QueueDepth = q.Len()
		snap.DroppedMoves = q.Drops()
		snap.CoalescedMoves = q.CoalescedCount()
	}
	return snap
}

// Connect dials the specified peer.
func (ds *DaemonService) Connect(peerID, addr string) error {
	if ds.connMgr.State() == control.ConnStateConnected {
		ds.kvmActive.Store(ds.ensureCapture() == nil)
		ds.mu.RLock()
		cp := ds.currentPeer
		side := string(ds.router.PeerSide())
		ds.mu.RUnlock()
		var pi PeerInfo
		if cp != nil {
			pi = *cp
		}
		ds.emit(IPCEvent{
			Event: "connected",
			Data: ConnectedEvent{
				Peer: pi,
				Side: side,
			},
		})
		return nil // already connected
	}

	targetAddr := addr
	var targetPeer *PeerInfo

	if peerID != "" {
		if p, ok := ds.peerStore.FindByNameOrID(peerID); ok {
			if targetAddr == "" {
				targetAddr = p.Address
			}
			info := FromDiscoveryPeer(p)
			targetPeer = &info
		}
	} else if targetAddr != "" {
		for _, p := range ds.peerStore.List() {
			if p.Address == targetAddr || p.IP == strings.Split(targetAddr, ":")[0] {
				info := FromDiscoveryPeer(p)
				targetPeer = &info
				break
			}
		}
	}

	if targetAddr == "" {
		return fmt.Errorf("%w: %s", ErrPeerNotFound, peerID)
	}

	// Guarantee valid TCP port 4545
	if host, port, err := net.SplitHostPort(targetAddr); err == nil {
		if port == "" || port == "0" {
			targetAddr = net.JoinHostPort(host, "4545")
		}
	} else if !strings.Contains(targetAddr, ":") {
		targetAddr = net.JoinHostPort(strings.TrimSpace(targetAddr), "4545")
	}

	ds.connMgr.SetState(control.ConnStateConnecting)

	client := transport.NewClient(5 * time.Second)
	conn, err := client.Connect(ds.ctx, targetAddr)
	if err != nil {
		ds.connMgr.SetState(control.ConnStateDisconnected)
		ds.logger.Printf("[CONNECT ERROR] target=%s: %v", targetAddr, err)
		return fmt.Errorf("failed to connect to peer %s: %w", targetAddr, err)
	}

	ds.clipboardConn.Store(nil)
	ds.imageClipboardConn.Store(nil)
	ds.fileTransferConn.Store(nil)
	ds.activeConn.Store(conn)
	ds.router.SetConnection(conn)
	ds.connMgr.RecordActivity()

	if targetPeer != nil {
		ds.mu.Lock()
		ds.currentPeer = targetPeer
		ds.mu.Unlock()
	}

	// Send Handshake
	bounds := ds.backend.ScreenBounds()
	hsMsg, err := protocol.NewHandshakeMessageWithDevice(
		ds.seqCounter.Add(1),
		ds.localDev.ID,
		ds.localDev.Name,
		runtime.GOOS,
		runtime.GOARCH,
		bounds.Width,
		bounds.Height,
		ds.localDev.Capabilities,
	)
	if err == nil {
		_ = conn.Send(hsMsg)
	}

	ds.connMgr.StartHeartbeat(conn)

	// Client receive loop
	go func() {
		for {
			if ds.ctx.Err() != nil || conn.IsClosed() {
				break
			}
			msg, err := conn.Receive()
			if err != nil {
				ds.handleDisconnect(conn, err)
				break
			}
			ds.handleMessage(conn, msg)
		}
	}()

	return nil
}

// Disconnect severs active connection and releases input.
func (ds *DaemonService) Disconnect() error {
	ds.clipboardConn.Store(nil)
	ds.imageClipboardConn.Store(nil)
	ds.fileTransferConn.Store(nil)
	ds.kvmActive.Store(false)
	ds.router.ReleaseToLocal()
	conn := ds.activeConn.Swap(nil)
	if conn != nil {
		_ = conn.Close()
	}
	ds.connMgr.SetState(control.ConnStateDisconnected)
	ds.mu.Lock()
	ds.currentPeer = nil
	ds.mu.Unlock()
	return nil
}

// Router transitions call listeners with the router lock held. This listener
// must not query cursor coordinates or any other router state synchronously.
func (ds *DaemonService) notifyControlState(oldState, newState control.State) {
	ds.emit(IPCEvent{Event: "control_switched", Data: ControlSwitchedEvent{State: string(newState)}})
}

func (ds *DaemonService) reportInputError(err error) {
	ds.mu.Lock()
	changed := ds.inputError != err.Error()
	ds.inputError = err.Error()
	ds.mu.Unlock()
	if changed {
		ds.logger.Printf("[INPUT ERROR] %v", err)
		ds.emit(IPCEvent{Event: "error", Data: map[string]string{"message": err.Error()}})
	}
}

func (ds *DaemonService) ensureCapture() error {
	ds.captureMu.Lock()
	defer ds.captureMu.Unlock()
	err := ds.backend.StartCapture(ds.eventsCh)
	if err != nil && !errors.Is(err, input.ErrCaptureAlreadyBusy) {
		ds.reportInputError(err)
		return err
	}
	ds.captureReady.Store(true)
	ds.mu.Lock()
	ds.inputError = ""
	ds.mu.Unlock()
	return nil
}

// StartKVM activates KVM edge switching and connects to peer if needed.
func (ds *DaemonService) StartKVM(peerID, addr, side string) error {
	normalizedSide := control.PeerSide(strings.ToLower(strings.TrimSpace(side)))
	if !normalizedSide.IsScreenSide() {
		normalizedSide = control.PeerSideRight
	}

	ds.router.SetPeerSide(normalizedSide)

	// Ensure physical input capture is active
	if err := ds.ensureCapture(); err != nil {
		return err
	}

	// If not connected, attempt connection
	if ds.connMgr.State() != control.ConnStateConnected {
		if peerID != "" || addr != "" {
			if err := ds.Connect(peerID, addr); err != nil {
				return err
			}
		} else {
			return ErrNoActiveConn
		}
	}

	ds.kvmActive.Store(true)
	return nil
}

// StopKVM ends the sharing connection and restores local control on both peers.
func (ds *DaemonService) StopKVM() error {
	ds.kvmActive.Store(false)
	ds.router.SetPeerSide(control.PeerSideNone)
	return ds.Disconnect()
}

// SetPeerSide updates the peer screen side locally and syncs with the remote peer over TCP.
func (ds *DaemonService) SetPeerSide(side string) error {
	normalizedSide := control.PeerSide(strings.ToLower(strings.TrimSpace(side)))
	if !normalizedSide.IsScreenSide() && normalizedSide != control.PeerSideNone {
		return fmt.Errorf("invalid peer side: %s", side)
	}
	ds.router.SetPeerSide(normalizedSide)

	// Emit event locally so UI updates
	ds.emit(IPCEvent{
		Event: "peer_side_changed",
		Data:  map[string]string{"side": string(normalizedSide)},
	})

	// If connected to a peer, send layout update message so peer updates in real time
	conn := ds.activeConn.Load()
	if conn != nil && !conn.IsClosed() && normalizedSide.IsScreenSide() {
		msg, err := protocol.NewLayoutUpdateMessage(ds.seqCounter.Add(1), string(normalizedSide))
		if err == nil {
			_ = conn.Send(msg)
		}
	}
	return nil
}

// Close gracefully stops all daemon subsystems.
func (ds *DaemonService) Close() error {
	if ds.closed.Swap(true) {
		return nil
	}

	ds.cancel()
	_ = ds.StopKVM()
	_ = ds.Disconnect()

	if ds.server != nil {
		_ = ds.server.Stop()
	}
	if ds.discService != nil {
		ds.discService.Close()
	}
	if ds.connMgr != nil {
		ds.connMgr.Close()
	}
	if ds.router != nil {
		ds.router.Close()
	}
	if ds.backend != nil {
		_ = ds.backend.Close()
	}

	return nil
}
