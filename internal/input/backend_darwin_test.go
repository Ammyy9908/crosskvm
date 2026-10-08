//go:build darwin

package input

import "testing"

func TestDarwinPhysicalMovementUpdatesRemoteInjectionOrigin(t *testing.T) {
	tracker := NewDarwinStateTracker(DarwinPoint{X: 1468, Y: 955}, DarwinRect{Width: 1470, Height: 956})
	b := &DarwinBackend{tracker: tracker, captureState: NewDarwinCaptureState(), eventsCh: make(chan InputEvent, 4)}
	b.handleCapturedEvent(CGEventTypeMouseMoved, 500, 300, 0, 0, 0, 0, 0, 0)
	spec, err := tracker.Translate(NewMouseMoveEvent(6, 0))
	if err != nil || spec.Mouse.Location != (DarwinPoint{X: 506, Y: 300}) {
		t.Fatalf("remote move did not follow physical cursor: %+v, err=%v", spec, err)
	}
	b.handleCapturedEvent(CGEventTypeMouseMoved, 1469, 955, 0, 0, 0, 0, 0, CrossKVMMarker)
	if tracker.CursorPos() != (DarwinPoint{X: 506, Y: 300}) {
		t.Fatal("synthetic capture changed injection origin")
	}
}

func TestDarwinCursorVisibilityBalancedAndRestoredOnClose(t *testing.T) {
	var calls []bool
	b := &DarwinBackend{cursorVisibility: func(hidden bool) error { calls = append(calls, hidden); return nil }}
	for _, hidden := range []bool{true, true, false, false, true} {
		if err := b.setCursorHidden(hidden); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	b.Close()
	want := []bool{true, false, true, false}
	if len(calls) != len(want) {
		t.Fatalf("unbalanced calls: %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls: %v", calls)
		}
	}
}
