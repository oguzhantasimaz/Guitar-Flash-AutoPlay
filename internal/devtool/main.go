// Temporary diagnostics against the real game (not committed): a
// time/height picture of each lane, with the sensor bands marked.
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"time"

	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/engine"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/platform"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/vision"
)

func main() {
	ds, _ := platform.Displays()
	img, _ := platform.Capture(ds[0])
	b, err := vision.FindBoard(img)
	if err != nil { panic(err) }
	p := engine.DefaultParams()
	dur, _ := time.ParseDuration(os.Args[1])
	const hTop, hBot = 2.4, 0.3
	y0, y1 := int(b.RowY(hTop)), int(b.RowY(hBot))
	region := image.Rect(int(b.FretX(0)-b.Spacing), y0, int(b.FretX(4)+b.Spacing), y1)
	var cols [5][][]color.RGBA
	var times []time.Duration
	start := time.Now()
	for time.Since(start) < dur {
		t := time.Since(start)
		im, err := platform.Capture(region)
		if err != nil { continue }
		times = append(times, t)
		for k := 0; k < 5; k++ {
			col := make([]color.RGBA, y1-y0)
			for y := y0; y < y1; y++ {
				h := (b.Y - float64(y)) / b.Spacing
				col[y-y0] = im.RGBAAt(int(b.LaneX(k, h)), y)
			}
			cols[k] = append(cols[k], col)
		}
		time.Sleep(4 * time.Millisecond)
	}
	// One picture per lane: 2 px per sample in time, sensor bands in magenta.
	for k := 0; k < 5; k++ {
		out := image.NewRGBA(image.Rect(0, 0, 2*len(cols[k]), y1-y0))
		for x, col := range cols[k] {
			for y, c := range col {
				out.Set(2*x, y, c); out.Set(2*x+1, y, c)
			}
		}
		for _, h := range []float64{p.Upper, p.Upper - vision.BandHeight, p.Lower, p.Lower - vision.BandHeight} {
			y := int(b.RowY(h)) - y0
			for x := 0; x < out.Rect.Dx(); x += 6 { out.Set(x, y, color.RGBA{255, 0, 255, 255}) }
		}
		f, _ := os.Create("kymo" + string(rune('0'+k)) + ".png"); png.Encode(f, out); f.Close()
	}
}
