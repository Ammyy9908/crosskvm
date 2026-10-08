//go:build darwin

package input

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
)

type darwinPoster func(spec *DarwinEventSpec) error

// DarwinBackend is the native macOS implementation of InputBackend using CoreGraphics CGEvents.
type DarwinBackend struct {
	mu               sync.Mutex
	tracker          *DarwinStateTracker
	captureState     *DarwinCaptureState
	poster           darwinPoster
	cursorHidden     bool
	cursorVisibility func(bool) error
	session          *NativeCaptureSession
	eventsCh         chan<- InputEvent
	captureDone      chan struct{}
	suppressed       atomic.Bool
	closed           atomic.Bool
	bounds           DarwinRect
}

// NewNativeBackend initializes the native macOS input backend.
func NewNativeBackend() (InputBackend, error) {
	return NewDarwinBackend()
}

// NewDarwinBackend creates a new DarwinBackend.
func NewDarwinBackend() (*DarwinBackend, error) {
	// Startup must not repeatedly request the system permission dialog.
	// Capture and injection check trust without prompting; the desktop UI
	// reports missing access and directs the user to System Settings.

	loc := GetNativeCursorLocation()
	bounds := GetPrimaryDisplayBounds()

	tracker := NewDarwinStateTracker(loc, bounds)
	return &DarwinBackend{
		tracker: tracker,
		poster:  PostDarwinSpec,
	}, nil
}

// Inject translates a platform-neutral InputEvent into a native CGEvent and posts it to macOS window server.
func (b *DarwinBackend) Inject(event InputEvent) error {
	if b.closed.Load() {
		return ErrBackendClosed
	}
	b.mu.Lock()
	tracker := b.tracker
	poster := b.poster
	b.mu.Unlock()

	// Check accessibility permission without re-triggering dialogs
	if !CheckAccessibilityPermission(false) {
		return ErrAccessibilityNotGranted
	}

	spec, err := tracker.Translate(event)
	if err != nil {
		return fmt.Errorf("macos inject: %w", err)
	}

	if spec == nil {
		return nil
	}

	if poster == nil {
		poster = PostDarwinSpec
	}

	if err := poster(spec); err != nil {
		return fmt.Errorf("macos inject: %w", err)
	}

	return nil
}

// StartCapture starts observing physical macOS input events via CGEventTap and sends them to events.
func (b *DarwinBackend) StartCapture(events chan<- InputEvent) error {
	if events == nil {
		return errors.New("events channel cannot be nil")
	}

	b.mu.Lock()
	if b.closed.Load() {
		b.mu.Unlock()
		return ErrBackendClosed
	}
	if b.eventsCh != nil || b.session != nil {
		b.mu.Unlock()
		return ErrCaptureAlreadyBusy
	}

	if !CheckAccessibilityPermission(false) {
		b.mu.Unlock()
		return ErrAccessibilityNotGranted
	}

	session, err := StartNativeCapture(b)
	if err != nil {
		b.mu.Unlock()
		return err
	}

	b.eventsCh = events
	b.captureState = NewDarwinCaptureState()
	b.session = session
	b.bounds = GetPrimaryDisplayBounds()
	if b.suppressed.Load() {
		session.SetSuppressed(true)
	}
	captureDone := make(chan struct{})
	b.captureDone = captureDone
	b.mu.Unlock()

	startedCh := make(chan struct{})

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		session.Run(startedCh, captureDone)

		b.mu.Lock()
		b.session = nil
		b.mu.Unlock()
	}()

	<-startedCh
	return nil
}

// handleCapturedEvent processes raw event tap notifications and dispatches translated events.
// Designed for extreme performance: lock-free hot path, zero per-event syscalls, zero per-event logging.
func (b *DarwinBackend) handleCapturedEvent(eventType uint32, x, y float64, buttonNum, deltaY, deltaX int64, keyCode uint16, flags uint64, userData int64) {
	if b.closed.Load() {
		return
	}
	eventsCh := b.eventsCh
	captureState := b.captureState
	if eventsCh == nil || captureState == nil {
		return
	}
	suppressed := b.suppressed.Load()

	// Physical trackpad movement must update the injection origin. Otherwise
	// the next remote event snaps back to the last remotely tracked position.
	if !suppressed && userData != CrossKVMMarker {
		switch eventType {
		case CGEventTypeMouseMoved, CGEventTypeLeftMouseDragged, CGEventTypeRightMouseDragged, CGEventTypeOtherMouseDragged:
			b.tracker.SetCursorPos(DarwinPoint{X: x, Y: y})
		}
	}

	switch eventType {
	case CGEventTypeMouseMoved, CGEventTypeLeftMouseDragged, CGEventTypeRightMouseDragged, CGEventTypeOtherMouseDragged,
		CGEventTypeLeftMouseDown, CGEventTypeLeftMouseUp,
		CGEventTypeRightMouseDown, CGEventTypeRightMouseUp,
		CGEventTypeOtherMouseDown, CGEventTypeOtherMouseUp:
		if ev, ok := captureState.TranslateCapturedMouse(eventType, x, y, buttonNum, deltaY, deltaX, userData); ok {
			select {
			case eventsCh <- ev:
			default:
			}
			if suppressed && ev.Type == EventTypeMouseMove {
				cx, cy := b.bounds.Width/2, b.bounds.Height/2
				if x != cx || y != cy {
					captureState.SetCentering(true)
					WarpNativeCursor(cx, cy)
				}
			}
		}

	case CGEventTypeScrollWheel:
		if ev, ok := captureState.TranslateCapturedScroll(deltaY, deltaX, userData); ok {
			select {
			case eventsCh <- ev:
			default:
			}
		}

	case CGEventTypeKeyDown, CGEventTypeKeyUp, CGEventTypeFlagsChanged:
		if evs, ok := captureState.TranslateCapturedKeyboard(eventType, keyCode, flags, userData); ok {
			for _, ev := range evs {
				select {
				case eventsCh <- ev:
				default:
				}
			}
		}
	}
}

// setCursorHidden avoids unbalanced hide/show counts on repeated state changes.
func (b *DarwinBackend) setCursorHidden(hidden bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if os.Getenv("CROSSKVM_DESKTOP_CURSOR") == "1" {
		return nil
	}
	if b.cursorHidden == hidden {
		return nil
	}
	setVisibility := b.cursorVisibility
	if setVisibility == nil {
		setVisibility = SetNativeCursorHidden
	}
	if err := setVisibility(hidden); err != nil {
		return err
	}
	b.cursorHidden = hidden
	return nil
}

// SetLocalSuppressed configures whether captured events should be suppressed locally.
func (b *DarwinBackend) SetLocalSuppressed(enabled bool) error {
	if b.closed.Load() {
		return ErrBackendClosed
	}
	visibilityErr := b.setCursorHidden(enabled)
	b.suppressed.Store(enabled)

	b.mu.Lock()
	session := b.session
	captureState := b.captureState
	b.mu.Unlock()

	if session != nil {
		session.SetSuppressed(enabled)
	}
	if enabled {
		bounds := GetPrimaryDisplayBounds()
		b.bounds = bounds
		cx := bounds.Width / 2
		cy := bounds.Height / 2
		if captureState != nil {
			captureState.SetCentering(true)
		}
		WarpNativeCursor(cx, cy)
	} else {
		if captureState != nil {
			captureState.ResetBaseline()
		}
	}
	return visibilityErr
}

// ScreenBounds returns the pixel dimensions of the primary display.
func (b *DarwinBackend) ScreenBounds() ScreenBounds {
	bounds := GetPrimaryDisplayBounds()
	b.bounds = bounds
	return ScreenBounds{
		Width:  int(bounds.Width),
		Height: int(bounds.Height),
	}
}

// WarpCursor moves the native macOS cursor to the given coordinates.
func (b *DarwinBackend) WarpCursor(x, y int) error {
	if b.closed.Load() {
		return ErrBackendClosed
	}
	b.mu.Lock()
	tracker := b.tracker
	b.mu.Unlock()

	WarpNativeCursor(float64(x), float64(y))
	if tracker != nil {
		tracker.SetCursorPos(DarwinPoint{X: float64(x), Y: float64(y)})
	}
	return nil
}

// Close gracefully terminates the backend and any active event tap. Safe and idempotent.
func (b *DarwinBackend) Close() error {
	if b.closed.Swap(true) {
		return nil
	}
	visibilityErr := b.setCursorHidden(false)
	b.suppressed.Store(false)

	b.mu.Lock()
	session := b.session
	captureDone := b.captureDone
	b.mu.Unlock()

	if session != nil {
		session.SetSuppressed(false)
		session.Stop()
	}

	if captureDone != nil {
		<-captureDone
	}

	b.mu.Lock()
	b.eventsCh = nil
	b.mu.Unlock()

	return visibilityErr
}
