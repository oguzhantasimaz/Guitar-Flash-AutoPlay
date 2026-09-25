//go:build linux

package platform

// Linux support is experimental and X11 only (it also works under Xvfb,
// which is how the whole app can be tested against the real game without a
// physical screen). Everything goes through one connection to the X server:
// GetImage to read the screen, XTEST to press keys.

import (
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

var x11 struct {
	once     sync.Once
	err      error
	conn     *xgb.Conn
	root     xproto.Window
	size     image.Point
	keycodes map[xproto.Keysym]xproto.Keycode
}

func connect() error {
	x11.once.Do(func() {
		xgb.Logger = log.New(io.Discard, "", 0)
		c, err := xgb.NewConn()
		if err != nil {
			x11.err = fmt.Errorf("connecting to the X server: %w", err)
			return
		}
		if err := xtest.Init(c); err != nil {
			x11.err = fmt.Errorf("the X server has no XTEST extension: %w", err)
			return
		}
		setup := xproto.Setup(c)
		scr := setup.DefaultScreen(c)
		x11.conn, x11.root = c, scr.Root
		x11.size = image.Pt(int(scr.WidthInPixels), int(scr.HeightInPixels))

		n := byte(setup.MaxKeycode - setup.MinKeycode + 1)
		m, err := xproto.GetKeyboardMapping(c, setup.MinKeycode, n).Reply()
		if err != nil {
			x11.err = fmt.Errorf("reading the keyboard map: %w", err)
			return
		}
		x11.keycodes = map[xproto.Keysym]xproto.Keycode{}
		per := int(m.KeysymsPerKeycode)
		for i := 0; i < int(n); i++ {
			for j := 0; j < per; j++ {
				sym := m.Keysyms[i*per+j]
				if _, dup := x11.keycodes[sym]; sym != 0 && !dup {
					x11.keycodes[sym] = setup.MinKeycode + xproto.Keycode(i)
				}
			}
		}
	})
	return x11.err
}

// Displays returns the X screen.
func Displays() ([]image.Rectangle, error) {
	if err := connect(); err != nil {
		return nil, err
	}
	return []image.Rectangle{{Max: x11.size}}, nil
}

// Capture copies the screen rectangle r.
func Capture(r image.Rectangle) (*image.RGBA, error) {
	if err := connect(); err != nil {
		return nil, err
	}
	r = r.Intersect(image.Rectangle{Max: x11.size})
	if r.Empty() {
		return nil, errors.New("capture rectangle is off the screen")
	}
	reply, err := xproto.GetImage(x11.conn, xproto.ImageFormatZPixmap, xproto.Drawable(x11.root),
		int16(r.Min.X), int16(r.Min.Y), uint16(r.Dx()), uint16(r.Dy()), 0xffffffff).Reply()
	if err != nil {
		return nil, err
	}
	img := image.NewRGBA(r)
	if len(reply.Data) < len(img.Pix) {
		return nil, fmt.Errorf("unsupported X screen format (depth %d)", reply.Depth)
	}
	for i := 0; i < len(img.Pix); i += 4 {
		// 32 bits per pixel, BGRX.
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = reply.Data[i+2], reply.Data[i+1], reply.Data[i], 255
	}
	return img, nil
}

// X keysyms of the keys that are not a single character.
var keysyms = map[string]uint16{
	"space": 0x20, "enter": 0xff0d, "tab": 0xff09, "backspace": 0xff08,
	"left": 0xff51, "up": 0xff52, "right": 0xff53, "down": 0xff54,
}

func newKey(name string, c keyCodes) Key {
	switch {
	case len(name) == 1:
		return Key{Name: name, code: uint16(name[0])} // Latin-1 keysyms are the character
	case keysyms[name] != 0:
		return Key{Name: name, code: keysyms[name]}
	case len(name) >= 2 && name[0] == 'f':
		var n int
		fmt.Sscanf(name[1:], "%d", &n)
		return Key{Name: name, code: uint16(0xffbe + n - 1)}
	case len(name) == 4 && name[:3] == "num":
		return Key{Name: name, code: uint16(0xffb0 + int(name[3]-'0'))}
	}
	return Key{Name: name}
}

func sendKey(k Key, down bool) error {
	if err := connect(); err != nil {
		return err
	}
	kc, ok := x11.keycodes[xproto.Keysym(k.code)]
	if !ok {
		return fmt.Errorf("key %q is not on this keyboard map", k.Name)
	}
	typ := byte(xproto.KeyRelease)
	if down {
		typ = xproto.KeyPress
	}
	return xtest.FakeInputChecked(x11.conn, typ, byte(kc), xproto.TimeCurrentTime, x11.root, 0, 0, 0).Check()
}

// KeyDown presses k.
func KeyDown(k Key) error { return sendKey(k, true) }

// KeyUp releases k.
func KeyUp(k Key) error { return sendKey(k, false) }

// Cursor returns the mouse position.
func Cursor() (image.Point, error) {
	if err := connect(); err != nil {
		return image.Point{}, err
	}
	p, err := xproto.QueryPointer(x11.conn, x11.root).Reply()
	if err != nil {
		return image.Point{}, err
	}
	return image.Pt(int(p.RootX), int(p.RootY)), nil
}

// Check reports when there is no X server to talk to.
func Check(prompt, keys bool) []Problem {
	if os.Getenv("DISPLAY") == "" {
		return []Problem{{
			What: "No X11 display found.",
			Fix:  "On Linux, live play is experimental and needs an X11 session (DISPLAY must be set).",
		}}
	}
	if err := connect(); err != nil {
		return []Problem{{What: "Cannot use the X11 display.", Fix: err.Error()}}
	}
	return nil
}

// PauseBeforeExit does nothing on Linux.
func PauseBeforeExit() {}

// UseParentConsole does nothing on Linux.
func UseParentConsole() {}

// OpenURL opens a web page in the default browser.
func OpenURL(u string) error { return exec.Command("xdg-open", u).Start() }
