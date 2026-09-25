package vision

import (
	"image"
	"math"
)

// segment is a horizontal run of pixels on one screen row.
type segment struct{ x0, x1, y int }

func (s segment) coverage(img *image.RGBA, match func(r, g, b uint8) bool) float64 {
	n, hit := 0, 0
	for x := s.x0; x <= s.x1; x++ {
		r, g, b, ok := rgbAt(img, x, s.y)
		if !ok {
			continue
		}
		n++
		if match(r, g, b) {
			hit++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(hit) / float64(n)
}

// Line is a sensor across all five lanes at a fixed height above the frets.
// For every lane it measures how much of the lane centre is covered by a
// note gem: a gem fills it almost completely, while the thin tail of a
// sustained note only covers a small part.
//
// Each lane sensor is a short band of rows reaching down from Height. At
// high speed a gem moves further per screen frame than its own height, so a
// single row could miss it entirely. Gems come from above, so they still
// reach the sensor at Height first.
type Line struct {
	Height float64 // top of the sensor, in fret spacings above the fret line
	lanes  [5][]segment
	probes [4]segment // between the lanes, used to spot full-screen flashes
}

// LaneHalfWidth is half the width of a lane sensor, relative to the lane
// spacing at that height. Gems are about 0.7 lanes wide, tails about 0.1.
const LaneHalfWidth = 0.3

// BandHeight is how far each lane sensor reaches down from its Height, in
// fret spacings.
const BandHeight = 0.15

// NewLine places a sensor line on the board at height h.
func NewLine(b Board, h float64) Line {
	l := Line{Height: h}
	rows := 4
	for i := 0; i < rows; i++ {
		rh := h - BandHeight*float64(i)/float64(rows-1)
		y := int(math.Round(b.RowY(rh)))
		hw := math.Max(1, LaneHalfWidth*b.Spacing*b.Scale(rh))
		for k := 0; k < 5; k++ {
			if n := len(l.lanes[k]); n > 0 && l.lanes[k][n-1].y == y {
				continue // tiny boards: rows closer than a pixel
			}
			cx := b.LaneX(k, rh)
			l.lanes[k] = append(l.lanes[k], segment{int(math.Round(cx - hw)), int(math.Round(cx + hw)), y})
		}
	}
	y := int(math.Round(b.RowY(h)))
	pw := math.Max(1, 0.05*b.Spacing*b.Scale(h))
	for k := 0; k < 4; k++ {
		cx := (b.LaneX(k, h) + b.LaneX(k+1, h)) / 2
		l.probes[k] = segment{int(math.Round(cx - pw)), int(math.Round(cx + pw)), y}
	}
	return l
}

// Read returns the gem coverage (0..1) of every lane, the highest of its
// rows, and whether the frame looks like a full-screen flash, in which case
// the coverage is meaningless.
func (l Line) Read(img *image.RGBA) (cov [5]float64, flash bool) {
	for k, rows := range l.lanes {
		for _, s := range rows {
			cov[k] = math.Max(cov[k], s.coverage(img, isNote))
		}
	}
	bright := 0.0
	for _, s := range l.probes {
		bright += s.coverage(img, isBright)
	}
	return cov, bright/float64(len(l.probes)) > 0.6
}

// Bounds is the smallest rectangle containing every pixel the line reads.
func (l Line) Bounds() image.Rectangle {
	r := image.Rectangle{}
	add := func(s segment) { r = r.Union(image.Rect(s.x0, s.y, s.x1+1, s.y+1)) }
	for _, rows := range l.lanes {
		for _, s := range rows {
			add(s)
		}
	}
	for _, s := range l.probes {
		add(s)
	}
	return r
}

// Present checks that the fret rings are still where the board says, so the
// player can notice when the game was scrolled, resized or closed. A ring
// counts when any point on its outline still has its colour; notes and hit
// effects can hide one or two rings for a moment.
func (b Board) Present(img *image.RGBA) bool {
	seen := 0
	rx, ry := b.RingW/2, b.RingH/2
	for k, want := range FretColors {
		cx := b.FretX(k)
		pts := [][2]float64{{cx - rx, b.Y}, {cx + rx, b.Y}, {cx, b.Y - ry}, {cx, b.Y + ry}}
		ok := false
		for _, p := range pts {
			// The outline is only a few pixels thick; search a little
			// inwards from the outer edge.
			for d := 0.0; d <= 0.12 && !ok; d += 0.02 {
				x := cx + (p[0]-cx)*(1-d*2)
				y := b.Y + (p[1]-b.Y)*(1-d*2)
				if r, g, bb, in := rgbAt(img, int(math.Round(x)), int(math.Round(y))); in && Classify(r, g, bb) == want {
					ok = true
				}
			}
		}
		if ok {
			seen++
		}
	}
	return seen >= 3
}
