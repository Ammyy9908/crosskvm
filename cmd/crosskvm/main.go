package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/crosskvm/crosskvm/internal/config"
	"github.com/crosskvm/crosskvm/internal/control"
	"github.com/crosskvm/crosskvm/internal/daemon"
	"github.com/crosskvm/crosskvm/internal/discovery"
	"github.com/crosskvm/crosskvm/internal/input"
	"github.com/crosskvm/crosskvm/internal/protocol"
	"github.com/crosskvm/crosskvm/internal/transport"
)

const banner = `
   ____                      _  ____     ____  ___ 
  / ___|_ __ ___  ___ ___   | |/ /\ \   / /  \/  | 
 | |   | '__/ _ \/ __/ __|  | ' /  \ \ / /| |\/| | 
 | |___| | | (_) \__ \__ \  | . \   \ V / | |  | | 
  \____|_|  \___/|___/___/  |_|\_\   \_/  |_|  |_| 
  Open-Source Cross-Platform Software KVM (Phase 6)
`

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "kvm":
		runKVM(os.Args[2:])
	case "peers":
		runPeers(os.Args[2:])
	case "discover":
		runDiscover(os.Args[2:])
	case "listen":
		runListen(os.Args[2:])
	case "connect":
		runConnect(os.Args[2:])
	case "inject-test":
		runInjectTest(os.Args[2:])
	case "capture-test":
		runCaptureTest(os.Args[2:])
	case "capture-connect":
		runCaptureConnect(os.Args[2:])
	case "metrics":
		runMetrics(os.Args[2:])
	case "daemon":
		runDaemon(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(banner)
	fmt.Println("Usage:")
	fmt.Println("  crosskvm kvm [flags]             Launch seamless two-machine software KVM mode (Recommended)")
	fmt.Println("  crosskvm peers                   List known and cached LAN peers")
	fmt.Println("  crosskvm discover [flags]        Actively scan local network for CrossKVM peers")
	fmt.Println("  crosskvm listen [flags]          Start in listener (server) mode")
	fmt.Println("  crosskvm connect [flags]         Start in connect (client) mode")
	fmt.Println("  crosskvm inject-test [cmd]       Direct native OS input injection test")
	fmt.Println("  crosskvm capture-test [flags]    Direct physical OS input capture test")
	fmt.Println("  crosskvm capture-connect [flags] Forward physical captured input to remote peer")
	fmt.Println("  crosskvm metrics [flags]         View or stream live performance & latency metrics (--watch)")
	fmt.Println("  crosskvm daemon [flags]          Start background Go IPC daemon for Electron Desktop UI")
	fmt.Println("\nFlags for 'kvm':")
	fmt.Println("  --peer string                    Connect by discovered peer ID or name (e.g. \"xkvm-a1b2\" or \"WinPC\")")
	fmt.Println("  --connect string                 Direct TCP address of remote peer to connect to (e.g. \"192.168.1.50:4545\")")
	fmt.Println("  --listen string                  Start KVM listener on address (e.g. \":4545\")")
	fmt.Println("  --peer-side string               Peer position: \"left\", \"right\", \"top\", or \"bottom\" (default \"right\")")
	fmt.Println("  --edge-threshold int             Edge detection sensitivity in pixels (default 3)")
	fmt.Println("  --debug                          Enable verbose debug logging")
	fmt.Println("\nFlags for 'discover':")
	fmt.Println("  --timeout int                    Scan duration in seconds (default 2)")
	fmt.Println("\nFlags for 'listen':")
	fmt.Println("  --addr string                    TCP address to listen on (default \":4545\")")
	fmt.Println("  --inject                         Enable real native OS input injection for incoming remote events")
	fmt.Println("  --peer-side string               Peer screen position: \"left\", \"right\", \"top\", or \"bottom\" (default \"none\")")
	fmt.Println("  --debug                          Enable verbose debug logging")
	fmt.Println("\nFlags for 'connect':")
	fmt.Println("  --addr string                    TCP address of remote peer to connect to (required)")
	fmt.Println("  --peer-side string               Peer screen position: \"left\", \"right\", \"top\", or \"bottom\" (default \"none\")")
	fmt.Println("  --debug                          Enable verbose debug logging")
	fmt.Println("\nEmergency Release Shortcut:")
	fmt.Println("  Ctrl + Alt + Shift + Escape      Instantly restores local keyboard/mouse control from remote screen")
	fmt.Println("\nInteractive Test Commands (while running):")
	fmt.Println("  state                            Print current control state and display bounds")
	fmt.Println("  metrics                          Display live network latency, packet counts, queue stats")
	fmt.Println("  peers                            Display discovered LAN peers")
	fmt.Println("  switch                           Toggle local/remote control state")
	fmt.Println("  ping                             Send keepalive ping")
	fmt.Println("  help                             Show command list")
	fmt.Println("  exit                             Exit application")
}

func parsePort(addr string, defaultPort int) int {
	if addr == "" {
		return defaultPort
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err == nil {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			return p
		}
	}
	if strings.HasPrefix(addr, ":") {
		if p, err := strconv.Atoi(addr[1:]); err == nil && p > 0 {
			return p
		}
	}
	return defaultPort
}

func runPeers(args []string) {
	fs := flag.NewFlagSet("peers", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	localDev, _ := discovery.GetLocalDevice(4545, 0, 0, protocol.CurrentProtocolVersion)
	fmt.Printf("Local Machine Identity:\n  Device ID:   %s\n  Device Name: %s\n  OS / Arch:   %s/%s\n\n",
		localDev.ID, localDev.Name, localDev.OS, localDev.Arch)

	store := discovery.NewPeerStore()
	peers := store.List()
	if len(peers) == 0 {
		fmt.Println("No known peers cached yet.")
		fmt.Println("Tip: Run 'crosskvm discover' to scan the local network for peers.")
		return
	}

	fmt.Printf("Known & Discovered LAN Peers (%d):\n", len(peers))
	fmt.Printf("%-18s %-20s %-10s %-22s %-12s %-8s %s\n", "DEVICE ID", "DEVICE NAME", "OS", "ADDRESS", "RESOLUTION", "PROTO", "LAST SEEN")
	fmt.Println(strings.Repeat("-", 102))
	for _, p := range peers {
		res := fmt.Sprintf("%dx%d", p.ScreenWidth, p.ScreenHeight)
		age := time.Since(p.LastSeen).Truncate(time.Second)
		fmt.Printf("%-18s %-20s %-10s %-22s %-12s %-8s %s ago\n",
			p.DeviceID, p.DeviceName, p.OS, p.Address, res, p.ProtocolVersion, age)
	}
	fmt.Println("\nTo connect to a peer:")
	fmt.Printf("  crosskvm kvm --peer %s --peer-side right\n", peers[0].DeviceID)
}

func runDiscover(args []string) {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	timeoutSec := fs.Int("timeout", 2, "Discovery scan duration in seconds")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	localDev, _ := discovery.GetLocalDevice(4545, 0, 0, protocol.CurrentProtocolVersion)
	fmt.Printf("Scanning local network for CrossKVM peers (%ds)...\n", *timeoutSec)
	fmt.Printf("Local Machine: %s (%s, %s/%s)\n\n", localDev.ID, localDev.Name, localDev.OS, localDev.Arch)

	service := discovery.NewDiscoveryService(*localDev, nil, nil, nil)
	if err := service.Start(ctx); err != nil {
		fmt.Printf("[WARN] Note: %v\n", err)
	}
	defer service.Close()

	peers, err := service.DiscoverOnce(ctx, time.Duration(*timeoutSec)*time.Second)
	if err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "Discovery error: %v\n", err)
		return
	}

	if len(peers) == 0 {
		fmt.Println("No remote peers discovered on local network.")
		fmt.Println("Ensure CrossKVM is running on another machine connected to the same LAN.")
		return
	}

	fmt.Printf("Discovered %d Peer(s):\n\n", len(peers))
	fmt.Printf("%-18s %-20s %-10s %-22s %-12s %-8s\n", "DEVICE ID", "DEVICE NAME", "OS", "ADDRESS", "RESOLUTION", "PROTO")
	fmt.Println(strings.Repeat("-", 92))
	for _, p := range peers {
		res := fmt.Sprintf("%dx%d", p.ScreenWidth, p.ScreenHeight)
		fmt.Printf("%-18s %-20s %-10s %-22s %-12s %-8s\n",
			p.DeviceID, p.DeviceName, p.OS, p.Address, res, p.ProtocolVersion)
	}
	fmt.Println("\nTo connect to a discovered peer:")
	fmt.Printf("  crosskvm kvm --peer %s --peer-side right\n", peers[0].DeviceID)
}

func runKVM(args []string) {
	fs := flag.NewFlagSet("kvm", flag.ExitOnError)
	listenAddr := fs.String("listen", "", "TCP address to listen on (e.g. :4545)")
	connectAddr := fs.String("connect", "", "Direct TCP address of remote peer to connect to (e.g. 192.168.1.50:4545)")
	peerID := fs.String("peer", "", "Discovered peer ID or device name to connect to (e.g. xkvm-a1b2 or WinPC)")
	peerSideStr := fs.String("peer-side", "right", "Direction of peer screen: 'left', 'right', 'top', or 'bottom' (default 'right')")
	edgeThreshold := fs.Int("edge-threshold", 3, "Edge threshold in pixels (default 3)")
	debug := fs.Bool("debug", false, "Enable verbose debug logging")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *listenAddr == "" && *connectAddr == "" && *peerID == "" {
		fmt.Fprintln(os.Stderr, "Error: Specify --listen <addr>, --connect <addr>, or --peer <device-id> for KVM mode.")
		fmt.Fprintln(os.Stderr, "Example Listener:        crosskvm kvm --listen :4545 --peer-side left")
		fmt.Fprintln(os.Stderr, "Example Client (by IP):  crosskvm kvm --connect 192.168.1.50:4545 --peer-side right")
		fmt.Fprintln(os.Stderr, "Example Client (by ID):  crosskvm kvm --peer xkvm-a1b2 --peer-side right")
		os.Exit(1)
	}

	peerSide := control.PeerSide(strings.ToLower(strings.TrimSpace(*peerSideStr)))
	if !peerSide.IsScreenSide() {
		peerSide = control.PeerSideRight
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := log.New(os.Stdout, "[CrossKVM-KVM] ", log.LstdFlags|log.Lmsgprefix)
	logger.Printf("Starting CrossKVM Seamless Mode (OS: %s/%s, Peer-Side: %s)...", runtime.GOOS, runtime.GOARCH, peerSide)

	// Initialize native OS input backend
	backend, err := input.NewBackend()
	if err != nil {
		logger.Fatalf("Failed to initialize native input backend: %v", err)
	}
	defer backend.Close()

	localBounds := backend.ScreenBounds()
	logger.Printf("Primary display geometry: %dx%d", localBounds.Width, localBounds.Height)

	// Determine listening port for local device advertisement
	port := 4545
	if *listenAddr != "" {
		port = parsePort(*listenAddr, 4545)
	}

	// Stable Local Device Identity
	localDev, err := discovery.GetLocalDevice(port, localBounds.Width, localBounds.Height, protocol.CurrentProtocolVersion)
	if err != nil {
		logger.Printf("[WARN] Failed to load local device ID: %v", err)
	}
	logger.Printf("Local Identity: DeviceID=%s, Name=%s (Protocol: %s)", localDev.ID, localDev.Name, protocol.CurrentProtocolVersion)

	// Start background LAN peer discovery & announcement service
	peerStore := discovery.NewPeerStore()
	discService := discovery.NewDiscoveryService(*localDev, peerStore, nil, logger)
	if err := discService.Start(ctx); err != nil {
		if *debug {
			logger.Printf("[DISCOVERY] Background discovery notice: %v", err)
		}
	}
	defer discService.Close()

	// Create StateManager & Router
	stateMgr := control.NewStateManager()
	cfg := control.RouterConfig{
		PeerSide:         peerSide,
		EdgeThreshold:    *edgeThreshold,
		HysteresisOffset: 20,
	}
	router := control.NewRouterWithConfig(stateMgr, backend, cfg, logger)
	defer router.Close()

	// Connection lifecycle & liveness manager
	connMgr := control.NewConnectionManager(router, router.Metrics(), control.DefaultConnectionConfig(), logger)
	defer connMgr.Close()

	// Start physical input capture
	eventsCh := make(chan input.InputEvent, 1024)
	if err := backend.StartCapture(eventsCh); err != nil {
		logger.Fatalf("Failed to start physical input capture: %v", err)
	}
	logger.Println("Physical input capture & edge routing ACTIVE.")
	logger.Println("Emergency release hotkey: [ Ctrl + Alt + Shift + Escape ]")

	var activeConn atomic.Pointer[transport.Conn]
	var seqCounter atomic.Uint64

	// Goroutine for routing captured local physical events through Router
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-eventsCh:
				if err := router.RouteLocalEvent(ev); err != nil && *debug {
					logger.Printf("RouteLocalEvent error: %v", err)
				}
			}
		}
	}()

	// Background metrics file persistence for 'crosskvm metrics [--watch]' CLI command
	go func() {
		cfgDir, err := discovery.GetConfigDir()
		if err != nil {
			return
		}
		metricsPath := filepath.Join(cfgDir, "metrics.json")
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				_ = os.Remove(metricsPath)
				return
			case <-ticker.C:
				m := router.Metrics()
				if m == nil {
					continue
				}
				snap := m.Snapshot()
				q := router.EventQueue()
				if q != nil {
					snap.QueueDepth = q.Len()
					snap.DroppedMoves = q.Drops()
					snap.CoalescedMoves = q.CoalescedCount()
				}
				data, err := json.Marshal(snap)
				if err == nil {
					_ = os.WriteFile(metricsPath, data, 0644)
				}
			}
		}
	}()

	if *listenAddr != "" {
		// Server / Listener mode
		handler := transport.ServerFuncHandler{
			ConnectFunc: func(conn *transport.Conn) {
				logger.Printf("Peer connected from %s", conn.RemoteAddr())
				activeConn.Store(conn)
				router.SetConnection(conn)
				connMgr.RecordActivity()
				connMgr.SetState(control.ConnStateConnecting)

				// Send handshake ack with our screen geometry, device ID, and capabilities
				ackMsg, err := protocol.NewHandshakeAckMessageWithDevice(
					seqCounter.Add(1),
					true,
					localDev.ID,
					"kvm listener ready",
					localBounds.Width,
					localBounds.Height,
					localDev.Capabilities,
				)
				if err == nil {
					_ = conn.Send(ackMsg)
				}
				connMgr.StartHeartbeat(conn)
			},
			MessageFunc: func(conn *transport.Conn, msg protocol.Message) {
				connMgr.RecordActivity()

				// Validate handshake on receive
				if msg.Type == protocol.MessageTypeHandshake {
					var payload protocol.HandshakePayload
					if err := json.Unmarshal(msg.Payload, &payload); err == nil {
						if err := protocol.ValidateHandshake(payload); err != nil {
							logger.Printf("[SECURITY] Handshake rejected: %v", err)
							ackMsg, _ := protocol.NewHandshakeAckMessage(seqCounter.Add(1), false, err.Error(), localBounds.Width, localBounds.Height)
							_ = conn.Send(ackMsg)
							_ = conn.Close()
							return
						}
						logger.Printf("[HANDSHAKE] Peer accepted: %s (ID: %s, OS: %s/%s, %dx%d, Proto: %s)",
							payload.PeerName, payload.DeviceID, payload.OS, payload.Arch, payload.ScreenWidth, payload.ScreenHeight, payload.Version)
						connMgr.SetState(control.ConnStateConnected)
					}
				}

				if *debug {
					logger.Printf("Received message: %s", msg.String())
				}
				if err := router.HandleRemoteMessage(msg); err != nil && *debug {
					logger.Printf("Error handling remote message: %v", err)
				}
			},
			DisconnectFunc: func(conn *transport.Conn, err error) {
				logger.Printf("Peer %s disconnected: %v", conn.RemoteAddr(), err)
				activeConn.Store(nil)
				router.HandleDisconnect(err)
				connMgr.SetState(control.ConnStateDisconnected)
			},
		}

		server := transport.NewServer(*listenAddr, handler)
		go runInteractiveREPL(ctx, cancel, logger, func(line string) {
			conn := activeConn.Load()
			handleREPLCommand(line, conn, router, connMgr, discService, &seqCounter, logger)
		})

		serverErrCh := make(chan error, 1)
		go func() {
			serverErrCh <- server.ListenAndServe(ctx)
		}()

		select {
		case <-server.Ready():
			logger.Printf("KVM Listener active on %s. Move cursor toward the %s edge to switch screens.", *listenAddr, peerSide)
		case <-ctx.Done():
		}

		select {
		case err := <-serverErrCh:
			if err != nil {
				logger.Printf("Server exited with error: %v", err)
			}
		case <-ctx.Done():
			logger.Println("Shutting down gracefully...")
			_ = server.Stop()
		}

	} else {
		// Client / Connect mode with automatic reconnect & exponential backoff
		go runInteractiveREPL(ctx, cancel, logger, func(line string) {
			conn := activeConn.Load()
			handleREPLCommand(line, conn, router, connMgr, discService, &seqCounter, logger)
		})

		logger.Printf("KVM Client mode active! Move cursor toward the %s edge to switch screens.", peerSide)

		reconnectAttempt := 0
		for {
			if ctx.Err() != nil {
				break
			}

			targetAddr := *connectAddr
			if *peerID != "" {
				// Resolve or re-resolve peer by ID / name
				resolved, err := discService.ResolvePeer(ctx, *peerID, 2*time.Second)
				if err != nil {
					reconnectAttempt++
					connMgr.Metrics().RecordReconnectAttempt()
					connMgr.SetState(control.ConnStateReconnecting)
					backoff := connMgr.CalculateBackoff(reconnectAttempt)
					if *debug || reconnectAttempt <= 3 || reconnectAttempt%5 == 0 {
						logger.Printf("Could not find peer '%s' on LAN (%v). Retrying in %v (attempt #%d)...", *peerID, err, backoff, reconnectAttempt)
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(backoff):
						continue
					}
				}
				targetAddr = resolved.Address
			}

			connMgr.SetState(control.ConnStateConnecting)
			client := transport.NewClient(3 * time.Second)
			conn, err := client.Connect(ctx, targetAddr)
			if err != nil {
				reconnectAttempt++
				connMgr.Metrics().RecordReconnectAttempt()
				connMgr.SetState(control.ConnStateReconnecting)
				backoff := connMgr.CalculateBackoff(reconnectAttempt)
				if *debug || reconnectAttempt <= 3 || reconnectAttempt%5 == 0 {
					logger.Printf("Connection to %s failed (%v). Reconnecting in %v (attempt #%d)...", targetAddr, err, backoff, reconnectAttempt)
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
					continue
				}
			}

			// Connection established
			reconnectAttempt = 0
			activeConn.Store(conn)
			router.SetConnection(conn)
			connMgr.RecordActivity()
			connMgr.SetState(control.ConnStateConnecting)

			logger.Printf("Connected successfully to peer at %s!", conn.RemoteAddr())

			// Send initial handshake with local device identity and display bounds
			handshakeMsg, err := protocol.NewHandshakeMessageWithDevice(
				seqCounter.Add(1),
				localDev.ID,
				localDev.Name,
				localDev.OS,
				localDev.Arch,
				localBounds.Width,
				localBounds.Height,
				localDev.Capabilities,
			)
			if err == nil {
				_ = conn.Send(handshakeMsg)
			}

			// Receive loop
			for {
				if ctx.Err() != nil || conn.IsClosed() {
					break
				}
				msg, err := conn.Receive()
				if err != nil {
					break
				}
				connMgr.RecordActivity()

				if msg.Type == protocol.MessageTypeHandshakeAck {
					var ack protocol.HandshakeAckPayload
					if err := json.Unmarshal(msg.Payload, &ack); err == nil {
						if !ack.Success {
							logger.Printf("[SECURITY] Peer rejected handshake: %s", ack.Reason)
							_ = conn.Close()
							break
						}
						connMgr.SetState(control.ConnStateConnected)
						connMgr.StartHeartbeat(conn)
						logger.Printf("[HANDSHAKE] Handshake accepted by peer! (DeviceID: %s, RemoteBounds: %dx%d)",
							ack.DeviceID, ack.ScreenWidth, ack.ScreenHeight)
					}
				}

				if *debug {
					logger.Printf("Received message: %s", msg.String())
				}
				if err := router.HandleRemoteMessage(msg); err != nil && *debug {
					logger.Printf("Error handling remote message: %v", err)
				}
			}

			// Peer connection lost or closed
			if ctx.Err() != nil {
				break
			}

			logger.Printf("Peer connection lost. Enforcing local safety (suppression OFF, held keys released)...")
			activeConn.Store(nil)
			router.HandleDisconnect(io.EOF) // Enforces StateLocal, suppression=false, releases held keys
			connMgr.SetState(control.ConnStateReconnecting)

			// Graceful backoff before reconnect
			reconnectAttempt++
			connMgr.Metrics().RecordReconnectAttempt()
			backoff := connMgr.CalculateBackoff(reconnectAttempt)
			logger.Printf("Attempting reconnect in %v (attempt #%d)...", backoff, reconnectAttempt)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}

		logger.Println("Shutting down KVM connection...")
		router.HandleDisconnect(io.EOF)
	}
}

func runListen(args []string) {
	fs := flag.NewFlagSet("listen", flag.ExitOnError)
	addr := fs.String("addr", ":4545", "TCP address to listen on")
	debug := fs.Bool("debug", false, "Enable verbose logging")
	enableInject := fs.Bool("inject", false, "Enable real native OS input injection for incoming remote events")
	peerSideStr := fs.String("peer-side", "none", "Peer position: 'left', 'right', 'top', 'bottom', or 'none'")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	cfg := config.Config{
		Mode:  config.ModeListen,
		Addr:  *addr,
		Debug: *debug,
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := log.New(os.Stdout, "[CrossKVM-Listen] ", log.LstdFlags|log.Lmsgprefix)
	logger.Printf("Starting listener on %s (OS: %s/%s)...", cfg.Addr, runtime.GOOS, runtime.GOARCH)

	stateMgr := control.NewStateManager()

	var backend input.InputBackend
	if *enableInject {
		var err error
		backend, err = input.NewBackend()
		if err != nil {
			logger.Printf("[WARN] Failed to initialize native backend (%v), using mock backend", err)
			backend = input.NewMockBackend()
		} else {
			logger.Printf("Native OS input injection ACTIVE (incoming events will be injected into the OS)")
		}
	} else {
		backend = input.NewMockBackend()
		logger.Printf("Running with injection disabled (pass --inject to enable physical OS input injection)")
	}
	defer backend.Close()

	localBounds := backend.ScreenBounds()
	routerCfg := control.RouterConfig{
		PeerSide:         control.PeerSide(*peerSideStr),
		EdgeThreshold:    3,
		HysteresisOffset: 20,
	}
	router := control.NewRouterWithConfig(stateMgr, backend, routerCfg, logger)
	defer router.Close()

	var activeConn atomic.Pointer[transport.Conn]
	var seqCounter atomic.Uint64

	handler := transport.ServerFuncHandler{
		ConnectFunc: func(conn *transport.Conn) {
			logger.Printf("Peer connected from %s", conn.RemoteAddr())
			activeConn.Store(conn)
			router.SetConnection(conn)

			// Send handshake ack with local screen dimensions
			ackMsg, err := protocol.NewHandshakeAckMessage(seqCounter.Add(1), true, "connected to listener", localBounds.Width, localBounds.Height)
			if err == nil {
				_ = conn.Send(ackMsg)
			}
		},
		MessageFunc: func(conn *transport.Conn, msg protocol.Message) {
			if *debug {
				logger.Printf("Received message: %s", msg.String())
			}
			if err := router.HandleRemoteMessage(msg); err != nil && *debug {
				logger.Printf("Error handling remote message: %v", err)
			}
		},
		DisconnectFunc: func(conn *transport.Conn, err error) {
			if err != nil && err != io.EOF {
				logger.Printf("Peer %s disconnected with error: %v", conn.RemoteAddr(), err)
			} else {
				logger.Printf("Peer %s disconnected cleanly", conn.RemoteAddr())
			}
			activeConn.Store(nil)
			router.HandleDisconnect(err)
		},
	}

	server := transport.NewServer(cfg.Addr, handler)

	// Launch interactive CLI reader
	go runInteractiveREPL(ctx, cancel, logger, func(line string) {
		conn := activeConn.Load()
		handleREPLCommand(line, conn, router, nil, nil, &seqCounter, logger)
	})

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.ListenAndServe(ctx)
	}()

	select {
	case <-server.Ready():
		logger.Printf("Ready and listening on %s. Type 'help' for interactive commands, or Ctrl+C to quit.", cfg.Addr)
	case <-ctx.Done():
	}

	select {
	case err := <-serverErrCh:
		if err != nil {
			logger.Printf("Server exited with error: %v", err)
		}
	case <-ctx.Done():
		logger.Println("Shutting down gracefully...")
		_ = server.Stop()
	}
}

func runConnect(args []string) {
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	addr := fs.String("addr", "", "TCP address to connect to (e.g. 127.0.0.1:4545)")
	peerSideStr := fs.String("peer-side", "none", "Peer position: 'left', 'right', 'top', 'bottom', or 'none'")
	debug := fs.Bool("debug", false, "Enable verbose logging")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *addr == "" {
		fmt.Fprintln(os.Stderr, "Error: --addr is required for connect mode.")
		fmt.Fprintln(os.Stderr, "Example: crosskvm connect --addr 127.0.0.1:4545")
		os.Exit(1)
	}

	cfg := config.Config{
		Mode:  config.ModeConnect,
		Addr:  *addr,
		Debug: *debug,
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := log.New(os.Stdout, "[CrossKVM-Client] ", log.LstdFlags|log.Lmsgprefix)
	logger.Printf("Connecting to peer at %s (OS: %s/%s)...", cfg.Addr, runtime.GOOS, runtime.GOARCH)

	client := transport.NewClient(0)
	conn, err := client.Connect(ctx, cfg.Addr)
	if err != nil {
		logger.Fatalf("Connection failed: %v", err)
	}
	defer conn.Close()

	logger.Printf("Connected successfully to %s!", conn.RemoteAddr())

	stateMgr := control.NewStateManager()
	mockBackend := input.NewMockBackend()
	localBounds := mockBackend.ScreenBounds()
	routerCfg := control.RouterConfig{
		PeerSide:         control.PeerSide(*peerSideStr),
		EdgeThreshold:    3,
		HysteresisOffset: 20,
	}
	router := control.NewRouterWithConfig(stateMgr, mockBackend, routerCfg, logger)
	router.SetConnection(conn)
	defer router.Close()

	// In test connect mode without explicit peer side, default to Remote
	if *peerSideStr == "none" {
		_ = stateMgr.SwitchToRemote()
	}

	var seqCounter atomic.Uint64

	// Send initial handshake
	hostname, _ := os.Hostname()
	handshakeMsg, err := protocol.NewHandshakeMessage(seqCounter.Add(1), hostname, runtime.GOOS, runtime.GOARCH, localBounds.Width, localBounds.Height)
	if err == nil {
		_ = conn.Send(handshakeMsg)
	}

	// Goroutine for receiving messages from peer
	go func() {
		for {
			if ctx.Err() != nil || conn.IsClosed() {
				return
			}
			msg, err := conn.Receive()
			if err != nil {
				if ctx.Err() == nil && !conn.IsClosed() {
					logger.Printf("Connection closed by peer: %v", err)
					router.HandleDisconnect(err)
					cancel()
				}
				return
			}
			if *debug {
				logger.Printf("Received message: %s", msg.String())
			}
			if err := router.HandleRemoteMessage(msg); err != nil && *debug {
				logger.Printf("Error handling message: %v", err)
			}
		}
	}()

	// Launch interactive CLI reader
	go runInteractiveREPL(ctx, cancel, logger, func(line string) {
		handleREPLCommand(line, conn, router, nil, nil, &seqCounter, logger)
	})

	logger.Println("Ready! Enter test events (e.g. 'mousemove dx=10 dy=-3', 'keydown key=A', 'help', 'exit'):")

	<-ctx.Done()
	logger.Println("Closing connection and shutting down...")
}

func runInjectTest(args []string) {
	backend, err := input.NewBackend()
	if err != nil {
		log.Fatalf("Failed to initialize input backend: %v", err)
	}
	defer backend.Close()

	logger := log.New(os.Stdout, "[CrossKVM-Inject] ", log.LstdFlags|log.Lmsgprefix)
	logger.Printf("Native input backend initialized (OS: %s/%s)", runtime.GOOS, runtime.GOARCH)

	// Single command mode
	if len(args) > 0 {
		cmdStr := strings.Join(args, " ")
		ev, err := input.ParseDebugEvent(cmdStr)
		if err != nil {
			log.Fatalf("Error parsing event '%s': %v", cmdStr, err)
		}
		if err := backend.Inject(ev); err != nil {
			log.Fatalf("Error injecting event: %v", err)
		}
		logger.Printf("Successfully injected: %s", ev.String())
		return
	}

	// Interactive REPL mode
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Println("\nInteractive Input Injection REPL. Enter events to inject directly into the OS.")
	fmt.Println("Examples:")
	fmt.Println("  mousemove 100 0        (or mousemove dx=100 dy=0)")
	fmt.Println("  mousedown left         (or mousedown button=left)")
	fmt.Println("  mouseup left           (or mouseup button=left)")
	fmt.Println("  mousewheel 0 120       (or mousewheel dx=0 dy=120)")
	fmt.Println("  keydown A              (or keydown key=A)")
	fmt.Println("  keyup A                (or keyup key=A)")
	fmt.Println("  keydown Enter")
	fmt.Println("  keyup Enter")
	fmt.Println("  help / exit")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		if ctx.Err() != nil {
			break
		}
		fmt.Print("inject> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "exit") || strings.EqualFold(line, "quit") {
			logger.Println("Exiting inject-test.")
			break
		}
		if strings.EqualFold(line, "help") {
			fmt.Println("\nAvailable commands:")
			fmt.Println("  mousemove <dx> <dy>   (e.g. mousemove 100 0)")
			fmt.Println("  mousedown <button>    (e.g. mousedown left)")
			fmt.Println("  mouseup <button>      (e.g. mouseup left)")
			fmt.Println("  mousewheel <dx> <dy>  (e.g. mousewheel 0 120)")
			fmt.Println("  keydown <key>         (e.g. keydown A)")
			fmt.Println("  keyup <key>           (e.g. keyup A)")
			fmt.Println("  exit")
			continue
		}

		ev, err := input.ParseDebugEvent(line)
		if err != nil {
			logger.Printf("[WARN] Invalid command: %v", err)
			continue
		}

		if err := backend.Inject(ev); err != nil {
			logger.Printf("[ERROR] Injection failed: %v", err)
		} else {
			logger.Printf("[SUCCESS] Injected: %s", ev.String())
		}
	}
}

func runCaptureTest(args []string) {
	fs := flag.NewFlagSet("capture-test", flag.ExitOnError)
	debug := fs.Bool("debug", false, "Enable verbose logging")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	logger := log.New(os.Stdout, "[CrossKVM-Capture] ", log.LstdFlags|log.Lmsgprefix)
	logger.Printf("Starting physical input capture test (OS: %s/%s)...", runtime.GOOS, runtime.GOARCH)

	backend, err := input.NewBackend()
	if err != nil {
		log.Fatalf("Failed to initialize input backend: %v", err)
	}
	defer backend.Close()

	eventsCh := make(chan input.InputEvent, 512)
	if err := backend.StartCapture(eventsCh); err != nil {
		log.Fatalf("Failed to start physical input capture: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Println("\nPhysical Input Capture Active. Move mouse, click buttons, scroll, or type keys.")
	fmt.Println("Events will be captured and printed below in platform-neutral format.")
	fmt.Println("Press Ctrl+C to stop.")

	for {
		select {
		case <-ctx.Done():
			logger.Println("Stopping capture test...")
			return
		case ev := <-eventsCh:
			switch ev.Type {
			case input.EventTypeMouseMove:
				fmt.Printf("mousemove dx=%d dy=%d\n", ev.DX, ev.DY)
			case input.EventTypeMouseButtonDown:
				fmt.Printf("mousedown button=%s\n", ev.Button)
			case input.EventTypeMouseButtonUp:
				fmt.Printf("mouseup button=%s\n", ev.Button)
			case input.EventTypeMouseWheel:
				fmt.Printf("mousewheel dx=%d dy=%d\n", ev.WheelDX, ev.WheelDY)
			case input.EventTypeKeyDown:
				fmt.Printf("keydown key=%s\n", ev.Key)
			case input.EventTypeKeyUp:
				fmt.Printf("keyup key=%s\n", ev.Key)
			default:
				if *debug {
					fmt.Printf("%s\n", ev.String())
				}
			}
		}
	}
}

func runCaptureConnect(args []string) {
	fs := flag.NewFlagSet("capture-connect", flag.ExitOnError)
	addr := fs.String("addr", "", "TCP address of remote peer to send captured input to (e.g. 192.168.1.50:4545)")
	debug := fs.Bool("debug", false, "Enable verbose logging")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *addr == "" {
		fmt.Fprintln(os.Stderr, "Error: --addr is required for capture-connect mode.")
		fmt.Fprintln(os.Stderr, "Example: crosskvm capture-connect --addr 192.168.1.50:4545")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := log.New(os.Stdout, "[CrossKVM-CaptureConnect] ", log.LstdFlags|log.Lmsgprefix)
	logger.Printf("Connecting to peer at %s (OS: %s/%s)...", *addr, runtime.GOOS, runtime.GOARCH)

	client := transport.NewClient(0)
	conn, err := client.Connect(ctx, *addr)
	if err != nil {
		logger.Fatalf("Connection failed: %v", err)
	}
	defer conn.Close()

	logger.Printf("Connected successfully to %s!", conn.RemoteAddr())

	stateMgr := control.NewStateManager()
	mockBackend := input.NewMockBackend()
	router := control.NewRouter(stateMgr, mockBackend, logger)
	router.SetConnection(conn)
	defer router.Close()

	// Switch state to remote so routed events are transmitted to the server
	_ = stateMgr.SwitchToRemote()

	var seqCounter atomic.Uint64

	// Send handshake
	hostname, _ := os.Hostname()
	handshakeMsg, err := protocol.NewHandshakeMessage(seqCounter.Add(1), hostname, runtime.GOOS, runtime.GOARCH, 1920, 1080)
	if err == nil {
		_ = conn.Send(handshakeMsg)
	}

	// Start physical capture
	backend, err := input.NewBackend()
	if err != nil {
		logger.Fatalf("Failed to initialize input backend: %v", err)
	}
	defer backend.Close()

	eventsCh := make(chan input.InputEvent, 512)
	if err := backend.StartCapture(eventsCh); err != nil {
		logger.Fatalf("Failed to start physical capture: %v", err)
	}

	logger.Println("Physical input capture & network forwarding ACTIVE.")
	logger.Println("Move mouse or type on this machine to control the remote peer. Press Ctrl+C to stop.")

	// Goroutine for handling incoming peer messages (keepalive, etc.)
	go func() {
		for {
			if ctx.Err() != nil || conn.IsClosed() {
				return
			}
			msg, err := conn.Receive()
			if err != nil {
				if ctx.Err() == nil && !conn.IsClosed() {
					logger.Printf("Connection closed by peer: %v", err)
					cancel()
				}
				return
			}
			if *debug {
				logger.Printf("Received message: %s", msg.String())
			}
			_ = router.HandleRemoteMessage(msg)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Println("Stopping capture-connect...")
			return
		case ev := <-eventsCh:
			if err := router.RouteLocalEvent(ev); err != nil {
				if *debug {
					logger.Printf("Failed to forward event: %v", err)
				}
			}
		}
	}
}

func runInteractiveREPL(ctx context.Context, cancel context.CancelFunc, logger *log.Logger, onCommand func(string)) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "exit") || strings.EqualFold(line, "quit") {
			logger.Println("Exit requested.")
			cancel()
			return
		}
		onCommand(line)
	}
}

func handleREPLCommand(line string, conn *transport.Conn, router *control.Router, connMgr *control.ConnectionManager, discService *discovery.DiscoveryService, seq *atomic.Uint64, logger *log.Logger) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	switch cmd {
	case "help":
		fmt.Println("\nAvailable commands:")
		fmt.Println("  state                         Print current control state, display bounds, and peer position")
		fmt.Println("  metrics                       Print LAN latency instrumentation, queue stats, and metrics")
		fmt.Println("  peers                         List discovered LAN peers in active session")
		fmt.Println("  switch                        Toggle between local and remote state")
		fmt.Println("  ping                          Send keepalive ping")
		fmt.Println("  mousemove dx=<int> dy=<int>   Send relative mouse movement (e.g. mousemove dx=10 dy=-3)")
		fmt.Println("  keydown key=<str> [mod=...]   Send key down event (e.g. keydown key=A mod=ctrl)")
		fmt.Println("  keyup key=<str>               Send key up event (e.g. keyup key=A)")
		fmt.Println("  mousedown button=<left|right> Send mouse button down (e.g. mousedown button=left)")
		fmt.Println("  mouseup button=<left|right>   Send mouse button up (e.g. mouseup button=left)")
		fmt.Println("  mousewheel dx=<int> dy=<int>  Send mouse wheel scroll (e.g. mousewheel dx=0 dy=120)")
		fmt.Println("  exit                          Quit application")
		return

	case "state":
		localB := router.LocalBounds()
		remoteB := router.RemoteBounds()
		lx, ly := router.LocalCursor()
		rx, ry := router.RemoteLogicalCursor()
		connState := "unknown"
		if connMgr != nil {
			connState = string(connMgr.State())
		} else if conn != nil && !conn.IsClosed() {
			connState = "connected"
		} else {
			connState = "disconnected"
		}
		logger.Printf("State: %s (Conn: %s) | Peer: %s | Local: %dx%d (pos: %d,%d) | Remote: %dx%d (pos: %d,%d) | Edge Armed: %t",
			router.StateManager().Current(),
			connState,
			router.PeerSide(),
			localB.Width, localB.Height, lx, ly,
			remoteB.Width, remoteB.Height, rx, ry,
			router.IsEdgeArmed(),
		)
		return

	case "metrics":
		m := router.Metrics()
		if m == nil {
			logger.Println("Metrics tracker not available.")
			return
		}
		snap := m.Snapshot()
		q := router.EventQueue()
		qDepth := 0
		qDrops := uint64(0)
		qCoalesced := uint64(0)
		if q != nil {
			qDepth = q.Len()
			qDrops = q.Drops()
			qCoalesced = q.CoalescedCount()
		}
		connState := "connected"
		if connMgr != nil {
			connState = string(connMgr.State())
		} else if conn == nil || conn.IsClosed() {
			connState = "disconnected"
		}

		isWatch := len(parts) > 1 && (parts[1] == "--watch" || parts[1] == "-w")
		printMetricsDashboard(snap, connState, qDepth, qDrops, qCoalesced)

		if isWatch {
			fmt.Println("Streaming live metrics (updates every 1s, press Enter to return to REPL)...")
			stopWatch := make(chan struct{})
			go func() {
				ticker := time.NewTicker(1 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-stopWatch:
						return
					case <-ticker.C:
						s := m.Snapshot()
						curDepth := 0
						curDrops := uint64(0)
						curCoalesced := uint64(0)
						if q != nil {
							curDepth = q.Len()
							curDrops = q.Drops()
							curCoalesced = q.CoalescedCount()
						}
						printMetricsDashboard(s, connState, curDepth, curDrops, curCoalesced)
					}
				}
			}()
		}
		return

	case "peers":
		if discService == nil {
			logger.Println("Peer discovery service not active in this mode.")
			return
		}
		peers := discService.PeerStore().List()
		if len(peers) == 0 {
			fmt.Println("No peers currently discovered on LAN.")
			return
		}
		fmt.Printf("\nDiscovered LAN Peers (%d):\n", len(peers))
		fmt.Printf("%-18s %-20s %-10s %-22s %-12s\n", "DEVICE ID", "DEVICE NAME", "OS", "ADDRESS", "RESOLUTION")
		fmt.Println(strings.Repeat("-", 86))
		for _, p := range peers {
			fmt.Printf("%-18s %-20s %-10s %-22s %dx%d\n",
				p.DeviceID, p.DeviceName, p.OS, p.Address, p.ScreenWidth, p.ScreenHeight)
		}
		return

	case "switch":
		if router.StateManager().IsLocal() {
			_ = router.StateManager().SwitchToRemote()
			logger.Println("Control state switched to REMOTE (events forwarded to peer).")
		} else {
			_ = router.StateManager().RestoreLocal()
			logger.Println("Control state restored to LOCAL.")
		}
		return

	case "ping":
		if conn == nil || conn.IsClosed() {
			logger.Println("[WARN] No active connection to send ping")
			return
		}
		pingSeq := seq.Add(1)
		pingMsg := protocol.NewPingMessage(pingSeq)
		if m := router.Metrics(); m != nil {
			m.RecordPingSent(pingSeq)
		}
		if err := conn.Send(pingMsg); err != nil {
			logger.Printf("[ERROR] Failed to send ping: %v", err)
		} else {
			logger.Printf("[SENT] Ping seq=%d", pingMsg.Seq)
		}
		return
	}

	// Attempt to parse as an InputEvent
	ev, err := input.ParseDebugEvent(line)
	if err != nil {
		logger.Printf("[WARN] Unrecognized command or invalid format: %v (type 'help' for examples)", err)
		return
	}

	// Send through router
	if err := router.RouteLocalEvent(ev); err != nil {
		logger.Printf("[ERROR] RouteLocalEvent error: %v", err)
	} else {
		logger.Printf("[SENT] %s", ev.String())
	}
}

func printMetricsDashboard(snap control.MetricsSnapshot, connState string, qDepth int, qDrops, qCoalesced uint64) {
	fmt.Println("\n================ CrossKVM Performance & Latency Metrics ================")
	fmt.Printf("  Connection State:        %s\n", connState)
	fmt.Printf("  Packets Transmitted:     %d sent | %d received\n", snap.SentCount, snap.RecvCount)

	fmt.Println("\n  --- End-to-End Latency (Capture -> Inject) ---")
	e2e := snap.EndToEndStats
	if e2e.Count > 0 {
		fmt.Printf("  Average:                 %.2f ms\n", e2e.AvgMs)
		fmt.Printf("  p50 / p95 / p99:         %.2f ms / %.2f ms / %.2f ms\n", e2e.P50Ms, e2e.P95Ms, e2e.P99Ms)
		fmt.Printf("  Min / Max / Last:        %.2f ms / %.2f ms / %.2f ms (samples: %d)\n", e2e.MinMs, e2e.MaxMs, e2e.LastMs, e2e.Count)
	} else {
		fmt.Printf("  Smoothed Transit Est:    %.2f ms (awaiting remote input traffic)\n", snap.AvgLatencyMs)
	}

	fmt.Println("\n  --- Pipeline Stage Latencies ---")
	rtt := snap.RTTStats
	if rtt.Count > 0 {
		fmt.Printf("  Network RTT:             avg: %.2f ms | p50: %.2f ms | p95: %.2f ms (last: %.2f ms)\n",
			rtt.AvgMs, rtt.P50Ms, rtt.P95Ms, snap.LastRTTMs)
		fmt.Printf("  1-Way Network Flight:    %.2f ms (estimated RTT/2)\n", snap.LastRTTMs/2)
	} else {
		fmt.Printf("  Network RTT:             %.2f ms (last)\n", snap.LastRTTMs)
	}

	qd := snap.QueueDwellStats
	if qd.Count > 0 {
		fmt.Printf("  Outbound Queue Dwell:    avg: %.2f ms | p50: %.2f ms | p95: %.2f ms (last: %.2f ms)\n",
			qd.AvgMs, qd.P50Ms, qd.P95Ms, qd.LastMs)
	} else {
		fmt.Printf("  Outbound Queue Dwell:    < 0.05 ms\n")
	}

	inj := snap.InjectionStats
	if inj.Count > 0 {
		fmt.Printf("  Native OS Injection:     avg: %.2f ms | p50: %.2f ms | p95: %.2f ms | p99: %.2f ms (last: %.2f ms)\n",
			inj.AvgMs, inj.P50Ms, inj.P95Ms, inj.P99Ms, inj.LastMs)
	} else {
		fmt.Printf("  Native OS Injection:     awaiting remote events\n")
	}

	fmt.Println("\n  --- Event Rates & Throughput ---")
	fmt.Printf("  Physical Capture Rate:   %d events/sec\n", snap.CaptureRateSec)
	fmt.Printf("  Mouse Movement Rate:     %d events/sec\n", snap.MouseRateSec)

	fmt.Println("\n  --- Outbound Queue Health ---")
	fmt.Printf("  Queue Depth:             %d items\n", qDepth)
	fmt.Printf("  Coalesced Mouse Moves:   %d (under backpressure only)\n", qCoalesced)
	fmt.Printf("  Dropped Mouse Moves:     %d\n", qDrops)
	fmt.Printf("  Reconnect Attempts:      %d\n", snap.ReconnectAttempts)
	fmt.Println("========================================================================")
}

func runMetrics(args []string) {
	fs := flag.NewFlagSet("metrics", flag.ExitOnError)
	watch := fs.Bool("watch", false, "Continuously stream live performance and latency metrics")
	fs.BoolVar(watch, "w", false, "Alias for --watch")
	addr := fs.String("addr", "", "Remote peer TCP address to probe directly (optional)")
	interval := fs.Duration("interval", 1*time.Second, "Update interval in watch mode")
	_ = fs.Parse(args)

	if *addr != "" {
		probeRemotePeerMetrics(*addr, *watch, *interval)
		return
	}

	cfgDir, err := discovery.GetConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error locating config dir: %v\n", err)
		os.Exit(1)
	}
	metricsPath := filepath.Join(cfgDir, "metrics.json")

	for {
		data, err := os.ReadFile(metricsPath)
		if err != nil {
			fmt.Printf("No active CrossKVM instance detected locally (could not read %s).\n", metricsPath)
			fmt.Println("Start CrossKVM using 'crosskvm kvm' first, or probe remote via 'crosskvm metrics --addr <ip:port>'.")
			if !*watch {
				os.Exit(1)
			}
		} else {
			var snap control.MetricsSnapshot
			if err := json.Unmarshal(data, &snap); err == nil {
				if *watch {
					fmt.Print("\033[H\033[2J") // Clear screen
				}
				printMetricsDashboard(snap, "active", snap.QueueDepth, snap.DroppedMoves, snap.CoalescedMoves)
			}
		}

		if !*watch {
			break
		}
		time.Sleep(*interval)
	}
}

func probeRemotePeerMetrics(addr string, watch bool, interval time.Duration) {
	fmt.Printf("Probing CrossKVM peer at %s (TCP_NODELAY enabled)...\n", addr)
	seq := uint64(1)

	for {
		t0 := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			fmt.Printf("Connection error to %s: %v\n", addr, err)
			if !watch {
				os.Exit(1)
			}
			time.Sleep(interval)
			continue
		}

		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.SetNoDelay(true)
		}

		c := transport.NewConn(conn)
		pingMsg := protocol.NewPingMessage(seq)
		seq++
		sendErr := c.Send(pingMsg)
		if sendErr != nil {
			fmt.Printf("Failed to send ping: %v\n", sendErr)
			_ = c.Close()
			if !watch {
				os.Exit(1)
			}
			time.Sleep(interval)
			continue
		}

		resp, recvErr := c.Receive()
		rtt := time.Since(t0)
		_ = c.Close()

		if recvErr != nil {
			fmt.Printf("Failed to receive pong: %v\n", recvErr)
		} else {
			fmt.Printf("[%s] Pong received from %s (seq=%d): RTT = %.2f ms | 1-Way Transit ≈ %.2f ms\n",
				resp.Type, addr, resp.Seq, float64(rtt.Microseconds())/1000.0, float64(rtt.Microseconds())/2000.0)
		}

		if !watch {
			break
		}
		time.Sleep(interval)
	}
}

func runDaemon(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	port := fs.Int("port", 4545, "TCP port for peer KVM connections")
	ipcPort := fs.Int("ipc-port", daemon.DefaultIPCPort, "Loopback TCP port for Electron IPC")
	ipcSock := fs.String("ipc-sock", daemon.DefaultUnixSock, "Unix domain socket path for Electron IPC (macOS/Linux)")
	_ = fs.Bool("debug", false, "Enable verbose debug logging")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := log.New(os.Stdout, "[CrossKVM-Daemon] ", log.LstdFlags|log.Lmsgprefix)
	logger.Printf("Starting CrossKVM Daemon (OS: %s/%s)...", runtime.GOOS, runtime.GOARCH)

	ds, err := daemon.NewDaemonService(*port, logger)
	if err != nil {
		logger.Fatalf("Failed to initialize daemon service: %v", err)
	}
	defer ds.Close()

	if err := ds.Start(); err != nil {
		logger.Fatalf("Failed to start daemon background subsystems: %v", err)
	}

	ipcServer := daemon.NewIPCServer(ds, *ipcPort, *ipcSock, logger)
	if err := ipcServer.Start(ctx); err != nil {
		logger.Fatalf("Failed to start IPC server: %v", err)
	}
	defer ipcServer.Close()

	logger.Printf("CrossKVM Daemon running. Ready for Electron UI connections.")
	<-ctx.Done()
	logger.Printf("Shutting down daemon gracefully...")
}
