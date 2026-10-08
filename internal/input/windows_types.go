package input

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

// CrossKVMWindowsMarker is the unique identifier tagged on synthetic events created by CrossKVM on Windows
// (via dwExtraInfo) to distinguish them from physical user events and prevent feedback loops.
const CrossKVMWindowsMarker uintptr = 0x584B564D // ASCII for 'XKVM'

// Win32 Input Types
const (
	InputMouse    uint32 = 0
	InputKeyboard uint32 = 1
	InputHardware uint32 = 2
)

// Win32 Mouse Event Flags
const (
	MouseEventfMove        uint32 = 0x0001
	MouseEventfLeftDown    uint32 = 0x0002
	MouseEventfLeftUp      uint32 = 0x0004
	MouseEventfRightDown   uint32 = 0x0008
	MouseEventfRightUp     uint32 = 0x0010
	MouseEventfMiddleDown  uint32 = 0x0020
	MouseEventfMiddleUp    uint32 = 0x0040
	MouseEventfXDown       uint32 = 0x0080
	MouseEventfXUp         uint32 = 0x0100
	MouseEventfWheel       uint32 = 0x0800
	MouseEventfHWheel      uint32 = 0x1000
	MouseEventfVirtualDesk uint32 = 0x4000
	MouseEventfAbsolute    uint32 = 0x8000
)

// Win32 XBUTTON definitions (mouseData for XDOWN/XUP)
const (
	XButton1 uint32 = 0x0001 // Browser Back / Button 4
	XButton2 uint32 = 0x0002 // Browser Forward / Button 5
)

// Win32 Keyboard Event Flags
const (
	KeyEventfExtendedKey uint32 = 0x0001
	KeyEventfKeyUp       uint32 = 0x0002
	KeyEventfUnicode     uint32 = 0x0004
	KeyEventfScancode    uint32 = 0x0008
)

// Standard Windows Wheel Delta
const WheelDelta int32 = 120

// Win32 Hook and Message Constants
const (
	WH_KEYBOARD_LL = 13
	WH_MOUSE_LL    = 14

	WM_QUIT       = 0x0012
	WM_KEYDOWN    = 0x0100
	WM_KEYUP      = 0x0101
	WM_SYSKEYDOWN = 0x0104
	WM_SYSKEYUP   = 0x0105

	WM_MOUSEMOVE   = 0x0200
	WM_LBUTTONDOWN = 0x0201
	WM_LBUTTONUP   = 0x0202
	WM_RBUTTONDOWN = 0x0204
	WM_RBUTTONUP   = 0x0205
	WM_MBUTTONDOWN = 0x0207
	WM_MBUTTONUP   = 0x0208
	WM_MOUSEWHEEL  = 0x020A
	WM_XBUTTONDOWN = 0x020B
	WM_XBUTTONUP   = 0x020C
	WM_MOUSEHWHEEL = 0x020E

	LLKHF_EXTENDED          uint32 = 0x0001
	LLKHF_LOWER_IL_INJECTED uint32 = 0x0002
	LLKHF_INJECTED          uint32 = 0x0010
	LLKHF_ALTDOWN           uint32 = 0x0020
	LLKHF_UP                uint32 = 0x0080

	LLMHF_INJECTED          uint32 = 0x0001
	LLMHF_LOWER_IL_INJECTED uint32 = 0x0002
)

var (
	ErrUnsupportedButton = errors.New("windows: unsupported mouse button")
)

// Point mirrors Win32 POINT structure.
type Point struct {
	X int32
	Y int32
}

// MSLLHOOKSTRUCT mirrors Win32 MSLLHOOKSTRUCT structure for WH_MOUSE_LL.
type MSLLHOOKSTRUCT struct {
	Pt          Point
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DWExtraInfo uintptr
}

// KBDLLHOOKSTRUCT mirrors Win32 KBDLLHOOKSTRUCT structure for WH_KEYBOARD_LL.
type KBDLLHOOKSTRUCT struct {
	VKCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DWExtraInfo uintptr
}

// MSG mirrors Win32 MSG structure.
type MSG struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       Point
	LPrivate uint32
}

// MouseInput mirrors Win32 MOUSEINPUT structure.
type MouseInput struct {
	DX          int32
	DY          int32
	MouseData   uint32
	DWFlags     uint32
	Time        uint32
	_           uint32 // Padding for 8-byte alignment of dwExtraInfo on 64-bit
	DWExtraInfo uintptr
}

// KeybdInput mirrors Win32 KEYBDINPUT structure.
type KeybdInput struct {
	WVk         uint16
	WScan       uint16
	DWFlags     uint32
	Time        uint32
	_           uint32 // Padding for 8-byte alignment of dwExtraInfo on 64-bit
	DWExtraInfo uintptr
	_           [8]byte // Padding to match MouseInput size (32 bytes)
}

// TagINPUT is the binary-compatible layout for Win32 INPUT structure.
// On 64-bit Windows, sizeof(INPUT) is 40 bytes (4 type + 4 padding + 32 union).
type TagINPUT struct {
	Type uint32
	_    uint32 // Padding to align union to 8 bytes on 64-bit
	Data [32]byte
}

// NewMouseTagINPUT constructs a TagINPUT wrapping a MouseInput.
func NewMouseTagINPUT(mi MouseInput) TagINPUT {
	var input TagINPUT
	input.Type = InputMouse
	*(*MouseInput)(unsafe.Pointer(&input.Data[0])) = mi
	return input
}

// NewKeybdTagINPUT constructs a TagINPUT wrapping a KeybdInput.
func NewKeybdTagINPUT(ki KeybdInput) TagINPUT {
	var input TagINPUT
	input.Type = InputKeyboard
	*(*KeybdInput)(unsafe.Pointer(&input.Data[0])) = ki
	return input
}

// MouseInput safely retrieves the MouseInput from TagINPUT.
func (t TagINPUT) MouseInput() MouseInput {
	return *(*MouseInput)(unsafe.Pointer(&t.Data[0]))
}

// KeybdInput safely retrieves the KeybdInput from TagINPUT.
func (t TagINPUT) KeybdInput() KeybdInput {
	return *(*KeybdInput)(unsafe.Pointer(&t.Data[0]))
}

// TranslateWindowsEvent translates a platform-neutral InputEvent into Win32 TagINPUT structures.
func TranslateWindowsEvent(event InputEvent) ([]TagINPUT, error) {
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("windows: invalid input event: %w", err)
	}

	switch event.Type {
	case EventTypeMouseMove:
		return []TagINPUT{
			NewMouseTagINPUT(MouseInput{
				DX:          event.DX,
				DY:          event.DY,
				DWFlags:     MouseEventfMove,
				DWExtraInfo: CrossKVMWindowsMarker,
			}),
		}, nil

	case EventTypeMouseButtonDown:
		flags, mouseData, err := mapMouseButton(event.Button, true)
		if err != nil {
			return nil, err
		}
		return []TagINPUT{
			NewMouseTagINPUT(MouseInput{
				MouseData:   mouseData,
				DWFlags:     flags,
				DWExtraInfo: CrossKVMWindowsMarker,
			}),
		}, nil

	case EventTypeMouseButtonUp:
		flags, mouseData, err := mapMouseButton(event.Button, false)
		if err != nil {
			return nil, err
		}
		return []TagINPUT{
			NewMouseTagINPUT(MouseInput{
				MouseData:   mouseData,
				DWFlags:     flags,
				DWExtraInfo: CrossKVMWindowsMarker,
			}),
		}, nil

	case EventTypeMouseWheel:
		var inputs []TagINPUT
		if event.WheelDY != 0 {
			inputs = append(inputs, NewMouseTagINPUT(MouseInput{
				MouseData:   uint32(event.WheelDY),
				DWFlags:     MouseEventfWheel,
				DWExtraInfo: CrossKVMWindowsMarker,
			}))
		}
		if event.WheelDX != 0 {
			inputs = append(inputs, NewMouseTagINPUT(MouseInput{
				MouseData:   uint32(event.WheelDX),
				DWFlags:     MouseEventfHWheel,
				DWExtraInfo: CrossKVMWindowsMarker,
			}))
		}
		if len(inputs) == 0 {
			return nil, nil
		}
		return inputs, nil

	case EventTypeKeyDown:
		mapping, err := LookupWindowsKey(event.Key)
		if err != nil {
			return nil, err
		}
		flags := KeyEventfScancode
		if mapping.IsExtended {
			flags |= KeyEventfExtendedKey
		}
		return []TagINPUT{
			NewKeybdTagINPUT(KeybdInput{
				WVk:         mapping.VK,
				WScan:       mapping.ScanCode,
				DWFlags:     flags,
				DWExtraInfo: CrossKVMWindowsMarker,
			}),
		}, nil

	case EventTypeKeyUp:
		mapping, err := LookupWindowsKey(event.Key)
		if err != nil {
			return nil, err
		}
		flags := KeyEventfScancode | KeyEventfKeyUp
		if mapping.IsExtended {
			flags |= KeyEventfExtendedKey
		}
		return []TagINPUT{
			NewKeybdTagINPUT(KeybdInput{
				WVk:         mapping.VK,
				WScan:       mapping.ScanCode,
				DWFlags:     flags,
				DWExtraInfo: CrossKVMWindowsMarker,
			}),
		}, nil

	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidEventType, event.Type)
	}
}

func mapMouseButton(button MouseButton, isDown bool) (flags uint32, mouseData uint32, err error) {
	switch button {
	case MouseButtonLeft:
		if isDown {
			return MouseEventfLeftDown, 0, nil
		}
		return MouseEventfLeftUp, 0, nil

	case MouseButtonRight:
		if isDown {
			return MouseEventfRightDown, 0, nil
		}
		return MouseEventfRightUp, 0, nil

	case MouseButtonMiddle:
		if isDown {
			return MouseEventfMiddleDown, 0, nil
		}
		return MouseEventfMiddleUp, 0, nil

	case MouseButton4:
		if isDown {
			return MouseEventfXDown, XButton1, nil
		}
		return MouseEventfXUp, XButton1, nil

	case MouseButton5:
		if isDown {
			return MouseEventfXDown, XButton2, nil
		}
		return MouseEventfXUp, XButton2, nil

	default:
		return 0, 0, fmt.Errorf("%w: '%s'", ErrUnsupportedButton, button)
	}
}

// WindowsCaptureState manages state for physical event capture on Windows.
type WindowsCaptureState struct {
	mu          sync.Mutex
	hasBaseline bool
	isCentering bool
	suppressed  bool
	anchorX     int32
	anchorY     int32
	lastX       int32
	lastY       int32
}

// NewWindowsCaptureState creates a new WindowsCaptureState.
func NewWindowsCaptureState() *WindowsCaptureState {
	return &WindowsCaptureState{}
}

// ResetBaseline clears the baseline so the next mouse move establishes the initial position.
func (cs *WindowsCaptureState) ResetBaseline() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.hasBaseline = false
	cs.isCentering = false
}

// SetCentering marks that a cursor re-centering operation was issued.
func (cs *WindowsCaptureState) SetCentering(centering bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.isCentering = centering
}

// SetLastPos manually sets the last captured position.
func (cs *WindowsCaptureState) SetLastPos(x, y int32) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.lastX = x
	cs.lastY = y
	cs.hasBaseline = true
	cs.isCentering = false
}

// SetSuppressed configures the fixed native cursor position used while hooks
// consume movement. Each hook position is relative to this anchor, because
// Windows does not apply the previous suppressed movement to the cursor.
func (cs *WindowsCaptureState) SetSuppressed(enabled bool, x, y int32) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.suppressed = enabled
	cs.anchorX, cs.anchorY = x, y
	cs.hasBaseline = false
	cs.isCentering = false
}

// SetSuppressedAnchor updates the actual OS cursor position before a suppressed
// hook event. Cursor warps can be delayed, so the requested center is not enough.
func (cs *WindowsCaptureState) SetSuppressedAnchor(x, y int32) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.anchorX, cs.anchorY = x, y
}

// TranslateCapturedMouse converts a captured native MSLLHOOKSTRUCT into an InputEvent.
func (cs *WindowsCaptureState) TranslateCapturedMouse(msg uint32, data *MSLLHOOKSTRUCT) (InputEvent, bool) {
	if data == nil {
		return InputEvent{}, false
	}

	// Filter out CrossKVM's own synthetic injected events
	if data.DWExtraInfo == CrossKVMWindowsMarker {
		return InputEvent{}, false
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	switch msg {
	case WM_MOUSEMOVE:
		if cs.suppressed {
			dx, dy := data.Pt.X-cs.anchorX, data.Pt.Y-cs.anchorY
			if dx == 0 && dy == 0 {
				return InputEvent{}, false
			}
			return NewMouseMoveEvent(dx, dy), true
		}
		if !cs.hasBaseline {
			cs.lastX = data.Pt.X
			cs.lastY = data.Pt.Y
			cs.hasBaseline = true
			cs.isCentering = false
			return InputEvent{}, false
		}

		dx := data.Pt.X - cs.lastX
		dy := data.Pt.Y - cs.lastY

		// If this is a re-centering jump or large coordinate warp, absorb it as the new baseline
		if cs.isCentering || dx > 200 || dx < -200 || dy > 200 || dy < -200 {
			cs.lastX = data.Pt.X
			cs.lastY = data.Pt.Y
			cs.isCentering = false
			return InputEvent{}, false
		}

		cs.lastX = data.Pt.X
		cs.lastY = data.Pt.Y

		if dx == 0 && dy == 0 {
			return InputEvent{}, false
		}

		return NewMouseMoveEvent(dx, dy), true

	case WM_LBUTTONDOWN:
		cs.lastX = data.Pt.X
		cs.lastY = data.Pt.Y
		cs.hasBaseline = true
		return NewMouseButtonDownEvent(MouseButtonLeft), true

	case WM_LBUTTONUP:
		cs.lastX = data.Pt.X
		cs.lastY = data.Pt.Y
		cs.hasBaseline = true
		return NewMouseButtonUpEvent(MouseButtonLeft), true

	case WM_RBUTTONDOWN:
		cs.lastX = data.Pt.X
		cs.lastY = data.Pt.Y
		cs.hasBaseline = true
		return NewMouseButtonDownEvent(MouseButtonRight), true

	case WM_RBUTTONUP:
		cs.lastX = data.Pt.X
		cs.lastY = data.Pt.Y
		cs.hasBaseline = true
		return NewMouseButtonUpEvent(MouseButtonRight), true

	case WM_MBUTTONDOWN:
		cs.lastX = data.Pt.X
		cs.lastY = data.Pt.Y
		cs.hasBaseline = true
		return NewMouseButtonDownEvent(MouseButtonMiddle), true

	case WM_MBUTTONUP:
		cs.lastX = data.Pt.X
		cs.lastY = data.Pt.Y
		cs.hasBaseline = true
		return NewMouseButtonUpEvent(MouseButtonMiddle), true

	case WM_XBUTTONDOWN, WM_XBUTTONUP:
		cs.lastX = data.Pt.X
		cs.lastY = data.Pt.Y
		cs.hasBaseline = true
		xBtn := uint16(data.MouseData >> 16)
		btn := MouseButton4
		if xBtn == 2 {
			btn = MouseButton5
		}
		if msg == WM_XBUTTONDOWN {
			return NewMouseButtonDownEvent(btn), true
		}
		return NewMouseButtonUpEvent(btn), true

	case WM_MOUSEWHEEL:
		deltaY := int16(uint16(data.MouseData >> 16))
		if deltaY == 0 {
			return InputEvent{}, false
		}
		return NewMouseWheelEvent(0, int32(deltaY)), true

	case WM_MOUSEHWHEEL:
		deltaX := int16(uint16(data.MouseData >> 16))
		if deltaX == 0 {
			return InputEvent{}, false
		}
		return NewMouseWheelEvent(int32(deltaX), 0), true

	default:
		return InputEvent{}, false
	}
}

// TranslateCapturedKeyboard converts a captured native KBDLLHOOKSTRUCT into an InputEvent.
func (cs *WindowsCaptureState) TranslateCapturedKeyboard(msg uint32, data *KBDLLHOOKSTRUCT) (InputEvent, bool) {
	if data == nil {
		return InputEvent{}, false
	}

	// Filter out CrossKVM's own synthetic injected events
	if data.DWExtraInfo == CrossKVMWindowsMarker {
		return InputEvent{}, false
	}

	isExtended := (data.Flags & LLKHF_EXTENDED) != 0
	keyName := LookupWindowsKeyByVK(data.VKCode, data.ScanCode, isExtended)

	switch msg {
	case WM_KEYDOWN, WM_SYSKEYDOWN:
		return NewKeyDownEvent(keyName, 0), true
	case WM_KEYUP, WM_SYSKEYUP:
		return NewKeyUpEvent(keyName, 0), true
	default:
		return InputEvent{}, false
	}
}

// CheckInputStructSizes verifies that TagINPUT matches expected byte size.
func CheckInputStructSizes() (int, int) {
	return int(unsafe.Sizeof(TagINPUT{})), binary.Size(TagINPUT{})
}

// WindowsEmergencyChord runs on the keyboard hook thread before queuing events.
// Separate left/right modifier bits keep releasing one modifier from clearing the other.
type WindowsEmergencyChord struct{ ctrl, alt, shift uint8 }

func (c *WindowsEmergencyChord) Observe(msg uint32, data KBDLLHOOKSTRUCT) bool {
	if data.DWExtraInfo == CrossKVMWindowsMarker || data.Flags&LLKHF_INJECTED != 0 {
		return false
	}
	down := msg == WM_KEYDOWN || msg == WM_SYSKEYDOWN
	up := msg == WM_KEYUP || msg == WM_SYSKEYUP
	if !down && !up {
		return false
	}
	var bits *uint8
	var mask uint8 = 1
	switch data.VKCode {
	case 0x11, 0xA2:
		bits = &c.ctrl
	case 0xA3:
		bits = &c.ctrl
		mask = 2
	case 0x12, 0xA4:
		bits = &c.alt
	case 0xA5:
		bits = &c.alt
		mask = 2
	case 0x10, 0xA0:
		bits = &c.shift
	case 0xA1:
		bits = &c.shift
		mask = 2
	}
	if bits != nil {
		if down {
			*bits |= mask
		} else {
			*bits &^= mask
		}
	}
	return down && data.VKCode == 0x1B && c.ctrl != 0 && c.alt != 0 && c.shift != 0
}
