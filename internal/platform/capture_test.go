package platform

import (
	"errors"
	"image"
	"os"
	"testing"
)

// TestCaptureSmoke reads the real screen. It needs a desktop session (and on
// macOS the Screen Recording permission), so it only runs when asked to:
//
//	GF_SCREEN_TEST=1 go test -run CaptureSmoke -v ./internal/platform
func TestCaptureSmoke(t *testing.T) {
	if os.Getenv("GF_SCREEN_TEST") == "" {
		t.Skip("set GF_SCREEN_TEST=1 to capture the real screen")
	}
	ds, err := Displays()
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("displays: %v", ds)
	for _, r := range []image.Rectangle{
		ds[0],
		image.Rect(ds[0].Min.X+11, ds[0].Min.Y+7, ds[0].Min.X+112, ds[0].Min.Y+40), // odd size
	} {
		img, err := Capture(r)
		if err != nil {
			t.Fatalf("Capture(%v): %v", r, err)
		}
		if img.Rect != r {
			t.Errorf("Capture(%v) returned %v", r, img.Rect)
		}
		var sum int
		for i := 0; i < len(img.Pix); i += 4 {
			sum += int(img.Pix[i]) + int(img.Pix[i+1]) + int(img.Pix[i+2])
		}
		t.Logf("captured %v, mean brightness %d", r, sum/(3*r.Dx()*r.Dy()))
	}
	p, err := Cursor()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("mouse at %v", p)
}
