package control

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/crosskvm/crosskvm/internal/input"
	"github.com/crosskvm/crosskvm/internal/protocol"
	"github.com/crosskvm/crosskvm/internal/transport"
)

var (
	ErrNoActiveConnection = errors.New("control: no active peer connection")
	ErrRouterClosed       = errors.New("control: router is closed")
)

// PeerSide defines which side of the local display the remote peer is positioned.
type PeerSide string

const (
	PeerSideNone   PeerSide = "none"
	PeerSideLeft   PeerSide = "left"
	PeerSideRight  PeerSide = "right"
	PeerSideTop    PeerSide = "top"
	PeerSideBottom PeerSide = "bottom"
)

// IsScreenSide reports whether the peer is adjacent to one display edge.
func (side PeerSide) IsScreenSide() bool {
	return side == PeerSideLeft || side == PeerSideRight || side == PeerSideTop || side == PeerSideBottom
}
func (side PeerSide) vertical() bool { return side == PeerSideTop || side == PeerSideBottom }
func (side PeerSide) highEdge() bool { return side == PeerSideRight || side == PeerSideBottom }
func (side PeerSide) opposite() string {
	switch side {
	case PeerSideLeft:
		return "right"
	case PeerSideRight:
		return "left"
	case PeerSideTop:
		return "bottom"
	case PeerSideBottom:
		return "top"
	}
	return ""
}
func clampCoordinate(value, size int) int {
	if value < 0 {
		return 0
	}
	if value >= size {
		return size - 1
	}
	return value
}
func entryCoordinates(bounds input.ScreenBounds, side string, normX, normY float64, inset int) (int, int) {
	x, y := int(normX*float64(bounds.Width)), int(normY*float64(bounds.Height))
	switch side {
	case "left":
		x = inset
	case "right":
		x = bounds.Width - inset - 1
	case "top":
		y = inset
	case "bottom":
		y = bounds.Height - inset - 1
	}
	return clampCoordinate(x, bounds.Width), clampCoordinate(y, bounds.Height)
}

// RouterConfig contains tuning parameters for edge detection and screen switching.
type RouterConfig struct {
	PeerSide         PeerSide
	EdgeThreshold    int // Border proximity in pixels (default: 3)
	HysteresisOffset int // Offset in pixels from edge to re-arm switching (default: 20)
}

// DefaultRouterConfig returns the default configuration for the router.
func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		PeerSide:         PeerSideNone,
		EdgeThreshold:    3,
		HysteresisOffset: 20,
	}
}

// Router coordinates input events, ownership state, screen-edge detection, and network transport.
type Router struct {
	mu         sync.RWMutex
	stateMgr   *StateManager
	backend    input.InputBackend
	activeConn *transport.Conn
	connMu     sync.RWMutex
	seq        atomic.Uint64
	logger     *log.Logger

	// Configuration
	peerSide         PeerSide
	edgeThreshold    int
	hysteresisOffset int

	// Geometry and Cursor Tracking
	localBounds          input.ScreenBounds
	remoteBounds         input.ScreenBounds
	localCursorX         int
	localCursorY         int
	remoteLogicalX       int
	remoteLogicalY       int
	edgeArmed            bool
	remoteEdgeArmed      bool
	isControlledByRemote bool

	// Stuck-key and button tracking for clean release on transition/disconnect
	pressedKeys    map[string]bool
	pressedButtons map[input.MouseButton]bool

	// Emergency hotkey modifier tracking
	ctrlHeld  bool
	altHeld   bool
	shiftHeld bool

	// Protected prioritized outbound event queue & metrics
	eventQueue *EventQueue
	metrics    *MetricsTracker
	doneCh     chan struct{}
	closed     bool
}

// NewRouter creates a new Router with the provided state manager, backend, and logger.
func NewRouter(stateMgr *StateManager, backend input.InputBackend, logger *log.Logger) *Router {
	return NewRouterWithConfig(stateMgr, backend, DefaultRouterConfig(), logger)
}

// NewRouterWithConfig creates a new Router with custom configuration.
func NewRouterWithConfig(stateMgr *StateManager, backend input.InputBackend, cfg RouterConfig, logger *log.Logger) *Router {
	if stateMgr == nil {
		stateMgr = NewStateManager()
	}
	if backend == nil {
		backend = input.NewMockBackend()
	}

	if cfg.EdgeThreshold <= 0 {
		cfg.EdgeThreshold = 3
	}
	if cfg.HysteresisOffset <= 0 {
		cfg.HysteresisOffset = 20
	}

	localBounds := backend.ScreenBounds()
	if localBounds.Width <= 0 || localBounds.Height <= 0 {
		localBounds = input.ScreenBounds{Width: 1920, Height: 1080}
	}

	r := &Router{
		stateMgr:         stateMgr,
		backend:          backend,
		logger:           logger,
		peerSide:         cfg.PeerSide,
		edgeThreshold:    cfg.EdgeThreshold,
		hysteresisOffset: cfg.HysteresisOffset,
		localBounds:      localBounds,
		remoteBounds:     input.ScreenBounds{Width: 1920, Height: 1080},
		localCursorX:     localBounds.Width / 2,
		localCursorY:     localBounds.Height / 2,
		edgeArmed:        true,
		remoteEdgeArmed:  false,
		pressedKeys:      make(map[string]bool),
		pressedButtons:   make(map[input.MouseButton]bool),
		eventQueue:       NewEventQueue(1024),
		metrics:          NewMetricsTracker(),
		doneCh:           make(chan struct{}),
	}

	if recovery, ok := backend.(interface{ SetEmergencyReleaseHandler(func()) }); ok {
		recovery.SetEmergencyReleaseHandler(r.EmergencyStop)
	}

	// Start asynchronous outbound network writer
	go r.outboundWorker()

	// Synchronize backend suppression with state manager transitions
	stateMgr.OnStateChange(func(oldState, newState State) {
		if r.logger != nil {
			r.logger.Printf("[STATE] Transition from %s to %s", oldState, newState)
		}
		if newState == StateRemote {
			if err := r.backend.SetLocalSuppressed(true); err != nil && r.logger != nil {
				r.logger.Printf("[INPUT ERROR] Suppression/cursor control: %v", err)
			}
		} else {
			_ = r.backend.SetLocalSuppressed(false)
		}
	})

	return r
}

// StateManager returns the underlying StateManager.
func (r *Router) StateManager() *StateManager {
	return r.stateMgr
}

// Backend returns the underlying InputBackend.
func (r *Router) Backend() input.InputBackend {
	return r.backend
}

// SetPeerSide configures the peer position ("left", "right", "top", "bottom", or "none").
func (r *Router) SetPeerSide(side PeerSide) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peerSide = side
}

// PeerSide returns the configured peer position.
func (r *Router) PeerSide() PeerSide {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.peerSide
}

// SetLocalBounds sets the local display pixel bounds.
func (r *Router) SetLocalBounds(bounds input.ScreenBounds) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if bounds.Width > 0 && bounds.Height > 0 {
		r.localBounds = bounds
	}
}

// LocalBounds returns the local display pixel bounds.
func (r *Router) LocalBounds() input.ScreenBounds {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.localBounds
}

// SetRemoteBounds sets the peer's display pixel bounds received during handshake.
func (r *Router) SetRemoteBounds(bounds input.ScreenBounds) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if bounds.Width > 0 && bounds.Height > 0 {
		r.remoteBounds = bounds
	}
}

// RemoteBounds returns the peer's display pixel bounds.
func (r *Router) RemoteBounds() input.ScreenBounds {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.remoteBounds
}

// SetLocalCursor sets the current local cursor coordinates (used by tests/synchronizers).
func (r *Router) SetLocalCursor(x, y int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.localCursorX = x
	r.localCursorY = y
}

// LocalCursor returns the currently tracked local cursor coordinates.
func (r *Router) LocalCursor() (int, int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.localCursorX, r.localCursorY
}

// RemoteLogicalCursor returns the currently tracked remote logical cursor coordinates.
func (r *Router) RemoteLogicalCursor() (int, int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.remoteLogicalX, r.remoteLogicalY
}

// IsEdgeArmed returns whether screen edge switching is currently armed.
func (r *Router) IsEdgeArmed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.edgeArmed
}

// IsControlledByRemote returns whether this machine is currently controlled by the remote peer.
func (r *Router) IsControlledByRemote() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isControlledByRemote
}

// SetConnection assigns the active peer transport connection.
func (r *Router) SetConnection(conn *transport.Conn) {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	r.activeConn = conn
}

// Connection returns the active peer connection, or nil.
func (r *Router) Connection() *transport.Conn {
	r.connMu.RLock()
	defer r.connMu.RUnlock()
	return r.activeConn
}

// RouteLocalEvent processes an event captured locally from the OS or simulated via debug REPL.
func (r *Router) RouteLocalEvent(event input.InputEvent) error {
	if r.metrics != nil {
		r.metrics.RecordCaptureEvent()
		if event.Type == input.EventTypeMouseMove {
			r.metrics.RecordMouseEvent()
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrRouterClosed
	}

	// 1. Emergency release shortcut detection: Ctrl + Alt + Shift + Escape
	if r.checkEmergencyChord(event) {
		if r.logger != nil {
			r.logger.Printf("[EMERGENCY] Ctrl+Alt+Shift+Escape detected! Restoring local control immediately.")
		}
		r.emergencyRestoreLocked()
		return nil
	}

	// 2. Local State: Handle edge detection and normal local movement
	if r.stateMgr.IsLocal() {
		return r.handleLocalEventLocked(event)
	}

	// 3. Remote State: Handle remote cursor integration, return-edge detection, and event forwarding
	if r.stateMgr.IsRemote() {
		return r.handleRemoteEventLocked(event)
	}

	return nil
}

// EmergencyStop severs the peer session so incoming remote events cannot keep
// controlling this machine after its physical keyboard requests recovery.
func (r *Router) EmergencyStop() {
	conn := r.Connection()
	if conn != nil {
		_ = conn.Close()
	}
	r.mu.Lock()
	r.emergencyRestoreLocked()
	r.edgeArmed = false
	r.remoteEdgeArmed = false
	r.connMu.Lock()
	r.activeConn = nil
	r.connMu.Unlock()
	r.mu.Unlock()
}

// checkEmergencyChord tracks modifier keys and checks if the Ctrl+Alt+Shift+Escape chord was pressed.
func (r *Router) checkEmergencyChord(event input.InputEvent) bool {
	if event.Type == input.EventTypeKeyDown {
		keyLower := strings.ToLower(strings.TrimSpace(event.Key))
		switch keyLower {
		case "controlleft", "controlright", "control", "ctrl":
			r.ctrlHeld = true
		case "altleft", "altright", "alt", "option":
			r.altHeld = true
		case "shiftleft", "shiftright", "shift":
			r.shiftHeld = true
		}

		// Also check bitmask flags if available
		ctrlMask := (event.Modifiers & input.ModCtrl) != 0
		altMask := (event.Modifiers & input.ModAlt) != 0
		shiftMask := (event.Modifiers & input.ModShift) != 0

		isCtrl := r.ctrlHeld || ctrlMask
		isAlt := r.altHeld || altMask
		isShift := r.shiftHeld || shiftMask

		if (keyLower == "escape" || keyLower == "esc") && isCtrl && isAlt && isShift {
			return true
		}
	} else if event.Type == input.EventTypeKeyUp {
		keyLower := strings.ToLower(strings.TrimSpace(event.Key))
		switch keyLower {
		case "controlleft", "controlright", "control", "ctrl":
			r.ctrlHeld = false
		case "altleft", "altright", "alt", "option":
			r.altHeld = false
		case "shiftleft", "shiftright", "shift":
			r.shiftHeld = false
		}
	}

	return false
}

// emergencyRestoreLocked restores local control unconditionally. Must be called with r.mu held.
func (r *Router) emergencyRestoreLocked() {
	r.isControlledByRemote = false
	if r.stateMgr.IsRemote() {
		r.releaseAllHeldInputsLocked()
		_ = r.stateMgr.RestoreLocal()
		_ = r.backend.SetLocalSuppressed(false)
		r.edgeArmed = false
		r.remoteEdgeArmed = false
	}
}

// ReleaseToLocal safely transitions input ownership back to local machine and disables suppression.
func (r *Router) ReleaseToLocal() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emergencyRestoreLocked()
}

// handleLocalEventLocked handles events when control is on the local machine. Must be called with r.mu held.
func (r *Router) handleLocalEventLocked(event input.InputEvent) error {
	r.connMu.RLock()
	conn := r.activeConn
	r.connMu.RUnlock()

	hasPeer := conn != nil && !conn.IsClosed()

	if event.Type == input.EventTypeMouseMove {
		if event.HasAbs {
			r.localCursorX = int(event.AbsX)
			r.localCursorY = int(event.AbsY)
		} else {
			r.localCursorX += int(event.DX)
			r.localCursorY += int(event.DY)
		}

		// Clamp local cursor coordinates within bounds
		if r.localCursorX < 0 {
			r.localCursorX = 0
		} else if r.localCursorX >= r.localBounds.Width {
			r.localCursorX = r.localBounds.Width - 1
		}
		if r.localCursorY < 0 {
			r.localCursorY = 0
		} else if r.localCursorY >= r.localBounds.Height {
			r.localCursorY = r.localBounds.Height - 1
		}

		position, size, delta := r.localCursorX, r.localBounds.Width, event.DX
		if r.peerSide.vertical() {
			position, size, delta = r.localCursorY, r.localBounds.Height, event.DY
		}
		armDistance := r.hysteresisOffset * 2
		if r.peerSide.IsScreenSide() {
			if (r.peerSide.highEdge() && position < size-armDistance) || (!r.peerSide.highEdge() && position > armDistance) {
				r.edgeArmed = true
			}
			if hasPeer && r.edgeArmed && ((r.peerSide.highEdge() && position >= size-r.edgeThreshold && delta > 0) || (!r.peerSide.highEdge() && position <= r.edgeThreshold && delta < 0)) {
				return r.switchToRemoteLocked(r.peerSide.opposite())
			}
		}
	}

	return nil
}

// switchToRemoteLocked switches input ownership to the remote peer. Must be called with r.mu held.
func (r *Router) switchToRemoteLocked(remoteEntrySide string) error {
	r.isControlledByRemote = false
	normY := float64(r.localCursorY) / float64(r.localBounds.Height)
	if normY < 0.0 {
		normY = 0.0
	} else if normY > 1.0 {
		normY = 1.0
	}

	normX := float64(r.localCursorX) / float64(r.localBounds.Width)
	r.remoteLogicalX, r.remoteLogicalY = entryCoordinates(r.remoteBounds, remoteEntrySide, normX, normY, 1)

	// Remote return edge starts unarmed until the user moves into the remote screen
	r.remoteEdgeArmed = false

	if err := r.stateMgr.SwitchToRemote(); err != nil {
		return err
	}

	if r.logger != nil {
		r.logger.Printf("[CONTROL] Switched ownership to REMOTE (entrySide=%s, normY=%.3f, remoteX=%d, remoteY=%d)",
			remoteEntrySide, normY, r.remoteLogicalX, r.remoteLogicalY)
	}

	// Notify peer to prepare remote entry point
	switchMsg, err := protocol.NewSwitchControlMessageAtPosition(r.seq.Add(1), remoteEntrySide, normX, normY, "edge transition")
	if err == nil {
		r.enqueueOutboundLocked(switchMsg)
	}

	return nil
}

// handleRemoteEventLocked handles events when ownership is with the remote peer. Must be called with r.mu held.
func (r *Router) handleRemoteEventLocked(event input.InputEvent) error {
	r.connMu.RLock()
	conn := r.activeConn
	r.connMu.RUnlock()

	if conn == nil || conn.IsClosed() {
		// Peer connection dropped: restore local ownership immediately
		r.emergencyRestoreLocked()
		return ErrNoActiveConnection
	}

	if event.Type == input.EventTypeMouseMove {
		r.remoteLogicalX += int(event.DX)
		r.remoteLogicalY += int(event.DY)

		// Clamp coordinates within remote display bounds
		if r.remoteLogicalX < 0 {
			r.remoteLogicalX = 0
		} else if r.remoteLogicalX >= r.remoteBounds.Width {
			r.remoteLogicalX = r.remoteBounds.Width - 1
		}
		if r.remoteLogicalY < 0 {
			r.remoteLogicalY = 0
		} else if r.remoteLogicalY >= r.remoteBounds.Height {
			r.remoteLogicalY = r.remoteBounds.Height - 1
		}

		position, size, delta := r.remoteLogicalX, r.remoteBounds.Width, event.DX
		if r.peerSide.vertical() {
			position, size, delta = r.remoteLogicalY, r.remoteBounds.Height, event.DY
		}
		armDistance := r.hysteresisOffset * 2
		if r.peerSide.IsScreenSide() {
			if (r.peerSide.highEdge() && position > armDistance) || (!r.peerSide.highEdge() && position < size-armDistance) {
				r.remoteEdgeArmed = true
			}
			if r.remoteEdgeArmed && ((r.peerSide.highEdge() && position <= r.edgeThreshold && delta < 0) || (!r.peerSide.highEdge() && position >= size-r.edgeThreshold && delta > 0)) {
				return r.returnToLocalLocked()
			}
		}
	}

	// Track pressed keys and buttons
	switch event.Type {
	case input.EventTypeKeyDown:
		if event.Key != "" {
			r.pressedKeys[event.Key] = true
		}
	case input.EventTypeKeyUp:
		delete(r.pressedKeys, event.Key)
	case input.EventTypeMouseButtonDown:
		if event.Button != "" {
			r.pressedButtons[event.Button] = true
		}
	case input.EventTypeMouseButtonUp:
		delete(r.pressedButtons, event.Button)
	}

	// Forward event to peer
	msg := protocol.NewInputMessage(r.seq.Add(1), event)
	r.enqueueOutboundLocked(msg)
	return nil
}

// returnToLocalLocked transitions ownership from REMOTE back to LOCAL. Must be called with r.mu held.
func (r *Router) returnToLocalLocked() error {
	normY := float64(r.remoteLogicalY) / float64(r.remoteBounds.Height)
	if normY < 0.0 {
		normY = 0.0
	} else if normY > 1.0 {
		normY = 1.0
	}

	normX := float64(r.remoteLogicalX) / float64(r.remoteBounds.Width)
	returnLocalX, returnLocalY := entryCoordinates(r.localBounds, string(r.peerSide), normX, normY, r.edgeThreshold+r.hysteresisOffset)
	// Preserve the existing horizontal return offset.
	if r.peerSide.highEdge() {
		if r.peerSide.vertical() {
			returnLocalY = clampCoordinate(returnLocalY+1, r.localBounds.Height)
		} else {
			returnLocalX = clampCoordinate(returnLocalX+1, r.localBounds.Width)
		}
	}

	// Notify peer of release
	releaseMsg, err := protocol.NewReleaseControlMessage(r.seq.Add(1), "return edge transition")
	if err == nil {
		r.enqueueOutboundLocked(releaseMsg)
	}

	// Release any pressed keys/buttons on remote before switching
	r.releaseAllHeldInputsLocked()

	// Switch state manager to local
	if err := r.stateMgr.RestoreLocal(); err != nil {
		return err
	}

	// Reposition native cursor
	_ = r.backend.WarpCursor(returnLocalX, returnLocalY)
	r.localCursorX = returnLocalX
	r.localCursorY = returnLocalY
	r.edgeArmed = false
	r.remoteEdgeArmed = false

	if r.logger != nil {
		r.logger.Printf("[CONTROL] Restored ownership to LOCAL (returnX=%d, returnY=%d, edgeArmed=false)", returnLocalX, returnLocalY)
	}

	return nil
}

// releaseAllHeldInputsLocked enqueues KeyUp and MouseButtonUp for all tracked pressed inputs. Must be called with r.mu held.
func (r *Router) releaseAllHeldInputsLocked() {
	for key := range r.pressedKeys {
		upEv := input.NewKeyUpEvent(key, 0)
		msg := protocol.NewInputMessage(r.seq.Add(1), upEv)
		r.enqueueOutboundLocked(msg)
	}
	clear(r.pressedKeys)

	for btn := range r.pressedButtons {
		upEv := input.NewMouseButtonUpEvent(btn)
		msg := protocol.NewInputMessage(r.seq.Add(1), upEv)
		r.enqueueOutboundLocked(msg)
	}
	clear(r.pressedButtons)
}

// enqueueOutboundLocked pushes a message to the prioritized event queue.
func (r *Router) enqueueOutboundLocked(msg protocol.Message) {
	if r.eventQueue != nil {
		r.eventQueue.Enqueue(msg)
	}
}

// outboundWorker runs on a dedicated goroutine and flushes messages to the active TCP connection.
func (r *Router) outboundWorker() {
	for {
		select {
		case <-r.doneCh:
			return
		default:
		}

		msg, ok := r.eventQueue.Dequeue()
		if !ok {
			return
		}

		now := time.Now().UnixNano()
		msg.WriteTime = now
		if r.metrics != nil && msg.QueueTime > 0 {
			dwell := time.Duration(now - msg.QueueTime)
			if dwell > 0 {
				r.metrics.RecordQueueDwell(dwell)
			}
		}

		r.connMu.RLock()
		conn := r.activeConn
		r.connMu.RUnlock()

		if conn != nil && !conn.IsClosed() {
			if err := conn.Send(msg); err != nil {
				if r.logger != nil {
					r.logger.Printf("[ERROR] Outbound send error: %v", err)
				}
				// On write failure, if in remote state, restore local control safely
				r.mu.Lock()
				if r.stateMgr.IsRemote() {
					r.emergencyRestoreLocked()
				}
				r.mu.Unlock()
			} else {
				if r.metrics != nil {
					r.metrics.RecordSent()
				}
			}
		}
	}
}

// Metrics returns the router's metrics tracker.
func (r *Router) Metrics() *MetricsTracker {
	return r.metrics
}

// EventQueue returns the prioritized outbound event queue.
func (r *Router) EventQueue() *EventQueue {
	return r.eventQueue
}

// HandleRemoteMessage processes an incoming protocol message from the peer.
func (r *Router) HandleRemoteMessage(msg protocol.Message) error {
	tRecv := time.Now().UnixNano()
	if r.metrics != nil {
		r.metrics.RecordRecv()
	}

	switch msg.Type {
	case protocol.MessageTypeInput:
		if msg.Input == nil {
			return protocol.ErrMissingInputEvent
		}
		if msg.Input.Type == input.EventTypeMouseMove {
			r.mu.Lock()
			r.localCursorX += int(msg.Input.DX)
			r.localCursorY += int(msg.Input.DY)
			if r.localCursorX < 0 {
				r.localCursorX = 0
			} else if r.localCursorX >= r.localBounds.Width {
				r.localCursorX = r.localBounds.Width - 1
			}
			if r.localCursorY < 0 {
				r.localCursorY = 0
			} else if r.localCursorY >= r.localBounds.Height {
				r.localCursorY = r.localBounds.Height - 1
			}
			r.mu.Unlock()
		}

		err := r.backend.Inject(*msg.Input)
		tInjectEnd := time.Now().UnixNano()

		if r.metrics != nil {
			injLatency := time.Duration(tInjectEnd - tRecv)
			r.metrics.RecordInjectionLatency(injLatency)

			capTime := msg.Input.Timestamp
			if capTime == 0 {
				capTime = msg.Timestamp
			}

			if capTime > 0 {
				clockOffset := r.metrics.ClockOffset().Nanoseconds()
				calibratedE2ENs := (tInjectEnd - capTime) - clockOffset
				calibratedE2E := time.Duration(calibratedE2ENs)

				// Synthetic pipeline stages calculation:
				// (CapToQueue) + (QueueDwell) + (NetworkTransit RTT/2) + (InjectionLatency)
				var syntheticE2E time.Duration
				networkTransit := r.metrics.AverageLatency()
				if networkTransit <= 0 {
					networkTransit = 500 * time.Microsecond
				}
				var queueDwell time.Duration
				if msg.WriteTime > 0 && msg.QueueTime > 0 && msg.WriteTime >= msg.QueueTime {
					queueDwell = time.Duration(msg.WriteTime - msg.QueueTime)
				}
				var capToQueue time.Duration
				if msg.QueueTime > 0 && capTime > 0 && msg.QueueTime >= capTime {
					capToQueue = time.Duration(msg.QueueTime - capTime)
				}
				syntheticE2E = capToQueue + queueDwell + networkTransit + injLatency

				if calibratedE2E > 0 && calibratedE2E < 2*time.Second {
					r.metrics.RecordEndToEndLatency(calibratedE2E)
				} else if syntheticE2E > 0 {
					r.metrics.RecordEndToEndLatency(syntheticE2E)
				}
			}
		}
		return err

	case protocol.MessageTypeSwitchControl:
		var payload protocol.ControlPayload
		if len(msg.Payload) > 0 {
			_ = json.Unmarshal(msg.Payload, &payload)
		}

		r.mu.Lock()
		if r.stateMgr.IsRemote() {
			_ = r.stateMgr.RestoreLocal()
			_ = r.backend.SetLocalSuppressed(false)
		}
		r.isControlledByRemote = true
		targetX, targetY := entryCoordinates(r.localBounds, payload.Side, payload.NormalizedX, payload.NormalizedY, 1)
		// Existing peers enter left at x=2; retain that wire behavior.
		if payload.Side == "left" {
			targetX = clampCoordinate(2, r.localBounds.Width)
		}
		if payload.Side == "top" {
			targetY = clampCoordinate(2, r.localBounds.Height)
		}
		r.localCursorX = targetX
		r.localCursorY = targetY
		r.edgeArmed = false
		r.mu.Unlock()

		_ = r.backend.WarpCursor(targetX, targetY)
		if r.logger != nil {
			r.logger.Printf("[RECV SWITCH_CONTROL] Remote entered screen at (%d, %d)", targetX, targetY)
		}
		return nil

	case protocol.MessageTypeReleaseControl:
		r.mu.Lock()
		if r.stateMgr.IsRemote() {
			_ = r.stateMgr.RestoreLocal()
			_ = r.backend.SetLocalSuppressed(false)
		}
		r.isControlledByRemote = false
		r.edgeArmed = false
		r.mu.Unlock()
		if r.logger != nil {
			r.logger.Printf("[RECV RELEASE_CONTROL] Peer released control back to this machine")
		}
		return nil

	case protocol.MessageTypeHandshake:
		var payload protocol.HandshakePayload
		if len(msg.Payload) > 0 {
			if err := json.Unmarshal(msg.Payload, &payload); err == nil {
				if payload.ScreenWidth > 0 && payload.ScreenHeight > 0 {
					r.SetRemoteBounds(input.ScreenBounds{Width: payload.ScreenWidth, Height: payload.ScreenHeight})
					if r.logger != nil {
						r.logger.Printf("[HANDSHAKE] Peer display bounds: %dx%d (OS: %s, DeviceID: %s)",
							payload.ScreenWidth, payload.ScreenHeight, payload.OS, payload.DeviceID)
					}
				}
			}
		}
		return nil

	case protocol.MessageTypeHandshakeAck:
		var payload protocol.HandshakeAckPayload
		if len(msg.Payload) > 0 {
			if err := json.Unmarshal(msg.Payload, &payload); err == nil {
				if payload.ScreenWidth > 0 && payload.ScreenHeight > 0 {
					r.SetRemoteBounds(input.ScreenBounds{Width: payload.ScreenWidth, Height: payload.ScreenHeight})
					if r.logger != nil {
						r.logger.Printf("[HANDSHAKE_ACK] Peer display bounds: %dx%d (DeviceID: %s)",
							payload.ScreenWidth, payload.ScreenHeight, payload.DeviceID)
					}
				}
			}
		}
		return nil

	case protocol.MessageTypePing:
		r.connMu.RLock()
		conn := r.activeConn
		r.connMu.RUnlock()
		if conn != nil && !conn.IsClosed() {
			return conn.Send(protocol.NewPongMessageWithTiming(msg.Seq, msg.Timestamp))
		}
		return nil

	case protocol.MessageTypePong:
		if r.metrics != nil {
			var remoteTime int64
			if len(msg.Payload) > 0 {
				var pongPayload protocol.PongPayload
				if err := json.Unmarshal(msg.Payload, &pongPayload); err == nil {
					remoteTime = pongPayload.RemoteTime
				}
			}
			r.metrics.RecordPongRecvWithRemoteTime(msg.Seq, remoteTime)
		}
		if r.logger != nil {
			r.logger.Printf("[RECV PONG] seq=%d", msg.Seq)
		}
		return nil

	default:
		if r.logger != nil {
			r.logger.Printf("[RECV %s] %s", msg.Type, msg.String())
		}
		return nil
	}
}

// HandleDisconnect handles peer disconnection, ensuring local suppression is removed and control restored.
func (r *Router) HandleDisconnect(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.isControlledByRemote = false
	if r.stateMgr.IsRemote() {
		if r.logger != nil {
			r.logger.Printf("[DISCONNECT] Peer disconnected while Remote (%v). Restoring LOCAL control.", err)
		}
		r.emergencyRestoreLocked()
	}

	r.connMu.Lock()
	r.activeConn = nil
	r.connMu.Unlock()
}

// Close gracefully terminates the router and frees background workers.
func (r *Router) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true

	if r.stateMgr.IsRemote() {
		r.emergencyRestoreLocked()
	}
	_ = r.backend.SetLocalSuppressed(false)

	if r.eventQueue != nil {
		r.eventQueue.Close()
	}
	close(r.doneCh)
	r.mu.Unlock()

	return nil
}
