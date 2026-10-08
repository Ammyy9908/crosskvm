package control

import (
	"net"
	"testing"
	"time"

	"github.com/crosskvm/crosskvm/internal/input"
	"github.com/crosskvm/crosskvm/internal/protocol"
	"github.com/crosskvm/crosskvm/internal/transport"
)

// TestEventQueue_OrderingAndNoDrops verifies that high-frequency mouse moves are coalesced
// under pressure and critical keyboard/button/control messages are NEVER dropped,
// with absolute semantic chronological order strictly preserved.
func TestEventQueue_OrderingAndNoDrops(t *testing.T) {
	q := NewEventQueue(5)
	q.SetPressureThreshold(2) // pressure threshold low for testing

	// 1. Enqueue critical KeyDown
	keyDown := protocol.NewInputMessage(1, input.NewKeyDownEvent("KeyA", 0))
	if ok := q.Enqueue(keyDown); !ok {
		t.Fatalf("Critical KeyDown was dropped!")
	}

	// 2. Enqueue multiple mouse moves to cause coalescing under pressure
	for i := 0; i < 10; i++ {
		move := protocol.NewInputMessage(uint64(2+i), input.NewMouseMoveEvent(1, 2))
		q.Enqueue(move)
	}

	// 3. Enqueue critical ButtonDown (e.g. start of click/drag)
	btnDown := protocol.NewInputMessage(100, input.NewMouseButtonDownEvent(input.MouseButtonLeft))
	if ok := q.Enqueue(btnDown); !ok {
		t.Fatalf("Critical ButtonDown was dropped!")
	}

	// 4. Enqueue subsequent drag mouse move
	dragMove := protocol.NewInputMessage(101, input.NewMouseMoveEvent(5, 5))
	q.Enqueue(dragMove)

	// 5. Enqueue ButtonUp (end of drag)
	btnUp := protocol.NewInputMessage(102, input.NewMouseButtonUpEvent(input.MouseButtonLeft))
	if ok := q.Enqueue(btnUp); !ok {
		t.Fatalf("Critical ButtonUp was dropped!")
	}

	// 6. Enqueue SwitchControl
	switchCtrl, _ := protocol.NewSwitchControlMessage(103, "left", 0.5, "edge")
	if ok := q.Enqueue(switchCtrl); !ok {
		t.Fatalf("Critical SwitchControl was dropped!")
	}

	// Verify strict chronological order of dequeued events:
	// 1st: KeyDown
	msg1, ok := q.Dequeue()
	if !ok || msg1.Input == nil || msg1.Input.Type != input.EventTypeKeyDown {
		t.Fatalf("Expected 1st dequeued message to be KeyDown, got %+v", msg1)
	}

	// 2nd: Coalesced MouseMove
	msg2, ok := q.Dequeue()
	if !ok || msg2.Input == nil || msg2.Input.Type != input.EventTypeMouseMove {
		t.Fatalf("Expected 2nd dequeued message to be MouseMove, got %+v", msg2)
	}
	if msg2.Input.DX <= 1 && msg2.Input.DY <= 2 {
		t.Errorf("Expected coalesced DX/DY, got DX=%d, DY=%d", msg2.Input.DX, msg2.Input.DY)
	}

	// 3rd: ButtonDown
	msg3, ok := q.Dequeue()
	if !ok || msg3.Input == nil || msg3.Input.Type != input.EventTypeMouseButtonDown {
		t.Fatalf("Expected 3rd dequeued message to be MouseButtonDown, got %+v", msg3)
	}

	// 4th: Drag MouseMove
	msg4, ok := q.Dequeue()
	if !ok || msg4.Input == nil || msg4.Input.Type != input.EventTypeMouseMove {
		t.Fatalf("Expected 4th dequeued message to be drag MouseMove, got %+v", msg4)
	}

	// 5th: ButtonUp
	msg5, ok := q.Dequeue()
	if !ok || msg5.Input == nil || msg5.Input.Type != input.EventTypeMouseButtonUp {
		t.Fatalf("Expected 5th dequeued message to be MouseButtonUp, got %+v", msg5)
	}

	// 6th: SwitchControl
	msg6, ok := q.Dequeue()
	if !ok || msg6.Type != protocol.MessageTypeSwitchControl {
		t.Fatalf("Expected 6th dequeued message to be SwitchControl, got %+v", msg6)
	}

	if q.CoalescedCount() == 0 {
		t.Errorf("Expected coalesced move count > 0")
	}
}

// TestHeartbeat_TimeoutAndRemoteRecovery verifies that a heartbeat timeout while in REMOTE state
// immediately drops suppression, restores LOCAL control, and releases held keys.
func TestHeartbeat_TimeoutAndRemoteRecovery(t *testing.T) {
	stateMgr := NewStateManager()
	mockBackend := input.NewMockBackend()
	router := NewRouter(stateMgr, mockBackend, nil)
	defer router.Close()

	metrics := NewMetricsTracker()
	cfg := ConnectionConfig{
		HeartbeatInterval: 10 * time.Millisecond,
		HeartbeatTimeout:  50 * time.Millisecond,
	}
	cm := NewConnectionManager(router, metrics, cfg, nil)
	defer cm.Close()

	// 1. Establish simulated connection and transition to REMOTE
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	conn := transport.NewConn(c1)
	router.SetConnection(conn)

	_ = stateMgr.SwitchToRemote()
	_ = mockBackend.SetLocalSuppressed(true)
	router.RouteLocalEvent(input.NewKeyDownEvent("ShiftLeft", 0))
	router.RouteLocalEvent(input.NewMouseButtonDownEvent(input.MouseButtonLeft))

	if !stateMgr.IsRemote() || !mockBackend.IsSuppressed() {
		t.Fatalf("Failed to establish simulated Remote suppressed state")
	}

	// 2. Trigger dead peer recovery
	cm.handleDeadPeer(nil, ErrHeartbeatTimeout)

	// 3. Verify safety invariants: LOCAL restored, suppression = false, state = Reconnecting
	if !stateMgr.IsLocal() {
		t.Errorf("Expected StateManager to be restored to LOCAL on heartbeat timeout")
	}
	if mockBackend.IsSuppressed() {
		t.Errorf("SAFETY INVARIANT VIOLATION: Local suppression must be FALSE after dead peer detection!")
	}
	if cm.State() != ConnStateReconnecting {
		t.Errorf("Expected connection state to be Reconnecting, got %s", cm.State())
	}
}

// TestReconnect_SuppressionSafety verifies that reconnect backoff never enables suppression
// until the connection is fully healthy.
func TestReconnect_SuppressionSafety(t *testing.T) {
	stateMgr := NewStateManager()
	mockBackend := input.NewMockBackend()
	router := NewRouter(stateMgr, mockBackend, nil)
	defer router.Close()

	metrics := NewMetricsTracker()
	cm := NewConnectionManager(router, metrics, DefaultConnectionConfig(), nil)
	defer cm.Close()

	cm.SetState(ConnStateReconnecting)

	// While reconnecting, physical input must NEVER be suppressed
	if mockBackend.IsSuppressed() {
		t.Fatalf("Suppression should be false while in Reconnecting state")
	}

	// Test backoff calculation
	b1 := cm.CalculateBackoff(1)
	b2 := cm.CalculateBackoff(2)
	b3 := cm.CalculateBackoff(3)

	if b1 != 1*time.Second {
		t.Errorf("Expected 1s initial backoff, got %v", b1)
	}
	if b2 != 2*time.Second {
		t.Errorf("Expected 2s backoff on second attempt, got %v", b2)
	}
	if b3 != 4*time.Second {
		t.Errorf("Expected 4s backoff on third attempt, got %v", b3)
	}

	// Suppression must remain false throughout reconnection attempts
	if mockBackend.IsSuppressed() {
		t.Fatalf("Suppression must remain false throughout reconnecting attempts")
	}
}

// TestMetrics_LatencyTracking verifies RTT and moving average latency calculation.
func TestMetrics_LatencyTracking(t *testing.T) {
	mt := NewMetricsTracker()

	mt.RecordPingSent(1)
	time.Sleep(5 * time.Millisecond)
	rtt := mt.RecordPongRecv(1)

	if rtt <= 0 {
		t.Errorf("Expected positive RTT, got %v", rtt)
	}

	avg := mt.AverageLatency()
	if avg <= 0 {
		t.Errorf("Expected positive smoothed latency, got %v", avg)
	}

	mt.RecordSent()
	mt.RecordRecv()
	mt.RecordReconnectAttempt()

	summary := mt.Summary()
	if summary == "" {
		t.Errorf("Expected non-empty summary string")
	}
}
