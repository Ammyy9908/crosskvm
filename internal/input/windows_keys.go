package input

import (
	"fmt"
	"strings"
)

// Windows Virtual Key codes (VK_*)
const (
	vkLButton    = 0x01
	vkRButton    = 0x02
	vkCancel     = 0x03
	vkMButton    = 0x04
	vkXButton1   = 0x05
	vkXButton2   = 0x06
	vkBack       = 0x08
	vkTab        = 0x09
	vkClear      = 0x0C
	vkReturn     = 0x0D
	vkShift      = 0x10
	vkControl    = 0x11
	vkMenu       = 0x12 // Alt
	vkPause      = 0x13
	vkCapital    = 0x14 // Caps Lock
	vkEscape     = 0x1B
	vkSpace      = 0x20
	vkPrior      = 0x21 // Page Up
	vkNext       = 0x22 // Page Down
	vkEnd        = 0x23
	vkHome       = 0x24
	vkLeft       = 0x25
	vkUp         = 0x26
	vkRight      = 0x27
	vkDown       = 0x28
	vkSelect     = 0x29
	vkPrint      = 0x2A
	vkExecute    = 0x2B
	vkSnapshot   = 0x2C // Print Screen
	vkInsert     = 0x2D
	vkDelete     = 0x2E
	vkHelp       = 0x2F
	vkLWin       = 0x5B // Left Windows Key
	vkRWin       = 0x5C // Right Windows Key
	vkApps       = 0x5D // Applications / Context Menu
	vkSleep      = 0x5F
	vkNumpad0    = 0x60
	vkNumpad1    = 0x61
	vkNumpad2    = 0x62
	vkNumpad3    = 0x63
	vkNumpad4    = 0x64
	vkNumpad5    = 0x65
	vkNumpad6    = 0x66
	vkNumpad7    = 0x67
	vkNumpad8    = 0x68
	vkNumpad9    = 0x69
	vkMultiply   = 0x6A
	vkAdd        = 0x6B
	vkSeparator  = 0x6C
	vkSubtract   = 0x6D
	vkDecimal    = 0x6E
	vkDivide     = 0x6F
	vkF1         = 0x70
	vkF2         = 0x71
	vkF3         = 0x72
	vkF4         = 0x73
	vkF5         = 0x74
	vkF6         = 0x75
	vkF7         = 0x76
	vkF8         = 0x77
	vkF9         = 0x78
	vkF10        = 0x79
	vkF11        = 0x7A
	vkF12        = 0x7B
	vkF13        = 0x7C
	vkF14        = 0x7D
	vkF15        = 0x7E
	vkF16        = 0x7F
	vkF17        = 0x80
	vkF18        = 0x81
	vkF19        = 0x82
	vkF20        = 0x83
	vkF21        = 0x84
	vkF22        = 0x85
	vkF23        = 0x86
	vkF24        = 0x87
	vkNumLock    = 0x90
	vkScroll     = 0x91
	vkLShift     = 0xA0
	vkRShift     = 0xA1
	vkLControl   = 0xA2
	vkRControl   = 0xA3
	vkLMenu      = 0xA4 // Left Alt
	vkRMenu      = 0xA5 // Right Alt
	vkOem1       = 0xBA // ;:
	vkOemPlus    = 0xBB // =+
	vkOemComma   = 0xBC // ,<
	vkOemMinus   = 0xBD // -_
	vkOemPeriod  = 0xBE // .>
	vkOem2       = 0xBF // /?
	vkOem3       = 0xC0 // `~
	vkOem4       = 0xDB // [{
	vkOem5       = 0xDC // \|
	vkOem6       = 0xDD // ]}
	vkOem7       = 0xDE // '"
)

// KeyMapping holds the Windows Virtual Key code, PS/2 Set 1 Scan Code, and extended key flag.
type KeyMapping struct {
	VK         uint16
	ScanCode   uint16
	IsExtended bool
}

// keyTable maps platform-neutral key identifiers (case-insensitive) to Windows KeyMapping.
var keyTable = map[string]KeyMapping{
	// Letters
	"a": {VK: 'A', ScanCode: 0x1E},
	"b": {VK: 'B', ScanCode: 0x30},
	"c": {VK: 'C', ScanCode: 0x2E},
	"d": {VK: 'D', ScanCode: 0x20},
	"e": {VK: 'E', ScanCode: 0x12},
	"f": {VK: 'F', ScanCode: 0x21},
	"g": {VK: 'G', ScanCode: 0x22},
	"h": {VK: 'H', ScanCode: 0x23},
	"i": {VK: 'I', ScanCode: 0x17},
	"j": {VK: 'J', ScanCode: 0x24},
	"k": {VK: 'K', ScanCode: 0x25},
	"l": {VK: 'L', ScanCode: 0x26},
	"m": {VK: 'M', ScanCode: 0x32},
	"n": {VK: 'N', ScanCode: 0x31},
	"o": {VK: 'O', ScanCode: 0x18},
	"p": {VK: 'P', ScanCode: 0x19},
	"q": {VK: 'Q', ScanCode: 0x10},
	"r": {VK: 'R', ScanCode: 0x13},
	"s": {VK: 'S', ScanCode: 0x1F},
	"t": {VK: 'T', ScanCode: 0x14},
	"u": {VK: 'U', ScanCode: 0x16},
	"v": {VK: 'V', ScanCode: 0x2F},
	"w": {VK: 'W', ScanCode: 0x11},
	"x": {VK: 'X', ScanCode: 0x2D},
	"y": {VK: 'Y', ScanCode: 0x15},
	"z": {VK: 'Z', ScanCode: 0x2C},

	// Numbers
	"0": {VK: '0', ScanCode: 0x0B},
	"1": {VK: '1', ScanCode: 0x02},
	"2": {VK: '2', ScanCode: 0x03},
	"3": {VK: '3', ScanCode: 0x04},
	"4": {VK: '4', ScanCode: 0x05},
	"5": {VK: '5', ScanCode: 0x06},
	"6": {VK: '6', ScanCode: 0x07},
	"7": {VK: '7', ScanCode: 0x08},
	"8": {VK: '8', ScanCode: 0x09},
	"9": {VK: '9', ScanCode: 0x0A},

	// Common Functional Keys
	"enter":      {VK: vkReturn, ScanCode: 0x1C},
	"return":     {VK: vkReturn, ScanCode: 0x1C},
	"escape":     {VK: vkEscape, ScanCode: 0x01},
	"esc":        {VK: vkEscape, ScanCode: 0x01},
	"space":      {VK: vkSpace, ScanCode: 0x39},
	" ":          {VK: vkSpace, ScanCode: 0x39},
	"tab":        {VK: vkTab, ScanCode: 0x0F},
	"backspace":  {VK: vkBack, ScanCode: 0x0E},
	"bksp":       {VK: vkBack, ScanCode: 0x0E},
	"capslock":   {VK: vkCapital, ScanCode: 0x3A},
	"caps":       {VK: vkCapital, ScanCode: 0x3A},
	"numlock":    {VK: vkNumLock, ScanCode: 0x45, IsExtended: true},
	"scrolllock": {VK: vkScroll, ScanCode: 0x46},

	// Modifiers
	"shift":        {VK: vkLShift, ScanCode: 0x2A},
	"shiftleft":    {VK: vkLShift, ScanCode: 0x2A},
	"leftshift":    {VK: vkLShift, ScanCode: 0x2A},
	"shift_l":      {VK: vkLShift, ScanCode: 0x2A},
	"shiftright":   {VK: vkRShift, ScanCode: 0x36},
	"rightshift":   {VK: vkRShift, ScanCode: 0x36},
	"shift_r":      {VK: vkRShift, ScanCode: 0x36},
	"control":      {VK: vkLControl, ScanCode: 0x1D},
	"ctrl":         {VK: vkLControl, ScanCode: 0x1D},
	"controlleft":  {VK: vkLControl, ScanCode: 0x1D},
	"ctrlleft":     {VK: vkLControl, ScanCode: 0x1D},
	"leftctrl":     {VK: vkLControl, ScanCode: 0x1D},
	"control_l":    {VK: vkLControl, ScanCode: 0x1D},
	"controlright": {VK: vkRControl, ScanCode: 0x1D, IsExtended: true},
	"ctrlright":    {VK: vkRControl, ScanCode: 0x1D, IsExtended: true},
	"rightctrl":    {VK: vkRControl, ScanCode: 0x1D, IsExtended: true},
	"control_r":    {VK: vkRControl, ScanCode: 0x1D, IsExtended: true},
	"alt":          {VK: vkLMenu, ScanCode: 0x38},
	"altleft":      {VK: vkLMenu, ScanCode: 0x38},
	"leftalt":      {VK: vkLMenu, ScanCode: 0x38},
	"alt_l":        {VK: vkLMenu, ScanCode: 0x38},
	"opt":          {VK: vkLMenu, ScanCode: 0x38},
	"option":       {VK: vkLMenu, ScanCode: 0x38},
	"optionleft":   {VK: vkLMenu, ScanCode: 0x38},
	"altright":     {VK: vkRMenu, ScanCode: 0x38, IsExtended: true},
	"rightalt":     {VK: vkRMenu, ScanCode: 0x38, IsExtended: true},
	"alt_r":        {VK: vkRMenu, ScanCode: 0x38, IsExtended: true},
	"altgr":        {VK: vkRMenu, ScanCode: 0x38, IsExtended: true},
	"optionright":  {VK: vkRMenu, ScanCode: 0x38, IsExtended: true},
	"meta":         {VK: vkLWin, ScanCode: 0x5B, IsExtended: true},
	"win":          {VK: vkLWin, ScanCode: 0x5B, IsExtended: true},
	"cmd":          {VK: vkLWin, ScanCode: 0x5B, IsExtended: true},
	"command":      {VK: vkLWin, ScanCode: 0x5B, IsExtended: true},
	"super":        {VK: vkLWin, ScanCode: 0x5B, IsExtended: true},
	"metaleft":     {VK: vkLWin, ScanCode: 0x5B, IsExtended: true},
	"winleft":      {VK: vkLWin, ScanCode: 0x5B, IsExtended: true},
	"cmdleft":      {VK: vkLWin, ScanCode: 0x5B, IsExtended: true},
	"commandleft":  {VK: vkLWin, ScanCode: 0x5B, IsExtended: true},
	"superleft":    {VK: vkLWin, ScanCode: 0x5B, IsExtended: true},
	"metaright":    {VK: vkRWin, ScanCode: 0x5C, IsExtended: true},
	"winright":     {VK: vkRWin, ScanCode: 0x5C, IsExtended: true},
	"cmdright":     {VK: vkRWin, ScanCode: 0x5C, IsExtended: true},
	"commandright": {VK: vkRWin, ScanCode: 0x5C, IsExtended: true},
	"superright":   {VK: vkRWin, ScanCode: 0x5C, IsExtended: true},
	"apps":         {VK: vkApps, ScanCode: 0x5D, IsExtended: true},
	"menu":         {VK: vkApps, ScanCode: 0x5D, IsExtended: true},

	// Navigation and Editing (Extended keys)
	"insert":      {VK: vkInsert, ScanCode: 0x52, IsExtended: true},
	"ins":         {VK: vkInsert, ScanCode: 0x52, IsExtended: true},
	"delete":      {VK: vkDelete, ScanCode: 0x53, IsExtended: true},
	"del":         {VK: vkDelete, ScanCode: 0x53, IsExtended: true},
	"home":        {VK: vkHome, ScanCode: 0x47, IsExtended: true},
	"end":         {VK: vkEnd, ScanCode: 0x4F, IsExtended: true},
	"pageup":      {VK: vkPrior, ScanCode: 0x49, IsExtended: true},
	"pgup":        {VK: vkPrior, ScanCode: 0x49, IsExtended: true},
	"pagedown":    {VK: vkNext, ScanCode: 0x51, IsExtended: true},
	"pgdn":        {VK: vkNext, ScanCode: 0x51, IsExtended: true},
	"arrowup":     {VK: vkUp, ScanCode: 0x48, IsExtended: true},
	"up":          {VK: vkUp, ScanCode: 0x48, IsExtended: true},
	"arrowdown":   {VK: vkDown, ScanCode: 0x50, IsExtended: true},
	"down":        {VK: vkDown, ScanCode: 0x50, IsExtended: true},
	"arrowleft":   {VK: vkLeft, ScanCode: 0x4B, IsExtended: true},
	"left":        {VK: vkLeft, ScanCode: 0x4B, IsExtended: true},
	"arrowright":  {VK: vkRight, ScanCode: 0x4D, IsExtended: true},
	"right":       {VK: vkRight, ScanCode: 0x4D, IsExtended: true},
	"printscreen": {VK: vkSnapshot, ScanCode: 0x37, IsExtended: true},
	"prtscn":      {VK: vkSnapshot, ScanCode: 0x37, IsExtended: true},
	"pause":       {VK: vkPause, ScanCode: 0x45},

	// Function Keys
	"f1":  {VK: vkF1, ScanCode: 0x3B},
	"f2":  {VK: vkF2, ScanCode: 0x3C},
	"f3":  {VK: vkF3, ScanCode: 0x3D},
	"f4":  {VK: vkF4, ScanCode: 0x3E},
	"f5":  {VK: vkF5, ScanCode: 0x3F},
	"f6":  {VK: vkF6, ScanCode: 0x40},
	"f7":  {VK: vkF7, ScanCode: 0x41},
	"f8":  {VK: vkF8, ScanCode: 0x42},
	"f9":  {VK: vkF9, ScanCode: 0x43},
	"f10": {VK: vkF10, ScanCode: 0x44},
	"f11": {VK: vkF11, ScanCode: 0x57},
	"f12": {VK: vkF12, ScanCode: 0x58},
	"f13": {VK: vkF13, ScanCode: 0x64},
	"f14": {VK: vkF14, ScanCode: 0x65},
	"f15": {VK: vkF15, ScanCode: 0x66},
	"f16": {VK: vkF16, ScanCode: 0x67},
	"f17": {VK: vkF17, ScanCode: 0x68},
	"f18": {VK: vkF18, ScanCode: 0x69},
	"f19": {VK: vkF19, ScanCode: 0x6A},
	"f20": {VK: vkF20, ScanCode: 0x6B},
	"f21": {VK: vkF21, ScanCode: 0x6C},
	"f22": {VK: vkF22, ScanCode: 0x6D},
	"f23": {VK: vkF23, ScanCode: 0x6E},
	"f24": {VK: vkF24, ScanCode: 0x76},

	// Punctuation and Symbols (OEM Keys)
	"`":  {VK: vkOem3, ScanCode: 0x29},
	"~":  {VK: vkOem3, ScanCode: 0x29},
	"-":  {VK: vkOemMinus, ScanCode: 0x0C},
	"_":  {VK: vkOemMinus, ScanCode: 0x0C},
	"=":  {VK: vkOemPlus, ScanCode: 0x0D},
	"+":  {VK: vkOemPlus, ScanCode: 0x0D},
	"[":  {VK: vkOem4, ScanCode: 0x1A},
	"{":  {VK: vkOem4, ScanCode: 0x1A},
	"]":  {VK: vkOem6, ScanCode: 0x1B},
	"}":  {VK: vkOem6, ScanCode: 0x1B},
	"\\": {VK: vkOem5, ScanCode: 0x2B},
	"|":  {VK: vkOem5, ScanCode: 0x2B},
	";":  {VK: vkOem1, ScanCode: 0x27},
	":":  {VK: vkOem1, ScanCode: 0x27},
	"'":  {VK: vkOem7, ScanCode: 0x28},
	"\"": {VK: vkOem7, ScanCode: 0x28},
	",":  {VK: vkOemComma, ScanCode: 0x33},
	"<":  {VK: vkOemComma, ScanCode: 0x33},
	".":  {VK: vkOemPeriod, ScanCode: 0x34},
	">":  {VK: vkOemPeriod, ScanCode: 0x34},
	"/":  {VK: vkOem2, ScanCode: 0x35},
	"?":  {VK: vkOem2, ScanCode: 0x35},

	// Numpad Keys
	"numpad0":        {VK: vkNumpad0, ScanCode: 0x52},
	"numpad1":        {VK: vkNumpad1, ScanCode: 0x4F},
	"numpad2":        {VK: vkNumpad2, ScanCode: 0x50},
	"numpad3":        {VK: vkNumpad3, ScanCode: 0x51},
	"numpad4":        {VK: vkNumpad4, ScanCode: 0x4B},
	"numpad5":        {VK: vkNumpad5, ScanCode: 0x4C},
	"numpad6":        {VK: vkNumpad6, ScanCode: 0x4D},
	"numpad7":        {VK: vkNumpad7, ScanCode: 0x47},
	"numpad8":        {VK: vkNumpad8, ScanCode: 0x48},
	"numpad9":        {VK: vkNumpad9, ScanCode: 0x49},
	"numpadadd":      {VK: vkAdd, ScanCode: 0x4E},
	"numpadsubtract": {VK: vkSubtract, ScanCode: 0x4A},
	"numpadmultiply": {VK: vkMultiply, ScanCode: 0x37},
	"numpaddivide":   {VK: vkDivide, ScanCode: 0x35, IsExtended: true},
	"numpaddecimal":  {VK: vkDecimal, ScanCode: 0x53},
	"numpadenter":    {VK: vkReturn, ScanCode: 0x1C, IsExtended: true},
}

// LookupWindowsKey maps a platform-neutral key name into a Windows KeyMapping.
func LookupWindowsKey(key string) (KeyMapping, error) {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if mapping, found := keyTable[normalized]; found {
		return mapping, nil
	}

	// Fallback single character matching (e.g. single uppercase letter or number)
	if len(normalized) == 1 {
		char := normalized[0]
		if char >= 'a' && char <= 'z' {
			return KeyMapping{
				VK:       uint16(strings.ToUpper(normalized)[0]),
				ScanCode: keyTable[normalized].ScanCode,
			}, nil
		}
		if char >= '0' && char <= '9' {
			return KeyMapping{
				VK:       uint16(char),
				ScanCode: keyTable[normalized].ScanCode,
			}, nil
		}
	}

	return KeyMapping{}, fmt.Errorf("windows: unsupported or unknown key '%s'", key)
}

// Reverse lookup table mapping Windows Virtual Key codes to platform-neutral key names.
var windowsReverseVKTable = map[uint32]string{
	'A': "A", 'B': "B", 'C': "C", 'D': "D", 'E': "E",
	'F': "F", 'G': "G", 'H': "H", 'I': "I", 'J': "J",
	'K': "K", 'L': "L", 'M': "M", 'N': "N", 'O': "O",
	'P': "P", 'Q': "Q", 'R': "R", 'S': "S", 'T': "T",
	'U': "U", 'V': "V", 'W': "W", 'X': "X", 'Y': "Y",
	'Z': "Z",

	'0': "0", '1': "1", '2': "2", '3': "3", '4': "4",
	'5': "5", '6': "6", '7': "7", '8': "8", '9': "9",

	vkEscape:   "Escape",
	vkSpace:    "Space",
	vkTab:      "Tab",
	vkBack:     "Backspace",
	vkDelete:   "Delete",
	vkInsert:   "Insert",
	vkHome:     "Home",
	vkEnd:      "End",
	vkPrior:    "PageUp",
	vkNext:     "PageDown",
	vkLeft:     "ArrowLeft",
	vkUp:       "ArrowUp",
	vkRight:    "ArrowRight",
	vkDown:     "ArrowDown",
	vkCapital:  "CapsLock",
	vkNumLock:  "NumLock",
	vkScroll:   "ScrollLock",
	vkSnapshot: "PrintScreen",
	vkPause:    "Pause",
	vkApps:     "Apps",

	vkLWin: "MetaLeft",
	vkRWin: "MetaRight",

	vkLShift: "ShiftLeft",
	vkRShift: "ShiftRight",

	vkLControl: "ControlLeft",
	vkRControl: "ControlRight",

	vkLMenu: "AltLeft",
	vkRMenu: "AltRight",

	vkF1: "F1", vkF2: "F2", vkF3: "F3", vkF4: "F4",
	vkF5: "F5", vkF6: "F6", vkF7: "F7", vkF8: "F8",
	vkF9: "F9", vkF10: "F10", vkF11: "F11", vkF12: "F12",
	vkF13: "F13", vkF14: "F14", vkF15: "F15", vkF16: "F16",
	vkF17: "F17", vkF18: "F18", vkF19: "F19", vkF20: "F20",
	vkF21: "F21", vkF22: "F22", vkF23: "F23", vkF24: "F24",

	vkNumpad0: "Numpad0", vkNumpad1: "Numpad1", vkNumpad2: "Numpad2",
	vkNumpad3: "Numpad3", vkNumpad4: "Numpad4", vkNumpad5: "Numpad5",
	vkNumpad6: "Numpad6", vkNumpad7: "Numpad7", vkNumpad8: "Numpad8",
	vkNumpad9: "Numpad9",
	vkMultiply: "NumpadMultiply",
	vkAdd:      "NumpadAdd",
	vkSubtract: "NumpadSubtract",
	vkDecimal:  "NumpadDecimal",
	vkDivide:   "NumpadDivide",

	vkOem3:      "`",
	vkOemMinus:  "-",
	vkOemPlus:   "=",
	vkOem4:      "[",
	vkOem6:      "]",
	vkOem5:      "\\",
	vkOem1:      ";",
	vkOem7:      "'",
	vkOemComma:  ",",
	vkOemPeriod: ".",
	vkOem2:      "/",
}

// LookupWindowsKeyByVK maps a Windows Virtual Key code, scan code, and extended flag into a platform-neutral key name.
func LookupWindowsKeyByVK(vk uint32, scanCode uint32, isExtended bool) string {
	// Specific modifier distinctions
	switch vk {
	case vkShift:
		if scanCode == 0x36 {
			return "ShiftRight"
		}
		return "ShiftLeft"
	case vkControl:
		if isExtended {
			return "ControlRight"
		}
		return "ControlLeft"
	case vkMenu:
		if isExtended {
			return "AltRight"
		}
		return "AltLeft"
	case vkReturn:
		if isExtended {
			return "NumpadEnter"
		}
		return "Enter"
	}

	if name, found := windowsReverseVKTable[vk]; found {
		return name
	}

	// ASCII printable character fallback
	if vk >= 'A' && vk <= 'Z' {
		return string(rune(vk))
	}
	if vk >= '0' && vk <= '9' {
		return string(rune(vk))
	}

	return fmt.Sprintf("VK_0x%02X", vk)
}
