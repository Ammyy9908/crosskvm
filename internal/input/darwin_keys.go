package input

import (
	"fmt"
	"strings"
)

// macOS CGKeyCode constants based on Carbon / HIToolbox / Events.h
const (
	kVK_ANSI_A              uint16 = 0x00
	kVK_ANSI_S              uint16 = 0x01
	kVK_ANSI_D              uint16 = 0x02
	kVK_ANSI_F              uint16 = 0x03
	kVK_ANSI_H              uint16 = 0x04
	kVK_ANSI_G              uint16 = 0x05
	kVK_ANSI_Z              uint16 = 0x06
	kVK_ANSI_X              uint16 = 0x07
	kVK_ANSI_C              uint16 = 0x08
	kVK_ANSI_V              uint16 = 0x09
	kVK_ANSI_B              uint16 = 0x0B
	kVK_ANSI_Q              uint16 = 0x0C
	kVK_ANSI_W              uint16 = 0x0D
	kVK_ANSI_E              uint16 = 0x0E
	kVK_ANSI_R              uint16 = 0x0F
	kVK_ANSI_Y              uint16 = 0x10
	kVK_ANSI_T              uint16 = 0x11
	kVK_ANSI_1              uint16 = 0x12
	kVK_ANSI_2              uint16 = 0x13
	kVK_ANSI_3              uint16 = 0x14
	kVK_ANSI_4              uint16 = 0x15
	kVK_ANSI_6              uint16 = 0x16
	kVK_ANSI_5              uint16 = 0x17
	kVK_ANSI_Equal          uint16 = 0x18
	kVK_ANSI_9              uint16 = 0x19
	kVK_ANSI_7              uint16 = 0x1A
	kVK_ANSI_Minus          uint16 = 0x1B
	kVK_ANSI_8              uint16 = 0x1C
	kVK_ANSI_0              uint16 = 0x1D
	kVK_ANSI_RightBracket   uint16 = 0x1E
	kVK_ANSI_O              uint16 = 0x1F
	kVK_ANSI_U              uint16 = 0x20
	kVK_ANSI_LeftBracket    uint16 = 0x21
	kVK_ANSI_I              uint16 = 0x22
	kVK_ANSI_P              uint16 = 0x23
	kVK_Return              uint16 = 0x24
	kVK_ANSI_L              uint16 = 0x25
	kVK_ANSI_J              uint16 = 0x26
	kVK_ANSI_Quote          uint16 = 0x27
	kVK_ANSI_K              uint16 = 0x28
	kVK_ANSI_Semicolon      uint16 = 0x29
	kVK_ANSI_Backslash      uint16 = 0x2A
	kVK_ANSI_Comma          uint16 = 0x2B
	kVK_ANSI_Slash          uint16 = 0x2C
	kVK_ANSI_N              uint16 = 0x2D
	kVK_ANSI_M              uint16 = 0x2E
	kVK_ANSI_Period         uint16 = 0x2F
	kVK_Tab                 uint16 = 0x30
	kVK_Space               uint16 = 0x31
	kVK_ANSI_Grave          uint16 = 0x32
	kVK_Delete              uint16 = 0x33 // Backspace on Mac
	kVK_Escape              uint16 = 0x35
	kVK_RightCommand        uint16 = 0x36
	kVK_Command             uint16 = 0x37
	kVK_Shift               uint16 = 0x38
	kVK_CapsLock            uint16 = 0x39
	kVK_Option              uint16 = 0x3A
	kVK_Control             uint16 = 0x3B
	kVK_RightShift          uint16 = 0x3C
	kVK_RightOption         uint16 = 0x3D
	kVK_RightControl        uint16 = 0x3E
	kVK_Function            uint16 = 0x3F
	kVK_F17                 uint16 = 0x40
	kVK_ANSI_KeypadDecimal  uint16 = 0x41
	kVK_ANSI_KeypadMultiply uint16 = 0x43
	kVK_ANSI_KeypadPlus     uint16 = 0x45
	kVK_ANSI_KeypadClear    uint16 = 0x47
	kVK_VolumeUp            uint16 = 0x48
	kVK_VolumeDown          uint16 = 0x49
	kVK_Mute                uint16 = 0x4A
	kVK_ANSI_KeypadDivide   uint16 = 0x4B
	kVK_ANSI_KeypadEnter    uint16 = 0x4C
	kVK_ANSI_KeypadMinus    uint16 = 0x4E
	kVK_F18                 uint16 = 0x4F
	kVK_F19                 uint16 = 0x50
	kVK_ANSI_KeypadEquals   uint16 = 0x51
	kVK_ANSI_Keypad0        uint16 = 0x52
	kVK_ANSI_Keypad1        uint16 = 0x53
	kVK_ANSI_Keypad2        uint16 = 0x54
	kVK_ANSI_Keypad3        uint16 = 0x55
	kVK_ANSI_Keypad4        uint16 = 0x56
	kVK_ANSI_Keypad5        uint16 = 0x57
	kVK_ANSI_Keypad6        uint16 = 0x58
	kVK_ANSI_Keypad7        uint16 = 0x59
	kVK_F20                 uint16 = 0x5A
	kVK_ANSI_Keypad8        uint16 = 0x5B
	kVK_ANSI_Keypad9        uint16 = 0x5C
	kVK_F5                  uint16 = 0x60
	kVK_F6                  uint16 = 0x61
	kVK_F7                  uint16 = 0x62
	kVK_F3                  uint16 = 0x63
	kVK_F8                  uint16 = 0x64
	kVK_F9                  uint16 = 0x65
	kVK_F11                 uint16 = 0x67
	kVK_F13                 uint16 = 0x69
	kVK_F16                 uint16 = 0x6A
	kVK_F14                 uint16 = 0x6B
	kVK_F10                 uint16 = 0x6D
	kVK_F12                 uint16 = 0x6F
	kVK_F15                 uint16 = 0x71
	kVK_Help                uint16 = 0x72
	kVK_Home                uint16 = 0x73
	kVK_PageUp              uint16 = 0x74
	kVK_ForwardDelete       uint16 = 0x75
	kVK_F4                  uint16 = 0x76
	kVK_End                 uint16 = 0x77
	kVK_F2                  uint16 = 0x78
	kVK_PageDown            uint16 = 0x79
	kVK_F1                  uint16 = 0x7A
	kVK_LeftArrow           uint16 = 0x7B
	kVK_RightArrow          uint16 = 0x7C
	kVK_DownArrow           uint16 = 0x7D
	kVK_UpArrow             uint16 = 0x7E
)

// CGEventFlags bitmasks
const (
	CGEventFlagMaskAlphaShift uint64 = 0x00010000 // CapsLock
	CGEventFlagMaskShift      uint64 = 0x00020000 // Shift
	CGEventFlagMaskControl    uint64 = 0x00040000 // Control
	CGEventFlagMaskAlternate  uint64 = 0x00080000 // Option / Alt
	CGEventFlagMaskCommand    uint64 = 0x00100000 // Command / Meta
	CGEventFlagMaskNumericPad uint64 = 0x00200000 // Keypad
	CGEventFlagMaskSecondaryFn uint64 = 0x00800000 // Fn
)

// ModifierType identifies which modifier flag a key modifies.
type ModifierType uint8

const (
	ModNone ModifierType = iota
	ModTypeShift
	ModTypeControl
	ModTypeAlternate
	ModTypeCommand
)

// DarwinKeyMapping maps a logical key to a macOS CGKeyCode and modifier classification.
type DarwinKeyMapping struct {
	KeyCode  uint16
	Modifier ModifierType
}

var darwinKeyTable = map[string]DarwinKeyMapping{
	// Letters
	"a": {KeyCode: kVK_ANSI_A},
	"b": {KeyCode: kVK_ANSI_B},
	"c": {KeyCode: kVK_ANSI_C},
	"d": {KeyCode: kVK_ANSI_D},
	"e": {KeyCode: kVK_ANSI_E},
	"f": {KeyCode: kVK_ANSI_F},
	"g": {KeyCode: kVK_ANSI_G},
	"h": {KeyCode: kVK_ANSI_H},
	"i": {KeyCode: kVK_ANSI_I},
	"j": {KeyCode: kVK_ANSI_J},
	"k": {KeyCode: kVK_ANSI_K},
	"l": {KeyCode: kVK_ANSI_L},
	"m": {KeyCode: kVK_ANSI_M},
	"n": {KeyCode: kVK_ANSI_N},
	"o": {KeyCode: kVK_ANSI_O},
	"p": {KeyCode: kVK_ANSI_P},
	"q": {KeyCode: kVK_ANSI_Q},
	"r": {KeyCode: kVK_ANSI_R},
	"s": {KeyCode: kVK_ANSI_S},
	"t": {KeyCode: kVK_ANSI_T},
	"u": {KeyCode: kVK_ANSI_U},
	"v": {KeyCode: kVK_ANSI_V},
	"w": {KeyCode: kVK_ANSI_W},
	"x": {KeyCode: kVK_ANSI_X},
	"y": {KeyCode: kVK_ANSI_Y},
	"z": {KeyCode: kVK_ANSI_Z},

	// Numbers
	"0": {KeyCode: kVK_ANSI_0},
	"1": {KeyCode: kVK_ANSI_1},
	"2": {KeyCode: kVK_ANSI_2},
	"3": {KeyCode: kVK_ANSI_3},
	"4": {KeyCode: kVK_ANSI_4},
	"5": {KeyCode: kVK_ANSI_5},
	"6": {KeyCode: kVK_ANSI_6},
	"7": {KeyCode: kVK_ANSI_7},
	"8": {KeyCode: kVK_ANSI_8},
	"9": {KeyCode: kVK_ANSI_9},

	// Functional & Whitespace
	"enter":     {KeyCode: kVK_Return},
	"return":    {KeyCode: kVK_Return},
	"space":     {KeyCode: kVK_Space},
	" ":         {KeyCode: kVK_Space},
	"tab":       {KeyCode: kVK_Tab},
	"backspace": {KeyCode: kVK_Delete},
	"delete":    {KeyCode: kVK_ForwardDelete},
	"del":       {KeyCode: kVK_ForwardDelete},
	"escape":    {KeyCode: kVK_Escape},
	"esc":       {KeyCode: kVK_Escape},
	"capslock":  {KeyCode: kVK_CapsLock},
	"caps":      {KeyCode: kVK_CapsLock},

	// Navigation
	"arrowup":    {KeyCode: kVK_UpArrow},
	"up":         {KeyCode: kVK_UpArrow},
	"arrowdown":  {KeyCode: kVK_DownArrow},
	"down":       {KeyCode: kVK_DownArrow},
	"arrowleft":  {KeyCode: kVK_LeftArrow},
	"left":       {KeyCode: kVK_LeftArrow},
	"arrowright": {KeyCode: kVK_RightArrow},
	"right":      {KeyCode: kVK_RightArrow},
	"home":       {KeyCode: kVK_Home},
	"end":        {KeyCode: kVK_End},
	"pageup":     {KeyCode: kVK_PageUp},
	"pgup":       {KeyCode: kVK_PageUp},
	"pagedown":   {KeyCode: kVK_PageDown},
	"pgdn":       {KeyCode: kVK_PageDown},
	"insert":     {KeyCode: kVK_Help},
	"help":       {KeyCode: kVK_Help},

	// Modifiers (with Modifier flag classification)
	"shift":        {KeyCode: kVK_Shift, Modifier: ModTypeShift},
	"shiftleft":    {KeyCode: kVK_Shift, Modifier: ModTypeShift},
	"leftshift":    {KeyCode: kVK_Shift, Modifier: ModTypeShift},
	"shift_l":      {KeyCode: kVK_Shift, Modifier: ModTypeShift},
	"shiftright":   {KeyCode: kVK_RightShift, Modifier: ModTypeShift},
	"rightshift":   {KeyCode: kVK_RightShift, Modifier: ModTypeShift},
	"shift_r":      {KeyCode: kVK_RightShift, Modifier: ModTypeShift},
	"control":      {KeyCode: kVK_Control, Modifier: ModTypeControl},
	"ctrl":         {KeyCode: kVK_Control, Modifier: ModTypeControl},
	"controlleft":  {KeyCode: kVK_Control, Modifier: ModTypeControl},
	"ctrlleft":     {KeyCode: kVK_Control, Modifier: ModTypeControl},
	"leftctrl":     {KeyCode: kVK_Control, Modifier: ModTypeControl},
	"control_l":    {KeyCode: kVK_Control, Modifier: ModTypeControl},
	"controlright": {KeyCode: kVK_RightControl, Modifier: ModTypeControl},
	"ctrlright":    {KeyCode: kVK_RightControl, Modifier: ModTypeControl},
	"rightctrl":    {KeyCode: kVK_RightControl, Modifier: ModTypeControl},
	"control_r":    {KeyCode: kVK_RightControl, Modifier: ModTypeControl},
	"alt":          {KeyCode: kVK_Option, Modifier: ModTypeAlternate},
	"opt":          {KeyCode: kVK_Option, Modifier: ModTypeAlternate},
	"option":       {KeyCode: kVK_Option, Modifier: ModTypeAlternate},
	"altleft":      {KeyCode: kVK_Option, Modifier: ModTypeAlternate},
	"optleft":      {KeyCode: kVK_Option, Modifier: ModTypeAlternate},
	"optionleft":   {KeyCode: kVK_Option, Modifier: ModTypeAlternate},
	"leftalt":      {KeyCode: kVK_Option, Modifier: ModTypeAlternate},
	"alt_l":        {KeyCode: kVK_Option, Modifier: ModTypeAlternate},
	"altright":     {KeyCode: kVK_RightOption, Modifier: ModTypeAlternate},
	"optright":     {KeyCode: kVK_RightOption, Modifier: ModTypeAlternate},
	"optionright":  {KeyCode: kVK_RightOption, Modifier: ModTypeAlternate},
	"rightalt":     {KeyCode: kVK_RightOption, Modifier: ModTypeAlternate},
	"alt_r":        {KeyCode: kVK_RightOption, Modifier: ModTypeAlternate},
	"meta":         {KeyCode: kVK_Command, Modifier: ModTypeCommand},
	"cmd":          {KeyCode: kVK_Command, Modifier: ModTypeCommand},
	"command":      {KeyCode: kVK_Command, Modifier: ModTypeCommand},
	"win":          {KeyCode: kVK_Command, Modifier: ModTypeCommand},
	"super":        {KeyCode: kVK_Command, Modifier: ModTypeCommand},
	"metaleft":     {KeyCode: kVK_Command, Modifier: ModTypeCommand},
	"cmdleft":      {KeyCode: kVK_Command, Modifier: ModTypeCommand},
	"commandleft":  {KeyCode: kVK_Command, Modifier: ModTypeCommand},
	"winleft":      {KeyCode: kVK_Command, Modifier: ModTypeCommand},
	"superleft":    {KeyCode: kVK_Command, Modifier: ModTypeCommand},
	"metaright":    {KeyCode: kVK_RightCommand, Modifier: ModTypeCommand},
	"cmdright":     {KeyCode: kVK_RightCommand, Modifier: ModTypeCommand},
	"commandright": {KeyCode: kVK_RightCommand, Modifier: ModTypeCommand},
	"winright":     {KeyCode: kVK_RightCommand, Modifier: ModTypeCommand},
	"superright":   {KeyCode: kVK_RightCommand, Modifier: ModTypeCommand},
	"fn":           {KeyCode: kVK_Function},
	"function":     {KeyCode: kVK_Function},

	// Function Keys
	"f1":  {KeyCode: kVK_F1},
	"f2":  {KeyCode: kVK_F2},
	"f3":  {KeyCode: kVK_F3},
	"f4":  {KeyCode: kVK_F4},
	"f5":  {KeyCode: kVK_F5},
	"f6":  {KeyCode: kVK_F6},
	"f7":  {KeyCode: kVK_F7},
	"f8":  {KeyCode: kVK_F8},
	"f9":  {KeyCode: kVK_F9},
	"f10": {KeyCode: kVK_F10},
	"f11": {KeyCode: kVK_F11},
	"f12": {KeyCode: kVK_F12},
	"f13": {KeyCode: kVK_F13},
	"f14": {KeyCode: kVK_F14},
	"f15": {KeyCode: kVK_F15},
	"f16": {KeyCode: kVK_F16},
	"f17": {KeyCode: kVK_F17},
	"f18": {KeyCode: kVK_F18},
	"f19": {KeyCode: kVK_F19},
	"f20": {KeyCode: kVK_F20},

	// Punctuation & Symbols
	"`":  {KeyCode: kVK_ANSI_Grave},
	"~":  {KeyCode: kVK_ANSI_Grave},
	"-":  {KeyCode: kVK_ANSI_Minus},
	"_":  {KeyCode: kVK_ANSI_Minus},
	"=":  {KeyCode: kVK_ANSI_Equal},
	"+":  {KeyCode: kVK_ANSI_Equal},
	"[":  {KeyCode: kVK_ANSI_LeftBracket},
	"{":  {KeyCode: kVK_ANSI_LeftBracket},
	"]":  {KeyCode: kVK_ANSI_RightBracket},
	"}":  {KeyCode: kVK_ANSI_RightBracket},
	"\\": {KeyCode: kVK_ANSI_Backslash},
	"|":  {KeyCode: kVK_ANSI_Backslash},
	";":  {KeyCode: kVK_ANSI_Semicolon},
	":":  {KeyCode: kVK_ANSI_Semicolon},
	"'":  {KeyCode: kVK_ANSI_Quote},
	"\"": {KeyCode: kVK_ANSI_Quote},
	",":  {KeyCode: kVK_ANSI_Comma},
	"<":  {KeyCode: kVK_ANSI_Comma},
	".":  {KeyCode: kVK_ANSI_Period},
	">":  {KeyCode: kVK_ANSI_Period},
	"/":  {KeyCode: kVK_ANSI_Slash},
	"?":  {KeyCode: kVK_ANSI_Slash},

	// Keypad
	"numpad0":        {KeyCode: kVK_ANSI_Keypad0},
	"numpad1":        {KeyCode: kVK_ANSI_Keypad1},
	"numpad2":        {KeyCode: kVK_ANSI_Keypad2},
	"numpad3":        {KeyCode: kVK_ANSI_Keypad3},
	"numpad4":        {KeyCode: kVK_ANSI_Keypad4},
	"numpad5":        {KeyCode: kVK_ANSI_Keypad5},
	"numpad6":        {KeyCode: kVK_ANSI_Keypad6},
	"numpad7":        {KeyCode: kVK_ANSI_Keypad7},
	"numpad8":        {KeyCode: kVK_ANSI_Keypad8},
	"numpad9":        {KeyCode: kVK_ANSI_Keypad9},
	"numpaddecimal":  {KeyCode: kVK_ANSI_KeypadDecimal},
	"numpadplus":     {KeyCode: kVK_ANSI_KeypadPlus},
	"numpadadd":      {KeyCode: kVK_ANSI_KeypadPlus},
	"numpadminus":    {KeyCode: kVK_ANSI_KeypadMinus},
	"numpadsubtract": {KeyCode: kVK_ANSI_KeypadMinus},
	"numpadmultiply": {KeyCode: kVK_ANSI_KeypadMultiply},
	"numpaddivide":   {KeyCode: kVK_ANSI_KeypadDivide},
	"numpadenter":    {KeyCode: kVK_ANSI_KeypadEnter},
	"numpadequals":   {KeyCode: kVK_ANSI_KeypadEquals},
}

// LookupDarwinKey maps a platform-neutral key name to a macOS CGKeyCode and modifier classification.
func LookupDarwinKey(key string) (DarwinKeyMapping, error) {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if mapping, found := darwinKeyTable[normalized]; found {
		return mapping, nil
	}

	if len(normalized) == 1 {
		char := normalized[0]
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			if m, ok := darwinKeyTable[string(char)]; ok {
				return m, nil
			}
		}
	}

	return DarwinKeyMapping{}, fmt.Errorf("macos: unsupported or unknown key '%s'", key)
}

// Reverse mapping from macOS CGKeyCode to platform-neutral key name and modifier type.
var darwinReverseKeyTable = map[uint16]struct {
	Name     string
	Modifier ModifierType
}{
	// Letters
	kVK_ANSI_A: {Name: "A"},
	kVK_ANSI_B: {Name: "B"},
	kVK_ANSI_C: {Name: "C"},
	kVK_ANSI_D: {Name: "D"},
	kVK_ANSI_E: {Name: "E"},
	kVK_ANSI_F: {Name: "F"},
	kVK_ANSI_G: {Name: "G"},
	kVK_ANSI_H: {Name: "H"},
	kVK_ANSI_I: {Name: "I"},
	kVK_ANSI_J: {Name: "J"},
	kVK_ANSI_K: {Name: "K"},
	kVK_ANSI_L: {Name: "L"},
	kVK_ANSI_M: {Name: "M"},
	kVK_ANSI_N: {Name: "N"},
	kVK_ANSI_O: {Name: "O"},
	kVK_ANSI_P: {Name: "P"},
	kVK_ANSI_Q: {Name: "Q"},
	kVK_ANSI_R: {Name: "R"},
	kVK_ANSI_S: {Name: "S"},
	kVK_ANSI_T: {Name: "T"},
	kVK_ANSI_U: {Name: "U"},
	kVK_ANSI_V: {Name: "V"},
	kVK_ANSI_W: {Name: "W"},
	kVK_ANSI_X: {Name: "X"},
	kVK_ANSI_Y: {Name: "Y"},
	kVK_ANSI_Z: {Name: "Z"},

	// Numbers
	kVK_ANSI_0: {Name: "0"},
	kVK_ANSI_1: {Name: "1"},
	kVK_ANSI_2: {Name: "2"},
	kVK_ANSI_3: {Name: "3"},
	kVK_ANSI_4: {Name: "4"},
	kVK_ANSI_5: {Name: "5"},
	kVK_ANSI_6: {Name: "6"},
	kVK_ANSI_7: {Name: "7"},
	kVK_ANSI_8: {Name: "8"},
	kVK_ANSI_9: {Name: "9"},

	// Functional & Whitespace
	kVK_Return:        {Name: "Enter"},
	kVK_Tab:           {Name: "Tab"},
	kVK_Space:         {Name: "Space"},
	kVK_Delete:        {Name: "Backspace"},
	kVK_ForwardDelete: {Name: "Delete"},
	kVK_Escape:        {Name: "Escape"},
	kVK_CapsLock:      {Name: "CapsLock"},

	// Navigation
	kVK_UpArrow:   {Name: "ArrowUp"},
	kVK_DownArrow: {Name: "ArrowDown"},
	kVK_LeftArrow: {Name: "ArrowLeft"},
	kVK_RightArrow:{Name: "ArrowRight"},
	kVK_Home:      {Name: "Home"},
	kVK_End:       {Name: "End"},
	kVK_PageUp:    {Name: "PageUp"},
	kVK_PageDown:  {Name: "PageDown"},
	kVK_Help:      {Name: "Insert"},

	// Modifiers
	kVK_Shift:        {Name: "ShiftLeft", Modifier: ModTypeShift},
	kVK_RightShift:   {Name: "ShiftRight", Modifier: ModTypeShift},
	kVK_Control:      {Name: "ControlLeft", Modifier: ModTypeControl},
	kVK_RightControl: {Name: "ControlRight", Modifier: ModTypeControl},
	kVK_Option:       {Name: "AltLeft", Modifier: ModTypeAlternate},
	kVK_RightOption:  {Name: "AltRight", Modifier: ModTypeAlternate},
	kVK_Command:      {Name: "MetaLeft", Modifier: ModTypeCommand},
	kVK_RightCommand: {Name: "MetaRight", Modifier: ModTypeCommand},
	kVK_Function:     {Name: "Function"},

	// Function Keys
	kVK_F1:  {Name: "F1"},
	kVK_F2:  {Name: "F2"},
	kVK_F3:  {Name: "F3"},
	kVK_F4:  {Name: "F4"},
	kVK_F5:  {Name: "F5"},
	kVK_F6:  {Name: "F6"},
	kVK_F7:  {Name: "F7"},
	kVK_F8:  {Name: "F8"},
	kVK_F9:  {Name: "F9"},
	kVK_F10: {Name: "F10"},
	kVK_F11: {Name: "F11"},
	kVK_F12: {Name: "F12"},
	kVK_F13: {Name: "F13"},
	kVK_F14: {Name: "F14"},
	kVK_F15: {Name: "F15"},
	kVK_F16: {Name: "F16"},
	kVK_F17: {Name: "F17"},
	kVK_F18: {Name: "F18"},
	kVK_F19: {Name: "F19"},
	kVK_F20: {Name: "F20"},

	// Punctuation & Symbols
	kVK_ANSI_Grave:        {Name: "`"},
	kVK_ANSI_Minus:        {Name: "-"},
	kVK_ANSI_Equal:        {Name: "="},
	kVK_ANSI_LeftBracket:  {Name: "["},
	kVK_ANSI_RightBracket: {Name: "]"},
	kVK_ANSI_Backslash:    {Name: "\\"},
	kVK_ANSI_Semicolon:    {Name: ";"},
	kVK_ANSI_Quote:        {Name: "'"},
	kVK_ANSI_Comma:        {Name: ","},
	kVK_ANSI_Period:       {Name: "."},
	kVK_ANSI_Slash:        {Name: "/"},

	// Keypad
	kVK_ANSI_Keypad0:        {Name: "Numpad0"},
	kVK_ANSI_Keypad1:        {Name: "Numpad1"},
	kVK_ANSI_Keypad2:        {Name: "Numpad2"},
	kVK_ANSI_Keypad3:        {Name: "Numpad3"},
	kVK_ANSI_Keypad4:        {Name: "Numpad4"},
	kVK_ANSI_Keypad5:        {Name: "Numpad5"},
	kVK_ANSI_Keypad6:        {Name: "Numpad6"},
	kVK_ANSI_Keypad7:        {Name: "Numpad7"},
	kVK_ANSI_Keypad8:        {Name: "Numpad8"},
	kVK_ANSI_Keypad9:        {Name: "Numpad9"},
	kVK_ANSI_KeypadDecimal:  {Name: "NumpadDecimal"},
	kVK_ANSI_KeypadPlus:     {Name: "NumpadAdd"},
	kVK_ANSI_KeypadMinus:    {Name: "NumpadSubtract"},
	kVK_ANSI_KeypadMultiply: {Name: "NumpadMultiply"},
	kVK_ANSI_KeypadDivide:   {Name: "NumpadDivide"},
	kVK_ANSI_KeypadEnter:    {Name: "NumpadEnter"},
	kVK_ANSI_KeypadEquals:   {Name: "NumpadEquals"},
}

// LookupDarwinKeyByCode maps a macOS CGKeyCode to a platform-neutral key name and modifier type.
func LookupDarwinKeyByCode(keyCode uint16) (string, ModifierType, bool) {
	entry, found := darwinReverseKeyTable[keyCode]
	if found {
		return entry.Name, entry.Modifier, true
	}
	return fmt.Sprintf("KeyCode_%d", keyCode), ModNone, false
}
