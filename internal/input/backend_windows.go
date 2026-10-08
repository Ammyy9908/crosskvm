//go:build windows

package input

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	modUser32                = syscall.NewLazyDLL("user32.dll")
	procSendInput            = modUser32.NewProc("SendInput")
	procSetWindowsHookEx     = modUser32.NewProc("SetWindowsHookExW")
	procCallNextHookEx       = modUser32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx  = modUser32.NewProc("UnhookWindowsHookEx")
	procGetMessage           = modUser32.NewProc("GetMessageW")
	procTranslateMessage     = modUser32.NewProc("TranslateMessage")
	procDispatchMessage      = modUser32.NewProc("DispatchMessageW")
	procPostThreadMessage    = modUser32.NewProc("PostThreadMessageW")
	procGetSystemMetrics     = modUser32.NewProc("GetSystemMetrics")
	procSetProcessDPIAware   = modUser32.NewProc("SetProcessDPIAware")
	procSetCursorPos         = modUser32.NewProc("SetCursorPos")
	procGetPhysicalCursorPos = modUser32.NewProc("GetPhysicalCursorPos")

	modKernel32            = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentThreadId = modKernel32.NewProc("GetCurrentThreadId")
)

var (
	activeWindowsBackend atomic.Pointer[WindowsBackend]
)

type inputSender func(inputs []TagINPUT) error

// WindowsBackend is the native Windows implementation of InputBackend using Win32 SendInput and low-level hooks.
type WindowsBackend struct {
	mu               sync.Mutex
	closed           atomic.Bool
	suppressed       atomic.Bool
	eventsCh         chan<- InputEvent
	captureState     *WindowsCaptureState
	hookThreadID     uint32
	hMouseHook       uintptr
	hKeyboardHook    uintptr
	captureDone      chan struct{}
	sender           inputSender
	emergencyChord   WindowsEmergencyChord
	emergencyHandler func()
	emergencyPending atomic.Bool
}

// NewNativeBackend initializes the native Windows input backend.
func NewNativeBackend() (InputBackend, error) {
	return NewWindowsBackend(), nil
}

// NewWindowsBackend creates a new WindowsBackend instance.
func NewWindowsBackend() *WindowsBackend {
	// Keep screen geometry and cursor warps in physical pixels, matching
	// MSLLHOOKSTRUCT. The explicit physical query also handles inherited DPI contexts.
	procSetProcessDPIAware.Call()
	return &WindowsBackend{
		sender: callSendInput,
	}
}

// Inject translates a platform-neutral InputEvent into Win32 INPUT structures and injects them into the OS.
func (b *WindowsBackend) Inject(event InputEvent) error {
	if b.closed.Load() {
		return ErrBackendClosed
	}
	b.mu.Lock()
	sender := b.sender
	b.mu.Unlock()

	inputs, err := TranslateWindowsEvent(event)
	if err != nil {
		return fmt.Errorf("windows inject: translation failed: %w", err)
	}

	if len(inputs) == 0 {
		return nil
	}

	if sender == nil {
		sender = callSendInput
	}

	if err := sender(inputs); err != nil {
		return fmt.Errorf("windows inject: native SendInput error: %w", err)
	}

	return nil
}

// StartCapture installs WH_MOUSE_LL and WH_KEYBOARD_LL hooks and begins capturing physical input.
func (b *WindowsBackend) StartCapture(events chan<- InputEvent) error {
	if events == nil {
		return errors.New("events channel cannot be nil")
	}

	b.mu.Lock()
	if b.closed.Load() {
		b.mu.Unlock()
		return ErrBackendClosed
	}
	if b.eventsCh != nil || b.hookThreadID != 0 {
		b.mu.Unlock()
		return ErrCaptureAlreadyBusy
	}

	b.eventsCh = events
	b.captureState = NewWindowsCaptureState()
	captureDone := make(chan struct{})
	b.captureDone = captureDone
	b.mu.Unlock()

	activeWindowsBackend.Store(b)

	startedCh := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(captureDone)

		threadID, _, _ := procGetCurrentThreadId.Call()

		b.mu.Lock()
		b.hookThreadID = uint32(threadID)
		b.mu.Unlock()

		mouseCb := syscall.NewCallback(lowLevelMouseProc)
		hMouse, _, errMouse := procSetWindowsHookEx.Call(
			uintptr(WH_MOUSE_LL),
			mouseCb,
			0,
			0,
		)
		if hMouse == 0 {
			startedCh <- fmt.Errorf("windows: failed to install WH_MOUSE_LL hook: %w", errMouse)
			return
		}

		kbdCb := syscall.NewCallback(lowLevelKeyboardProc)
		hKbd, _, errKbd := procSetWindowsHookEx.Call(
			uintptr(WH_KEYBOARD_LL),
			kbdCb,
			0,
			0,
		)
		if hKbd == 0 {
			procUnhookWindowsHookEx.Call(hMouse)
			startedCh <- fmt.Errorf("windows: failed to install WH_KEYBOARD_LL hook: %w", errKbd)
			return
		}

		b.mu.Lock()
		b.hMouseHook = hMouse
		b.hKeyboardHook = hKbd
		b.mu.Unlock()

		// Signal success to caller
		close(startedCh)

		// Run message loop required for Windows low-level hooks
		var msg MSG
		for {
			ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if ret == 0 || int32(ret) == -1 {
				// WM_QUIT or error
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		}

		// Cleanup hooks on the hook thread
		procUnhookWindowsHookEx.Call(hMouse)
		procUnhookWindowsHookEx.Call(hKbd)

		b.mu.Lock()
		b.hMouseHook = 0
		b.hKeyboardHook = 0
		b.hookThreadID = 0
		b.mu.Unlock()

		activeWindowsBackend.CompareAndSwap(b, nil)
	}()

	err := <-startedCh
	if err != nil {
		b.mu.Lock()
		b.eventsCh = nil
		b.hookThreadID = 0
		b.mu.Unlock()
		return err
	}

	return nil
}

// handleCapturedMouse handles a mouse hook event without mutex contention on hot path.
func (b *WindowsBackend) handleCapturedMouse(msg uint32, data MSLLHOOKSTRUCT) {
	if b.closed.Load() {
		return
	}
	eventsCh := b.eventsCh
	captureState := b.captureState
	if eventsCh == nil || captureState == nil {
		return
	}

	if b.suppressed.Load() {
		var point Point
		if ok, _, _ := procGetPhysicalCursorPos.Call(uintptr(unsafe.Pointer(&point))); ok != 0 {
			captureState.SetSuppressedAnchor(point.X, point.Y)
		}
	}
	if ev, ok := captureState.TranslateCapturedMouse(msg, &data); ok {
		select {
		case eventsCh <- ev:
		default:
			// Non-blocking drop to keep Windows hook thread responsive
		}
	}
}

// handleCapturedKeyboard handles a keyboard hook event.
func (b *WindowsBackend) handleCapturedKeyboard(msg uint32, data KBDLLHOOKSTRUCT) {
	if b.closed.Load() {
		return
	}
	eventsCh := b.eventsCh
	captureState := b.captureState
	if eventsCh == nil || captureState == nil {
		return
	}

	if ev, ok := captureState.TranslateCapturedKeyboard(msg, &data); ok {
		select {
		case eventsCh <- ev:
		default:
			// Non-blocking drop to keep Windows hook thread responsive
		}
	}
}

func lowLevelMouseProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && lParam != 0 {
		hookStruct := (*MSLLHOOKSTRUCT)(unsafe.Pointer(lParam))
		data := *hookStruct

		// If this is a synthetic event injected by CrossKVM, never suppress it
		if data.DWExtraInfo == CrossKVMWindowsMarker {
			ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
			return ret
		}

		b := activeWindowsBackend.Load()
		if b != nil {
			b.handleCapturedMouse(uint32(wParam), data)
			if b.IsSuppressed() {
				// Suppress local processing of the physical mouse event
				return 1
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func lowLevelKeyboardProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && lParam != 0 {
		hookStruct := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
		data := *hookStruct

		// If this is a synthetic event injected by CrossKVM, never suppress it
		if data.DWExtraInfo == CrossKVMWindowsMarker {
			ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
			return ret
		}

		b := activeWindowsBackend.Load()
		if b != nil {
			if b.emergencyChord.Observe(uint32(wParam), data) {
				b.suppressed.Store(false)
				if b.captureState != nil {
					b.captureState.SetSuppressed(false, 0, 0)
				}
				if b.emergencyHandler != nil && b.emergencyPending.CompareAndSwap(false, true) {
					go func() { defer b.emergencyPending.Store(false); b.emergencyHandler() }()
				}
				// Pass escape keystroke through to OS
				ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
				return ret
			}
			b.handleCapturedKeyboard(uint32(wParam), data)
			if b.IsSuppressed() {
				// Suppress local processing of the physical keyboard event
				return 1
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

// IsSuppressed returns whether local input suppression is currently active. Lock-free.
func (b *WindowsBackend) IsSuppressed() bool {
	return b.suppressed.Load()
}

// SetLocalSuppressed configures whether captured events should be suppressed locally.
func (b *WindowsBackend) SetLocalSuppressed(enabled bool) error {
	if b.closed.Load() {
		return ErrBackendClosed
	}
	b.suppressed.Store(enabled)
	b.mu.Lock()
	captureState := b.captureState
	b.mu.Unlock()

	if !enabled {
		if captureState != nil {
			captureState.SetSuppressed(false, 0, 0)
		}
		return nil
	}

	// Tagged SendInput passes through our hook even during suppression. Do not
	// hold b.mu across a native call that can invoke the hook on another thread.
	bounds := b.ScreenBounds()
	cx, cy := int32(bounds.Width/2), int32(bounds.Height/2)
	if bounds.Width <= 1 || bounds.Height <= 1 {
		b.suppressed.Store(false)
		return errors.New("windows: invalid screen bounds for cursor centering")
	}

	if captureState != nil {
		captureState.SetSuppressed(true, cx, cy)
	}
	err := callSendInput([]TagINPUT{NewMouseTagINPUT(MouseInput{
		DX:          int32(int64(cx) * 65535 / int64(bounds.Width-1)),
		DY:          int32(int64(cy) * 65535 / int64(bounds.Height-1)),
		DWFlags:     MouseEventfMove | MouseEventfAbsolute,
		DWExtraInfo: CrossKVMWindowsMarker,
	})})
	if err != nil {
		b.suppressed.Store(false)
		if captureState != nil {
			captureState.SetSuppressed(false, 0, 0)
		}
	}
	return err
}

// ScreenBounds returns the pixel dimensions of the primary Windows display.
func (b *WindowsBackend) ScreenBounds() ScreenBounds {
	w, _, _ := procGetSystemMetrics.Call(0) // SM_CXSCREEN
	h, _, _ := procGetSystemMetrics.Call(1) // SM_CYSCREEN
	if w <= 0 || h <= 0 {
		return ScreenBounds{Width: 1920, Height: 1080}
	}
	return ScreenBounds{Width: int(w), Height: int(h)}
}

// WarpCursor moves the native Windows cursor to the specified coordinates.
func (b *WindowsBackend) WarpCursor(x, y int) error {
	bounds := b.ScreenBounds()
	if bounds.Width <= 1 || bounds.Height <= 1 {
		return errors.New("windows: invalid screen bounds for cursor warp")
	}
	if x < 0 {
		x = 0
	}
	if x >= bounds.Width {
		x = bounds.Width - 1
	}
	if y < 0 {
		y = 0
	}
	if y >= bounds.Height {
		y = bounds.Height - 1
	}
	// Tag the warp so our hook cannot interpret a remote entry as physical input.
	return callSendInput([]TagINPUT{NewMouseTagINPUT(MouseInput{
		DX: int32(int64(x) * 65535 / int64(bounds.Width-1)), DY: int32(int64(y) * 65535 / int64(bounds.Height-1)),
		DWFlags: MouseEventfMove | MouseEventfAbsolute, DWExtraInfo: CrossKVMWindowsMarker,
	})})
}

// SetEmergencyReleaseHandler must be called before capture starts. The hook
// restores OS input immediately; router cleanup runs outside the hook thread.
func (b *WindowsBackend) SetEmergencyReleaseHandler(handler func()) { b.emergencyHandler = handler }

// Close gracefully terminates the backend and unhooks any active capture hooks. Safe and idempotent.
func (b *WindowsBackend) Close() error {
	if b.closed.Swap(true) {
		return nil
	}
	b.suppressed.Store(false)

	b.mu.Lock()
	threadID := b.hookThreadID
	captureDone := b.captureDone
	b.mu.Unlock()

	if threadID != 0 {
		procPostThreadMessage.Call(uintptr(threadID), uintptr(WM_QUIT), 0, 0)
	}

	if captureDone != nil {
		<-captureDone
	}

	b.mu.Lock()
	b.eventsCh = nil
	b.mu.Unlock()

	return nil
}

// callSendInput invokes the native Win32 SendInput function.
func callSendInput(inputs []TagINPUT) error {
	if len(inputs) == 0 {
		return nil
	}

	cbSize := int32(unsafe.Sizeof(inputs[0]))
	ret, _, err := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		uintptr(cbSize),
	)

	if ret == 0 {
		if err != nil && err != syscall.Errno(0) {
			return fmt.Errorf("SendInput error: %w", err)
		}
		return fmt.Errorf("SendInput failed to inject any events (0 of %d injected)", len(inputs))
	}

	if int(ret) < len(inputs) {
		return fmt.Errorf("SendInput partially succeeded (%d of %d injected)", ret, len(inputs))
	}

	return nil
}
