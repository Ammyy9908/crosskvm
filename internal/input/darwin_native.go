//go:build darwin

package input

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics

#include <ApplicationServices/ApplicationServices.h>
#include <stdbool.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdlib.h>

#define CROSSKVM_USER_DATA_MARKER 0x584B564D
#define EVENT_MASK_BIT(t) (((CGEventMask)1) << (t))

extern void goOnDarwinEvent(uintptr_t handle, uint32_t eventType, double x, double y, int64_t buttonNum, int64_t deltaY, int64_t deltaX, uint16_t keyCode, uint64_t flags, int64_t userData);

typedef struct {
    uintptr_t handle;
    CFMachPortRef machPort;
    CFRunLoopSourceRef runLoopSource;
    CFRunLoopRef runLoop;
    atomic_bool active;
    atomic_bool suppressed;
} EventTapSession;

static CGEventRef darwinEventTapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    EventTapSession *session = (EventTapSession *)refcon;
    if (!session) {
        return event;
    }

    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        if (session->machPort) {
            CGEventTapEnable(session->machPort, true);
        }
        return event;
    }

    if (!session->active) {
        return event;
    }

    int64_t userData = CGEventGetIntegerValueField(event, kCGEventSourceUserData);
    if (userData == CROSSKVM_USER_DATA_MARKER) {
        // Ignore CrossKVM synthetic events to prevent feedback loops; never suppress injected input
        return event;
    }

    CGPoint loc = CGEventGetLocation(event);
    int64_t btn = CGEventGetIntegerValueField(event, kCGMouseEventButtonNumber);
    int64_t dY = 0;
    int64_t dX = 0;
    if (type == kCGEventScrollWheel) {
        dY = CGEventGetIntegerValueField(event, kCGScrollWheelEventDeltaAxis1);
        dX = CGEventGetIntegerValueField(event, kCGScrollWheelEventDeltaAxis2);
        if (dY == 0 && dX == 0) {
            dY = CGEventGetIntegerValueField(event, kCGScrollWheelEventPointDeltaAxis1);
            dX = CGEventGetIntegerValueField(event, kCGScrollWheelEventPointDeltaAxis2);
        }
    } else if (type == kCGEventMouseMoved ||
               type == kCGEventLeftMouseDragged ||
               type == kCGEventRightMouseDragged ||
               type == kCGEventOtherMouseDragged) {
        dX = CGEventGetIntegerValueField(event, kCGMouseEventDeltaX);
        dY = CGEventGetIntegerValueField(event, kCGMouseEventDeltaY);
    }
    uint16_t key = (uint16_t)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
    uint64_t flags = (uint64_t)CGEventGetFlags(event);

    goOnDarwinEvent(session->handle, (uint32_t)type, loc.x, loc.y, btn, dY, dX, key, flags, userData);

    if (session->suppressed) {
        return NULL;
    }
    return event;
}

static bool isAccessibilityTrusted(bool prompt) {
    if (prompt) {
        const void *keys[] = { kAXTrustedCheckOptionPrompt };
        const void *values[] = { kCFBooleanTrue };
        CFDictionaryRef options = CFDictionaryCreate(
            kCFAllocatorDefault,
            keys,
            values,
            1,
            &kCFTypeDictionaryKeyCallBacks,
            &kCFTypeDictionaryValueCallBacks
        );
        bool trusted = AXIsProcessTrustedWithOptions(options);
        if (options) {
            CFRelease(options);
        }
        return trusted;
    }
    return AXIsProcessTrusted();
}

static CGPoint getCursorLocation() {
    CGEventRef event = CGEventCreate(NULL);
    if (!event) {
        return CGPointMake(0, 0);
    }
    CGPoint loc = CGEventGetLocation(event);
    CFRelease(event);
    return loc;
}

static void warpCursorPosition(double x, double y) {
    CGPoint pt = CGPointMake(x, y);
    CGWarpMouseCursorPosition(pt);
}

static CGRect getDisplayBounds() {
    CGDirectDisplayID mainDisplay = CGMainDisplayID();
    return CGDisplayBounds(mainDisplay);
}

static int postMouseEvent(uint32_t eventType, double x, double y, uint32_t button, uint64_t flags) {
    CGPoint point = CGPointMake(x, y);
    CGEventRef event = CGEventCreateMouseEvent(NULL, (CGEventType)eventType, point, (CGMouseButton)button);
    if (!event) {
        return -1;
    }
    if (flags != 0) {
        CGEventSetFlags(event, (CGEventFlags)flags);
    }
    // Tag event with CrossKVM marker so capture tap ignores it
    CGEventSetIntegerValueField(event, kCGEventSourceUserData, CROSSKVM_USER_DATA_MARKER);
    CGEventPost(kCGHIDEventTap, event);
    CFRelease(event);
    return 0;
}

static int postScrollEvent(int32_t wheelDY, int32_t wheelDX) {
    CGEventRef event = NULL;
    if (wheelDX != 0 && wheelDY != 0) {
        event = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 2, wheelDY, wheelDX);
    } else if (wheelDX != 0) {
        event = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 2, 0, wheelDX);
    } else {
        event = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 1, wheelDY);
    }
    if (!event) {
        return -1;
    }
    // Tag event with CrossKVM marker so capture tap ignores it
    CGEventSetIntegerValueField(event, kCGEventSourceUserData, CROSSKVM_USER_DATA_MARKER);
    CGEventPost(kCGHIDEventTap, event);
    CFRelease(event);
    return 0;
}

static int postKeyboardEvent(uint16_t keyCode, bool keyDown, uint64_t flags) {
    CGEventRef event = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keyCode, keyDown);
    if (!event) {
        return -1;
    }
    if (flags != 0) {
        CGEventSetFlags(event, (CGEventFlags)flags);
    }
    // Tag event with CrossKVM marker so capture tap ignores it
    CGEventSetIntegerValueField(event, kCGEventSourceUserData, CROSSKVM_USER_DATA_MARKER);
    CGEventPost(kCGHIDEventTap, event);
    CFRelease(event);
    return 0;
}

static EventTapSession* createEventTapSession(uintptr_t handle) {
    EventTapSession *session = (EventTapSession *)calloc(1, sizeof(EventTapSession));
    if (!session) return NULL;
    session->handle = handle;
    session->suppressed = false;

    CGEventMask mask = (EVENT_MASK_BIT(kCGEventLeftMouseDown) |
                        EVENT_MASK_BIT(kCGEventLeftMouseUp) |
                        EVENT_MASK_BIT(kCGEventRightMouseDown) |
                        EVENT_MASK_BIT(kCGEventRightMouseUp) |
                        EVENT_MASK_BIT(kCGEventMouseMoved) |
                        EVENT_MASK_BIT(kCGEventLeftMouseDragged) |
                        EVENT_MASK_BIT(kCGEventRightMouseDragged) |
                        EVENT_MASK_BIT(kCGEventKeyDown) |
                        EVENT_MASK_BIT(kCGEventKeyUp) |
                        EVENT_MASK_BIT(kCGEventFlagsChanged) |
                        EVENT_MASK_BIT(kCGEventScrollWheel) |
                        EVENT_MASK_BIT(kCGEventOtherMouseDown) |
                        EVENT_MASK_BIT(kCGEventOtherMouseUp) |
                        EVENT_MASK_BIT(kCGEventOtherMouseDragged));

    session->machPort = CGEventTapCreate(
        kCGHIDEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionDefault,
        mask,
        darwinEventTapCallback,
        session
    );

    if (!session->machPort) {
        free(session);
        return NULL;
    }

    session->runLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, session->machPort, 0);
    if (!session->runLoopSource) {
        CFRelease(session->machPort);
        free(session);
        return NULL;
    }

    session->active = true;
    return session;
}

static void setSessionSuppressed(EventTapSession *session, bool suppressed) {
    if (!session) return;
    session->suppressed = suppressed;
}

static void runEventTapSession(EventTapSession *session) {
    if (!session) return;
    session->runLoop = CFRunLoopGetCurrent();
    CFRunLoopAddSource(session->runLoop, session->runLoopSource, kCFRunLoopCommonModes);
    CGEventTapEnable(session->machPort, true);
    CFRunLoopRun();
}

static void stopEventTapSession(EventTapSession *session) {
    if (!session) return;
    session->active = false;
    if (session->machPort) {
        CGEventTapEnable(session->machPort, false);
    }
    if (session->runLoop) {
        CFRunLoopStop(session->runLoop);
    }
    if (session->runLoop && session->runLoopSource) {
        CFRunLoopRemoveSource(session->runLoop, session->runLoopSource, kCFRunLoopCommonModes);
    }
}

static void freeEventTapSession(EventTapSession *session) {
    if (!session) return;
    if (session->runLoopSource) {
        CFRelease(session->runLoopSource);
        session->runLoopSource = NULL;
    }
    if (session->machPort) {
        CFRelease(session->machPort);
        session->machPort = NULL;
    }
    free(session);
}
*/
import "C"
import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrAccessibilityNotGranted = errors.New("CrossKVM requires Accessibility permission to inject and capture keyboard/mouse input (System Settings -> Privacy & Security -> Accessibility)")
	ErrCGEventCreationFailed   = errors.New("macos: failed to create CoreGraphics CGEvent")
)

var (
	darwinSessionsMu sync.RWMutex
	darwinSessions           = make(map[uintptr]*DarwinBackend)
	nextSessionID    uintptr = 1
)

//export goOnDarwinEvent
func goOnDarwinEvent(handle C.uintptr_t, eventType C.uint32_t, x C.double, y C.double, buttonNum C.int64_t, deltaY C.int64_t, deltaX C.int64_t, keyCode C.uint16_t, flags C.uint64_t, userData C.int64_t) {
	darwinSessionsMu.RLock()
	b, ok := darwinSessions[uintptr(handle)]
	darwinSessionsMu.RUnlock()
	if !ok || b == nil {
		return
	}

	b.handleCapturedEvent(
		uint32(eventType),
		float64(x),
		float64(y),
		int64(buttonNum),
		int64(deltaY),
		int64(deltaX),
		uint16(keyCode),
		uint64(flags),
		int64(userData),
	)
}

// NativeCaptureSession encapsulates an active CoreGraphics event tap session.
type NativeCaptureSession struct {
	mu        sync.Mutex
	session   *C.EventTapSession
	sessionID uintptr
	stopped   bool
}

// StartNativeCapture creates and starts a CGEventTap session in the background.
func StartNativeCapture(b *DarwinBackend) (*NativeCaptureSession, error) {
	darwinSessionsMu.Lock()
	sessionID := nextSessionID
	nextSessionID++
	darwinSessions[sessionID] = b
	darwinSessionsMu.Unlock()

	session := C.createEventTapSession(C.uintptr_t(sessionID))
	if session == nil {
		darwinSessionsMu.Lock()
		delete(darwinSessions, sessionID)
		darwinSessionsMu.Unlock()
		return nil, errors.New("macos: failed to create CGEventTap (check Accessibility permissions)")
	}

	capSession := &NativeCaptureSession{
		session:   session,
		sessionID: sessionID,
	}

	return capSession, nil
}

// Run executes the CFRunLoop on the caller's thread (which must be locked to OS thread).
func (s *NativeCaptureSession) Run(startedCh chan<- struct{}, doneCh chan<- struct{}) {
	defer close(doneCh)

	close(startedCh)
	C.runEventTapSession(s.session)

	s.mu.Lock()
	defer s.mu.Unlock()

	C.freeEventTapSession(s.session)
	s.session = nil

	darwinSessionsMu.Lock()
	delete(darwinSessions, s.sessionID)
	darwinSessionsMu.Unlock()
}

// Stop stops the CFRunLoop and disables the event tap.
func (s *NativeCaptureSession) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped || s.session == nil {
		return
	}
	s.stopped = true
	C.stopEventTapSession(s.session)
}

// SetSuppressed dynamically toggles whether the event tap consumes physical events.
func (s *NativeCaptureSession) SetSuppressed(suppressed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil {
		C.setSessionSuppressed(s.session, C.bool(suppressed))
	}
}

// CheckAccessibilityPermission returns true if CrossKVM has Accessibility permissions.
func CheckAccessibilityPermission(prompt bool) bool {
	return bool(C.isAccessibilityTrusted(C.bool(prompt)))
}

// GetNativeCursorLocation queries the current physical cursor location from CoreGraphics.
func GetNativeCursorLocation() DarwinPoint {
	pt := C.getCursorLocation()
	return DarwinPoint{
		X: float64(pt.x),
		Y: float64(pt.y),
	}
}

// WarpNativeCursor instantaneously repositions the physical OS cursor.
func WarpNativeCursor(x, y float64) {
	C.warpCursorPosition(C.double(x), C.double(y))
}

// SetNativeCursorHidden balances CoreGraphics cursor visibility and separates
// physical mouse motion from the local pointer while forwarding input.
func SetNativeCursorHidden(hidden bool) error {
	var result C.CGError
	if hidden {
		result = C.CGDisplayHideCursor(C.kCGNullDirectDisplay)
		if result == C.kCGErrorSuccess {
			result = C.CGAssociateMouseAndMouseCursorPosition(0)
			if result != C.kCGErrorSuccess {
				C.CGDisplayShowCursor(C.kCGNullDirectDisplay)
			}
		}
	} else {
		result = C.CGAssociateMouseAndMouseCursorPosition(1)
		showResult := C.CGDisplayShowCursor(C.kCGNullDirectDisplay)
		if result == C.kCGErrorSuccess {
			result = showResult
		}
	}
	if result != C.kCGErrorSuccess {
		return fmt.Errorf("macos: cursor visibility failed: CoreGraphics error %d", int(result))
	}
	return nil
}

// GetPrimaryDisplayBounds returns the bounding rectangle of the primary display.
func GetPrimaryDisplayBounds() DarwinRect {
	rect := C.getDisplayBounds()
	return DarwinRect{
		X:      float64(rect.origin.x),
		Y:      float64(rect.origin.y),
		Width:  float64(rect.size.width),
		Height: float64(rect.size.height),
	}
}

// PostNativeMouseEvent posts a mouse move/click/drag CGEvent to the macOS window server.
func PostNativeMouseEvent(eventType uint32, x, y float64, button uint32, flags uint64) error {
	res := C.postMouseEvent(C.uint32_t(eventType), C.double(x), C.double(y), C.uint32_t(button), C.uint64_t(flags))
	if res != 0 {
		return ErrCGEventCreationFailed
	}
	return nil
}

// PostNativeScrollWheelEvent posts a scroll wheel CGEvent to the macOS window server.
func PostNativeScrollWheelEvent(wheelDY, wheelDX int32) error {
	res := C.postScrollEvent(C.int32_t(wheelDY), C.int32_t(wheelDX))
	if res != 0 {
		return ErrCGEventCreationFailed
	}
	return nil
}

// PostNativeKeyboardEvent posts a keyboard key-down or key-up CGEvent to the macOS window server.
func PostNativeKeyboardEvent(keyCode uint16, keyDown bool, flags uint64) error {
	res := C.postKeyboardEvent(C.uint16_t(keyCode), C.bool(keyDown), C.uint64_t(flags))
	if res != 0 {
		return ErrCGEventCreationFailed
	}
	return nil
}

// PostDarwinSpec executes a translated DarwinEventSpec using the native CoreGraphics functions.
func PostDarwinSpec(spec *DarwinEventSpec) error {
	if spec == nil {
		return nil
	}
	switch spec.Kind {
	case DarwinEventKindMouse:
		if spec.Mouse == nil {
			return nil
		}
		return PostNativeMouseEvent(
			spec.Mouse.EventType,
			spec.Mouse.Location.X,
			spec.Mouse.Location.Y,
			spec.Mouse.MouseButton,
			spec.Mouse.Flags,
		)
	case DarwinEventKindKeyboard:
		if spec.Keyboard == nil {
			return nil
		}
		return PostNativeKeyboardEvent(
			spec.Keyboard.KeyCode,
			spec.Keyboard.KeyDown,
			spec.Keyboard.Flags,
		)
	case DarwinEventKindScroll:
		if spec.Scroll == nil {
			return nil
		}
		return PostNativeScrollWheelEvent(
			spec.Scroll.WheelDY,
			spec.Scroll.WheelDX,
		)
	default:
		return fmt.Errorf("macos: unknown event spec kind: %d", spec.Kind)
	}
}
