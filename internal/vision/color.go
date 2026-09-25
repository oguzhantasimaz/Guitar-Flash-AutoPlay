// Package vision finds the Guitar Flash fretboard in a screenshot and reads
// the note lanes. It is pure Go and works on any image, so everything here
// can be tested without a screen.
package vision

import "image"

// Class is the colour class of a single pixel.
type Class uint8

const (
	None Class = iota
	Green
	Red
	Yellow
	Blue
	Orange
)

// FretColors lists the fret colours from left to right.
var FretColors = [5]Class{Green, Red, Yellow, Blue, Orange}

func (c Class) String() string {
	switch c {
	case Green:
		return "green"
	case Red:
		return "red"
	case Yellow:
		return "yellow"
	case Blue:
		return "blue"
	case Orange:
		return "orange"
	}
	return "none"
}

// hsv returns hue in degrees [0,360), saturation and value in [0,255].
func hsv(r, g, b uint8) (h int, s, v int) {
	mx, mn := int(r), int(r)
	if int(g) > mx {
		mx = int(g)
	}
	if int(b) > mx {
		mx = int(b)
	}
	if int(g) < mn {
		mn = int(g)
	}
	if int(b) < mn {
		mn = int(b)
	}
	v = mx
	d := mx - mn
	if mx == 0 || d == 0 {
		return 0, 0, v
	}
	s = d * 255 / mx
	switch mx {
	case int(r):
		h = 60 * (int(g) - int(b)) / d
	case int(g):
		h = 120 + 60*(int(b)-int(r))/d
	default:
		h = 240 + 60*(int(r)-int(g))/d
	}
	if h < 0 {
		h += 360
	}
	return h, s, v
}

// Classify returns the fret colour of a pixel, or None. The thresholds were
// measured on the fret rings of the HTML5 game: green (11,151,18),
// red (238,14,13), yellow (225,226,20), blue (17,146,227), orange (238,99,16).
func Classify(r, g, b uint8) Class {
	h, s, v := hsv(r, g, b)
	if s < 140 || v < 115 {
		return None
	}
	switch {
	case h < 12 || h >= 340:
		return Red
	case h < 42:
		return Orange
	case h >= 45 && h < 75:
		return Yellow
	case h >= 90 && h < 160:
		return Green
	case h >= 185 && h < 230:
		return Blue
	}
	return None
}

// isNote reports whether a pixel looks like part of a note gem: either a
// bright saturated colour or the white highlight on top of the gem. The
// highway itself is a dark translucent overlay (around 33,33,33), so the band
// photo behind it never gets this bright.
func isNote(r, g, b uint8) bool {
	_, s, v := hsv(r, g, b)
	if v >= 125 && s >= 115 {
		return true
	}
	mn := r
	if g < mn {
		mn = g
	}
	if b < mn {
		mn = b
	}
	return mn >= 170
}

// isBright is used to spot the full-screen white flash the game plays on
// special effects: anything clearly lighter than the dark highway counts.
func isBright(r, g, b uint8) bool {
	_, _, v := hsv(r, g, b)
	return v >= 150
}

func rgbAt(img *image.RGBA, x, y int) (uint8, uint8, uint8, bool) {
	if !(image.Point{x, y}.In(img.Rect)) {
		return 0, 0, 0, false
	}
	i := img.PixOffset(x, y)
	return img.Pix[i], img.Pix[i+1], img.Pix[i+2], true
}
