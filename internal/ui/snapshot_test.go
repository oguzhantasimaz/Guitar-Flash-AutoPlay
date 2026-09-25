package ui

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/player"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/vision"
)

// TestSnapshots renders the window in a few states to PNG files, to look at
// the design without a real screen or game. It needs a GPU or a software
// renderer, so it only runs when asked to:
//
//	GF_UI_SNAPSHOT=/tmp/shots go test -run Snapshots ./internal/ui
//
// GF_UI_PREVIEW can point to a PNG to show as the fretboard preview.
func TestSnapshots(t *testing.T) {
	dir := os.Getenv("GF_UI_SNAPSHOT")
	if dir == "" {
		t.Skip("set GF_UI_SNAPSHOT to a directory to render the window")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // default settings
	t.Setenv("HOME", t.TempDir())

	preview := image.NewRGBA(image.Rect(0, 0, 600, 260))
	draw.Draw(preview, preview.Rect, &image.Uniform{color.RGBA{40, 40, 48, 255}}, image.Point{}, draw.Src)
	if path := os.Getenv("GF_UI_PREVIEW"); path != "" {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		src, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		preview = image.NewRGBA(src.Bounds())
		draw.Draw(preview, preview.Rect, src, src.Bounds().Min, draw.Src)
	}

	now := time.Now()
	for _, tc := range []struct {
		name string
		set  func(u *UI)
	}{
		{"ready", func(u *UI) {}},
		{"readme", func(u *UI) { u.st.problems = nil }},
		// The end of the page, with the log and the footer.
		{"bottom", func(u *UI) { u.st.problems, u.page.ScrollToEnd = nil, true }},
		{"searching", func(u *UI) {
			u.st.problems = nil
			u.st.running, u.st.state = true, player.Searching
			u.log("Looking for the fretboard...")
		}},
		{"playing", func(u *UI) {
			u.st.problems = nil
			u.st.running, u.st.state = true, player.Playing
			u.st.board = vision.Board{X0: 886, Y: 1049, Spacing: 99.6}
			u.st.notes, u.st.speed = 312, 8.3
			u.st.keyDown[2] = true
			u.st.lastPress[3] = now
			u.st.preview, u.st.hasPreview = paint.NewImageOp(preview), true
			u.log("Found the fretboard: frets at y=1049, x=886..1284 (spacing 99.6px)")
			u.log("Note speed: 8.3 fret spacings per second.")
		}},
		{"permissions", func(u *UI) {
			u.st.problems[0].Settings = "x-apple.systempreferences:"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := newUI("v3.0.0")
			tc.set(u)
			const scale = 1.5
			size := image.Pt(int(480*scale), int(860*scale))
			w, err := headless.NewWindow(size.X, size.Y)
			if err != nil {
				t.Skip("no headless GPU:", err)
			}
			defer w.Release()
			gtx := layout.Context{
				Ops:         new(op.Ops),
				Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
				Constraints: layout.Exact(size),
				Now:         now,
				Source:      new(input.Router).Source(),
			}
			u.layout(gtx)
			if err := w.Frame(gtx.Ops); err != nil {
				t.Fatal(err)
			}
			img := image.NewRGBA(image.Rectangle{Max: size})
			if err := w.Screenshot(img); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, tc.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
		})
	}
}
