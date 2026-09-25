//go:build !windows && !linux && !(darwin && cgo)

package platform

import (
	"fmt"
	"image"
)

// Displays is not supported on this system.
func Displays() ([]image.Rectangle, error) { return nil, ErrUnsupported }

// Capture is not supported on this system.
func Capture(r image.Rectangle) (*image.RGBA, error) { return nil, ErrUnsupported }

func newKey(name string, c keyCodes) Key { return Key{Name: name} }

// KeyDown is not supported on this system.
func KeyDown(k Key) error { return ErrUnsupported }

// KeyUp is not supported on this system.
func KeyUp(k Key) error { return ErrUnsupported }

// Cursor is not supported on this system.
func Cursor() (image.Point, error) { return image.Point{}, ErrUnsupported }

// Check reports that live play is not available here.
func Check(prompt, keys bool) []Problem {
	return []Problem{{
		What: "Live play only works on Windows and macOS.",
		Fix:  "Use -analyze with a screenshot to test fretboard detection on this system.",
	}}
}

// PauseBeforeExit does nothing on this system.
func PauseBeforeExit() {}

// OpenSettings does nothing here: no problem on this system has a settings
// page.
func OpenSettings(p Problem) error { return fmt.Errorf("no settings page for this problem") }

// UseParentConsole does nothing here: programs always have their terminal.
func UseParentConsole() {}
