package discovery

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDiscovery_Identity(t *testing.T) {
	id, err := GetOrCreateDeviceID()
	if err != nil {
		t.Fatalf("GetOrCreateDeviceID error: %v", err)
	}
	if id == "" {
		t.Fatalf("Expected non-empty device ID")
	}

	// Verify persistence returns same ID
	id2, err := GetOrCreateDeviceID()
	if err != nil {
		t.Fatalf("Second call error: %v", err)
	}
	if id != id2 {
		t.Errorf("Device ID changed across calls: %s != %s", id, id2)
	}

	dev, err := GetLocalDevice(4545, 1920, 1080, "1.0.0")
	if err != nil {
		t.Fatalf("GetLocalDevice error: %v", err)
	}
	if dev.ID != id {
		t.Errorf("Device ID mismatch: got %s, want %s", dev.ID, id)
	}
	if dev.Port != 4545 || dev.ScreenWidth != 1920 || dev.ScreenHeight != 1080 {
		t.Errorf("Device specs mismatch: %+v", dev)
	}
}

func TestDiscovery_PeerStore_DuplicatesAndLookup(t *testing.T) {
	store := NewEmptyPeerStore()

	p1 := Peer{
		DeviceID:        "xkvm-device-1",
		DeviceName:      "MacBook-Air",
		OS:              "darwin",
		Port:            4545,
		Address:         "192.168.1.5:4545",
		ScreenWidth:     1470,
		ScreenHeight:    956,
		ProtocolVersion: "1.0.0",
		LastSeen:        time.Now(),
	}

	// First insert
	if isNew := store.AddOrUpdate(p1); !isNew {
		t.Errorf("Expected newly added peer to return isNew=true")
	}

	// Duplicate insert (e.g. updated coordinates or IP)
	p1Updated := p1
	p1Updated.Address = "192.168.1.6:4545"
	if isNew := store.AddOrUpdate(p1Updated); isNew {
		t.Errorf("Expected duplicate peer update to return isNew=false")
	}

	// Verify count is still 1
	list := store.List()
	if len(list) != 1 {
		t.Fatalf("Expected 1 peer in store, got %d", len(list))
	}
	if list[0].Address != "192.168.1.6:4545" {
		t.Errorf("Expected updated address, got %s", list[0].Address)
	}

	// Lookup by exact ID
	found, ok := store.Get("xkvm-device-1")
	if !ok || found.DeviceName != "MacBook-Air" {
		t.Errorf("Exact ID lookup failed: ok=%v, %+v", ok, found)
	}

	// Lookup by Name (case insensitive)
	found, ok = store.FindByNameOrID("macbook-air")
	if !ok || found.DeviceID != "xkvm-device-1" {
		t.Errorf("Name lookup failed: ok=%v, %+v", ok, found)
	}

	// Lookup by ID Prefix
	found, ok = store.FindByNameOrID("xkvm-dev")
	if !ok || found.DeviceID != "xkvm-device-1" {
		t.Errorf("Prefix lookup failed: ok=%v, %+v", ok, found)
	}

	// Pruning test
	pOld := Peer{
		DeviceID: "xkvm-old",
		LastSeen: time.Now().Add(-10 * time.Minute),
	}
	store.AddOrUpdate(pOld)
	if len(store.List()) != 2 {
		t.Fatalf("Expected 2 peers before prune")
	}

	pruned := store.Prune(1 * time.Minute)
	if pruned != 1 {
		t.Errorf("Expected 1 pruned peer, got %d", pruned)
	}
	if len(store.List()) != 1 {
		t.Errorf("Expected 1 peer remaining after prune")
	}
}

func TestDiscovery_TrustStore(t *testing.T) {
	ts := NewTrustStore()

	// Default trust policy
	if !ts.IsTrusted("xkvm-unknown") {
		t.Errorf("Expected new LAN device to be auto-trusted by default")
	}

	// Explicit block
	ts.SetTrust("xkvm-bad", "Bad-Actor", TrustStatusBlocked)
	if ts.IsTrusted("xkvm-bad") {
		t.Errorf("Expected blocked device to NOT be trusted")
	}

	// Explicit trust
	ts.SetTrust("xkvm-good", "Good-Host", TrustStatusTrusted)
	if !ts.IsTrusted("xkvm-good") {
		t.Errorf("Expected trusted device to be trusted")
	}
}

func TestDiscovery_Announcements_Loopback(t *testing.T) {
	// Find free UDP port for test
	l, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Skipf("UDP listen not available in this environment: %v", err)
	}
	testPort := l.LocalAddr().(*net.UDPAddr).Port
	_ = l.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dev1 := LocalDevice{
		ID:           "test-node-1",
		Name:         "MacNode",
		OS:           "darwin",
		Port:         4545,
		ScreenWidth:  1470,
		ScreenHeight: 956,
		Version:      "1.0.0",
	}

	dev2 := LocalDevice{
		ID:           "test-node-2",
		Name:         "WinNode",
		OS:           "windows",
		Port:         4545,
		ScreenWidth:  1280,
		ScreenHeight: 720,
		Version:      "1.0.0",
	}

	store1 := NewPeerStore()
	store2 := NewPeerStore()

	svc1 := NewDiscoveryService(dev1, store1, nil, nil)
	svc1.SetPort(testPort)
	if err := svc1.Start(ctx); err != nil {
		t.Skipf("Cannot start discovery service in sandbox: %v", err)
	}
	defer svc1.Close()

	svc2 := NewDiscoveryService(dev2, store2, nil, nil)
	svc2.SetPort(testPort)
	if err := svc2.Start(ctx); err != nil {
		t.Skipf("Cannot start second discovery service: %v", err)
	}
	defer svc2.Close()

	// Direct broadcast simulation if broadcast socket is sandboxed
	pkt := BeaconPacket{
		Type:         "beacon",
		DeviceID:     dev2.ID,
		DeviceName:   dev2.Name,
		OS:           dev2.OS,
		Port:         dev2.Port,
		ScreenWidth:  dev2.ScreenWidth,
		ScreenHeight: dev2.ScreenHeight,
		Version:      dev2.Version,
		Timestamp:    time.Now().UnixNano(),
	}

	uaddr, err := net.ResolveUDPAddr("udp", l.LocalAddr().String())
	if err == nil {
		conn, err := net.DialUDP("udp", nil, uaddr)
		if err == nil {
			defer conn.Close()
			svc1.broadcast(pkt)
		}
	}
}
