package vision

import (
	"image"
	"math"
)

// Column watches a stretch of every lane around a sensor height and finds
// the gems in it. A gem shows up as a run of rows in the lane's colour (its
// body); the white cap on top and the dark highway separate one gem from
// the next, even in the fastest streams where gems almost touch. Finding
// gems by position within one frame, rather than waiting for a sensor to
// clear, keeps every note of a stream apart at any frame rate.
type Column struct {
	Height float64 // the sensor line the gems are timed at
	rows   [5][]segment
	hs     [5][]float64 // height of each row, in spacings
	minRun int          // shortest run of rows that counts as a gem body
}

// Column window around the sensor line, in fret spacings. It stops short
// of the flame drawn on a hit, which reaches 0.8 spacings up: in the yellow
// and orange lanes it has the lane's colour.
const (
	columnAbove = 0.45
	columnBelow = 0.2
)

// NewColumn places a column on the board around height h.
func NewColumn(b Board, h float64) Column {
	c := Column{Height: h}
	top, bottom := int(math.Round(b.RowY(h+columnAbove))), int(math.Round(b.RowY(h-columnBelow)))
	for y := top; y <= bottom; y++ {
		rh := (b.Y - float64(y)) / b.Spacing
		hw := math.Max(1, 0.2*b.Spacing*b.Scale(rh))
		for k := 0; k < 5; k++ {
			cx := b.LaneX(k, rh)
			c.rows[k] = append(c.rows[k], segment{int(math.Round(cx - hw)), int(math.Round(cx + hw)), y})
			c.hs[k] = append(c.hs[k], rh)
		}
	}
	// A gem body is about 0.15 spacings tall; anything much shorter is an
	// edge of something else.
	c.minRun = int(math.Max(1, math.Round(0.05*b.Spacing)))
	return c
}

// Read returns, for every lane, the heights of the gems' leading (bottom)
// edges inside the column, lowest first.
func (c Column) Read(img *image.RGBA) (gems [5][]float64) {
	for k := 0; k < 5; k++ {
		col := FretColors[k]
		body := func(r, g, b uint8) bool { return isBody(col, r, g, b) }
		run := 0
		// Walk from the bottom up so each gem is found at its bottom edge.
		for i := len(c.rows[k]) - 1; i >= -1; i-- {
			on := i >= 0 && c.rows[k][i].coverage(img, body) >= 0.5
			if on {
				run++
				continue
			}
			// A body cut off by the bottom of the column has passed the
			// line already, and its edge there is not its real edge.
			if bottom := i + run; run >= c.minRun && bottom < len(c.rows[k])-1 {
				gems[k] = append(gems[k], c.hs[k][bottom])
			}
			run = 0
		}
	}
	return gems
}

// Bounds is the smallest rectangle containing every pixel the column reads.
func (c Column) Bounds() image.Rectangle {
	r := image.Rectangle{}
	for _, rows := range c.rows {
		for _, s := range rows {
			r = r.Union(image.Rect(s.x0, s.y, s.x1+1, s.y+1))
		}
	}
	return r
}
