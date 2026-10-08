package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/crosskvm/crosskvm/internal/control"
	"github.com/crosskvm/crosskvm/internal/input"
	"io"
	"log"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func skipIfNoListen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("TCP listen not permitted in this environment: %v", err)
	}
	_ = ln.Close()
}

func TestDaemon_Dispatch_Unit(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	ds, err := NewDaemonService(14545, logger)
	if err != nil {
		t.Fatalf("Failed to create DaemonService: %v", err)
	}
	defer ds.Close()

	server := NewIPCServer(ds, 14548, "/tmp/test.sock", logger)

	// Test dispatch get_local_info
	resp1 := server.dispatch(IPCRequest{ID: 10, Method: "get_local_info"})
	if resp1.ID != 10 || resp1.Error != "" {
		t.Fatalf("Unexpected get_local_info response: %+v", resp1)
	}

	// Test dispatch get_status
	resp2 := server.dispatch(IPCRequest{ID: 20, Method: "get_status"})
	if resp2.ID != 20 || resp2.Error != "" {
		t.Fatalf("Unexpected get_status response: %+v", resp2)
	}

	// Test dispatch set_peer_side
	param, _ := json.Marshal(SetPeerSideParams{Side: "left"})
	resp3 := server.dispatch(IPCRequest{ID: 30, Method: "set_peer_side", Params: param})
	if resp3.ID != 30 || resp3.Error != "" {
		t.Fatalf("Unexpected set_peer_side response: %+v", resp3)
	}
	if ds.router.PeerSide() != "left" {
		t.Errorf("Expected peerSide left, got %s", ds.router.PeerSide())
	}
}

func TestDaemon_IPC_RoundTrip(t *testing.T) {
	skipIfNoListen(t)
	logger := log.New(io.Discard, "", 0)
	ds, err := NewDaemonService(14545, logger)
	if err != nil {
		t.Fatalf("Failed to create DaemonService: %v", err)
	}
	defer ds.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")
	ipcPort := 14548

	server := NewIPCServer(ds, ipcPort, sockPath, logger)
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start IPCServer: %v", err)
	}
	defer server.Close()

	// Connect to loopback TCP
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", ipcPort))
	if err != nil {
		t.Fatalf("Failed to dial loopback IPC server: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Test 1: get_local_info
	req1 := IPCRequest{ID: 1, Method: "get_local_info"}
	req1Data, _ := json.Marshal(req1)
	req1Data = append(req1Data, '\n')
	if _, err := conn.Write(req1Data); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	var resp1 IPCResponse
	if err := json.Unmarshal(line, &resp1); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if resp1.ID != 1 || resp1.Error != "" {
		t.Errorf("Unexpected response: %+v", resp1)
	}

	// Test 2: get_status
	req2 := IPCRequest{ID: 2, Method: "get_status"}
	req2Data, _ := json.Marshal(req2)
	req2Data = append(req2Data, '\n')
	if _, err := conn.Write(req2Data); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	line, err = reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	var resp2 IPCResponse
	if err := json.Unmarshal(line, &resp2); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if resp2.ID != 2 || resp2.Error != "" {
		t.Errorf("Unexpected response: %+v", resp2)
	}

	// Test 3: set_peer_side
	paramData, _ := json.Marshal(SetPeerSideParams{Side: "left"})
	req3 := IPCRequest{ID: 3, Method: "set_peer_side", Params: paramData}
	req3Data, _ := json.Marshal(req3)
	req3Data = append(req3Data, '\n')
	if _, err := conn.Write(req3Data); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	line, err = reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	var resp3 IPCResponse
	if err := json.Unmarshal(line, &resp3); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if resp3.ID != 3 || resp3.Error != "" {
		t.Errorf("Unexpected response: %+v", resp3)
	}

	// Test 4: Broadcast event
	testEvt := IPCEvent{
		Event: "peer_discovered",
		Data: PeerInfo{
			ID:   "test-id",
			Name: "Test Peer",
			OS:   "windows",
		},
	}
	server.Broadcast(testEvt)

	line, err = reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Read event error: %v", err)
	}

	var evtReceived IPCEvent
	if err := json.Unmarshal(line, &evtReceived); err != nil {
		t.Fatalf("Unmarshal event error: %v", err)
	}

	if evtReceived.Event != "peer_discovered" {
		t.Errorf("Expected event peer_discovered, got %s", evtReceived.Event)
	}
}

type failingCaptureBackend struct {
	*input.MockBackend
	err error
}

func (b *failingCaptureBackend) StartCapture(ch chan<- input.InputEvent) error { return b.err }

func TestStartKVMSurfacesCaptureFailure(t *testing.T) {
	failure := errors.New("accessibility permission missing")
	ds := &DaemonService{
		backend: &failingCaptureBackend{MockBackend: input.NewMockBackend(), err: failure},
		router:  control.NewRouter(nil, input.NewMockBackend(), nil),
		logger:  log.New(io.Discard, "", 0), eventsCh: make(chan input.InputEvent, 1),
	}
	defer ds.router.Close()
	if err := ds.StartKVM("", "", "left"); !errors.Is(err, failure) {
		t.Fatalf("expected capture failure, got %v", err)
	}
	if ds.kvmActive.Load() || ds.inputError != failure.Error() {
		t.Fatal("failed capture must remain inactive with a visible error")
	}
}

func TestDaemonControlNotificationDoesNotLockRouter(t *testing.T) {
	sm := control.NewStateManager()
	router := control.NewRouter(sm, input.NewMockBackend(), nil)
	ds := &DaemonService{router: router}
	sm.OnStateChange(ds.notifyControlState)
	if err := sm.SwitchToRemote(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { router.ReleaseToLocal(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("control notification deadlocked router")
	}
	router.Close()
}

func TestCaptureRetryClearsPermissionError(t *testing.T) {
	backend := &failingCaptureBackend{MockBackend: input.NewMockBackend(), err: errors.New("permission pending")}
	ds := &DaemonService{backend: backend, logger: log.New(io.Discard, "", 0), eventsCh: make(chan input.InputEvent, 1)}
	if ds.ensureCapture() == nil || ds.captureReady.Load() {
		t.Fatal("denied capture must remain retryable")
	}
	backend.err = nil
	if err := ds.ensureCapture(); err != nil {
		t.Fatal(err)
	}
	if !ds.captureReady.Load() || ds.inputError != "" {
		t.Fatal("successful retry must clear permission error")
	}
}

func TestIPCNotificationsNeverBlockRouting(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	s := &IPCServer{logger: logger, clients: make(map[net.Conn]chan []byte)}
	a, b := net.Pipe()
	defer b.Close()
	defer s.Close()
	queue := make(chan []byte, 64)
	s.clients[a] = queue
	go s.writeClient(a, queue)
	done := make(chan struct{})
	go func() { s.Broadcast(IPCEvent{Event: "control_switched"}); close(done) }()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("unread desktop blocked ownership notification")
	}
	// The writer is blocked by the unread UI, but RPC responses queue independently.
	s.sendResponse(a, IPCResponse{ID: 42})
	reader := bufio.NewReader(b)
	_ = b.SetReadDeadline(time.Now().Add(time.Second))
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var event IPCEvent
	if err = json.Unmarshal(line, &event); err != nil || event.Event != "control_switched" {
		t.Fatalf("event corrupted: %s", line)
	}
	line, err = reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var response IPCResponse
	if err = json.Unmarshal(line, &response); err != nil || response.ID != 42 {
		t.Fatalf("response corrupted: %s", line)
	}
}
