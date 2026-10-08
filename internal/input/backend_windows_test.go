//go:build windows

package input

import (
	"errors"
	"testing"
)

func TestWindows_BackendMockSender(t *testing.T) {
	var captured []TagINPUT
	mockSender := func(inputs []TagINPUT) error {
		captured = append(captured, inputs...)
		return nil
	}

	backend := &WindowsBackend{
		sender: mockSender,
	}

	// Inject mouse move
	err := backend.Inject(NewMouseMoveEvent(10, 20))
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("Expected 1 captured input, got %d", len(captured))
	}
	mi := captured[0].MouseInput()
	if mi.DX != 10 || mi.DY != 20 {
		t.Errorf("Captured mouse input mismatch: DX=%d DY=%d", mi.DX, mi.DY)
	}
	if mi.DWExtraInfo != CrossKVMWindowsMarker {
		t.Errorf("Captured DWExtraInfo mismatch: want 0x%X, got 0x%X", CrossKVMWindowsMarker, mi.DWExtraInfo)
	}

	// Test Close is idempotent
	if err := backend.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Second Close error: %v", err)
	}

	// StartCapture after Close returns ErrBackendClosed
	ch := make(chan InputEvent, 1)
	if !errors.Is(backend.StartCapture(ch), ErrBackendClosed) {
		t.Errorf("Expected ErrBackendClosed on StartCapture after Close")
	}

	// Inject after close returns ErrBackendClosed
	if !errors.Is(backend.Inject(NewMouseMoveEvent(1, 1)), ErrBackendClosed) {
		t.Errorf("Expected ErrBackendClosed on Inject after Close")
	}
}
