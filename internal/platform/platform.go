// Package platform wraps the few OS features the player needs: reading the
// screen, pressing keys and reading the mouse position.
//
// Coordinates are the ones the OS uses for the mouse: physical pixels on
// Windows (the process is made DPI aware), points on macOS (a Retina screen
// is captured at its "looks like" resolution). Captured images have exactly
// one pixel per unit and their Rect is the requested rectangle, so a
// position found in a screenshot can be used directly for the next capture.
package platform

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrUnsupported is returned on systems without screen capture or keyboard
// support. Only Windows and macOS are supported.
var ErrUnsupported = errors.New("only Windows and macOS are supported")

// Key is a keyboard key that can be pressed with KeyDown and KeyUp.
type Key struct {
	Name     string
	code     uint16
	extended bool // Windows: arrow keys and friends need the extended flag
}

type keyCodes struct {
	win, mac uint16
	extended bool
}

// keyTable maps key names to Windows virtual-key codes and macOS virtual
// keycodes. macOS keycodes are physical positions on a US keyboard.
var keyTable = map[string]keyCodes{
	"a": {0x41, 0x00, false}, "b": {0x42, 0x0B, false}, "c": {0x43, 0x08, false},
	"d": {0x44, 0x02, false}, "e": {0x45, 0x0E, false}, "f": {0x46, 0x03, false},
	"g": {0x47, 0x05, false}, "h": {0x48, 0x04, false}, "i": {0x49, 0x22, false},
	"j": {0x4A, 0x26, false}, "k": {0x4B, 0x28, false}, "l": {0x4C, 0x25, false},
	"m": {0x4D, 0x2E, false}, "n": {0x4E, 0x2D, false}, "o": {0x4F, 0x1F, false},
	"p": {0x50, 0x23, false}, "q": {0x51, 0x0C, false}, "r": {0x52, 0x0F, false},
	"s": {0x53, 0x01, false}, "t": {0x54, 0x11, false}, "u": {0x55, 0x20, false},
	"v": {0x56, 0x09, false}, "w": {0x57, 0x0D, false}, "x": {0x58, 0x07, false},
	"y": {0x59, 0x10, false}, "z": {0x5A, 0x06, false},

	"0": {0x30, 0x1D, false}, "1": {0x31, 0x12, false}, "2": {0x32, 0x13, false},
	"3": {0x33, 0x14, false}, "4": {0x34, 0x15, false}, "5": {0x35, 0x17, false},
	"6": {0x36, 0x16, false}, "7": {0x37, 0x1A, false}, "8": {0x38, 0x1C, false},
	"9": {0x39, 0x19, false},

	"space": {0x20, 0x31, false}, "enter": {0x0D, 0x24, false},
	"tab": {0x09, 0x30, false}, "backspace": {0x08, 0x33, false},
	"left": {0x25, 0x7B, true}, "right": {0x27, 0x7C, true},
	"up": {0x26, 0x7E, true}, "down": {0x28, 0x7D, true},
	";": {0xBA, 0x29, false}, "'": {0xDE, 0x27, false}, ",": {0xBC, 0x2B, false},
	".": {0xBE, 0x2F, false}, "/": {0xBF, 0x2C, false}, "-": {0xBD, 0x1B, false},
	"=": {0xBB, 0x18, false}, "[": {0xDB, 0x21, false}, "]": {0xDD, 0x1E, false},
	"\\": {0xDC, 0x2A, false}, "`": {0xC0, 0x32, false},

	"f1": {0x70, 0x7A, false}, "f2": {0x71, 0x78, false}, "f3": {0x72, 0x63, false},
	"f4": {0x73, 0x76, false}, "f5": {0x74, 0x60, false}, "f6": {0x75, 0x61, false},
	"f7": {0x76, 0x62, false}, "f8": {0x77, 0x64, false}, "f9": {0x78, 0x65, false},
	"f10": {0x79, 0x6D, false}, "f11": {0x7A, 0x67, false}, "f12": {0x7B, 0x6F, false},

	"num0": {0x60, 0x52, false}, "num1": {0x61, 0x53, false}, "num2": {0x62, 0x54, false},
	"num3": {0x63, 0x55, false}, "num4": {0x64, 0x56, false}, "num5": {0x65, 0x57, false},
	"num6": {0x66, 0x58, false}, "num7": {0x67, 0x59, false}, "num8": {0x68, 0x5B, false},
	"num9": {0x69, 0x5C, false},
}

var keyAliases = map[string]string{
	"return": "enter", "semicolon": ";", "comma": ",", "period": ".",
	"slash": "/", "minus": "-", "equal": "=", "quote": "'",
}

// KeyNames lists every key name ParseKey accepts.
func KeyNames() []string {
	names := make([]string, 0, len(keyTable))
	for n := range keyTable {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ParseKey looks up a key by name, e.g. "a", "enter" or "left".
func ParseKey(name string) (Key, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if a, ok := keyAliases[n]; ok {
		n = a
	}
	c, ok := keyTable[n]
	if !ok {
		return Key{}, fmt.Errorf("unknown key %q (known keys: %s)", name, strings.Join(KeyNames(), " "))
	}
	return newKey(n, c), nil
}

// Problem is something the user has to fix before the player can work, with
// instructions on how to do it.
type Problem struct {
	What, Fix string
	// Settings, if set, opens the system settings page where it is fixed
	// (see OpenSettings).
	Settings string
}
