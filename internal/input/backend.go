package input

import (
	"errors"
	"sync"
)

var (
	ErrBackendClosed      = errors.New("input backend is closed")
	ErrCaptureAlreadyBusy = errors.New("input capture is already active")
	ErrNotImplemented     = errors.New("native input capture not implemented yet")
)

// ScreenBounds represents the pixel dimensions of a display.
type ScreenBounds struct {
	Width  int
	Height int
}

// NewBackend creates and initializes the native InputBackend for the running OS.
func NewBackend() (InputBackend, error) {
	return NewNativeBackend()
}

// InputBackend defines the interface for platform input capture and injection.
type InputBackend interface {
	// StartCapture starts capturing input events from the OS and sends them to the events channel.
	StartCapture(events chan<- InputEvent) error

	// Inject simulates/injects an input event into the local OS.
	Inject(event InputEvent) error

	// SetLocalSuppressed configures whether captured events should be suppressed locally (not sent to local OS).
	SetLocalSuppressed(enabled bool) error

	// ScreenBounds returns the pixel dimensions of the primary display.
	ScreenBounds() ScreenBounds

	// WarpCursor moves the OS cursor to the specified coordinates (used on return from remote screen).
	WarpCursor(x, y int) error

	// Close terminates any active capture hooks and frees OS resources.
	Close() error
}

// MockBackend is an in-memory test/fake implementation of InputBackend.
type MockBackend struct {
	mu           sync.Mutex
	eventsCh     chan<- InputEvent
	injected     []InputEvent
	suppressed   bool
	screenBounds ScreenBounds
	cursorX      int
	cursorY      int
	warpedX      int
	warpedY      int
	closed       bool
}

// NewMockBackend creates a mock input backend for testing and simulation.
func NewMockBackend() *MockBackend {
	return &MockBackend{
		injected:     make([]InputEvent, 0),
		screenBounds: ScreenBounds{Width: 1920, Height: 1080},
		cursorX:      960,
		cursorY:      540,
		warpedX:      960,
		warpedY:      540,
	}
}

func (m *MockBackend) StartCapture(events chan<- InputEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrBackendClosed
	}
	if m.eventsCh != nil {
		return ErrCaptureAlreadyBusy
	}
	m.eventsCh = events
	return nil
}

func (m *MockBackend) Inject(event InputEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrBackendClosed
	}
	if err := event.Validate(); err != nil {
		return err
	}
	m.injected = append(m.injected, event)
	return nil
}

func (m *MockBackend) SetLocalSuppressed(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrBackendClosed
	}
	m.suppressed = enabled
	return nil
}

func (m *MockBackend) ScreenBounds() ScreenBounds {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.screenBounds.Width <= 0 || m.screenBounds.Height <= 0 {
		return ScreenBounds{Width: 1920, Height: 1080}
	}
	return m.screenBounds
}

func (m *MockBackend) SetScreenBounds(bounds ScreenBounds) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.screenBounds = bounds
}

func (m *MockBackend) WarpCursor(x, y int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrBackendClosed
	}
	m.warpedX = x
	m.warpedY = y
	m.cursorX = x
	m.cursorY = y
	return nil
}

func (m *MockBackend) GetWarpedCursor() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.warpedX, m.warpedY
}

func (m *MockBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	m.suppressed = false
	m.eventsCh = nil
	return nil
}

// EmitFakeEvent allows tests and debug CLI to simulate an input event captured from OS.
func (m *MockBackend) EmitFakeEvent(event InputEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrBackendClosed
	}
	if m.eventsCh != nil {
		select {
		case m.eventsCh <- event:
		default:
		}
	}
	return nil
}

// GetInjectedEvents returns a copy of injected events (for testing).
func (m *MockBackend) GetInjectedEvents() []InputEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]InputEvent, len(m.injected))
	copy(copied, m.injected)
	return copied
}

// IsSuppressed returns current suppression state.
func (m *MockBackend) IsSuppressed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.suppressed
}
