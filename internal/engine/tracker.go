package engine

import (
	"math"
	"time"
)

// tracker follows the gems of one lane through a sensor column (see
// vision.Column) from frame to frame and reports when each one's leading
// edge crosses the sensor line. Gems only ever move down, by about
// speed × frame time, which is enough to tell which gem is which even in
// a stream where several are in view at once. The crossing time is
// interpolated between the two frames around it, so it is more precise than
// the frame rate.
type tracker struct {
	prev  []seen
	at    time.Time
	valid bool
}

// seen is a gem's leading edge at the last frame. Ghosts are gems that were
// not found in the latest frame; they are kept, at their predicted place,
// for one frame in case the reading just flickered.
type seen struct {
	h     float64
	ghost bool
}

// update takes the leading-edge heights of the gems in view (lowest first)
// at time t and returns when gems crossed the line at height line since
// the previous frame. speed is the expected speed in spacings per second,
// only used when the gems themselves cannot tell how far they moved.
func (tr *tracker) update(t time.Time, gems []float64, line, speed float64) []time.Time {
	if !tr.valid {
		tr.set(t, gems, nil)
		return nil
	}
	dt := t.Sub(tr.at)
	move := tr.shift(gems, speed*dt.Seconds())
	used := make([]bool, len(gems))
	var crossed []time.Time
	var ghosts []float64
	for _, p := range tr.prev {
		if p.h <= line {
			continue // already past the line
		}
		c := p.h - move
		if i := nearest(gems, used, c); i >= 0 {
			used[i], c = true, gems[i]
		} else if !p.ghost && c > line {
			ghosts = append(ghosts, c)
		}
		if c <= line {
			frac := 1.0
			if p.h > c {
				frac = (p.h - line) / (p.h - c)
			}
			crossed = append(crossed, tr.at.Add(time.Duration(frac*float64(dt))))
		}
	}
	tr.set(t, gems, ghosts)
	return crossed
}

// matchTol is how far a gem may be from where it should be, in spacings.
const matchTol = 0.04

// shift finds how far the gems moved down since the last frame: the
// distance that lines up the most of them, the smallest one if several do
// (in a stream, moving by one more gem lines them up too). Without any gem
// to go by it returns the expected distance.
func (tr *tracker) shift(gems []float64, expected float64) float64 {
	best, bestScore := expected, 0
	limit := math.Max(3*expected, 0.5)
	for _, p := range tr.prev {
		if p.ghost {
			continue
		}
		for _, c := range gems {
			d := p.h - c
			if d < -matchTol || d > limit {
				continue
			}
			score := 0
			for _, q := range tr.prev {
				if !q.ghost && nearest(gems, nil, q.h-d) >= 0 {
					score++
				}
			}
			if score > bestScore || (score == bestScore && d < best) {
				best, bestScore = d, score
			}
		}
	}
	return math.Max(best, 0)
}

// nearest returns the index of the unused gem closest to h, if one is
// within matchTol.
func nearest(gems []float64, used []bool, h float64) int {
	best, bestD := -1, matchTol
	for i, g := range gems {
		if used != nil && used[i] {
			continue
		}
		if d := math.Abs(g - h); d <= bestD {
			best, bestD = i, d
		}
	}
	return best
}

func (tr *tracker) set(t time.Time, gems, ghosts []float64) {
	tr.prev = tr.prev[:0]
	for _, h := range gems {
		tr.prev = append(tr.prev, seen{h: h})
	}
	for _, h := range ghosts {
		tr.prev = append(tr.prev, seen{h: h, ghost: true})
	}
	tr.at, tr.valid = t, true
}
