package input

import "testing"

func TestCrossPlatformModifierChords(t *testing.T) {
	for _, direction := range [][2]string{{"windows", "darwin"}, {"darwin", "windows"}} {
		for _, kind := range []EventType{EventTypeKeyDown, EventTypeKeyUp} {
			for _, pair := range [][2]string{{"ControlLeft", "MetaLeft"}, {"ControlRight", "MetaRight"}, {"MetaLeft", "ControlLeft"}, {"MetaRight", "ControlRight"}} {
				e := InputEvent{Type: kind, Key: pair[0], Modifiers: ModCtrl | ModShift | ModAlt}
				got := TranslateModifiers(e, direction[0], direction[1])
				if got.Key != pair[1] || got.Modifiers != ModMeta|ModShift|ModAlt {
					t.Fatalf("wrong mapping: %+v", got)
				}
			}
			for _, key := range []string{"C", "V", "X", "A", "Z"} {
				e := InputEvent{Type: kind, Key: key, Modifiers: ModMeta}
				got := TranslateModifiers(e, "darwin", "windows")
				if got.Key != key || got.Modifiers != ModCtrl {
					t.Fatalf("wrong shortcut: %+v", got)
				}
			}
		}
	}
}
func TestModifierTranslationPreservesSamePlatformAndLegacy(t *testing.T) {
	e := InputEvent{Type: EventTypeKeyDown, Key: "ControlLeft", Modifiers: ModCtrl}
	for _, pair := range [][2]string{{"", "darwin"}, {"windows", "windows"}, {"darwin", "darwin"}, {"linux", "darwin"}} {
		if got := TranslateModifiers(e, pair[0], pair[1]); got != e {
			t.Fatal("unexpected translation")
		}
	}
	e.Modifiers = ModCtrl | ModMeta | ModShift
	if got := TranslateModifiers(e, "windows", "darwin"); got.Modifiers != e.Modifiers {
		t.Fatal("combined modifiers lost")
	}
}
