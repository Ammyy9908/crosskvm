package input

import (
	"errors"
	"fmt"
	"sync"
)

// CoreGraphics Event Types (CGEventType)
const (
	CGEventTypeNull                uint32 = 0
	CGEventTypeLeftMouseDown       uint32 = 1
	CGEventTypeLeftMouseUp         uint32 = 2
	CGEventTypeRightMouseDown      uint32 = 3
	CGEventTypeRightMouseUp        uint32 = 4
	CGEventTypeMouseMoved          uint32 = 5
	CGEventTypeLeftMouseDragged    uint32 = 6
	CGEventTypeRightMouseDragged   uint32 = 7
	CGEventTypeKeyDown             uint32 = 10
	CGEventTypeKeyUp               uint32 = 11
	CGEventTypeFlagsChanged        uint32 = 12
	CGEventTypeScrollWheel         uint32 = 22
	CGEventTypeOtherMouseDown      uint32 = 25
	CGEventTypeOtherMouseUp        uint32 = 26
	CGEventTypeOtherMouseDragged   uint32 = 27
)

// CoreGraphics Mouse Button Constants (CGMouseButton)
const (
	CGMouseButtonLeft   uint32 = 0
	CGMouseButtonRight  uint32 = 1
	CGMouseButtonCenter uint32 = 2
)

var (
	ErrDarwinUnsupportedButton = errors.New("macos: unsupported mouse button")
)

// DarwinPoint represents a 2D coordinate on macOS screen.
type DarwinPoint struct {
	X float64
	Y float64
}

// DarwinRect represents the bounding box of a display on macOS.
type DarwinRect struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// DarwinEventKind discriminates the type of macOS event specification.
type DarwinEventKind uint8

const (
	DarwinEventKindMouse DarwinEventKind = iota
	DarwinEventKindKeyboard
	DarwinEventKindScroll
)

// DarwinMouseEventSpec contains arguments required to construct a mouse CGEvent.
type DarwinMouseEventSpec struct {
	EventType   uint32
	Location    DarwinPoint
	MouseButton uint32
	Flags       uint64
}

// DarwinKeyboardEventSpec contains arguments required to construct a keyboard CGEvent.
type DarwinKeyboardEventSpec struct {
	KeyCode uint16
	KeyDown bool
	Flags   uint64
}

// DarwinScrollEventSpec contains arguments required to construct a scroll wheel CGEvent.
type DarwinScrollEventSpec struct {
	WheelDY int32
	WheelDX int32
}

// DarwinEventSpec is a decoded platform-agnostic representation of a native macOS input event.
type DarwinEventSpec struct {
	Kind     DarwinEventKind
	Mouse    *DarwinMouseEventSpec
	Keyboard *DarwinKeyboardEventSpec
	Scroll   *DarwinScrollEventSpec
}

// DarwinStateTracker maintains ongoing cursor position, button drag state, and modifier flags.
type DarwinStateTracker struct {
	mu            sync.Mutex
	currentPos    DarwinPoint
	leftDown      bool
	rightDown     bool
	middleDown    bool
	activeFlags   uint64
	displayBounds DarwinRect
}

// NewDarwinStateTracker initializes state tracking.
func NewDarwinStateTracker(initialPos DarwinPoint, displayBounds DarwinRect) *DarwinStateTracker {
	return &DarwinStateTracker{
		currentPos:    initialPos,
		displayBounds: displayBounds,
	}
}

// SetCursorPos updates the tracked cursor position.
func (st *DarwinStateTracker) SetCursorPos(p DarwinPoint) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.currentPos = p
}

// CursorPos returns the current tracked cursor position.
func (st *DarwinStateTracker) CursorPos() DarwinPoint {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.currentPos
}

// SetDisplayBounds updates the display bounding box for clamping.
func (st *DarwinStateTracker) SetDisplayBounds(bounds DarwinRect) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.displayBounds = bounds
}

// ActiveFlags returns the currently tracked modifier flags.
func (st *DarwinStateTracker) ActiveFlags() uint64 {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.activeFlags
}

// Translate converts an InputEvent into a native DarwinEventSpec while updating internal tracker state.
func (st *DarwinStateTracker) Translate(event InputEvent) (*DarwinEventSpec, error) {
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("macos: invalid event: %w", err)
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	switch event.Type {
	case EventTypeMouseMove:
		st.currentPos.X += float64(event.DX)
		st.currentPos.Y += float64(event.DY)

		// Clamp to display bounds if configured
		if st.displayBounds.Width > 0 && st.displayBounds.Height > 0 {
			minX := st.displayBounds.X
			maxX := st.displayBounds.X + st.displayBounds.Width - 1
			minY := st.displayBounds.Y
			maxY := st.displayBounds.Y + st.displayBounds.Height - 1

			if st.currentPos.X < minX {
				st.currentPos.X = minX
			} else if st.currentPos.X > maxX {
				st.currentPos.X = maxX
			}

			if st.currentPos.Y < minY {
				st.currentPos.Y = minY
			} else if st.currentPos.Y > maxY {
				st.currentPos.Y = maxY
			}
		}

		// Determine event type based on held button drag state
		var eventType uint32
		var mouseBtn uint32 = CGMouseButtonLeft

		if st.leftDown {
			eventType = CGEventTypeLeftMouseDragged
			mouseBtn = CGMouseButtonLeft
		} else if st.rightDown {
			eventType = CGEventTypeRightMouseDragged
			mouseBtn = CGMouseButtonRight
		} else if st.middleDown {
			eventType = CGEventTypeOtherMouseDragged
			mouseBtn = CGMouseButtonCenter
		} else {
			eventType = CGEventTypeMouseMoved
		}

		return &DarwinEventSpec{
			Kind: DarwinEventKindMouse,
			Mouse: &DarwinMouseEventSpec{
				EventType:   eventType,
				Location:    st.currentPos,
				MouseButton: mouseBtn,
				Flags:       st.activeFlags,
			},
		}, nil

	case EventTypeMouseButtonDown, EventTypeMouseButtonUp:
		isDown := event.Type == EventTypeMouseButtonDown
		var eventType uint32
		var mouseBtn uint32

		switch event.Button {
		case MouseButtonLeft:
			st.leftDown = isDown
			mouseBtn = CGMouseButtonLeft
			if isDown {
				eventType = CGEventTypeLeftMouseDown
			} else {
				eventType = CGEventTypeLeftMouseUp
			}
		case MouseButtonRight:
			st.rightDown = isDown
			mouseBtn = CGMouseButtonRight
			if isDown {
				eventType = CGEventTypeRightMouseDown
			} else {
				eventType = CGEventTypeRightMouseUp
			}
		case MouseButtonMiddle:
			st.middleDown = isDown
			mouseBtn = CGMouseButtonCenter
			if isDown {
				eventType = CGEventTypeOtherMouseDown
			} else {
				eventType = CGEventTypeOtherMouseUp
			}
		default:
			return nil, fmt.Errorf("%w: '%s'", ErrDarwinUnsupportedButton, event.Button)
		}

		return &DarwinEventSpec{
			Kind: DarwinEventKindMouse,
			Mouse: &DarwinMouseEventSpec{
				EventType:   eventType,
				Location:    st.currentPos,
				MouseButton: mouseBtn,
				Flags:       st.activeFlags,
			},
		}, nil

	case EventTypeMouseWheel:
		return &DarwinEventSpec{
			Kind: DarwinEventKindScroll,
			Scroll: &DarwinScrollEventSpec{
				WheelDY: event.WheelDY,
				WheelDX: event.WheelDX,
			},
		}, nil

	case EventTypeKeyDown, EventTypeKeyUp:
		isDown := event.Type == EventTypeKeyDown
		mapping, err := LookupDarwinKey(event.Key)
		if err != nil {
			return nil, err
		}

		// Update tracked modifier flags
		switch mapping.Modifier {
		case ModTypeShift:
			if isDown {
				st.activeFlags |= CGEventFlagMaskShift
			} else {
				st.activeFlags &= ^CGEventFlagMaskShift
			}
		case ModTypeControl:
			if isDown {
				st.activeFlags |= CGEventFlagMaskControl
			} else {
				st.activeFlags &= ^CGEventFlagMaskControl
			}
		case ModTypeAlternate:
			if isDown {
				st.activeFlags |= CGEventFlagMaskAlternate
			} else {
				st.activeFlags &= ^CGEventFlagMaskAlternate
			}
		case ModTypeCommand:
			if isDown {
				st.activeFlags |= CGEventFlagMaskCommand
			} else {
				st.activeFlags &= ^CGEventFlagMaskCommand
			}
		}

		return &DarwinEventSpec{
			Kind: DarwinEventKindKeyboard,
			Keyboard: &DarwinKeyboardEventSpec{
				KeyCode: mapping.KeyCode,
				KeyDown: isDown,
				Flags:   st.activeFlags,
			},
		}, nil

	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidEventType, event.Type)
	}
}

// CrossKVMMarker is the unique identifier tagged on synthetic events created by CrossKVM
// to distinguish them from physical user events and prevent feedback loops.
const CrossKVMMarker int64 = 0x584B564D // ASCII for 'XKVM'

// DarwinCaptureState manages state for physical event capture on macOS.
type DarwinCaptureState struct {
	mu          sync.Mutex
	hasBaseline bool
	isCentering bool
	lastX       float64
	lastY       float64
	prevFlags   uint64
}

// NewDarwinCaptureState creates a new DarwinCaptureState.
func NewDarwinCaptureState() *DarwinCaptureState {
	return &DarwinCaptureState{}
}

// ResetBaseline clears the baseline so the next mouse event re-establishes position.
func (cs *DarwinCaptureState) ResetBaseline() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.hasBaseline = false
	cs.isCentering = false
}

// SetCentering marks that a cursor re-centering operation was issued.
func (cs *DarwinCaptureState) SetCentering(centering bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.isCentering = centering
}

// SetLastPos manually sets the last captured position (used when centering cursor during suppression).
func (cs *DarwinCaptureState) SetLastPos(x, y float64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.lastX = x
	cs.lastY = y
	cs.hasBaseline = true
	cs.isCentering = false
}

// TranslateCapturedMouse converts a captured native mouse event into an InputEvent.
func (cs *DarwinCaptureState) TranslateCapturedMouse(eventType uint32, x, y float64, buttonNum, deltaY, deltaX int64, userData int64) (InputEvent, bool) {
	// If the event was generated synthetically by CrossKVM, ignore it!
	if userData == CrossKVMMarker {
		return InputEvent{}, false
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	switch eventType {
	case CGEventTypeMouseMoved, CGEventTypeLeftMouseDragged, CGEventTypeRightMouseDragged, CGEventTypeOtherMouseDragged:
		var dx, dy int32
		if deltaX != 0 || deltaY != 0 {
			dx = int32(deltaX)
			dy = int32(deltaY)
			cs.lastX = x
			cs.lastY = y
			cs.hasBaseline = true
			cs.isCentering = false
		} else {
			if !cs.hasBaseline {
				cs.lastX = x
				cs.lastY = y
				cs.hasBaseline = true
				cs.isCentering = false
				return InputEvent{}, false
			}

			dx = int32(x - cs.lastX)
			dy = int32(y - cs.lastY)

			if cs.isCentering || dx > 200 || dx < -200 || dy > 200 || dy < -200 {
				cs.lastX = x
				cs.lastY = y
				cs.isCentering = false
				return InputEvent{}, false
			}

			cs.lastX = x
			cs.lastY = y
		}

		// Ignore zero movement
		if dx == 0 && dy == 0 {
			return InputEvent{}, false
		}

		// Filter out massive teleport jumps caused by cursor warps
		if dx > 200 || dx < -200 || dy > 200 || dy < -200 {
			return InputEvent{}, false
		}

		return NewMouseMoveEventWithAbs(dx, dy, int32(x), int32(y)), true

	case CGEventTypeLeftMouseDown:
		cs.lastX = x
		cs.lastY = y
		cs.hasBaseline = true
		return NewMouseButtonDownEvent(MouseButtonLeft), true

	case CGEventTypeLeftMouseUp:
		cs.lastX = x
		cs.lastY = y
		cs.hasBaseline = true
		return NewMouseButtonUpEvent(MouseButtonLeft), true

	case CGEventTypeRightMouseDown:
		cs.lastX = x
		cs.lastY = y
		cs.hasBaseline = true
		return NewMouseButtonDownEvent(MouseButtonRight), true

	case CGEventTypeRightMouseUp:
		cs.lastX = x
		cs.lastY = y
		cs.hasBaseline = true
		return NewMouseButtonUpEvent(MouseButtonRight), true

	case CGEventTypeOtherMouseDown:
		cs.lastX = x
		cs.lastY = y
		cs.hasBaseline = true
		btn := mapDarwinOtherButton(buttonNum)
		return NewMouseButtonDownEvent(btn), true

	case CGEventTypeOtherMouseUp:
		cs.lastX = x
		cs.lastY = y
		cs.hasBaseline = true
		btn := mapDarwinOtherButton(buttonNum)
		return NewMouseButtonUpEvent(btn), true

	default:
		return InputEvent{}, false
	}
}

// TranslateCapturedScroll converts a captured native scroll wheel event into an InputEvent.
func (cs *DarwinCaptureState) TranslateCapturedScroll(deltaY, deltaX int64, userData int64) (InputEvent, bool) {
	if userData == CrossKVMMarker {
		return InputEvent{}, false
	}
	if deltaY == 0 && deltaX == 0 {
		return InputEvent{}, false
	}
	return NewMouseWheelEvent(int32(deltaX), int32(deltaY)), true
}

// TranslateCapturedKeyboard converts a captured native keyboard event into InputEvents.
func (cs *DarwinCaptureState) TranslateCapturedKeyboard(eventType uint32, keyCode uint16, flags uint64, userData int64) ([]InputEvent, bool) {
	if userData == CrossKVMMarker {
		return nil, false
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	switch eventType {
	case CGEventTypeKeyDown:
		keyName, _, _ := LookupDarwinKeyByCode(keyCode)
		return []InputEvent{NewKeyDownEvent(keyName, 0)}, true

	case CGEventTypeKeyUp:
		keyName, _, _ := LookupDarwinKeyByCode(keyCode)
		return []InputEvent{NewKeyUpEvent(keyName, 0)}, true

	case CGEventTypeFlagsChanged:
		diff := flags ^ cs.prevFlags
		cs.prevFlags = flags

		if diff == 0 {
			return nil, false
		}

		keyName, _, _ := LookupDarwinKeyByCode(keyCode)
		isDown := (flags & diff) != 0

		if isDown {
			return []InputEvent{NewKeyDownEvent(keyName, 0)}, true
		}
		return []InputEvent{NewKeyUpEvent(keyName, 0)}, true

	default:
		return nil, false
	}
}

func mapDarwinOtherButton(buttonNum int64) MouseButton {
	switch buttonNum {
	case 2:
		return MouseButtonMiddle
	case 3:
		return MouseButton4
	case 4:
		return MouseButton5
	default:
		return MouseButtonMiddle
	}
}
