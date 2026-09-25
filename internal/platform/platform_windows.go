//go:build windows

package platform

import (
	"bufio"
	"fmt"
	"image"
	"os"
	"syscall"
	"unsafe"

	"github.com/kbinani/screenshot"
)

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	procSendInput          = user32.NewProc("SendInput")
	procMapVirtualKey      = user32.NewProc("MapVirtualKeyW")
	procGetCursorPos       = user32.NewProc("GetCursorPos")
	procSetDpiAwarenessCtx = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	shcore                 = syscall.NewLazyDLL("shcore.dll")
	procSetDpiAwareness    = shcore.NewProc("SetProcessDpiAwareness")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleProcs    = kernel32.NewProc("GetConsoleProcessList")
	procAttachConsole      = kernel32.NewProc("AttachConsole")
)

func init() {
	// Without this, Windows display scaling (125%, 150%, ...) hands us a
	// shrunken, blurry copy of the screen with the wrong coordinates.
	const perMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	if procSetDpiAwarenessCtx.Find() == nil {
		if ok, _, _ := procSetDpiAwarenessCtx.Call(perMonitorAwareV2); ok != 0 {
			return
		}
	}
	if procSetDpiAwareness.Find() == nil {
		const processPerMonitorDPIAware = 2
		if hr, _, _ := procSetDpiAwareness.Call(processPerMonitorDPIAware); hr == 0 {
			return
		}
	}
	if procSetProcessDPIAware.Find() == nil {
		procSetProcessDPIAware.Call()
	}
}

// Displays returns the bounds of every monitor, the primary one first.
func Displays() ([]image.Rectangle, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, fmt.Errorf("no displays found")
	}
	ds := make([]image.Rectangle, n)
	for i := range ds {
		ds[i] = screenshot.GetDisplayBounds(i)
	}
	return ds, nil
}

// Capture copies the screen rectangle r.
func Capture(r image.Rectangle) (*image.RGBA, error) {
	img, err := screenshot.CaptureRect(r)
	if err != nil {
		return nil, err
	}
	img.Rect = r
	return img, nil
}

func newKey(name string, c keyCodes) Key {
	return Key{Name: name, code: c.win, extended: c.extended}
}

type keyboardInput struct {
	vk, scan    uint16
	flags, time uint32
	extraInfo   uintptr
}

// input mirrors the Win32 INPUT struct for a keyboard event. The padding
// makes it as large as the biggest member of the union (MOUSEINPUT).
type input struct {
	typ uint32
	ki  keyboardInput
	_   [8]byte
}

const (
	inputKeyboard     = 1
	keyEventExtended  = 0x0001
	keyEventKeyUp     = 0x0002
	mapVirtualKeyToSC = 0
)

func sendKey(k Key, up bool) error {
	in := input{typ: inputKeyboard}
	in.ki.vk = k.code
	sc, _, _ := procMapVirtualKey.Call(uintptr(k.code), mapVirtualKeyToSC)
	in.ki.scan = uint16(sc)
	if k.extended {
		in.ki.flags |= keyEventExtended
	}
	if up {
		in.ki.flags |= keyEventKeyUp
	}
	n, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
	if n != 1 {
		return fmt.Errorf("SendInput: %v", err)
	}
	return nil
}

// KeyDown presses k.
func KeyDown(k Key) error { return sendKey(k, false) }

// KeyUp releases k.
func KeyUp(k Key) error { return sendKey(k, true) }

// Cursor returns the mouse position.
func Cursor() (image.Point, error) {
	var p struct{ X, Y int32 }
	if ok, _, err := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); ok == 0 {
		return image.Point{}, fmt.Errorf("GetCursorPos: %v", err)
	}
	return image.Pt(int(p.X), int(p.Y)), nil
}

// Check reports missing permissions. Windows needs none.
func Check(prompt, keys bool) []Problem { return nil }

// PauseBeforeExit keeps the console window open when the program was
// started by double-clicking it, so any message can still be read.
func PauseBeforeExit() {
	var ids [2]uint32
	if n, _, _ := procGetConsoleProcs.Call(uintptr(unsafe.Pointer(&ids[0])), 2); n == 1 {
		fmt.Print("\nPress Enter to close this window...")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
}

// OpenSettings does nothing here: no problem on this system has a settings
// page.
func OpenSettings(p Problem) error { return fmt.Errorf("no settings page for this problem") }

// UseParentConsole lets the windowed build (which has no console of its
// own) print to the terminal it was started from, when it is used on the
// command line.
func UseParentConsole() {
	const attachParentProcess = ^uintptr(0)
	if ok, _, _ := procAttachConsole.Call(attachParentProcess); ok == 0 {
		return // already has a console, or was not started from one
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
	if f, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = f
	}
}
