package input

import (
	"testing"
)

func TestInputEvents_Validate(t *testing.T) {
	tests := []struct {
		name    string
		event   InputEvent
		wantErr bool
	}{
		{
			name:    "valid mouse move",
			event:   NewMouseMoveEvent(10, -5),
			wantErr: false,
		},
		{
			name:    "valid mouse down",
			event:   NewMouseButtonDownEvent(MouseButtonLeft),
			wantErr: false,
		},
		{
			name:    "invalid mouse down - empty button",
			event:   InputEvent{Type: EventTypeMouseButtonDown},
			wantErr: true,
		},
		{
			name:    "valid key down",
			event:   NewKeyDownEvent("A", ModCtrl),
			wantErr: false,
		},
		{
			name:    "invalid key down - empty key",
			event:   NewKeyDownEvent("  ", 0),
			wantErr: true,
		},
		{
			name:    "valid mouse wheel",
			event:   NewMouseWheelEvent(0, 120),
			wantErr: false,
		},
		{
			name:    "invalid event type",
			event:   InputEvent{Type: "random_type"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseDebugEvent(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantType  EventType
		wantDX    int32
		wantDY    int32
		wantKey   string
		wantBtn   MouseButton
		wantMods  uint32
		wantErr   bool
	}{
		{
			name:     "mousemove with key=val",
			input:    "mousemove dx=10 dy=-3",
			wantType: EventTypeMouseMove,
			wantDX:   10,
			wantDY:   -3,
		},
		{
			name:     "mousemove positional",
			input:    "mousemove 25 -15",
			wantType: EventTypeMouseMove,
			wantDX:   25,
			wantDY:   -15,
		},
		{
			name:     "keydown key=A",
			input:    "keydown key=A",
			wantType: EventTypeKeyDown,
			wantKey:  "A",
		},
		{
			name:     "keyup key=A",
			input:    "keyup key=A",
			wantType: EventTypeKeyUp,
			wantKey:  "A",
		},
		{
			name:     "keydown with modifiers",
			input:    "keydown key=C mod=ctrl+shift",
			wantType: EventTypeKeyDown,
			wantKey:  "C",
			wantMods: ModCtrl | ModShift,
		},
		{
			name:     "mousedown left",
			input:    "mousedown button=left",
			wantType: EventTypeMouseButtonDown,
			wantBtn:  MouseButtonLeft,
		},
		{
			name:     "mouseup right",
			input:    "mouseup button=right",
			wantType: EventTypeMouseButtonUp,
			wantBtn:  MouseButtonRight,
		},
		{
			name:     "mousewheel dx=0 dy=120",
			input:    "mousewheel dx=0 dy=120",
			wantType: EventTypeMouseWheel,
			wantDX:   0,
			wantDY:   120, // in event it's WheelDX/WheelDY
		},
		{
			name:    "empty command",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "unknown command",
			input:   "foo bar",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, err := ParseDebugEvent(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDebugEvent(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if ev.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", ev.Type, tt.wantType)
			}
			if tt.wantType == EventTypeMouseMove {
				if ev.DX != tt.wantDX || ev.DY != tt.wantDY {
					t.Errorf("DX/DY = (%d, %d), want (%d, %d)", ev.DX, ev.DY, tt.wantDX, tt.wantDY)
				}
			}
			if tt.wantType == EventTypeMouseWheel {
				if ev.WheelDX != tt.wantDX || ev.WheelDY != tt.wantDY {
					t.Errorf("WheelDX/WheelDY = (%d, %d), want (%d, %d)", ev.WheelDX, ev.WheelDY, tt.wantDX, tt.wantDY)
				}
			}
			if tt.wantKey != "" && ev.Key != tt.wantKey {
				t.Errorf("Key = %s, want %s", ev.Key, tt.wantKey)
			}
			if tt.wantBtn != "" && ev.Button != tt.wantBtn {
				t.Errorf("Button = %s, want %s", ev.Button, tt.wantBtn)
			}
			if tt.wantMods != 0 && ev.Modifiers != tt.wantMods {
				t.Errorf("Modifiers = %d, want %d", ev.Modifiers, tt.wantMods)
			}
		})
	}
}
