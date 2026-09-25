package vision

import (
	"image"
	"image/color"
	"math"
)

// ManualBoard builds a board from the centres of the green and orange frets,
// for when automatic detection does not work.
func ManualBoard(green, orange image.Point) Board {
	s := float64(orange.X-green.X) / 4
	return Board{
		X0:      float64(green.X),
		Y:       float64(green.Y+orange.Y) / 2,
		Spacing: s,
		RingW:   s,
		RingH:   0.33 * s,
		Found:   5,
	}
}

var (
	overlayFret  = color.RGBA{0, 255, 255, 255}
	overlayLane  = color.RGBA{255, 255, 255, 255}
	overlayLine  = color.RGBA{255, 0, 255, 255}
	overlayProbe = color.RGBA{128, 128, 128, 255}
)

// DrawOverlay marks what the player looks at: fret centres (cyan), lane
// paths (white), lane sensors (magenta) and flash probes (grey).
func DrawOverlay(img *image.RGBA, b Board, lines ...Line) {
	top := 0.0
	for _, l := range lines {
		top = math.Max(top, l.Height)
	}
	top += 0.5
	for k := 0; k < 5; k++ {
		for h := -0.3; h <= top; h += 0.25 / b.Spacing {
			dot(img, b.LaneX(k, h), b.RowY(h), 0, overlayLane)
		}
		cx, r := b.FretX(k), math.Max(3, b.Spacing/12)
		for d := -r; d <= r; d++ {
			dot(img, cx+d, b.Y, 1, overlayFret)
			dot(img, cx, b.Y+d, 1, overlayFret)
		}
	}
	for _, l := range lines {
		for _, rows := range l.lanes {
			top, bottom := rows[0], rows[len(rows)-1]
			for x := top.x0; x <= top.x1; x++ {
				dot(img, float64(x), float64(top.y-1), 0, overlayLine)
			}
			for x := bottom.x0; x <= bottom.x1; x++ {
				dot(img, float64(x), float64(bottom.y+1), 0, overlayLine)
			}
		}
		for _, s := range l.probes {
			for x := s.x0; x <= s.x1; x++ {
				dot(img, float64(x), float64(s.y), 0, overlayProbe)
			}
		}
	}
}

func dot(img *image.RGBA, x, y float64, r int, c color.RGBA) {
	cx, cy := int(math.Round(x)), int(math.Round(y))
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if p := image.Pt(cx+dx, cy+dy); p.In(img.Rect) {
				img.SetRGBA(p.X, p.Y, c)
			}
		}
	}
}
