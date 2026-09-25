// Command mkicon draws the app icon: a small fretboard with the five fret
// rings and a couple of notes, on a rounded square. It writes a 1024x1024
// PNG; the build turns it into the Windows and macOS icon formats.
//
//	go run ./tools/mkicon assets/icon.png
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const (
	size = 1024
	ss   = 4 // supersampling for smooth edges
)

var frets = [5]color.RGBA{
	{11, 151, 18, 255}, {238, 14, 13, 255}, {225, 226, 20, 255}, {17, 146, 227, 255}, {238, 99, 16, 255},
}

type canvas struct {
	w   int
	pix []color.RGBA
}

func (c *canvas) blend(x, y int, col color.RGBA, a float64) {
	if x < 0 || y < 0 || x >= c.w || y >= c.w || a <= 0 {
		return
	}
	p := &c.pix[y*c.w+x]
	mix := func(dst, src uint8) uint8 { return uint8(float64(dst)*(1-a) + float64(src)*a) }
	p.R, p.G, p.B = mix(p.R, col.R), mix(p.G, col.G), mix(p.B, col.B)
	if na := float64(p.A)/255*(1-a) + a; na > 0 {
		p.A = uint8(na * 255)
	}
}

// fill paints every pixel for which inside(x, y) is true, in icon units
// (0..1024).
func (c *canvas) fill(col color.RGBA, a float64, inside func(x, y float64) bool) {
	for y := 0; y < c.w; y++ {
		for x := 0; x < c.w; x++ {
			if inside((float64(x)+0.5)/ss, (float64(y)+0.5)/ss) {
				c.blend(x, y, col, a)
			}
		}
	}
}

func ellipse(cx, cy, rx, ry float64) func(x, y float64) bool {
	return func(x, y float64) bool {
		dx, dy := (x-cx)/rx, (y-cy)/ry
		return dx*dx+dy*dy <= 1
	}
}

func main() {
	out := "assets/icon.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	c := &canvas{w: size * ss, pix: make([]color.RGBA, size*size*ss*ss)}

	// Rounded square, with the margin macOS expects around an icon.
	const m, r = 100.0, 185.0
	inIcon := func(x, y float64) bool {
		if x < m || y < m || x > size-m || y > size-m {
			return false
		}
		cx := math.Max(m+r, math.Min(size-m-r, x))
		cy := math.Max(m+r, math.Min(size-m-r, y))
		return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
	}
	c.fill(color.RGBA{24, 24, 34, 255}, 1, inIcon)

	// The highway, narrowing towards the top like in the game.
	const top, bottom, frY = 190.0, 924.0, 760.0
	halfAt := func(y float64) float64 { return 70 + (y-top)/(bottom-top)*330 }
	c.fill(color.RGBA{58, 58, 74, 255}, 1, func(x, y float64) bool {
		return y >= top && y <= bottom && math.Abs(x-512) <= halfAt(y) && inIcon(x, y)
	})
	c.fill(color.RGBA{235, 235, 240, 255}, 1, func(x, y float64) bool {
		d := math.Abs(math.Abs(x-512) - halfAt(y))
		return y >= top && y <= bottom && d <= 9 && inIcon(x, y)
	})
	laneX := func(k int, y float64) float64 { return 512 + float64(k-2)*halfAt(y)/2.5 }

	// Fret rings.
	for k := 0; k < 5; k++ {
		cx := laneX(k, frY)
		c.fill(frets[k], 1, ellipse(cx, frY, 60, 30))
		c.fill(color.RGBA{16, 16, 20, 255}, 1, ellipse(cx, frY, 38, 15))
	}
	// Two notes on their way down: gem with a white cap.
	for _, n := range []struct {
		k int
		y float64
	}{{1, 590}, {3, 420}} {
		cx := laneX(n.k, n.y)
		s := halfAt(n.y) / halfAt(frY)
		c.fill(frets[n.k], 1, ellipse(cx, n.y, 58*s, 28*s))
		c.fill(color.RGBA{245, 245, 245, 255}, 1, ellipse(cx, n.y-11*s, 26*s, 11*s))
	}

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					p := c.pix[(y*ss+sy)*c.w+x*ss+sx]
					pa := float64(p.A) / 255
					r += float64(p.R) * pa
					g += float64(p.G) * pa
					b += float64(p.B) * pa
					a += pa
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(a / ss / ss * 255)})
		}
	}
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
