//go:build darwin
package input

import (
	"errors"
	"testing"
)

func TestDarwin_KeyMapping(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		wantCode uint16
		wantMod  ModifierType
		wantErr  bool
	}{
		{name: "Letter A", key: "A", wantCode: kVK_ANSI_A},
		{name: "Letter Z", key: "z", wantCode: kVK_ANSI_Z},
		{name: "Number 1", key: "1", wantCode: kVK_ANSI_1},
		{name: "Enter", key: "Enter", wantCode: kVK_Return},
		{name: "Escape", key: "Escape", wantCode: kVK_Escape},
		{name: "Space", key: "Space", wantCode: kVK_Space},
		{name: "Tab", key: "Tab", wantCode: kVK_Tab},
		{name: "Backspace", key: "Backspace", wantCode: kVK_Delete},
		{name: "Forward Delete", key: "Delete", wantCode: kVK_ForwardDelete},
		{name: "ArrowUp", key: "ArrowUp", wantCode: kVK_UpArrow},
		{name: "ArrowDown", key: "ArrowDown", wantCode: kVK_DownArrow},
		{name: "ArrowLeft", key: "ArrowLeft", wantCode: kVK_LeftArrow},
		{name: "ArrowRight", key: "ArrowRight", wantCode: kVK_RightArrow},
		{name: "ShiftLeft", key: "ShiftLeft", wantCode: kVK_Shift, wantMod: ModTypeShift},
		{name: "ControlLeft", key: "ControlLeft", wantCode: kVK_Control, wantMod: ModTypeControl},
		{name: "AltLeft", key: "AltLeft", wantCode: kVK_Option, wantMod: ModTypeAlternate},
		{name: "MetaLeft", key: "MetaLeft", wantCode: kVK_Command, wantMod: ModTypeCommand},
		{name: "F1", key: "F1", wantCode: kVK_F1},
		{name: "F12", key: "F12", wantCode: kVK_F12},
		{name: "Unknown key", key: "SuperUnknownKey123", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := LookupDarwinKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LookupDarwinKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if m.KeyCode != tt.wantCode {
				t.Errorf("KeyCode = 0x%X, want 0x%X", m.KeyCode, tt.wantCode)
			}
			if m.Modifier != tt.wantMod {
				t.Errorf("Modifier = %v, want %v", m.Modifier, tt.wantMod)
			}
		})
	}
}

func TestDarwin_LookupKeyByCode(t *testing.T) {
	tests := []struct {
		code    uint16
		wantKey string
		wantMod ModifierType
	}{
		{code: kVK_ANSI_A, wantKey: "A", wantMod: ModNone},
		{code: kVK_Return, wantKey: "Enter", wantMod: ModNone},
		{code: kVK_Space, wantKey: "Space", wantMod: ModNone},
		{code: kVK_Delete, wantKey: "Backspace", wantMod: ModNone},
		{code: kVK_ForwardDelete, wantKey: "Delete", wantMod: ModNone},
		{code: kVK_UpArrow, wantKey: "ArrowUp", wantMod: ModNone},
		{code: kVK_DownArrow, wantKey: "ArrowDown", wantMod: ModNone},
		{code: kVK_Shift, wantKey: "ShiftLeft", wantMod: ModTypeShift},
		{code: kVK_RightShift, wantKey: "ShiftRight", wantMod: ModTypeShift},
		{code: kVK_Control, wantKey: "ControlLeft", wantMod: ModTypeControl},
		{code: kVK_RightControl, wantKey: "ControlRight", wantMod: ModTypeControl},
		{code: kVK_Option, wantKey: "AltLeft", wantMod: ModTypeAlternate},
		{code: kVK_RightOption, wantKey: "AltRight", wantMod: ModTypeAlternate},
		{code: kVK_Command, wantKey: "MetaLeft", wantMod: ModTypeCommand},
		{code: kVK_RightCommand, wantKey: "MetaRight", wantMod: ModTypeCommand},
		{code: kVK_F1, wantKey: "F1", wantMod: ModNone},
		{code: kVK_F12, wantKey: "F12", wantMod: ModNone},
	}

	for _, tt := range tests {
		t.Run(tt.wantKey, func(t *testing.T) {
			name, mod, ok := LookupDarwinKeyByCode(tt.code)
			if !ok {
				t.Fatalf("LookupDarwinKeyByCode(0x%X) expected ok=true", tt.code)
			}
			if name != tt.wantKey {
				t.Errorf("name = %s, want %s", name, tt.wantKey)
			}
			if mod != tt.wantMod {
				t.Errorf("mod = %v, want %v", mod, tt.wantMod)
			}
		})
	}
}

func TestDarwin_MouseMoveAndClamping(t *testing.T) {
	bounds := DarwinRect{X: 0, Y: 0, Width: 1920, Height: 1080}
	tracker := NewDarwinStateTracker(DarwinPoint{X: 100, Y: 100}, bounds)

	// Move positive
	spec, err := tracker.Translate(NewMouseMoveEvent(25, 50))
	if err != nil {
		t.Fatalf("Translate mouse move failed: %v", err)
	}
	if spec.Kind != DarwinEventKindMouse || spec.Mouse == nil {
		t.Fatalf("Expected mouse spec, got %+v", spec)
	}
	if spec.Mouse.EventType != CGEventTypeMouseMoved {
		t.Errorf("EventType = %d, want %d", spec.Mouse.EventType, CGEventTypeMouseMoved)
	}
	if spec.Mouse.Location.X != 125 || spec.Mouse.Location.Y != 150 {
		t.Errorf("Location = (%f, %f), want (125, 150)", spec.Mouse.Location.X, spec.Mouse.Location.Y)
	}

	// Move negative
	spec, err = tracker.Translate(NewMouseMoveEvent(-25, -50))
	if err != nil {
		t.Fatalf("Translate negative move failed: %v", err)
	}
	if spec.Mouse.Location.X != 100 || spec.Mouse.Location.Y != 100 {
		t.Errorf("Location = (%f, %f), want (100, 100)", spec.Mouse.Location.X, spec.Mouse.Location.Y)
	}

	// Move beyond left/top bounds (clamp to 0, 0)
	spec, err = tracker.Translate(NewMouseMoveEvent(-500, -500))
	if err != nil {
		t.Fatalf("Translate clamp left/top failed: %v", err)
	}
	if spec.Mouse.Location.X != 0 || spec.Mouse.Location.Y != 0 {
		t.Errorf("Clamped location = (%f, %f), want (0, 0)", spec.Mouse.Location.X, spec.Mouse.Location.Y)
	}

	// Move beyond right/bottom bounds (clamp to 1919, 1079)
	spec, err = tracker.Translate(NewMouseMoveEvent(3000, 3000))
	if err != nil {
		t.Fatalf("Translate clamp right/bottom failed: %v", err)
	}
	if spec.Mouse.Location.X != 1919 || spec.Mouse.Location.Y != 1079 {
		t.Errorf("Clamped location = (%f, %f), want (1919, 1079)", spec.Mouse.Location.X, spec.Mouse.Location.Y)
	}
}

func TestDarwin_MouseButtonsAndDragState(t *testing.T) {
	tracker := NewDarwinStateTracker(DarwinPoint{X: 500, Y: 500}, DarwinRect{})

	// 1. Left Down
	spec, err := tracker.Translate(NewMouseButtonDownEvent(MouseButtonLeft))
	if err != nil || spec.Mouse.EventType != CGEventTypeLeftMouseDown || spec.Mouse.MouseButton != CGMouseButtonLeft {
		t.Fatalf("Left down mismatch: err=%v, spec=%+v", err, spec.Mouse)
	}

	// 2. Mouse Move while Left Down -> LeftMouseDragged
	spec, err = tracker.Translate(NewMouseMoveEvent(10, 10))
	if err != nil || spec.Mouse.EventType != CGEventTypeLeftMouseDragged {
		t.Fatalf("Expected LeftMouseDragged, got %d", spec.Mouse.EventType)
	}

	// 3. Left Up
	spec, err = tracker.Translate(NewMouseButtonUpEvent(MouseButtonLeft))
	if err != nil || spec.Mouse.EventType != CGEventTypeLeftMouseUp {
		t.Fatalf("Expected LeftMouseUp, got %d", spec.Mouse.EventType)
	}

	// 4. Mouse Move after Left Up -> MouseMoved
	spec, err = tracker.Translate(NewMouseMoveEvent(5, 5))
	if err != nil || spec.Mouse.EventType != CGEventTypeMouseMoved {
		t.Fatalf("Expected MouseMoved, got %d", spec.Mouse.EventType)
	}

	// 5. Right Down & Drag
	_, _ = tracker.Translate(NewMouseButtonDownEvent(MouseButtonRight))
	spec, _ = tracker.Translate(NewMouseMoveEvent(10, 10))
	if spec.Mouse.EventType != CGEventTypeRightMouseDragged {
		t.Fatalf("Expected RightMouseDragged, got %d", spec.Mouse.EventType)
	}
	_, _ = tracker.Translate(NewMouseButtonUpEvent(MouseButtonRight))

	// 6. Middle Down & Drag
	_, _ = tracker.Translate(NewMouseButtonDownEvent(MouseButtonMiddle))
	spec, _ = tracker.Translate(NewMouseMoveEvent(10, 10))
	if spec.Mouse.EventType != CGEventTypeOtherMouseDragged {
		t.Fatalf("Expected OtherMouseDragged, got %d", spec.Mouse.EventType)
	}
	_, _ = tracker.Translate(NewMouseButtonUpEvent(MouseButtonMiddle))

	// 7. Unsupported button error
	_, err = tracker.Translate(NewMouseButtonDownEvent(MouseButton("unsupported_btn")))
	if !errors.Is(err, ErrDarwinUnsupportedButton) {
		t.Fatalf("Expected ErrDarwinUnsupportedButton, got %v", err)
	}
}

func TestDarwin_MouseWheel(t *testing.T) {
	tracker := NewDarwinStateTracker(DarwinPoint{}, DarwinRect{})

	// Vertical wheel
	spec, err := tracker.Translate(NewMouseWheelEvent(0, 120))
	if err != nil || spec.Kind != DarwinEventKindScroll || spec.Scroll == nil {
		t.Fatalf("Translate vertical wheel error: %v", err)
	}
	if spec.Scroll.WheelDY != 120 || spec.Scroll.WheelDX != 0 {
		t.Errorf("Wheel delta mismatch: DY=%d DX=%d", spec.Scroll.WheelDY, spec.Scroll.WheelDX)
	}

	// Negative vertical wheel & horizontal wheel
	spec, err = tracker.Translate(NewMouseWheelEvent(-50, -120))
	if err != nil || spec.Scroll.WheelDY != -120 || spec.Scroll.WheelDX != -50 {
		t.Errorf("Wheel delta mismatch: DY=%d DX=%d", spec.Scroll.WheelDY, spec.Scroll.WheelDX)
	}
}

func TestDarwin_ModifierTrackingSequence(t *testing.T) {
	tracker := NewDarwinStateTracker(DarwinPoint{}, DarwinRect{})

	// Initial flags should be 0
	if tracker.ActiveFlags() != 0 {
		t.Fatalf("Expected initial flags 0, got %X", tracker.ActiveFlags())
	}

	// 1. MetaLeft (Command) KeyDown
	spec, err := tracker.Translate(NewKeyDownEvent("MetaLeft", 0))
	if err != nil || !spec.Keyboard.KeyDown || spec.Keyboard.KeyCode != kVK_Command {
		t.Fatalf("MetaLeft keydown failed: %v", err)
	}
	if tracker.ActiveFlags()&CGEventFlagMaskCommand == 0 {
		t.Fatalf("Expected CGEventFlagMaskCommand to be active")
	}

	// 2. 'C' KeyDown while Command is held
	spec, err = tracker.Translate(NewKeyDownEvent("C", 0))
	if err != nil || !spec.Keyboard.KeyDown || spec.Keyboard.KeyCode != kVK_ANSI_C {
		t.Fatalf("C keydown failed: %v", err)
	}
	if spec.Keyboard.Flags&CGEventFlagMaskCommand == 0 {
		t.Errorf("Expected 'C' keydown to carry Command modifier flag %X, got %X", CGEventFlagMaskCommand, spec.Keyboard.Flags)
	}

	// 3. 'C' KeyUp
	spec, err = tracker.Translate(NewKeyUpEvent("C", 0))
	if err != nil || spec.Keyboard.KeyDown || spec.Keyboard.KeyCode != kVK_ANSI_C {
		t.Fatalf("C keyup failed: %v", err)
	}
	if spec.Keyboard.Flags&CGEventFlagMaskCommand == 0 {
		t.Errorf("Expected 'C' keyup to carry Command modifier flag %X", spec.Keyboard.Flags)
	}

	// 4. MetaLeft (Command) KeyUp
	spec, err = tracker.Translate(NewKeyUpEvent("MetaLeft", 0))
	if err != nil || spec.Keyboard.KeyDown || spec.Keyboard.KeyCode != kVK_Command {
		t.Fatalf("MetaLeft keyup failed: %v", err)
	}
	if tracker.ActiveFlags()&CGEventFlagMaskCommand != 0 {
		t.Errorf("Expected Command modifier flag to be cleared")
	}
}

func TestDarwinCapture_MouseBaselineAndDeltas(t *testing.T) {
	state := NewDarwinCaptureState()

	// 1. First event establishes baseline -> no event emitted
	ev, ok := state.TranslateCapturedMouse(CGEventTypeMouseMoved, 500, 300, 0, 0, 0, 0)
	if ok {
		t.Fatalf("First mouse move should establish baseline and not emit event, got %+v", ev)
	}

	// 2. Subsequent positive movement
	ev, ok = state.TranslateCapturedMouse(CGEventTypeMouseMoved, 508, 304, 0, 0, 0, 0)
	if !ok {
		t.Fatalf("Expected mouse move event, got ok=false")
	}
	if ev.Type != EventTypeMouseMove || ev.DX != 8 || ev.DY != 4 {
		t.Errorf("Expected dx=8 dy=4, got %+v", ev)
	}

	// 3. Subsequent negative movement
	ev, ok = state.TranslateCapturedMouse(CGEventTypeMouseMoved, 503, 298, 0, 0, 0, 0)
	if !ok {
		t.Fatalf("Expected mouse move event, got ok=false")
	}
	if ev.DX != -5 || ev.DY != -6 {
		t.Errorf("Expected dx=-5 dy=-6, got %+v", ev)
	}

	// 4. Zero delta ignored
	ev, ok = state.TranslateCapturedMouse(CGEventTypeMouseMoved, 503, 298, 0, 0, 0, 0)
	if ok {
		t.Fatalf("Zero movement delta should be ignored, got %+v", ev)
	}

	// 5. Drag event produces move delta
	ev, ok = state.TranslateCapturedMouse(CGEventTypeLeftMouseDragged, 510, 300, 0, 0, 0, 0)
	if !ok || ev.DX != 7 || ev.DY != 2 {
		t.Errorf("Expected drag move dx=7 dy=2, got ok=%v %+v", ok, ev)
	}

	// 6. Direct hardware delta (deltaX, deltaY from CoreGraphics)
	ev, ok = state.TranslateCapturedMouse(CGEventTypeMouseMoved, 510, 300, 0, -3, 12, 0)
	if !ok || ev.DX != 12 || ev.DY != -3 {
		t.Errorf("Expected direct delta dx=12 dy=-3, got ok=%v %+v", ok, ev)
	}
}

func TestDarwinCapture_MouseButtons(t *testing.T) {
	state := NewDarwinCaptureState()

	// Left button down / up
	ev, ok := state.TranslateCapturedMouse(CGEventTypeLeftMouseDown, 100, 200, 0, 0, 0, 0)
	if !ok || ev.Type != EventTypeMouseButtonDown || ev.Button != MouseButtonLeft {
		t.Errorf("Expected LeftMouseDown, got ok=%v %+v", ok, ev)
	}

	ev, ok = state.TranslateCapturedMouse(CGEventTypeLeftMouseUp, 100, 200, 0, 0, 0, 0)
	if !ok || ev.Type != EventTypeMouseButtonUp || ev.Button != MouseButtonLeft {
		t.Errorf("Expected LeftMouseUp, got ok=%v %+v", ok, ev)
	}

	// Right button down / up
	ev, ok = state.TranslateCapturedMouse(CGEventTypeRightMouseDown, 100, 200, 1, 0, 0, 0)
	if !ok || ev.Type != EventTypeMouseButtonDown || ev.Button != MouseButtonRight {
		t.Errorf("Expected RightMouseDown, got ok=%v %+v", ok, ev)
	}

	ev, ok = state.TranslateCapturedMouse(CGEventTypeRightMouseUp, 100, 200, 1, 0, 0, 0)
	if !ok || ev.Type != EventTypeMouseButtonUp || ev.Button != MouseButtonRight {
		t.Errorf("Expected RightMouseUp, got ok=%v %+v", ok, ev)
	}

	// Middle button (other button 2)
	ev, ok = state.TranslateCapturedMouse(CGEventTypeOtherMouseDown, 100, 200, 2, 0, 0, 0)
	if !ok || ev.Type != EventTypeMouseButtonDown || ev.Button != MouseButtonMiddle {
		t.Errorf("Expected MiddleMouseDown, got ok=%v %+v", ok, ev)
	}

	ev, ok = state.TranslateCapturedMouse(CGEventTypeOtherMouseUp, 100, 200, 2, 0, 0, 0)
	if !ok || ev.Type != EventTypeMouseButtonUp || ev.Button != MouseButtonMiddle {
		t.Errorf("Expected MiddleMouseUp, got ok=%v %+v", ok, ev)
	}

	// Button 4 & 5
	ev, ok = state.TranslateCapturedMouse(CGEventTypeOtherMouseDown, 100, 200, 3, 0, 0, 0)
	if !ok || ev.Button != MouseButton4 {
		t.Errorf("Expected Button4, got ok=%v %+v", ok, ev)
	}

	ev, ok = state.TranslateCapturedMouse(CGEventTypeOtherMouseDown, 100, 200, 4, 0, 0, 0)
	if !ok || ev.Button != MouseButton5 {
		t.Errorf("Expected Button5, got ok=%v %+v", ok, ev)
	}
}

func TestDarwinCapture_Scroll(t *testing.T) {
	state := NewDarwinCaptureState()

	// Positive vertical
	ev, ok := state.TranslateCapturedScroll(120, 0, 0)
	if !ok || ev.Type != EventTypeMouseWheel || ev.WheelDY != 120 || ev.WheelDX != 0 {
		t.Errorf("Expected WheelDY=120, got ok=%v %+v", ok, ev)
	}

	// Negative vertical & horizontal
	ev, ok = state.TranslateCapturedScroll(-120, -50, 0)
	if !ok || ev.WheelDY != -120 || ev.WheelDX != -50 {
		t.Errorf("Expected WheelDY=-120 WheelDX=-50, got ok=%v %+v", ok, ev)
	}

	// Trackpad small point delta
	ev, ok = state.TranslateCapturedScroll(3, -2, 0)
	if !ok || ev.WheelDY != 3 || ev.WheelDX != -2 {
		t.Errorf("Expected small delta WheelDY=3 WheelDX=-2, got ok=%v %+v", ok, ev)
	}

	// Zero delta ignored
	ev, ok = state.TranslateCapturedScroll(0, 0, 0)
	if ok {
		t.Fatalf("Zero scroll delta should be ignored, got %+v", ev)
	}
}

func TestDarwinCapture_KeyboardAndModifiers(t *testing.T) {
	state := NewDarwinCaptureState()

	// 1. Regular KeyDown 'A'
	evs, ok := state.TranslateCapturedKeyboard(CGEventTypeKeyDown, kVK_ANSI_A, 0, 0)
	if !ok || len(evs) != 1 || evs[0].Type != EventTypeKeyDown || evs[0].Key != "A" {
		t.Fatalf("Expected KeyDown A, got ok=%v %+v", ok, evs)
	}

	// 2. Regular KeyUp 'A'
	evs, ok = state.TranslateCapturedKeyboard(CGEventTypeKeyUp, kVK_ANSI_A, 0, 0)
	if !ok || len(evs) != 1 || evs[0].Type != EventTypeKeyUp || evs[0].Key != "A" {
		t.Fatalf("Expected KeyUp A, got ok=%v %+v", ok, evs)
	}

	// 3. Modifier transition: Press Command
	evs, ok = state.TranslateCapturedKeyboard(CGEventTypeFlagsChanged, kVK_Command, CGEventFlagMaskCommand, 0)
	if !ok || len(evs) != 1 || evs[0].Type != EventTypeKeyDown || evs[0].Key != "MetaLeft" {
		t.Fatalf("Expected KeyDown MetaLeft, got ok=%v %+v", ok, evs)
	}

	// 4. Modifier transition: Press Shift while Command is held (Command + Shift)
	evs, ok = state.TranslateCapturedKeyboard(CGEventTypeFlagsChanged, kVK_Shift, CGEventFlagMaskCommand|CGEventFlagMaskShift, 0)
	if !ok || len(evs) != 1 || evs[0].Type != EventTypeKeyDown || evs[0].Key != "ShiftLeft" {
		t.Fatalf("Expected KeyDown ShiftLeft, got ok=%v %+v", ok, evs)
	}

	// 5. Modifier transition: Release Shift (Command remains held)
	evs, ok = state.TranslateCapturedKeyboard(CGEventTypeFlagsChanged, kVK_Shift, CGEventFlagMaskCommand, 0)
	if !ok || len(evs) != 1 || evs[0].Type != EventTypeKeyUp || evs[0].Key != "ShiftLeft" {
		t.Fatalf("Expected KeyUp ShiftLeft, got ok=%v %+v", ok, evs)
	}

	// 6. Modifier transition: Release Command (no modifiers left)
	evs, ok = state.TranslateCapturedKeyboard(CGEventTypeFlagsChanged, kVK_Command, 0, 0)
	if !ok || len(evs) != 1 || evs[0].Type != EventTypeKeyUp || evs[0].Key != "MetaLeft" {
		t.Fatalf("Expected KeyUp MetaLeft, got ok=%v %+v", ok, evs)
	}

	// 7. Duplicate flags event without changes -> ignored
	evs, ok = state.TranslateCapturedKeyboard(CGEventTypeFlagsChanged, kVK_Command, 0, 0)
	if ok || len(evs) > 0 {
		t.Fatalf("Expected duplicate flags changed to be ignored, got ok=%v %+v", ok, evs)
	}
}

func TestDarwinCapture_SyntheticEventFiltering(t *testing.T) {
	state := NewDarwinCaptureState()

	// Mouse event tagged with CrossKVMMarker should be ignored
	_, ok := state.TranslateCapturedMouse(CGEventTypeMouseMoved, 100, 100, 0, 0, 0, CrossKVMMarker)
	if ok {
		t.Fatalf("CrossKVM synthetic mouse event should be ignored")
	}

	// Scroll event tagged with CrossKVMMarker should be ignored
	_, ok = state.TranslateCapturedScroll(120, 0, CrossKVMMarker)
	if ok {
		t.Fatalf("CrossKVM synthetic scroll event should be ignored")
	}

	// Keyboard event tagged with CrossKVMMarker should be ignored
	_, ok = state.TranslateCapturedKeyboard(CGEventTypeKeyDown, kVK_ANSI_A, 0, CrossKVMMarker)
	if ok {
		t.Fatalf("CrossKVM synthetic keyboard event should be ignored")
	}

	// Physical (unmarked) event should be processed
	ev, ok := state.TranslateCapturedScroll(120, 0, 0)
	if !ok || ev.WheelDY != 120 {
		t.Fatalf("Physical scroll event should be accepted, got ok=%v %+v", ok, ev)
	}
}

func TestDarwinBackend_Lifecycle(t *testing.T) {
	backend, err := NewDarwinBackend()
	if err != nil {
		t.Fatalf("NewDarwinBackend failed: %v", err)
	}

	// Close is idempotent
	if err := backend.Close(); err != nil {
		t.Fatalf("backend.Close failed: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("second backend.Close failed: %v", err)
	}

	// StartCapture after Close should return ErrBackendClosed
	ch := make(chan InputEvent, 10)
	if err := backend.StartCapture(ch); !errors.Is(err, ErrBackendClosed) {
		t.Fatalf("StartCapture on closed backend expected ErrBackendClosed, got %v", err)
	}

	// Inject after Close should return ErrBackendClosed
	if err := backend.Inject(NewMouseMoveEvent(10, 10)); !errors.Is(err, ErrBackendClosed) {
		t.Fatalf("Inject on closed backend expected ErrBackendClosed, got %v", err)
	}
}
