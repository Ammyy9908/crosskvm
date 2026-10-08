package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	DefaultDiscoveryPort = 4546
	BeaconInterval       = 2 * time.Second
)

// BeaconPacket represents the UDP discovery payload.
type BeaconPacket struct {
	Type         string `json:"type"` // "beacon" or "probe"
	DeviceID     string `json:"device_id"`
	DeviceName   string `json:"device_name"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Port         int    `json:"port"`
	ScreenWidth  int    `json:"screen_width"`
	ScreenHeight int    `json:"screen_height"`
	Version      string `json:"version"`
	Timestamp    int64  `json:"timestamp"`
}

// DiscoveryService manages LAN announcement and peer discovery via UDP.
type DiscoveryService struct {
	mu           sync.Mutex
	local        LocalDevice
	peerStore    *PeerStore
	trustStore   *TrustStore
	port         int
	logger       *log.Logger
	conn         *net.UDPConn
	closed       bool
	stopCh       chan struct{}
	newPeerHooks []func(Peer)
}

// NewDiscoveryService creates a new DiscoveryService.
func NewDiscoveryService(local LocalDevice, peerStore *PeerStore, trustStore *TrustStore, logger *log.Logger) *DiscoveryService {
	if peerStore == nil {
		peerStore = NewPeerStore()
	}
	if trustStore == nil {
		trustStore = NewTrustStore()
	}
	return &DiscoveryService{
		local:      local,
		peerStore:  peerStore,
		trustStore: trustStore,
		port:       DefaultDiscoveryPort,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

// SetPort sets the UDP discovery broadcast port (used for tests or custom networks).
func (ds *DiscoveryService) SetPort(port int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.port = port
}

// OnPeerDiscovered registers a callback triggered when a new peer is discovered.
func (ds *DiscoveryService) OnPeerDiscovered(fn func(Peer)) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.newPeerHooks = append(ds.newPeerHooks, fn)
}

// PeerStore returns the underlying PeerStore.
func (ds *DiscoveryService) PeerStore() *PeerStore {
	return ds.peerStore
}

// Start begins continuous background advertising and listening.
func (ds *DiscoveryService) Start(ctx context.Context) error {
	ds.mu.Lock()
	port := ds.port
	ds.mu.Unlock()

	laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return fmt.Errorf("discovery: failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		// Fallback to ephemeral listener if port is occupied
		if ds.logger != nil {
			ds.logger.Printf("[DISCOVERY] Port %d busy, falling back to ephemeral port: %v", port, err)
		}
		conn, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
		if err != nil {
			return fmt.Errorf("discovery: failed to start UDP listener: %w", err)
		}
	}

	ds.mu.Lock()
	ds.conn = conn
	ds.mu.Unlock()

	// Initial probe to solicit immediate responses from existing peers
	go ds.BroadcastProbe()

	// 1. Inbound listener goroutine
	go ds.listenLoop(ctx, conn)

	// 2. Periodic beacon broadcaster
	go ds.beaconLoop(ctx)

	return nil
}

// BroadcastBeacon sends an announcement packet to the LAN broadcast address and known peers.
func (ds *DiscoveryService) BroadcastBeacon() {
	packet := BeaconPacket{
		Type:         "beacon",
		DeviceID:     ds.local.ID,
		DeviceName:   ds.local.Name,
		OS:           ds.local.OS,
		Arch:         ds.local.Arch,
		Port:         ds.local.Port,
		ScreenWidth:  ds.local.ScreenWidth,
		ScreenHeight: ds.local.ScreenHeight,
		Version:      ds.local.Version,
		Timestamp:    time.Now().UnixNano(),
	}
	ds.broadcast(packet)
}

// BroadcastProbe sends a probe packet requesting immediate beacons from all peers.
func (ds *DiscoveryService) BroadcastProbe() {
	packet := BeaconPacket{
		Type:       "probe",
		DeviceID:   ds.local.ID,
		DeviceName: ds.local.Name,
		Timestamp:  time.Now().UnixNano(),
	}
	ds.broadcast(packet)
}

// SendBeaconTo sends a direct unicast beacon to a specific remote UDP address.
func (ds *DiscoveryService) SendBeaconTo(addr *net.UDPAddr) {
	if addr == nil {
		return
	}
	packet := BeaconPacket{
		Type:         "beacon",
		DeviceID:     ds.local.ID,
		DeviceName:   ds.local.Name,
		OS:           ds.local.OS,
		Arch:         ds.local.Arch,
		Port:         ds.local.Port,
		ScreenWidth:  ds.local.ScreenWidth,
		ScreenHeight: ds.local.ScreenHeight,
		Version:      ds.local.Version,
		Timestamp:    time.Now().UnixNano(),
	}
	data, err := json.Marshal(packet)
	if err != nil {
		return
	}

	ds.mu.Lock()
	conn := ds.conn
	port := ds.port
	ds.mu.Unlock()

	if conn == nil {
		return
	}
	targetAddr := &net.UDPAddr{IP: addr.IP, Port: port}
	_, _ = conn.WriteTo(data, targetAddr)
}

func (ds *DiscoveryService) broadcast(packet BeaconPacket) {
	data, err := json.Marshal(packet)
	if err != nil {
		return
	}

	ds.mu.Lock()
	conn := ds.conn
	port := ds.port
	ds.mu.Unlock()

	if conn == nil {
		return
	}

	// 1. Send to 255.255.255.255
	dest, err := net.ResolveUDPAddr("udp", fmt.Sprintf("255.255.255.255:%d", port))
	if err == nil {
		_, _ = conn.WriteTo(data, dest)
	}

	// 2. Direct unicast to all known peers in the store (bypasses Wi-Fi broadcast drops)
	if ds.peerStore != nil {
		for _, p := range ds.peerStore.List() {
			if p.IP != "" && p.IP != "127.0.0.1" {
				paddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", p.IP, port))
				if err == nil {
					_, _ = conn.WriteTo(data, paddr)
				}
			}
		}
	}

	// 3. Broadcast to interface-specific broadcast addresses
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			ip := ipNet.IP.To4()
			mask := ipNet.Mask
			if len(mask) == 4 {
				broadcastIP := net.IPv4(
					ip[0]|^mask[0],
					ip[1]|^mask[1],
					ip[2]|^mask[2],
					ip[3]|^mask[3],
				)
				baddr := &net.UDPAddr{IP: broadcastIP, Port: port}
				_, _ = conn.WriteTo(data, baddr)
			}
		}
	}
}

func (ds *DiscoveryService) listenLoop(ctx context.Context, conn *net.UDPConn) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ds.stopCh:
			return
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		var packet BeaconPacket
		if err := json.Unmarshal(buf[:n], &packet); err != nil {
			continue
		}

		// Ignore our own broadcast packets
		if packet.DeviceID == ds.local.ID {
			continue
		}

		switch packet.Type {
		case "probe":
			// Peer probed for discovery; answer with immediate direct beacon back to sender
			ds.SendBeaconTo(remoteAddr)
			ds.BroadcastBeacon()

		case "beacon":
			// Acknowledge beacon with direct unicast beacon back so peer knows we are online
			ds.SendBeaconTo(remoteAddr)

			senderIP := remoteAddr.IP.String()
			tcpPort := packet.Port
			if tcpPort <= 0 {
				tcpPort = 4545
			}
			address := fmt.Sprintf("%s:%d", senderIP, tcpPort)

			peer := Peer{
				DeviceID:        packet.DeviceID,
				DeviceName:      packet.DeviceName,
				OS:              packet.OS,
				Arch:            packet.Arch,
				Port:            tcpPort,
				Address:         address,
				IP:              senderIP,
				ScreenWidth:     packet.ScreenWidth,
				ScreenHeight:    packet.ScreenHeight,
				ProtocolVersion: packet.Version,
				LastSeen:        time.Now(),
			}

			isNew := ds.peerStore.AddOrUpdate(peer)
			ds.trustStore.RecordSeen(peer.DeviceID, peer.DeviceName)

			if isNew && ds.logger != nil {
				ds.logger.Printf("[DISCOVERY] Discovered new peer: %s (%s, %s, %s, %dx%d)",
					peer.DeviceName, peer.DeviceID, peer.OS, peer.Address, peer.ScreenWidth, peer.ScreenHeight)
			}

			// Notify hooks on new peer OR whenever beacon is received
			ds.mu.Lock()
			hooks := make([]func(Peer), len(ds.newPeerHooks))
			copy(hooks, ds.newPeerHooks)
			ds.mu.Unlock()
			for _, h := range hooks {
				h(peer)
			}
		}
	}
}

func (ds *DiscoveryService) beaconLoop(ctx context.Context) {
	ticker := time.NewTicker(BeaconInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ds.stopCh:
			return
		case <-ticker.C:
			ds.BroadcastBeacon()
			// Periodically prune stale peers (older than 30s)
			ds.peerStore.Prune(30 * time.Second)
		}
	}
}

// DiscoverOnce triggers an active probe scan and returns all discovered peers after timeout.
func (ds *DiscoveryService) DiscoverOnce(ctx context.Context, timeout time.Duration) ([]Peer, error) {
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}

	ds.BroadcastProbe()
	select {
	case <-time.After(timeout):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return ds.peerStore.List(), nil
}

// ResolvePeer looks up a peer by ID or name, waiting up to timeout if discovery is still warm.
func (ds *DiscoveryService) ResolvePeer(ctx context.Context, query string, timeout time.Duration) (Peer, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return Peer{}, fmt.Errorf("empty peer query")
	}

	if p, ok := ds.peerStore.FindByNameOrID(query); ok {
		return p, nil
	}

	// Active scan probe
	ds.BroadcastProbe()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return Peer{}, ctx.Err()
		case <-time.After(200 * time.Millisecond):
			if p, ok := ds.peerStore.FindByNameOrID(query); ok {
				return p, nil
			}
		}
	}

	return Peer{}, fmt.Errorf("peer '%s' not found on local network", query)
}

// Close gracefully terminates the discovery listener and broadcaster.
func (ds *DiscoveryService) Close() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.closed {
		return nil
	}
	ds.closed = true
	close(ds.stopCh)

	if ds.conn != nil {
		_ = ds.conn.Close()
		ds.conn = nil
	}
	return nil
}
