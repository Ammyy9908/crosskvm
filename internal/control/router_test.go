package control

import (
	"bytes"
	"log"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/crosskvm/crosskvm/internal/input"
	"github.com/crosskvm/crosskvm/internal/protocol"
	"github.com/crosskvm/crosskvm/internal/transport"
)

func TestRouter_StateTransitionsAndSuppression(t *testing.T) {
	backend := input.NewMockBackend()
	stateMgr := NewStateManager()
	router := NewRouter(stateMgr, backend, nil)
	defer router.Close()

	if !stateMgr.IsLocal() {
		t.Fatalf("Expected initial state to be Local")
	}
	if backend.IsSuppressed() {
		t.Fatalf("Expected suppression to be false in Local state")
	}

	// Switch to Remote
	if err := stateMgr.SwitchToRemote(); err != nil {
		t.Fatalf("SwitchToRemote failed: %v", err)
	}
	if !stateMgr.IsRemote() {
		t.Fatalf("Expected state to be Remote")
	}
	if !backend.IsSuppressed() {
		t.Fatalf("Expected backend suppression to be true in Remote state")
	}

	// Restore to Local
	if err := stateMgr.RestoreLocal(); err != nil {
		t.Fatalf("RestoreLocal failed: %v", err)
	}
	if !stateMgr.IsLocal() {
		t.Fatalf("Expected state to be Local")
	}
	if backend.IsSuppressed() {
		t.Fatalf("Expected suppression to be false after RestoreLocal")
	}
}

func TestRouter_EdgeDetectionRightPeer(t *testing.T) {
	serverRaw, clientRaw := net.Pipe()
	defer serverRaw.Close()
	defer clientRaw.Close()

	clientConn := transport.NewConn(clientRaw)
	backend := input.NewMockBackend()
	backend.SetScreenBounds(input.ScreenBounds{Width: 1920, Height: 1080})

	cfg := RouterConfig{
		PeerSide:         PeerSideRight,
		EdgeThreshold:    3,
		HysteresisOffset: 20,
	}
	router := NewRouterWithConfig(nil, backend, cfg, nil)
	router.SetConnection(clientConn)
	router.SetRemoteBounds(input.ScreenBounds{Width: 2560, Height: 1440})
	defer router.Close()

	// Initial cursor at middle of screen
	router.SetLocalCursor(960, 540)

	// Case 1: Cursor in middle, moving right -> NO switch
	ev := input.NewMouseMoveEvent(20, 0)
	if err := router.RouteLocalEvent(ev); err != nil {
		t.Fatalf("RouteLocalEvent error: %v", err)
	}
	if !router.StateManager().IsLocal() {
		t.Fatalf("Expected StateLocal when in middle of screen")
	}

	// Case 2: Cursor near right edge (1918), moving left (DX = -5) -> NO switch
	router.SetLocalCursor(1918, 540)
	ev = input.NewMouseMoveEvent(-5, 0)
	if err := router.RouteLocalEvent(ev); err != nil {
		t.Fatalf("RouteLocalEvent error: %v", err)
	}
	if !router.StateManager().IsLocal() {
		t.Fatalf("Expected StateLocal when moving away from edge")
	}

	// Case 3: Cursor near right edge (1918), moving right (DX = +5) -> SWITCHES to Remote!
	router.SetLocalCursor(1918, 540)
	ev = input.NewMouseMoveEvent(5, 0)
	if err := router.RouteLocalEvent(ev); err != nil {
		t.Fatalf("RouteLocalEvent error: %v", err)
	}
	if !router.StateManager().IsRemote() {
		t.Fatalf("Expected transition to StateRemote when hitting right edge moving right")
	}
	if !backend.IsSuppressed() {
		t.Fatalf("Expected backend to be suppressed in Remote state")
	}

	// Verify normalized entry Y on remote: 540/1080 = 0.5 -> 0.5 * 1440 = 720
	rx, ry := router.RemoteLogicalCursor()
	if rx != 1 {
		t.Errorf("Expected remoteLogicalX = 1 on left entry, got: %d", rx)
	}
	if ry != 720 {
		t.Errorf("Expected remoteLogicalY = 720, got: %d", ry)
	}
}

func TestRouter_EdgeDetectionLeftPeer(t *testing.T) {
	serverRaw, clientRaw := net.Pipe()
	defer serverRaw.Close()
	defer clientRaw.Close()

	clientConn := transport.NewConn(clientRaw)
	backend := input.NewMockBackend()
	backend.SetScreenBounds(input.ScreenBounds{Width: 1920, Height: 1080})

	cfg := RouterConfig{
		PeerSide:         PeerSideLeft,
		EdgeThreshold:    3,
		HysteresisOffset: 20,
	}
	router := NewRouterWithConfig(nil, backend, cfg, nil)
	router.SetConnection(clientConn)
	router.SetRemoteBounds(input.ScreenBounds{Width: 1920, Height: 1080})
	defer router.Close()

	// Cursor at x=2 (within threshold <= 3), moving left (DX = -5) -> SWITCHES to Remote
	router.SetLocalCursor(2, 540)
	ev := input.NewMouseMoveEvent(-5, 0)
	if err := router.RouteLocalEvent(ev); err != nil {
		t.Fatalf("RouteLocalEvent error: %v", err)
	}
	if !router.StateManager().IsRemote() {
		t.Fatalf("Expected transition to StateRemote for left peer")
	}

	// Verify remote entry on right side: remoteLogicalX = 1920 - 2 = 1918
	rx, ry := router.RemoteLogicalCursor()
	if rx != 1918 {
		t.Errorf("Expected remoteLogicalX = 1918 on right entry, got: %d", rx)
	}
	if ry != 540 {
		t.Errorf("Expected remoteLogicalY = 540, got: %d", ry)
	}
}

func TestRouter_ReturnEdgeAndHysteresis(t *testing.T) {
	serverRaw, clientRaw := net.Pipe()
	defer serverRaw.Close()
	defer clientRaw.Close()

	clientConn := transport.NewConn(clientRaw)
	backend := input.NewMockBackend()
	backend.SetScreenBounds(input.ScreenBounds{Width: 1920, Height: 1080})

	cfg := RouterConfig{
		PeerSide:         PeerSideRight,
		EdgeThreshold:    3,
		HysteresisOffset: 20,
	}
	router := NewRouterWithConfig(nil, backend, cfg, nil)
	router.SetConnection(clientConn)
	router.SetRemoteBounds(input.ScreenBounds{Width: 1920, Height: 1080})
	defer router.Close()

	// Switch to Remote
	router.SetLocalCursor(1918, 540)
	_ = router.RouteLocalEvent(input.NewMouseMoveEvent(5, 0))
	if !router.StateManager().IsRemote() {
		t.Fatalf("Failed to transition to Remote")
	}

	// In Remote state: remote cursor starts at X=1. Moving right (+50) -> stays remote
	_ = router.RouteLocalEvent(input.NewMouseMoveEvent(50, 0))
	if !router.StateManager().IsRemote() {
		t.Fatalf("Expected to stay in Remote state")
	}

	// Move left past threshold: X=51 -> -60 -> X=-9 (clamped to threshold) and DX < 0 -> RETURNS to Local!
	_ = router.RouteLocalEvent(input.NewMouseMoveEvent(-60, 0))
	if !router.StateManager().IsLocal() {
		t.Fatalf("Expected return to Local state when hitting return edge")
	}
	if backend.IsSuppressed() {
		t.Fatalf("Expected suppression disabled on return to Local")
	}

	// Verify hysteresis: edge must NOT be armed immediately
	if router.IsEdgeArmed() {
		t.Fatalf("Expected edgeArmed to be false immediately after return")
	}

	// Verify cursor was warped to re-entry offset (1920 - 3 - 20 = 1897)
	warpedX, warpedY := backend.GetWarpedCursor()
	if warpedX != 1897 || warpedY != 540 {
		t.Errorf("Expected warped cursor at (1897, 540), got (%d, %d)", warpedX, warpedY)
	}

	// Moving slightly right (DX = +2, to 1899) must NOT trigger switch because edge is not armed
	_ = router.RouteLocalEvent(input.NewMouseMoveEvent(2, 0))
	if !router.StateManager().IsLocal() {
		t.Fatalf("Edge re-triggered before moving away from boundary")
	}

	// Moving away into the screen (DX = -30, cursor < 1900) re-arms edge
	_ = router.RouteLocalEvent(input.NewMouseMoveEvent(-30, 0))
	if !router.IsEdgeArmed() {
		t.Fatalf("Expected edge to re-arm after moving into the screen")
	}
}

func TestRouter_EmergencyRestoreChord(t *testing.T) {
	backend := input.NewMockBackend()
	stateMgr := NewStateManager()
	router := NewRouter(stateMgr, backend, nil)
	defer router.Close()

	_ = stateMgr.SwitchToRemote()
	if !stateMgr.IsRemote() {
		t.Fatal("Failed to set remote state")
	}

	// Sequence: Ctrl down, Alt down, Shift down, Escape down
	_ = router.RouteLocalEvent(input.NewKeyDownEvent("ControlLeft", 0))
	_ = router.RouteLocalEvent(input.NewKeyDownEvent("AltLeft", 0))
	_ = router.RouteLocalEvent(input.NewKeyDownEvent("ShiftLeft", 0))
	_ = router.RouteLocalEvent(input.NewKeyDownEvent("Escape", input.ModCtrl|input.ModAlt|input.ModShift))

	if !stateMgr.IsLocal() {
		t.Fatalf("Expected Emergency chord to restore Local state, got: %s", stateMgr.Current())
	}
	if backend.IsSuppressed() {
		t.Fatalf("Expected suppression disabled after emergency release")
	}
}

func TestRouter_StuckKeyCleanup(t *testing.T) {
	serverRaw, clientRaw := net.Pipe()
	defer serverRaw.Close()
	defer clientRaw.Close()

	serverConn := transport.NewConn(serverRaw)
	clientConn := transport.NewConn(clientRaw)

	clientBackend := input.NewMockBackend()
	clientRouter := NewRouter(nil, clientBackend, nil)
	clientRouter.SetConnection(clientConn)
	defer clientRouter.Close()

	serverBackend := input.NewMockBackend()
	serverRouter := NewRouter(nil, serverBackend, nil)
	serverRouter.SetConnection(serverConn)
	defer serverRouter.Close()

	_ = clientRouter.StateManager().SwitchToRemote()

	receivedMsgs := make([]protocol.Message, 0)
	var msgsMu sync.Mutex
	doneRead := make(chan struct{})

	go func() {
		defer close(doneRead)
		for {
			msg, err := serverConn.Receive()
			if err != nil {
				return
			}
			msgsMu.Lock()
			receivedMsgs = append(receivedMsgs, msg)
			msgsMu.Unlock()
			_ = serverRouter.HandleRemoteMessage(msg)
		}
	}()

	// Press Key "A" and MouseButton "left" while in remote
	_ = clientRouter.RouteLocalEvent(input.NewKeyDownEvent("A", 0))
	_ = clientRouter.RouteLocalEvent(input.NewMouseButtonDownEvent(input.MouseButtonLeft))

	// Emergency restore while keys are held
	_ = clientRouter.RouteLocalEvent(input.NewKeyDownEvent("Escape", input.ModCtrl|input.ModAlt|input.ModShift))

	// Allow outbound flush
	time.Sleep(50 * time.Millisecond)
	_ = serverRaw.Close()
	<-doneRead

	msgsMu.Lock()
	defer msgsMu.Unlock()

	// Check if KeyUp "A" and MouseButtonUp "left" were sent to clean up stuck keys
	hasKeyUpA := false
	hasMouseUpLeft := false
	for _, msg := range receivedMsgs {
		if msg.Type == protocol.MessageTypeInput && msg.Input != nil {
			if msg.Input.Type == input.EventTypeKeyUp && msg.Input.Key == "A" {
				hasKeyUpA = true
			}
			if msg.Input.Type == input.EventTypeMouseButtonUp && msg.Input.Button == input.MouseButtonLeft {
				hasMouseUpLeft = true
			}
		}
	}

	if !hasKeyUpA {
		t.Errorf("Expected stuck key cleanup: KeyUp 'A' not received by peer")
	}
	if !hasMouseUpLeft {
		t.Errorf("Expected stuck button cleanup: MouseButtonUp 'left' not received by peer")
	}
}

func TestRouter_DisconnectSafety(t *testing.T) {
	serverRaw, clientRaw := net.Pipe()

	clientConn := transport.NewConn(clientRaw)
	backend := input.NewMockBackend()
	router := NewRouter(nil, backend, nil)
	router.SetConnection(clientConn)
	defer router.Close()

	_ = router.StateManager().SwitchToRemote()
	if !backend.IsSuppressed() {
		t.Fatal("Expected suppressed true in remote")
	}

	// Close transport to simulate sudden network disconnect
	_ = serverRaw.Close()
	_ = clientRaw.Close()

	router.HandleDisconnect(net.ErrClosed)

	if !router.StateManager().IsLocal() {
		t.Fatalf("Expected StateLocal after disconnect")
	}
	if backend.IsSuppressed() {
		t.Fatalf("Expected suppression to be disabled after disconnect")
	}
}

func TestRouter_GeometryConversion(t *testing.T) {
	// Local: 1920x1080 -> Remote: 3840x2160
	serverRaw, clientRaw := net.Pipe()
	defer serverRaw.Close()
	defer clientRaw.Close()

	clientConn := transport.NewConn(clientRaw)
	backend := input.NewMockBackend()
	backend.SetScreenBounds(input.ScreenBounds{Width: 1920, Height: 1080})

	cfg := RouterConfig{
		PeerSide:         PeerSideRight,
		EdgeThreshold:    3,
		HysteresisOffset: 20,
	}
	router := NewRouterWithConfig(nil, backend, cfg, nil)
	router.SetConnection(clientConn)
	router.SetRemoteBounds(input.ScreenBounds{Width: 3840, Height: 2160})
	defer router.Close()

	// Top edge: Y = 0 -> normalized Y = 0.0 -> remote Y = 0
	router.SetLocalCursor(1918, 0)
	_ = router.RouteLocalEvent(input.NewMouseMoveEvent(5, 0))
	_, ry := router.RemoteLogicalCursor()
	if ry != 0 {
		t.Errorf("Expected remoteLogicalY = 0 at top edge, got: %d", ry)
	}

	// Reset to Local
	_ = router.StateManager().RestoreLocal()

	// Bottom edge: Y = 1079 -> normalized Y = 1079/1080 ~= 0.999 -> remote Y = 2158 (clamped < 2160)
	router.SetLocalCursor(1918, 1079)
	_ = router.RouteLocalEvent(input.NewMouseMoveEvent(5, 0))
	_, ry = router.RemoteLogicalCursor()
	if ry < 2150 || ry >= 2160 {
		t.Errorf("Expected remoteLogicalY ~= 2158 at bottom edge, got: %d", ry)
	}
}

func TestRouter_LocalLogging(t *testing.T) {
	backend := input.NewMockBackend()
	stateMgr := NewStateManager()
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)
	router := NewRouter(stateMgr, backend, logger)
	defer router.Close()

	ev := input.NewMouseMoveEvent(5, 5)
	if err := router.RouteLocalEvent(ev); err != nil {
		t.Fatalf("RouteLocalEvent failed: %v", err)
	}
}

func TestRouter_RemoteControlSuppressionAndCoordinates(t *testing.T) {
	serverRaw, clientRaw := net.Pipe()
	defer serverRaw.Close()
	defer clientRaw.Close()

	clientConn := transport.NewConn(clientRaw)
	backend := input.NewMockBackend()
	backend.SetScreenBounds(input.ScreenBounds{Width: 1920, Height: 1080})

	cfg := RouterConfig{
		PeerSide:         PeerSideRight,
		EdgeThreshold:    3,
		HysteresisOffset: 20,
	}
	router := NewRouterWithConfig(nil, backend, cfg, nil)
	router.SetConnection(clientConn)
	defer router.Close()

	// 1. Receive SwitchControl from remote peer (entering from right edge)
	switchMsg, err := protocol.NewSwitchControlMessage(1, "right", 0.5, "edge transition")
	if err != nil {
		t.Fatalf("NewSwitchControlMessage error: %v", err)
	}
	if err := router.HandleRemoteMessage(switchMsg); err != nil {
		t.Fatalf("HandleRemoteMessage SwitchControl error: %v", err)
	}

	if !router.IsControlledByRemote() {
		t.Fatalf("Expected IsControlledByRemote to be true")
	}

	// Verify local cursor placed at right edge
	lx, ly := router.LocalCursor()
	if lx != 1918 || ly != 540 {
		t.Errorf("Expected local cursor at (1918, 540), got (%d, %d)", lx, ly)
	}

	// 2. While controlled by remote, local events (even near edge) must NOT trigger switch to remote
	ev := input.NewMouseMoveEvent(5, 0)
	if err := router.RouteLocalEvent(ev); err != nil {
		t.Fatalf("RouteLocalEvent error: %v", err)
	}
	if !router.StateManager().IsLocal() {
		t.Fatalf("Expected to stay in Local state while remote is controlling")
	}

	// 3. Receive injected mouse moves from remote peer -> updates local cursor position
	inputMsg := protocol.NewInputMessage(2, input.NewMouseMoveEvent(-100, 50))
	if err := router.HandleRemoteMessage(inputMsg); err != nil {
		t.Fatalf("HandleRemoteMessage Input error: %v", err)
	}

	lx, ly = router.LocalCursor()
	if lx != 1819 || ly != 590 {
		t.Errorf("Expected local cursor updated to (1819, 590), got (%d, %d)", lx, ly)
	}

	// 4. Receive ReleaseControl -> resets isControlledByRemote
	releaseMsg, _ := protocol.NewReleaseControlMessage(3, "return edge")
	if err := router.HandleRemoteMessage(releaseMsg); err != nil {
		t.Fatalf("HandleRemoteMessage ReleaseControl error: %v", err)
	}

	if router.IsControlledByRemote() {
		t.Fatalf("Expected IsControlledByRemote to be false after ReleaseControl")
	}
}

func TestRouterVerticalRoundTrip(t *testing.T) {
	for _, side := range []PeerSide{PeerSideTop, PeerSideBottom} {
		t.Run(string(side), func(t *testing.T) {
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			backend := input.NewMockBackend()
			backend.SetScreenBounds(input.ScreenBounds{Width: 1920, Height: 1080})
			r := NewRouterWithConfig(nil, backend, RouterConfig{PeerSide: side, EdgeThreshold: 3, HysteresisOffset: 20}, nil)
			defer r.Close()
			r.SetConnection(transport.NewConn(a))
			r.SetRemoteBounds(input.ScreenBounds{Width: 1280, Height: 720})
			y, outward := 1, -5
			if side == PeerSideBottom {
				y = 1078
				outward = 5
			}
			r.SetLocalCursor(480, y)
			if err := r.RouteLocalEvent(input.NewMouseMoveEvent(5, 0)); err != nil {
				t.Fatal(err)
			}
			if !r.StateManager().IsLocal() {
				t.Fatal("horizontal movement crossed vertical edge")
			}
			r.SetLocalCursor(480, y)
			if err := r.RouteLocalEvent(input.NewMouseMoveEvent(0, int32(outward))); err != nil {
				t.Fatal(err)
			}
			if !r.StateManager().IsRemote() {
				t.Fatal("vertical edge did not hand off")
			}
			x, ry := r.RemoteLogicalCursor()
			if x != 320 {
				t.Fatalf("horizontal scaling: %d", x)
			}
			if side == PeerSideTop && ry != 718 || side == PeerSideBottom && ry != 1 {
				t.Fatalf("wrong entry: %d", ry)
			}
			// Move inward far enough to arm the return edge, then cross it.
			if err := r.RouteLocalEvent(input.NewMouseMoveEvent(0, int32(outward*20))); err != nil {
				t.Fatal(err)
			}
			if err := r.RouteLocalEvent(input.NewMouseMoveEvent(0, int32(-outward*40))); err != nil {
				t.Fatal(err)
			}
			if !r.StateManager().IsLocal() {
				t.Fatal("vertical return failed")
			}
		})
	}
}

func TestRouterVerticalReceivePosition(t *testing.T) {
	for _, side := range []string{"top", "bottom"} {
		t.Run(side, func(t *testing.T) {
			backend := input.NewMockBackend()
			backend.SetScreenBounds(input.ScreenBounds{Width: 1920, Height: 1080})
			r := NewRouter(nil, backend, nil)
			defer r.Close()
			msg, err := protocol.NewSwitchControlMessageAtPosition(1, side, .25, .5, "vertical")
			if err != nil {
				t.Fatal(err)
			}
			if err = r.HandleRemoteMessage(msg); err != nil {
				t.Fatal(err)
			}
			x, y := r.LocalCursor()
			if x != 480 {
				t.Fatalf("normalized horizontal position: %d", x)
			}
			want := 2
			if side == "bottom" {
				want = 1078
			}
			if y != want {
				t.Fatalf("entry Y=%d want %d", y, want)
			}
		})
	}
}

func TestEmergencyStopDisconnectsAndRestoresInput(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	backend := input.NewMockBackend()
	r := NewRouter(nil, backend, nil)
	defer r.Close()
	conn := transport.NewConn(a)
	r.SetConnection(conn)
	_ = r.StateManager().SwitchToRemote()
	r.EmergencyStop()
	if !conn.IsClosed() || r.Connection() != nil || !r.StateManager().IsLocal() || backend.IsSuppressed() {
		t.Fatal("emergency stop did not restore and disconnect")
	}
}
