//go:build darwin && cgo

package platform

/*
// CGDisplayCreateImageForRect is marked obsolete in the macOS 15 SDK in
// favour of ScreenCaptureKit, but it still works and is much faster for the
// tiny, frequent captures the player makes. Targeting macOS 11 keeps it
// available when building with a newer SDK.
#cgo CFLAGS: -mmacosx-version-min=11.0 -Wno-deprecated-declarations
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework ApplicationServices
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>

// gf_capture draws the global rectangle (x, y, w, h), in points, into buf as
// RGBA with w*4 bytes per row. Returns 0 on success.
static int gf_capture(int x, int y, int w, int h, void *buf) {
	CGRect rect = CGRectMake(x, y, w, h);
	CGDirectDisplayID ids[16];
	uint32_t n = 0;
	if (CGGetDisplaysWithRect(rect, 16, ids, &n) != kCGErrorSuccess || n == 0) {
		return 1;
	}
	CGColorSpaceRef cs = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
	CGContextRef ctx = CGBitmapContextCreate(buf, w, h, 8, w * 4, cs,
		kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
	CGColorSpaceRelease(cs);
	if (!ctx) {
		return 2;
	}
	CGContextSetInterpolationQuality(ctx, kCGInterpolationNone);
	int rc = 3;
	for (uint32_t i = 0; i < n; i++) {
		CGRect db = CGDisplayBounds(ids[i]);
		CGRect part = CGRectIntegral(CGRectIntersection(db, rect));
		if (CGRectIsNull(part) || CGRectIsEmpty(part)) {
			continue;
		}
		// Odd sizes can make CGDisplayCreateImageForRect fail; grow to even
		// (staying on the display) and let the bitmap context clip the rest.
		if ((int)part.size.width % 2 && CGRectGetMaxX(part) < CGRectGetMaxX(db)) {
			part.size.width += 1;
		}
		if ((int)part.size.height % 2 && CGRectGetMaxY(part) < CGRectGetMaxY(db)) {
			part.size.height += 1;
		}
		CGRect local = CGRectOffset(part, -db.origin.x, -db.origin.y);
		CGImageRef img = CGDisplayCreateImageForRect(ids[i], local);
		if (!img) {
			continue;
		}
		// Bitmap contexts have their origin at the bottom left.
		CGRect dst = CGRectMake(part.origin.x - x,
			(y + h) - (part.origin.y + part.size.height),
			part.size.width, part.size.height);
		CGContextDrawImage(ctx, dst, img);
		CGImageRelease(img);
		rc = 0;
	}
	CGContextRelease(ctx);
	return rc;
}

static int gf_displays(CGRect *out, int max) {
	CGDirectDisplayID ids[16];
	uint32_t n = 0;
	if (CGGetActiveDisplayList(16, ids, &n) != kCGErrorSuccess) {
		return 0;
	}
	int i;
	for (i = 0; i < (int)n && i < max; i++) {
		out[i] = CGDisplayBounds(ids[i]); // the main display comes first
	}
	return i;
}

static void gf_key(CGKeyCode code, bool down) {
	CGEventRef e = CGEventCreateKeyboardEvent(NULL, code, down);
	if (!e) {
		return;
	}
	CGEventSetFlags(e, 0); // do not pick up Cmd/Shift the user is holding
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

static int gf_cursor(double *x, double *y) {
	CGEventRef e = CGEventCreate(NULL);
	if (!e) {
		return 0;
	}
	CGPoint p = CGEventGetLocation(e);
	CFRelease(e);
	*x = p.x;
	*y = p.y;
	return 1;
}

static int gf_accessibility(int prompt) {
	if (!prompt) {
		return AXIsProcessTrusted();
	}
	const void *keys[] = { kAXTrustedCheckOptionPrompt };
	const void *vals[] = { kCFBooleanTrue };
	CFDictionaryRef opts = CFDictionaryCreate(NULL, keys, vals, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	int ok = AXIsProcessTrustedWithOptions(opts);
	CFRelease(opts);
	return ok;
}

static int gf_screen_recording(int prompt) {
	if (CGPreflightScreenCaptureAccess()) {
		return 1;
	}
	return prompt ? CGRequestScreenCaptureAccess() : 0;
}
*/
import "C"

import (
	"fmt"
	"image"
	"os/exec"
	"unsafe"
)

// Displays returns the bounds of every display, the main one first.
func Displays() ([]image.Rectangle, error) {
	var rects [16]C.CGRect
	n := int(C.gf_displays(&rects[0], C.int(len(rects))))
	if n == 0 {
		return nil, fmt.Errorf("no displays found")
	}
	ds := make([]image.Rectangle, n)
	for i := range ds {
		r := rects[i]
		ds[i] = image.Rect(int(r.origin.x), int(r.origin.y),
			int(r.origin.x+r.size.width), int(r.origin.y+r.size.height))
	}
	return ds, nil
}

// Capture copies the screen rectangle r.
func Capture(r image.Rectangle) (*image.RGBA, error) {
	if r.Empty() {
		return nil, fmt.Errorf("empty capture rectangle %v", r)
	}
	img := image.NewRGBA(r)
	rc := C.gf_capture(C.int(r.Min.X), C.int(r.Min.Y), C.int(r.Dx()), C.int(r.Dy()), unsafe.Pointer(&img.Pix[0]))
	if rc != 0 {
		return nil, fmt.Errorf("screen capture failed (code %d)", int(rc))
	}
	return img, nil
}

func newKey(name string, c keyCodes) Key {
	return Key{Name: name, code: c.mac}
}

// KeyDown presses k.
func KeyDown(k Key) error {
	C.gf_key(C.CGKeyCode(k.code), true)
	return nil
}

// KeyUp releases k.
func KeyUp(k Key) error {
	C.gf_key(C.CGKeyCode(k.code), false)
	return nil
}

// Cursor returns the mouse position.
func Cursor() (image.Point, error) {
	var x, y C.double
	if C.gf_cursor(&x, &y) == 0 {
		return image.Point{}, fmt.Errorf("cannot read the mouse position")
	}
	return image.Pt(int(x), int(y)), nil
}

// Check reports missing macOS privacy permissions: Screen Recording to see
// the game, and Accessibility to press keys (only checked when keys is set).
// With prompt set, macOS is asked to show its permission dialogs.
func Check(prompt, keys bool) []Problem {
	var ps []Problem
	if C.gf_screen_recording(boolInt(prompt)) == 0 {
		ps = append(ps, Problem{
			What: "Screen Recording permission is missing, so the game cannot be seen.",
			Fix: "In System Settings > Privacy & Security > Screen & System Audio Recording, " +
				"turn on Guitar Flash AutoPlay (or your terminal app if you started it from a terminal), then restart it.",
			Settings: "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture",
		})
	}
	if keys && C.gf_accessibility(boolInt(prompt)) == 0 {
		ps = append(ps, Problem{
			What: "Accessibility permission is missing, so key presses would be ignored.",
			Fix: "In System Settings > Privacy & Security > Accessibility, " +
				"turn on Guitar Flash AutoPlay (or your terminal app if you started it from a terminal), then restart it.",
			Settings: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility",
		})
	}
	return ps
}

func boolInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// PauseBeforeExit does nothing on macOS: Terminal keeps the window open.
func PauseBeforeExit() {}

// OpenURL opens a web page, or a System Settings page, with its app.
func OpenURL(u string) error { return exec.Command("open", u).Start() }

// UseParentConsole does nothing here: programs always have their terminal.
func UseParentConsole() {}
