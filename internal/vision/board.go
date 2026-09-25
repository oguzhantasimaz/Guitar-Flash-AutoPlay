package vision

import (
	"errors"
	"fmt"
	"image"
	"math"
	"sort"
)

// VanishHeight is how far above the fret line the highway's edges meet,
// measured in fret spacings (the distance between two neighbouring fret
// centres). The game draws into a fixed 822x580 canvas that the browser only
// scales, so this ratio is the same at every resolution and zoom level.
// Measured from the highway borders: they meet 442px above the frets, with
// the frets 99.6px apart.
const VanishHeight = 4.44

// Board is the fretboard as found on screen. All coordinates are in the pixel
// space of the image it was detected in.
type Board struct {
	X0      float64 // centre of the green fret
	Y       float64 // centre line of the fret rings
	Spacing float64 // distance between neighbouring fret centres
	RingW   float64 // outer width of a fret ring
	RingH   float64 // outer height of a fret ring
	Found   int     // how many of the 5 rings were seen during detection
}

// FretX returns the centre of fret k (0 = green ... 4 = orange).
func (b Board) FretX(k int) float64 { return b.X0 + float64(k)*b.Spacing }

// Scale is how much the highway has shrunk at height h (in spacings above
// the fret line) because of the perspective.
func (b Board) Scale(h float64) float64 { return 1 - h/VanishHeight }

// LaneX is the centre of lane k at height h above the fret line. The lanes
// converge towards the vanishing point above the yellow fret.
func (b Board) LaneX(k int, h float64) float64 {
	return b.FretX(2) + (b.FretX(k)-b.FretX(2))*b.Scale(h)
}

// RowY is the screen row at height h above the fret line.
func (b Board) RowY(h float64) float64 { return b.Y - h*b.Spacing }

func (b Board) String() string {
	return fmt.Sprintf("frets at y=%.0f, x=%.0f..%.0f (spacing %.1fpx)",
		b.Y, b.FretX(0), b.FretX(4), b.Spacing)
}

// ErrNotFound means no fretboard is visible in the image.
var ErrNotFound = errors.New("fretboard not found")

type blob struct {
	class                  Class
	minX, minY, maxX, maxY int
	count                  int
}

func (c blob) w() float64  { return float64(c.maxX - c.minX + 1) }
func (c blob) h() float64  { return float64(c.maxY - c.minY + 1) }
func (c blob) cx() float64 { return float64(c.minX+c.maxX) / 2 }
func (c blob) cy() float64 { return float64(c.minY+c.maxY) / 2 }

// ringLike keeps blobs shaped like a fret ring: a flat ellipse outline.
func (c blob) ringLike() bool {
	w, h := c.w(), c.h()
	if w < 10 || h < 4 || c.count < 20 {
		return false
	}
	aspect := w / h
	fill := float64(c.count) / (w * h)
	return aspect >= 1.4 && aspect <= 6 && fill <= 0.65
}

// FindBoard looks for the five fret rings (green, red, yellow, blue, orange,
// evenly spaced on one line) anywhere in img.
func FindBoard(img *image.RGBA) (Board, error) {
	blobs := findBlobs(img)

	var cands [5][]blob
	for _, c := range blobs {
		if c.ringLike() {
			k := int(c.class) - 1
			cands[k] = append(cands[k], c)
		}
	}
	for k := range cands {
		// Keep the search cheap on busy screens: the rings are large.
		sort.Slice(cands[k], func(i, j int) bool { return cands[k][i].w() > cands[k][j].w() })
		if len(cands[k]) > 60 {
			cands[k] = cands[k][:60]
		}
	}

	best := Board{}
	bestErr := math.Inf(1)
	// Any two rings define a candidate line; the others must line up with it.
	// Using every pair means one covered-up ring does not break detection.
	for i := 0; i < 5; i++ {
		for j := i + 1; j < 5; j++ {
			for _, a := range cands[i] {
				for _, b := range cands[j] {
					s := (b.cx() - a.cx()) / float64(j-i)
					if s < 12 || math.Abs(b.cy()-a.cy()) > 0.15*s {
						continue
					}
					if r := a.w() / b.w(); r < 0.75 || r > 1.33 {
						continue
					}
					if a.w() < 0.7*s || a.w() > 1.2*s {
						continue
					}
					board, fitErr, ok := fitBoard(cands, a, i, s)
					if ok && (board.Found > best.Found || (board.Found == best.Found && fitErr < bestErr)) {
						best, bestErr = board, fitErr
					}
				}
			}
		}
	}
	if best.Found < 4 {
		return Board{}, ErrNotFound
	}
	return best, nil
}

// fitBoard collects the ring of every colour closest to where it should be,
// given ring a at index i and spacing s, then least-squares fits the line.
func fitBoard(cands [5][]blob, a blob, i int, s float64) (Board, float64, bool) {
	var picked [5]*blob
	found := 0
	for k := 0; k < 5; k++ {
		ex := a.cx() + float64(k-i)*s
		best := math.Inf(1)
		for n := range cands[k] {
			c := &cands[k][n]
			dx, dy := c.cx()-ex, c.cy()-a.cy()
			if math.Abs(dx) > 0.15*s || math.Abs(dy) > 0.15*s || math.Abs(c.w()-a.w()) > 0.25*a.w() {
				continue
			}
			if d := dx*dx + dy*dy; d < best {
				best, picked[k] = d, c
			}
		}
		if picked[k] != nil {
			found++
		}
	}
	if found < 4 {
		return Board{}, 0, false
	}

	// x = x0 + k*spacing, fitted over the rings we found.
	var n, sk, sx, skk, skx float64
	var tops, bottoms, ws []float64
	for k, c := range picked {
		if c == nil {
			continue
		}
		fk := float64(k)
		n++
		sk += fk
		sx += c.cx()
		skk += fk * fk
		skx += fk * c.cx()
		tops = append(tops, float64(c.minY))
		bottoms = append(bottoms, float64(c.maxY))
		ws = append(ws, c.w())
	}
	spacing := (n*skx - sk*sx) / (n*skk - sk*sk)
	x0 := (sx - spacing*sk) / n
	// A note sitting on a ring merges with it and stretches its box
	// downwards, so take the median top and bottom edge separately.
	top, bottom := median(tops), median(bottoms)
	b := Board{X0: x0, Y: (top + bottom) / 2, Spacing: spacing, RingW: median(ws), RingH: bottom - top + 1, Found: found}

	var errSum float64
	for k, c := range picked {
		if c == nil {
			continue
		}
		dx, dy := c.cx()-b.FretX(k), c.cy()-b.Y
		errSum += dx*dx + dy*dy
	}
	return b, errSum / (n * spacing * spacing), true
}

func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// findBlobs labels 8-connected regions of the same fret colour. Diagonal
// neighbours matter: at small sizes a ring outline is a 1px staircase.
func findBlobs(img *image.RGBA) []blob {
	r := img.Rect
	w, h := r.Dx(), r.Dy()
	cls := make([]Class, w*h)
	for y := 0; y < h; y++ {
		row := img.Pix[(y)*img.Stride:]
		for x := 0; x < w; x++ {
			p := row[x*4 : x*4+3]
			cls[y*w+x] = Classify(p[0], p[1], p[2])
		}
	}

	var blobs []blob
	var stack []int
	for start, c := range cls {
		if c == None {
			continue
		}
		b := blob{class: c, minX: w, minY: h, maxX: -1, maxY: -1}
		cls[start] = None
		stack = append(stack[:0], start)
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := i%w, i/w
			b.count++
			b.minX, b.maxX = min(b.minX, x), max(b.maxX, x)
			b.minY, b.maxY = min(b.minY, y), max(b.maxY, y)
			for ny := max(y-1, 0); ny <= min(y+1, h-1); ny++ {
				for nx := max(x-1, 0); nx <= min(x+1, w-1); nx++ {
					if j := ny*w + nx; cls[j] == c {
						cls[j] = None
						stack = append(stack, j)
					}
				}
			}
		}
		b.minX += r.Min.X
		b.maxX += r.Min.X
		b.minY += r.Min.Y
		b.maxY += r.Min.Y
		blobs = append(blobs, b)
	}
	return blobs
}
