package input

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// EventType represents the type of input event.
type EventType string

const (
	EventTypeMouseMove      EventType = "mouse_move"
	EventTypeMouseButtonDown EventType = "mouse_button_down"
	EventTypeMouseButtonUp   EventType = "mouse_button_up"
	EventTypeMouseWheel      EventType = "mouse_wheel"
	EventTypeKeyDown         EventType = "key_down"
	EventTypeKeyUp           EventType = "key_up"
)

// MouseButton represents mouse buttons in a platform-neutral way.
type MouseButton string

const (
	MouseButtonNone   MouseButton = ""
	MouseButtonLeft   MouseButton = "left"
	MouseButtonRight  MouseButton = "right"
	MouseButtonMiddle MouseButton = "middle"
	MouseButton4      MouseButton = "button4"
	MouseButton5      MouseButton = "button5"
)

// Modifier key bitflags.
const (
	ModShift uint32 = 1 << 0
	ModCtrl  uint32 = 1 << 1
	ModAlt   uint32 = 1 << 2
	ModMeta  uint32 = 1 << 3 // Command on macOS, Windows Key on Windows
)

var (
	ErrInvalidEventType = errors.New("invalid event type")
	ErrInvalidButton    = errors.New("invalid mouse button")
	ErrEmptyKey         = errors.New("key cannot be empty for key event")
	ErrInvalidFormat    = errors.New("invalid event command format")
)

// InputEvent is a platform-neutral representation of user input.
type InputEvent struct {
	Type      EventType   `json:"type"`
	DX        int32       `json:"dx,omitempty"`
	DY        int32       `json:"dy,omitempty"`
	Button    MouseButton `json:"button,omitempty"`
	WheelDX   int32       `json:"wheel_dx,omitempty"`
	WheelDY   int32       `json:"wheel_dy,omitempty"`
	Key       string      `json:"key,omitempty"`
	Modifiers uint32      `json:"modifiers,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// NewMouseMoveEvent creates a relative mouse movement event.
func NewMouseMoveEvent(dx, dy int32) InputEvent {
	return InputEvent{
		Type:      EventTypeMouseMove,
		DX:        dx,
		DY:        dy,
		Timestamp: time.Now().UnixNano(),
	}
}

// NewMouseButtonDownEvent creates a mouse button down event.
func NewMouseButtonDownEvent(button MouseButton) InputEvent {
	return InputEvent{
		Type:      EventTypeMouseButtonDown,
		Button:    button,
		Timestamp: time.Now().UnixNano(),
	}
}

// NewMouseButtonUpEvent creates a mouse button up event.
func NewMouseButtonUpEvent(button MouseButton) InputEvent {
	return InputEvent{
		Type:      EventTypeMouseButtonUp,
		Button:    button,
		Timestamp: time.Now().UnixNano(),
	}
}

// NewMouseWheelEvent creates a mouse wheel scroll event.
func NewMouseWheelEvent(wheelDX, wheelDY int32) InputEvent {
	return InputEvent{
		Type:      EventTypeMouseWheel,
		WheelDX:   wheelDX,
		WheelDY:   wheelDY,
		Timestamp: time.Now().UnixNano(),
	}
}

// NewKeyDownEvent creates a keyboard key down event.
func NewKeyDownEvent(key string, modifiers uint32) InputEvent {
	return InputEvent{
		Type:      EventTypeKeyDown,
		Key:       key,
		Modifiers: modifiers,
		Timestamp: time.Now().UnixNano(),
	}
}

// NewKeyUpEvent creates a keyboard key up event.
func NewKeyUpEvent(key string, modifiers uint32) InputEvent {
	return InputEvent{
		Type:      EventTypeKeyUp,
		Key:       key,
		Modifiers: modifiers,
		Timestamp: time.Now().UnixNano(),
	}
}

// Validate validates that the event contains necessary and sound fields.
func (e InputEvent) Validate() error {
	switch e.Type {
	case EventTypeMouseMove:
		return nil
	case EventTypeMouseButtonDown, EventTypeMouseButtonUp:
		if e.Button == "" {
			return ErrInvalidButton
		}
		return nil
	case EventTypeMouseWheel:
		return nil
	case EventTypeKeyDown, EventTypeKeyUp:
		if strings.TrimSpace(e.Key) == "" {
			return ErrEmptyKey
		}
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidEventType, e.Type)
	}
}

// String returns a human-readable representation of the input event.
func (e InputEvent) String() string {
	var sb strings.Builder
	sb.WriteString(string(e.Type))

	switch e.Type {
	case EventTypeMouseMove:
		fmt.Fprintf(&sb, " dx=%d dy=%d", e.DX, e.DY)
	case EventTypeMouseButtonDown, EventTypeMouseButtonUp:
		fmt.Fprintf(&sb, " button=%s", e.Button)
	case EventTypeMouseWheel:
		fmt.Fprintf(&sb, " wheelDX=%d wheelDY=%d", e.WheelDX, e.WheelDY)
	case EventTypeKeyDown, EventTypeKeyUp:
		fmt.Fprintf(&sb, " key=%s", e.Key)
		if e.Modifiers != 0 {
			var mods []string
			if e.Modifiers&ModShift != 0 {
				mods = append(mods, "Shift")
			}
			if e.Modifiers&ModCtrl != 0 {
				mods = append(mods, "Ctrl")
			}
			if e.Modifiers&ModAlt != 0 {
				mods = append(mods, "Alt")
			}
			if e.Modifiers&ModMeta != 0 {
				mods = append(mods, "Meta")
			}
			fmt.Fprintf(&sb, " modifiers=[%s]", strings.Join(mods, "+"))
		}
	}

	return sb.String()
}

// ParseDebugEvent parses human-readable debug commands into an InputEvent.
// Supported examples:
//   - "mousemove dx=10 dy=-3" or "mousemove 10 -3"
//   - "keydown key=A" or "keydown A"
//   - "keyup key=A" or "keyup A"
//   - "mousedown button=left" or "mousedown left"
//   - "mouseup button=left" or "mouseup left"
//   - "mousewheel dx=0 dy=120" or "mousewheel 0 120"
func ParseDebugEvent(line string) (InputEvent, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return InputEvent{}, fmt.Errorf("%w: empty input", ErrInvalidFormat)
	}

	tokens := strings.Fields(trimmed)
	cmd := strings.ToLower(tokens[0])

	switch cmd {
	case "mousemove", "mm":
		var dx, dy int64
		var err error
		if len(tokens) == 2 {
			return InputEvent{}, fmt.Errorf("%w: mousemove requires dx and dy", ErrInvalidFormat)
		}
		if len(tokens) >= 3 {
			// check for key=value or positional
			dx, err = parseKeyOrPos(tokens[1], "dx")
			if err != nil {
				return InputEvent{}, err
			}
			dy, err = parseKeyOrPos(tokens[2], "dy")
			if err != nil {
				return InputEvent{}, err
			}
		}
		return NewMouseMoveEvent(int32(dx), int32(dy)), nil

	case "keydown", "kd":
		if len(tokens) < 2 {
			return InputEvent{}, fmt.Errorf("%w: keydown requires key", ErrInvalidFormat)
		}
		key := extractKeyValue(tokens[1], "key")
		var mods uint32
		if len(tokens) > 2 {
			mods = parseModifiers(tokens[2:])
		}
		return NewKeyDownEvent(key, mods), nil

	case "keyup", "ku":
		if len(tokens) < 2 {
			return InputEvent{}, fmt.Errorf("%w: keyup requires key", ErrInvalidFormat)
		}
		key := extractKeyValue(tokens[1], "key")
		var mods uint32
		if len(tokens) > 2 {
			mods = parseModifiers(tokens[2:])
		}
		return NewKeyUpEvent(key, mods), nil

	case "mousedown", "mouse_button_down", "md":
		btn := MouseButtonLeft
		if len(tokens) >= 2 {
			btn = MouseButton(strings.ToLower(extractKeyValue(tokens[1], "button")))
		}
		return NewMouseButtonDownEvent(btn), nil

	case "mouseup", "mouse_button_up", "mu":
		btn := MouseButtonLeft
		if len(tokens) >= 2 {
			btn = MouseButton(strings.ToLower(extractKeyValue(tokens[1], "button")))
		}
		return NewMouseButtonUpEvent(btn), nil

	case "mousewheel", "wheel", "mw":
		var dx, dy int64
		var err error
		if len(tokens) >= 2 {
			if len(tokens) == 2 {
				dy, err = parseKeyOrPos(tokens[1], "dy")
				if err != nil {
					return InputEvent{}, err
				}
			} else {
				dx, err = parseKeyOrPos(tokens[1], "dx")
				if err != nil {
					return InputEvent{}, err
				}
				dy, err = parseKeyOrPos(tokens[2], "dy")
				if err != nil {
					return InputEvent{}, err
				}
			}
		}
		return NewMouseWheelEvent(int32(dx), int32(dy)), nil

	default:
		return InputEvent{}, fmt.Errorf("%w: unknown command '%s'", ErrInvalidFormat, cmd)
	}
}

func parseKeyOrPos(token, expectedKey string) (int64, error) {
	val := extractKeyValue(token, expectedKey)
	num, err := strconv.ParseInt(val, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid integer '%s' for %s", ErrInvalidFormat, val, expectedKey)
	}
	return num, nil
}

func extractKeyValue(token, expectedKey string) string {
	parts := strings.SplitN(token, "=", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], expectedKey) {
		return parts[1]
	}
	return token
}

func parseModifiers(tokens []string) uint32 {
	var mods uint32
	for _, tok := range tokens {
		val := extractKeyValue(tok, "mod")
		val = extractKeyValue(val, "modifiers")
		subparts := strings.Split(val, "+")
		for _, part := range subparts {
			switch strings.ToLower(strings.TrimSpace(part)) {
			case "shift":
				mods |= ModShift
			case "ctrl", "control":
				mods |= ModCtrl
			case "alt", "opt", "option":
				mods |= ModAlt
			case "meta", "cmd", "command", "win", "super":
				mods |= ModMeta
			}
		}
	}
	return mods
}
