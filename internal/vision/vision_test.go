package vision

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"
)

var laneRGB = [5]color.RGBA{
	{11, 151, 18, 255},  // green
	{238, 14, 13, 255},  // red
	{225, 226, 20, 255}, // yellow
	{17, 146, 227, 255}, // blue
	{238, 99, 16, 255},  // orange
}

// scene draws a simplified Guitar Flash frame: a busy background, the dark
// highway, five fret rings, gems and sustain tails, with the proportions
// measured on the real game.
type scene struct {
	w, h  int
	board Board
	gems  []gemAt  // lane and centre height in spacings
	tails []tailAt // lane and height range in spacings
}

type gemAt struct {
	lane int
	h    float64
}

type tailAt struct {
	lane     int
	from, to float64
}

func newScene(w, h int, x0, y, spacing float64) scene {
	return scene{w: w, h: h, board: Board{X0: x0, Y: y, Spacing: spacing}}
}

func (s scene) render() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, s.w, s.h))
	// A dim, noisy photo behind everything, like the band pictures.
	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < len(img.Pix); i += 4 {
		v := rnd.Intn(110) + 20
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = uint8(v), uint8(v*3/4), uint8(v/2), 255
	}
	// Coloured decoys like the rest of the web page: the site's red menu
	// bar and a single yellow ring far away from the fretboard.
	fillRect(img, image.Rect(0, 0, s.w/2, s.h/30+3), laneRGB[1])
	ring(img, float64(s.w)*0.1, float64(s.h)*0.15, 20, 7, 3, laneRGB[2])

	b := s.board
	sp := b.Spacing
	for y := 0; y < s.h; y++ {
		h := (b.Y - float64(y)) / sp
		if h < -0.6 || h > 3.5 {
			continue
		}
		half := 2.48 * sp * b.Scale(h)
		for x := int(b.FretX(2) - half); x <= int(b.FretX(2)+half); x++ {
			set(img, x, y, color.RGBA{33, 33, 33, 255})
		}
	}
	for k := 0; k < 5; k++ {
		ring(img, b.FretX(k), b.Y, 0.5*sp, 0.165*sp, math.Max(2, 0.05*sp), laneRGB[k])
	}
	for _, t := range s.tails {
		for h := t.from; h <= t.to; h += 0.5 / sp {
			hw := math.Max(1, 0.05*sp*b.Scale(h))
			x := b.LaneX(t.lane, h)
			for dx := -hw; dx <= hw; dx++ {
				set(img, int(x+dx), int(b.RowY(h)), laneRGB[t.lane])
			}
		}
	}
	for _, g := range s.gems {
		sc := b.Scale(g.h)
		x, y := b.LaneX(g.lane, g.h), b.RowY(g.h)
		ellipse(img, x, y, 0.355*sp*sc, 0.12*sp*sc, laneRGB[g.lane])
		ellipse(img, x, y-0.07*sp*sc, 0.16*sp*sc, 0.05*sp*sc, color.RGBA{236, 234, 233, 255})
	}
	return img
}

func set(img *image.RGBA, x, y int, c color.RGBA) {
	if (image.Point{x, y}).In(img.Rect) {
		img.SetRGBA(x, y, c)
	}
}

func fillRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			set(img, x, y, c)
		}
	}
}

func ellipse(img *image.RGBA, cx, cy, rx, ry float64, c color.RGBA) {
	for y := int(cy - ry); y <= int(cy+ry)+1; y++ {
		for x := int(cx - rx); x <= int(cx+rx)+1; x++ {
			dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
			if dx*dx+dy*dy <= 1 {
				set(img, x, y, c)
			}
		}
	}
}

// ring draws an elliptical outline of the given thickness with a black
// inside, like the fret buttons.
func ring(img *image.RGBA, cx, cy, rx, ry, t float64, c color.RGBA) {
	ellipse(img, cx, cy, rx, ry, c)
	ellipse(img, cx, cy, rx-t, ry-t*ry/rx*1.5, color.RGBA{15, 15, 15, 255})
}

func TestFindBoardAtManySizes(t *testing.T) {
	for _, tc := range []struct {
		name          string
		w, h          int
		x0, y, spacng float64
	}{
		{"tiny window", 480, 320, 131.3, 287.2, 24},
		{"laptop zoomed out", 1280, 800, 452.7, 690.5, 40},
		{"1080p", 1920, 1080, 700.4, 931, 64},
		{"macbook retina points", 1512, 982, 671, 795, 75.3},
		{"measured screenshot", 2000, 1299, 886, 1050, 99.6},
		{"4k zoomed in", 3840, 2160, 1500.5, 1800, 150},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScene(tc.w, tc.h, tc.x0, tc.y, tc.spacng)
			b, err := FindBoard(s.render())
			if err != nil {
				t.Fatal(err)
			}
			tol := 0.05 * tc.spacng
			if math.Abs(b.X0-tc.x0) > tol || math.Abs(b.Y-tc.y) > tol || math.Abs(b.Spacing-tc.spacng) > 0.02*tc.spacng {
				t.Errorf("got %v (x0 %.1f), want x0 %.1f y %.1f spacing %.1f", b, b.X0, tc.x0, tc.y, tc.spacng)
			}
		})
	}
}

func TestFindBoardWithNotesOnTheFrets(t *testing.T) {
	s := newScene(1600, 1000, 500, 800, 80)
	// A chord sitting right on two frets merges gems into the rings, and a
	// third ring is hidden completely.
	s.gems = []gemAt{{2, -0.05}, {3, -0.05}}
	img := s.render()
	ellipse(img, s.board.FretX(0), s.board.Y, 45, 18, color.RGBA{250, 250, 250, 255})

	b, err := FindBoard(img)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(b.X0-500) > 4 || math.Abs(b.Y-800) > 4 || math.Abs(b.Spacing-80) > 1.6 {
		t.Errorf("got %v, want frets at y=800 from x=500 with spacing 80", b)
	}
	if b.Found != 4 {
		t.Errorf("found %d rings, want 4", b.Found)
	}
}

func TestFindBoardNothingThere(t *testing.T) {
	// Only the background and decoys: the board is drawn off-screen.
	s := newScene(800, 600, -1000, -1000, 50)
	if _, err := FindBoard(s.render()); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestLineTellsGemsFromTails(t *testing.T) {
	for _, sp := range []float64{30, 64, 99.6} {
		s := newScene(int(12*sp), int(7*sp), 4*sp, 6*sp, sp)
		s.gems = []gemAt{{0, 1.0}, {3, 2.0}}
		s.tails = []tailAt{{1, 0.5, 2.5}, {3, 2.15, 3}}
		img := s.render()

		lower, _ := NewLine(s.board, 1.0).Read(img)
		upper, _ := NewLine(s.board, 2.0).Read(img)
		want := func(name string, got float64, lo, hi float64) {
			t.Helper()
			if got < lo || got > hi {
				t.Errorf("spacing %.0f: %s coverage %.2f, want %.2f..%.2f", sp, name, got, lo, hi)
			}
		}
		want("gem on lower line", lower[0], 0.9, 1)
		want("tail on lower line", lower[1], 0.08, 0.5)
		want("empty lane", lower[2], 0, 0.02)
		want("gem on upper line", upper[3], 0.9, 1)
		want("tail on upper line", upper[1], 0.08, 0.5)
		want("empty lane", upper[4], 0, 0.02)
	}
}

func TestLineSpotsFlash(t *testing.T) {
	s := newScene(1200, 700, 400, 600, 100)
	img := s.render()
	if _, flash := NewLine(s.board, 1).Read(img); flash {
		t.Error("normal frame reported as flash")
	}
	fillRect(img, img.Rect, color.RGBA{255, 255, 255, 255})
	if _, flash := NewLine(s.board, 1).Read(img); !flash {
		t.Error("white frame not reported as flash")
	}
}

func TestPresent(t *testing.T) {
	s := newScene(1200, 700, 400, 600, 100)
	img := s.render()
	b, err := FindBoard(img)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Present(img) {
		t.Fatal("board not present on its own frame")
	}
	if !ManualBoard(image.Pt(400, 600), image.Pt(800, 600)).Present(img) {
		t.Error("manual board not present on its own frame")
	}
	moved := b
	moved.Y -= 150
	if moved.Present(img) {
		t.Error("board present after moving it away from the rings")
	}
}

func TestLanesConverge(t *testing.T) {
	b := Board{X0: 100, Y: 500, Spacing: 100}
	if got := b.LaneX(2, 2); got != b.FretX(2) {
		t.Errorf("centre lane moved to %v", got)
	}
	if got, want := b.LaneX(0, VanishHeight), b.FretX(2); math.Abs(got-want) > 1e-9 {
		t.Errorf("lanes meet at x=%v, want %v", got, want)
	}
}

func TestFindBoardNextToColourfulBackground(t *testing.T) {
	// A bright orange photo right next to the highway merges with the orange
	// ring; the other four rings are still enough.
	s := newScene(1400, 900, 400, 700, 90)
	img := s.render()
	right := int(s.board.FretX(4) + 0.45*s.board.Spacing)
	fillRect(img, image.Rect(right, 0, 1400, 900), laneRGB[4])

	b, err := FindBoard(img)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(b.X0-400) > 4 || math.Abs(b.Spacing-90) > 1.8 {
		t.Errorf("got %v, want frets from x=400 with spacing 90", b)
	}
}
