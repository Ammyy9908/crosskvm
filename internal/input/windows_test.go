package input

import (
	"testing"
	"unsafe"
)

func TestWindows_StructSizes(t *testing.T) {
	size := unsafe.Sizeof(TagINPUT{})
	// On 64-bit platforms, sizeof(INPUT) is 40 bytes.
	// On 32-bit platforms, sizeof(INPUT) is 28 bytes.
	if unsafe.Sizeof(uintptr(0)) == 8 {
		if size != 40 {
			t.Fatalf("Expected TagINPUT size 40 on 64-bit, got %d", size)
		}
	} else {
		if size != 28 {
			t.Fatalf("Expected TagINPUT size 28 on 32-bit, got %d", size)
		}
	}
}

func TestWindows_TranslateMouseMove(t *testing.T) {
	event := NewMouseMoveEvent(15, -8)
	inputs, err := TranslateWindowsEvent(event)
	if err != nil {
		t.Fatalf("TranslateWindowsEvent failed: %v", err)
	}

	if len(inputs) != 1 {
		t.Fatalf("Expected 1 input, got %d", len(inputs))
	}

	input := inputs[0]
	if input.Type != InputMouse {
		t.Errorf("Expected Type InputMouse (%d), got %d", InputMouse, input.Type)
	}

	mi := input.MouseInput()
	if mi.DX != 15 || mi.DY != -8 {
		t.Errorf("Expected DX=15 DY=-8, got DX=%d DY=%d", mi.DX, mi.DY)
	}
	if mi.DWFlags != MouseEventfMove {
		t.Errorf("Expected DWFlags %x, got %x", MouseEventfMove, mi.DWFlags)
	}
	if mi.DWExtraInfo != CrossKVMWindowsMarker {
		t.Errorf("Expected DWExtraInfo = 0x%X, got 0x%X", CrossKVMWindowsMarker, mi.DWExtraInfo)
	}
}

func TestWindows_TranslateMouseButtons(t *testing.T) {
	tests := []struct {
		name          string
		event         InputEvent
		wantFlags     uint32
		wantMouseData uint32
		wantErr       bool
	}{
		{
			name:      "left button down",
			event:     NewMouseButtonDownEvent(MouseButtonLeft),
			wantFlags: MouseEventfLeftDown,
		},
		{
			name:      "left button up",
			event:     NewMouseButtonUpEvent(MouseButtonLeft),
			wantFlags: MouseEventfLeftUp,
		},
		{
			name:      "right button down",
			event:     NewMouseButtonDownEvent(MouseButtonRight),
			wantFlags: MouseEventfRightDown,
		},
		{
			name:      "right button up",
			event:     NewMouseButtonUpEvent(MouseButtonRight),
			wantFlags: MouseEventfRightUp,
		},
		{
			name:      "middle button down",
			event:     NewMouseButtonDownEvent(MouseButtonMiddle),
			wantFlags: MouseEventfMiddleDown,
		},
		{
			name:      "middle button up",
			event:     NewMouseButtonUpEvent(MouseButtonMiddle),
			wantFlags: MouseEventfMiddleUp,
		},
		{
			name:          "button 4 down (XBUTTON1)",
			event:         NewMouseButtonDownEvent(MouseButton4),
			wantFlags:     MouseEventfXDown,
			wantMouseData: XButton1,
		},
		{
			name:          "button 4 up (XBUTTON1)",
			event:         NewMouseButtonUpEvent(MouseButton4),
			wantFlags:     MouseEventfXUp,
			wantMouseData: XButton1,
		},
		{
			name:          "button 5 down (XBUTTON2)",
			event:         NewMouseButtonDownEvent(MouseButton5),
			wantFlags:     MouseEventfXDown,
			wantMouseData: XButton2,
		},
		{
			name:          "button 5 up (XBUTTON2)",
			event:         NewMouseButtonUpEvent(MouseButton5),
			wantFlags:     MouseEventfXUp,
			wantMouseData: XButton2,
		},
		{
			name:    "unsupported button",
			event:   NewMouseButtonDownEvent(MouseButton("invalid_button")),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputs, err := TranslateWindowsEvent(tt.event)
			if (err != nil) != tt.wantErr {
				t.Fatalf("TranslateWindowsEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(inputs) != 1 {
				t.Fatalf("Expected 1 input, got %d", len(inputs))
			}
			mi := inputs[0].MouseInput()
			if mi.DWFlags != tt.wantFlags {
				t.Errorf("DWFlags = 0x%X, want 0x%X", mi.DWFlags, tt.wantFlags)
			}
			if mi.MouseData != tt.wantMouseData {
				t.Errorf("MouseData = %d, want %d", mi.MouseData, tt.wantMouseData)
			}
			if mi.DWExtraInfo != CrossKVMWindowsMarker {
				t.Errorf("Expected DWExtraInfo = 0x%X, got 0x%X", CrossKVMWindowsMarker, mi.DWExtraInfo)
			}
		})
	}
}

func TestWindows_TranslateMouseWheel(t *testing.T) {
	// Vertical wheel
	vEv := NewMouseWheelEvent(0, 120)
	vInputs, err := TranslateWindowsEvent(vEv)
	if err != nil {
		t.Fatalf("Vertical wheel error: %v", err)
	}
	if len(vInputs) != 1 {
		t.Fatalf("Expected 1 input for vertical wheel, got %d", len(vInputs))
	}
	vMi := vInputs[0].MouseInput()
	if vMi.DWFlags != MouseEventfWheel || int32(vMi.MouseData) != 120 {
		t.Errorf("Vertical wheel mismatch: flags=0x%X data=%d", vMi.DWFlags, int32(vMi.MouseData))
	}
	if vMi.DWExtraInfo != CrossKVMWindowsMarker {
		t.Errorf("Expected DWExtraInfo = 0x%X, got 0x%X", CrossKVMWindowsMarker, vMi.DWExtraInfo)
	}

	// Negative vertical wheel
	negVEv := NewMouseWheelEvent(0, -120)
	negVInputs, err := TranslateWindowsEvent(negVEv)
	if err != nil {
		t.Fatalf("Negative vertical wheel error: %v", err)
	}
	negVMi := negVInputs[0].MouseInput()
	if negVMi.DWFlags != MouseEventfWheel || int32(negVMi.MouseData) != -120 {
		t.Errorf("Negative vertical wheel mismatch: flags=0x%X data=%d", negVMi.DWFlags, int32(negVMi.MouseData))
	}

	// Horizontal wheel
	hEv := NewMouseWheelEvent(120, 0)
	hInputs, err := TranslateWindowsEvent(hEv)
	if err != nil {
		t.Fatalf("Horizontal wheel error: %v", err)
	}
	if len(hInputs) != 1 {
		t.Fatalf("Expected 1 input for horizontal wheel, got %d", len(hInputs))
	}
	hMi := hInputs[0].MouseInput()
	if hMi.DWFlags != MouseEventfHWheel || int32(hMi.MouseData) != 120 {
		t.Errorf("Horizontal wheel mismatch: flags=0x%X data=%d", hMi.DWFlags, int32(hMi.MouseData))
	}

	// Both vertical and horizontal
	bothEv := NewMouseWheelEvent(60, 120)
	bothInputs, err := TranslateWindowsEvent(bothEv)
	if err != nil {
		t.Fatalf("Both wheel error: %v", err)
	}
	if len(bothInputs) != 2 {
		t.Fatalf("Expected 2 inputs for combined wheel, got %d", len(bothInputs))
	}
}

func TestWindows_TranslateKeyboard(t *testing.T) {
	tests := []struct {
		name      string
		event     InputEvent
		wantVK    uint16
		wantScan  uint16
		wantFlags uint32
		wantErr   bool
	}{
		{
			name:      "key down A",
			event:     NewKeyDownEvent("A", 0),
			wantVK:    'A',
			wantScan:  0x1E,
			wantFlags: KeyEventfScancode,
		},
		{
			name:      "key up A",
			event:     NewKeyUpEvent("A", 0),
			wantVK:    'A',
			wantScan:  0x1E,
			wantFlags: KeyEventfScancode | KeyEventfKeyUp,
		},
		{
			name:      "key down Enter",
			event:     NewKeyDownEvent("Enter", 0),
			wantVK:    vkReturn,
			wantScan:  0x1C,
			wantFlags: KeyEventfScancode,
		},
		{
			name:      "key down Escape",
			event:     NewKeyDownEvent("Escape", 0),
			wantVK:    vkEscape,
			wantScan:  0x01,
			wantFlags: KeyEventfScancode,
		},
		{
			name:      "key down ArrowUp (extended key)",
			event:     NewKeyDownEvent("ArrowUp", 0),
			wantVK:    vkUp,
			wantScan:  0x48,
			wantFlags: KeyEventfScancode | KeyEventfExtendedKey,
		},
		{
			name:      "key up ArrowUp (extended key)",
			event:     NewKeyUpEvent("ArrowUp", 0),
			wantVK:    vkUp,
			wantScan:  0x48,
			wantFlags: KeyEventfScancode | KeyEventfExtendedKey | KeyEventfKeyUp,
		},
		{
			name:      "key down ControlRight (extended key)",
			event:     NewKeyDownEvent("ControlRight", 0),
			wantVK:    vkRControl,
			wantScan:  0x1D,
			wantFlags: KeyEventfScancode | KeyEventfExtendedKey,
		},
		{
			name:      "key down Left Win/Meta (extended key)",
			event:     NewKeyDownEvent("MetaLeft", 0),
			wantVK:    vkLWin,
			wantScan:  0x5B,
			wantFlags: KeyEventfScancode | KeyEventfExtendedKey,
		},
		{
			name:      "key down F5",
			event:     NewKeyDownEvent("F5", 0),
			wantVK:    vkF5,
			wantScan:  0x3F,
			wantFlags: KeyEventfScancode,
		},
		{
			name:    "unknown key",
			event:   NewKeyDownEvent("UnknownKeyName123", 0),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputs, err := TranslateWindowsEvent(tt.event)
			if (err != nil) != tt.wantErr {
				t.Fatalf("TranslateWindowsEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(inputs) != 1 {
				t.Fatalf("Expected 1 input, got %d", len(inputs))
			}
			ki := inputs[0].KeybdInput()
			if ki.WVk != tt.wantVK {
				t.Errorf("WVk = 0x%X, want 0x%X", ki.WVk, tt.wantVK)
			}
			if ki.WScan != tt.wantScan {
				t.Errorf("WScan = 0x%X, want 0x%X", ki.WScan, tt.wantScan)
			}
			if ki.DWFlags != tt.wantFlags {
				t.Errorf("DWFlags = 0x%X, want 0x%X", ki.DWFlags, tt.wantFlags)
			}
			if ki.DWExtraInfo != CrossKVMWindowsMarker {
				t.Errorf("Expected DWExtraInfo = 0x%X, got 0x%X", CrossKVMWindowsMarker, ki.DWExtraInfo)
			}
		})
	}
}

func TestWindowsCapture_MouseBaselineAndDeltas(t *testing.T) {
	state := NewWindowsCaptureState()

	// 1. First movement establishes baseline -> no event emitted
	ev, ok := state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{
		Pt: Point{X: 500, Y: 300},
	})
	if ok {
		t.Fatalf("First mouse move should establish baseline without emitting, got %+v", ev)
	}

	// 2. Subsequent positive movement
	ev, ok = state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{
		Pt: Point{X: 512, Y: 306},
	})
	if !ok {
		t.Fatalf("Expected mouse move event, got ok=false")
	}
	if ev.Type != EventTypeMouseMove || ev.DX != 12 || ev.DY != 6 {
		t.Errorf("Expected dx=12 dy=6, got %+v", ev)
	}

	// 3. Subsequent negative movement
	ev, ok = state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{
		Pt: Point{X: 508, Y: 301},
	})
	if !ok || ev.DX != -4 || ev.DY != -5 {
		t.Errorf("Expected dx=-4 dy=-5, got ok=%v %+v", ok, ev)
	}

	// 4. Zero movement delta ignored
	ev, ok = state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{
		Pt: Point{X: 508, Y: 301},
	})
	if ok {
		t.Fatalf("Zero delta should be ignored, got %+v", ev)
	}

	// 5. Centering suppression: SetCentering to (640, 360) absorbs the synthetic jump event cleanly
	state.SetCentering(true)
	ev, ok = state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{
		Pt: Point{X: 640, Y: 360},
	})
	if ok {
		t.Fatalf("Synthetic centering event should be suppressed, got %+v", ev)
	}

	// 6. Next physical movement from centered position produces clean relative delta
	ev, ok = state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{
		Pt: Point{X: 625, Y: 360},
	})
	if !ok || ev.DX != -15 || ev.DY != 0 {
		t.Errorf("Expected dx=-15 dy=0 after centering, got ok=%v %+v", ok, ev)
	}
}

func TestWindowsCapture_MouseButtons(t *testing.T) {
	state := NewWindowsCaptureState()

	// Left down & up
	ev, ok := state.TranslateCapturedMouse(WM_LBUTTONDOWN, &MSLLHOOKSTRUCT{Pt: Point{X: 100, Y: 200}})
	if !ok || ev.Type != EventTypeMouseButtonDown || ev.Button != MouseButtonLeft {
		t.Errorf("Expected LeftButtonDown, got ok=%v %+v", ok, ev)
	}
	ev, ok = state.TranslateCapturedMouse(WM_LBUTTONUP, &MSLLHOOKSTRUCT{Pt: Point{X: 100, Y: 200}})
	if !ok || ev.Type != EventTypeMouseButtonUp || ev.Button != MouseButtonLeft {
		t.Errorf("Expected LeftButtonUp, got ok=%v %+v", ok, ev)
	}

	// Right down & up
	ev, ok = state.TranslateCapturedMouse(WM_RBUTTONDOWN, &MSLLHOOKSTRUCT{Pt: Point{X: 100, Y: 200}})
	if !ok || ev.Type != EventTypeMouseButtonDown || ev.Button != MouseButtonRight {
		t.Errorf("Expected RightButtonDown, got ok=%v %+v", ok, ev)
	}
	ev, ok = state.TranslateCapturedMouse(WM_RBUTTONUP, &MSLLHOOKSTRUCT{Pt: Point{X: 100, Y: 200}})
	if !ok || ev.Type != EventTypeMouseButtonUp || ev.Button != MouseButtonRight {
		t.Errorf("Expected RightButtonUp, got ok=%v %+v", ok, ev)
	}

	// Middle down & up
	ev, ok = state.TranslateCapturedMouse(WM_MBUTTONDOWN, &MSLLHOOKSTRUCT{Pt: Point{X: 100, Y: 200}})
	if !ok || ev.Type != EventTypeMouseButtonDown || ev.Button != MouseButtonMiddle {
		t.Errorf("Expected MiddleButtonDown, got ok=%v %+v", ok, ev)
	}
	ev, ok = state.TranslateCapturedMouse(WM_MBUTTONUP, &MSLLHOOKSTRUCT{Pt: Point{X: 100, Y: 200}})
	if !ok || ev.Type != EventTypeMouseButtonUp || ev.Button != MouseButtonMiddle {
		t.Errorf("Expected MiddleButtonUp, got ok=%v %+v", ok, ev)
	}

	// XButton 1 (Button 4)
	ev, ok = state.TranslateCapturedMouse(WM_XBUTTONDOWN, &MSLLHOOKSTRUCT{
		Pt:        Point{X: 100, Y: 200},
		MouseData: uint32(XButton1) << 16,
	})
	if !ok || ev.Button != MouseButton4 {
		t.Errorf("Expected Button4, got ok=%v %+v", ok, ev)
	}

	// XButton 2 (Button 5)
	ev, ok = state.TranslateCapturedMouse(WM_XBUTTONUP, &MSLLHOOKSTRUCT{
		Pt:        Point{X: 100, Y: 200},
		MouseData: uint32(XButton2) << 16,
	})
	if !ok || ev.Button != MouseButton5 {
		t.Errorf("Expected Button5, got ok=%v %+v", ok, ev)
	}
}

func encodeWheelDelta(delta int16) uint32 {
	return uint32(uint16(delta)) << 16
}

func TestWindowsCapture_MouseWheel(t *testing.T) {
	state := NewWindowsCaptureState()

	// Vertical +120 (Wheel up)
	ev, ok := state.TranslateCapturedMouse(WM_MOUSEWHEEL, &MSLLHOOKSTRUCT{
		MouseData: encodeWheelDelta(120),
	})
	if !ok || ev.Type != EventTypeMouseWheel || ev.WheelDY != 120 || ev.WheelDX != 0 {
		t.Errorf("Expected WheelDY=120, got ok=%v %+v", ok, ev)
	}

	// Vertical -120 (Wheel down)
	ev, ok = state.TranslateCapturedMouse(WM_MOUSEWHEEL, &MSLLHOOKSTRUCT{
		MouseData: encodeWheelDelta(-120),
	})
	if !ok || ev.WheelDY != -120 || ev.WheelDX != 0 {
		t.Errorf("Expected WheelDY=-120, got ok=%v %+v", ok, ev)
	}

	// Horizontal +120
	ev, ok = state.TranslateCapturedMouse(WM_MOUSEHWHEEL, &MSLLHOOKSTRUCT{
		MouseData: encodeWheelDelta(120),
	})
	if !ok || ev.WheelDX != 120 || ev.WheelDY != 0 {
		t.Errorf("Expected WheelDX=120, got ok=%v %+v", ok, ev)
	}

	// Horizontal -120
	ev, ok = state.TranslateCapturedMouse(WM_MOUSEHWHEEL, &MSLLHOOKSTRUCT{
		MouseData: encodeWheelDelta(-120),
	})
	if !ok || ev.WheelDX != -120 || ev.WheelDY != 0 {
		t.Errorf("Expected WheelDX=-120, got ok=%v %+v", ok, ev)
	}

	// Zero delta ignored
	ev, ok = state.TranslateCapturedMouse(WM_MOUSEWHEEL, &MSLLHOOKSTRUCT{
		MouseData: 0,
	})
	if ok {
		t.Fatalf("Zero wheel delta should be ignored, got %+v", ev)
	}
}

func TestWindowsCapture_KeyboardAndModifiers(t *testing.T) {
	state := NewWindowsCaptureState()

	// 1. Regular key down 'A'
	ev, ok := state.TranslateCapturedKeyboard(WM_KEYDOWN, &KBDLLHOOKSTRUCT{VKCode: 'A', ScanCode: 0x1E})
	if !ok || ev.Type != EventTypeKeyDown || ev.Key != "A" {
		t.Errorf("Expected KeyDown A, got ok=%v %+v", ok, ev)
	}

	// 2. Regular key up 'A'
	ev, ok = state.TranslateCapturedKeyboard(WM_KEYUP, &KBDLLHOOKSTRUCT{VKCode: 'A', ScanCode: 0x1E})
	if !ok || ev.Type != EventTypeKeyUp || ev.Key != "A" {
		t.Errorf("Expected KeyUp A, got ok=%v %+v", ok, ev)
	}

	// 3. Modifiers: ShiftLeft (scan 0x2A) vs ShiftRight (scan 0x36)
	ev, ok = state.TranslateCapturedKeyboard(WM_KEYDOWN, &KBDLLHOOKSTRUCT{VKCode: vkShift, ScanCode: 0x2A})
	if !ok || ev.Key != "ShiftLeft" {
		t.Errorf("Expected ShiftLeft, got ok=%v %+v", ok, ev)
	}
	ev, ok = state.TranslateCapturedKeyboard(WM_KEYDOWN, &KBDLLHOOKSTRUCT{VKCode: vkShift, ScanCode: 0x36})
	if !ok || ev.Key != "ShiftRight" {
		t.Errorf("Expected ShiftRight, got ok=%v %+v", ok, ev)
	}

	// 4. Modifiers: ControlLeft vs ControlRight (extended)
	ev, ok = state.TranslateCapturedKeyboard(WM_KEYDOWN, &KBDLLHOOKSTRUCT{VKCode: vkControl, ScanCode: 0x1D})
	if !ok || ev.Key != "ControlLeft" {
		t.Errorf("Expected ControlLeft, got ok=%v %+v", ok, ev)
	}
	ev, ok = state.TranslateCapturedKeyboard(WM_KEYDOWN, &KBDLLHOOKSTRUCT{VKCode: vkControl, ScanCode: 0x1D, Flags: LLKHF_EXTENDED})
	if !ok || ev.Key != "ControlRight" {
		t.Errorf("Expected ControlRight, got ok=%v %+v", ok, ev)
	}

	// 5. Modifiers: AltLeft vs AltRight (extended)
	ev, ok = state.TranslateCapturedKeyboard(WM_SYSKEYDOWN, &KBDLLHOOKSTRUCT{VKCode: vkMenu, ScanCode: 0x38})
	if !ok || ev.Key != "AltLeft" {
		t.Errorf("Expected AltLeft, got ok=%v %+v", ok, ev)
	}
	ev, ok = state.TranslateCapturedKeyboard(WM_SYSKEYDOWN, &KBDLLHOOKSTRUCT{VKCode: vkMenu, ScanCode: 0x38, Flags: LLKHF_EXTENDED})
	if !ok || ev.Key != "AltRight" {
		t.Errorf("Expected AltRight, got ok=%v %+v", ok, ev)
	}

	// 6. Modifiers: MetaLeft (vkLWin) vs MetaRight (vkRWin)
	ev, ok = state.TranslateCapturedKeyboard(WM_KEYDOWN, &KBDLLHOOKSTRUCT{VKCode: vkLWin, ScanCode: 0x5B, Flags: LLKHF_EXTENDED})
	if !ok || ev.Key != "MetaLeft" {
		t.Errorf("Expected MetaLeft, got ok=%v %+v", ok, ev)
	}
	ev, ok = state.TranslateCapturedKeyboard(WM_KEYDOWN, &KBDLLHOOKSTRUCT{VKCode: vkRWin, ScanCode: 0x5C, Flags: LLKHF_EXTENDED})
	if !ok || ev.Key != "MetaRight" {
		t.Errorf("Expected MetaRight, got ok=%v %+v", ok, ev)
	}

	// 7. Navigation & Function keys
	ev, ok = state.TranslateCapturedKeyboard(WM_KEYDOWN, &KBDLLHOOKSTRUCT{VKCode: vkUp, ScanCode: 0x48, Flags: LLKHF_EXTENDED})
	if !ok || ev.Key != "ArrowUp" {
		t.Errorf("Expected ArrowUp, got ok=%v %+v", ok, ev)
	}
	ev, ok = state.TranslateCapturedKeyboard(WM_KEYDOWN, &KBDLLHOOKSTRUCT{VKCode: vkF12, ScanCode: 0x58})
	if !ok || ev.Key != "F12" {
		t.Errorf("Expected F12, got ok=%v %+v", ok, ev)
	}
}

func TestWindowsCapture_SyntheticEventFiltering(t *testing.T) {
	state := NewWindowsCaptureState()

	// Mouse event tagged with CrossKVM marker should be ignored
	_, ok := state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{
		Pt:          Point{X: 100, Y: 100},
		DWExtraInfo: CrossKVMWindowsMarker,
	})
	if ok {
		t.Fatalf("CrossKVM synthetic mouse event should be ignored")
	}

	// Wheel event tagged with CrossKVM marker should be ignored
	_, ok = state.TranslateCapturedMouse(WM_MOUSEWHEEL, &MSLLHOOKSTRUCT{
		MouseData:   uint32(uint16(120)) << 16,
		DWExtraInfo: CrossKVMWindowsMarker,
	})
	if ok {
		t.Fatalf("CrossKVM synthetic wheel event should be ignored")
	}

	// Keyboard event tagged with CrossKVM marker should be ignored
	_, ok = state.TranslateCapturedKeyboard(WM_KEYDOWN, &KBDLLHOOKSTRUCT{
		VKCode:      'A',
		ScanCode:    0x1E,
		DWExtraInfo: CrossKVMWindowsMarker,
	})
	if ok {
		t.Fatalf("CrossKVM synthetic keyboard event should be ignored")
	}

	// Physical (unmarked) event should be accepted
	ev, ok := state.TranslateCapturedKeyboard(WM_KEYDOWN, &KBDLLHOOKSTRUCT{
		VKCode:      'A',
		ScanCode:    0x1E,
		DWExtraInfo: 0,
	})
	if !ok || ev.Key != "A" {
		t.Fatalf("Physical event should be accepted, got ok=%v %+v", ok, ev)
	}
}

func TestWindowsSuppressedMouseUsesFixedCursorAnchor(t *testing.T) {
	state := NewWindowsCaptureState()
	state.SetSuppressed(true, 640, 360)
	// Windows hook coordinates are proposed positions relative to the frozen
	// OS cursor, not cumulative positions after earlier suppressed events.
	for _, delta := range []Point{{X: 8, Y: 3}, {X: 8, Y: 3}, {X: 2, Y: 1}, {X: -5, Y: -4}, {X: 300, Y: 0}} {
		ev, ok := state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{Pt: Point{X: 640 + delta.X, Y: 360 + delta.Y}})
		if !ok || ev.DX != delta.X || ev.DY != delta.Y {
			t.Fatalf("movement %+v: got %+v, emitted=%v", delta, ev, ok)
		}
	}
	state.TranslateCapturedMouse(WM_LBUTTONDOWN, &MSLLHOOKSTRUCT{Pt: Point{X: 648, Y: 363}})
	ev, ok := state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{Pt: Point{X: 648, Y: 363}})
	if !ok || ev.DX != 8 || ev.DY != 3 {
		t.Fatalf("drag movement changed anchor: %+v, emitted=%v", ev, ok)
	}
	if _, ok := state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{Pt: Point{X: 640, Y: 360}}); ok {
		t.Fatal("centering event must not emit movement")
	}
	state.SetSuppressed(false, 0, 0)
	if _, ok := state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{Pt: Point{X: 100, Y: 100}}); ok {
		t.Fatal("local capture must establish a fresh baseline")
	}
	ev, ok = state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{Pt: Point{X: 107, Y: 102}})
	if !ok || ev.DX != 7 || ev.DY != 2 {
		t.Fatalf("local capture failed: %+v, emitted=%v", ev, ok)
	}
}

func TestWindowsSuppressedMouseTracksActualAnchor(t *testing.T) {
	state := NewWindowsCaptureState()
	state.SetSuppressed(true, 640, 360)
	// The center request has not yet been applied; the cursor is still at
	// the exit edge. Movement must use that actual position.
	state.SetSuppressedAnchor(1918, 400)
	ev, ok := state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{Pt: Point{X: 1919, Y: 402}})
	if !ok || ev.DX != 1 || ev.DY != 2 {
		t.Fatalf("before centering: %+v, emitted=%v", ev, ok)
	}
	// The tagged centering event is ignored, and the next physical movement
	// uses the new actual cursor position.
	if _, ok := state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{Pt: Point{X: 640, Y: 360}, DWExtraInfo: CrossKVMWindowsMarker}); ok {
		t.Fatal("synthetic centering must not be forwarded")
	}
	state.SetSuppressedAnchor(640, 360)
	for i := 0; i < 3; i++ {
		ev, ok = state.TranslateCapturedMouse(WM_MOUSEMOVE, &MSLLHOOKSTRUCT{Pt: Point{X: 645, Y: 357}})
		if !ok || ev.DX != 5 || ev.DY != -3 {
			t.Fatalf("after centering: %+v, emitted=%v", ev, ok)
		}
	}
}

func TestWindowsEmergencyChord(t *testing.T) {
	var chord WindowsEmergencyChord
	for _, vk := range []uint32{0xA2, 0xA4, 0xA0} {
		if chord.Observe(WM_KEYDOWN, KBDLLHOOKSTRUCT{VKCode: vk}) {
			t.Fatal("premature release")
		}
	}
	if chord.Observe(WM_KEYDOWN, KBDLLHOOKSTRUCT{VKCode: 0x1B, Flags: LLKHF_INJECTED}) {
		t.Fatal("synthetic chord released input")
	}
	if !chord.Observe(WM_KEYDOWN, KBDLLHOOKSTRUCT{VKCode: 0x1B}) {
		t.Fatal("physical chord did not release")
	}
	chord.Observe(WM_KEYUP, KBDLLHOOKSTRUCT{VKCode: 0xA4})
	if chord.Observe(WM_KEYDOWN, KBDLLHOOKSTRUCT{VKCode: 0x1B}) {
		t.Fatal("released Alt stayed held")
	}
}
