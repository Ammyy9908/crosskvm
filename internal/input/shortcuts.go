package input

import "strings"

// TranslateModifiers swaps the two primary modifier positions between macOS
// and Windows. Translate key up as well as key down, and the modifier mask, so
// native backends agree about which modifier is held throughout a chord.
// Missing source metadata (older peers) and same-platform input stay unchanged.
func TranslateModifiers(event InputEvent, sourceOS, targetOS string) InputEvent {
	if !((sourceOS == "windows" && targetOS == "darwin") || (sourceOS == "darwin" && targetOS == "windows")) {
		return event
	}
	ctrl, meta := event.Modifiers&ModCtrl != 0, event.Modifiers&ModMeta != 0
	event.Modifiers &^= ModCtrl | ModMeta
	if ctrl {
		event.Modifiers |= ModMeta
	}
	if meta {
		event.Modifiers |= ModCtrl
	}
	if event.Type != EventTypeKeyDown && event.Type != EventTypeKeyUp {
		return event
	}
	switch strings.ToLower(event.Key) {
	case "control", "ctrl", "controlleft", "ctrlleft", "leftctrl", "control_l":
		event.Key = "MetaLeft"
	case "controlright", "ctrlright", "rightctrl", "control_r":
		event.Key = "MetaRight"
	case "meta", "metaleft", "command", "cmd", "commandleft", "cmdleft", "super", "superleft", "win", "winleft":
		event.Key = "ControlLeft"
	case "metaright", "commandright", "cmdright", "superright", "winright":
		event.Key = "ControlRight"
	}
	return event
}
