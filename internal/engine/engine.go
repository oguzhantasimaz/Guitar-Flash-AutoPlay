// Package engine turns lane readings into timed key presses.
//
// Two sensor lines sit above the frets. Every note crosses the upper line
// first and the lower line a little later; the time between the two tells
// how fast the song scrolls, so the engine knows when the note will reach
// the frets and can press the key at that moment. Measuring instead of
// hard-coding a delay makes it work on every difficulty, screen size and
// computer speed.
//
// Notes do not move at a constant speed over the whole highway: the game
// draws them in perspective, so they are slow at the far end and speed up
// towards the frets. What the engine needs is only the last stretch, so it
// uses a ratio measured on the game, Reach: the time from the lower line to
// the moment a note must be hit, divided by the time from the upper line to
// the lower one. It does not depend on the screen size or the scroll speed,
// only on where the sensors are.
package engine

import (
	"math"
	"sort"
	"time"
)

// Params tune the engine. Heights are in fret spacings above the fret line.
type Params struct {
	Upper, Lower float64
	// Reach is how long a note needs from the lower line until it has to be
	// hit, in units of its travel time from the upper line to the lower one
	// (see the package documentation). It belongs to Upper and Lower.
	Reach float64
	// Lead moves every press this much before the moment the note looks
	// centred on its fret. The game counts a note as hit a bit before that,
	// and the screenshot is a few milliseconds old by the time it is read.
	// Measured on the game: the best results come at about 120 ms, on every
	// difficulty (the old hard-coded version pressed about 100 ms early too).
	Lead time.Duration
	// Offset is a manual correction: positive presses later.
	Offset time.Duration
	// DefaultTravel is the upper-to-lower travel time used until the first
	// note has been measured.
	DefaultTravel time.Duration
	// MinPress is the shortest time a key is held down.
	MinPress time.Duration
	// Hold keeps keys down through the tail of sustained notes.
	Hold bool
}

// DefaultParams are tuned on the HTML5 version of guitarflash.com.
func DefaultParams() Params {
	return Params{
		// The flame of a hit note reaches 0.8 spacings up; stay above it.
		Upper:         1.9,
		Lower:         1.05,
		Reach:         1.45,
		Lead:          120 * time.Millisecond,
		DefaultTravel: 230 * time.Millisecond,
		MinPress:      25 * time.Millisecond,
		Hold:          true,
	}
}

// Action is a key going down or up at a given time.
type Action struct {
	At   time.Time
	Lane int
	Down bool
}

// Engine consumes one reading per captured frame. It is not safe for
// concurrent use.
type Engine struct {
	p      Params
	upper  [5]detector
	lower  [5]detector
	queue  [5][]time.Time // upper-line crossings waiting for their lower one
	travel estimate       // upper-to-lower travel time, in seconds
	down   [5]bool
	downAt [5]time.Time
	lastUp [5]time.Time
}

// New returns an engine with the given parameters.
func New(p Params) *Engine { return &Engine{p: p} }

// Params returns the engine's parameters.
func (e *Engine) Params() Params { return e.p }

// Reset forgets all notes and the measured speed, e.g. for a new song. Keys
// that are still down should be released with ReleaseAll first.
func (e *Engine) Reset() { *e = Engine{p: e.p} }

// Travel returns the measured time a note takes from the upper line to the
// lower one, and whether it has been measured yet.
func (e *Engine) Travel() (time.Duration, bool) {
	if t, ok := e.travel.value(); ok {
		return seconds(t), true
	}
	return e.p.DefaultTravel, false
}

// Speed returns how fast notes move between the sensor lines, in spacings
// per second, and whether it has been measured yet.
func (e *Engine) Speed() (float64, bool) {
	t, ok := e.Travel()
	return (e.p.Upper - e.p.Lower) / t.Seconds(), ok
}

// delay is the time from a note crossing the lower line until the key
// should go down.
func (e *Engine) delay() time.Duration {
	t, _ := e.Travel()
	return time.Duration(float64(t)*e.p.Reach) - e.p.Lead + e.p.Offset
}

// maxHold releases a key that has been held suspiciously long, in case a
// sensor gets stuck on something that is not a note.
const maxHold = 8 * time.Second

// Update processes one frame captured at time t. upper and lower are the
// gem coverage of each lane on the two sensor lines. Frames that are
// flashing white are skipped.
func (e *Engine) Update(t time.Time, upper, lower [5]float64, flash bool) []Action {
	if flash {
		return nil
	}
	v, _ := e.Speed()
	cfg := detectorConfig{
		grace:  maxDur(35*time.Millisecond, seconds(0.25/v)),
		minGap: seconds(0.12 / v),
	}
	var acts []Action
	for k := 0; k < 5; k++ {
		for _, ev := range e.upper[k].update(t, upper[k], cfg) {
			if ev.start {
				if len(e.queue[k]) >= 16 {
					e.queue[k] = e.queue[k][1:] // the lower line missed these
				}
				e.queue[k] = append(e.queue[k], ev.at)
			}
		}
		for _, ev := range e.lower[k].update(t, lower[k], cfg) {
			if ev.start {
				e.measure(k, ev.at)
				acts = append(acts, e.press(k, ev.at)...)
			} else {
				acts = append(acts, e.release(k, ev.at)...)
			}
		}
		if e.down[k] && t.Sub(e.downAt[k]) > maxHold {
			acts = append(acts, e.forceUp(k, t))
			e.lower[k] = detector{}
		}
	}
	return acts
}

// ReleaseAll returns key-up actions at time t for every key still down.
func (e *Engine) ReleaseAll(t time.Time) []Action {
	var acts []Action
	for k := range e.down {
		if e.down[k] {
			acts = append(acts, e.forceUp(k, t))
		}
	}
	return acts
}

func (e *Engine) forceUp(k int, t time.Time) Action {
	e.down[k] = false
	e.lastUp[k] = t
	return Action{At: t, Lane: k, Down: false}
}

// measure pairs a lower-line crossing with the upper-line crossing of the
// same note and records the travel time.
func (e *Engine) measure(k int, t time.Time) {
	const minTravel = 15 * time.Millisecond
	maxTravel := 1500 * time.Millisecond
	expected := time.Duration(0)
	if tr, ok := e.travel.value(); ok {
		expected = seconds(tr)
		maxTravel = expected * 5 / 2
	}

	q := e.queue[k]
	for len(q) > 0 && t.Sub(q[0]) > maxTravel {
		q = q[1:]
	}
	pick := -1
	for i, tu := range q {
		d := t.Sub(tu)
		if d < minTravel {
			break
		}
		// Without a speed yet, the oldest crossing is this note. With one,
		// take the crossing that fits it best, so a single missed reading
		// does not shift every pairing after it.
		if pick < 0 || (expected > 0 && absDur(d-expected) < absDur(t.Sub(q[pick])-expected)) {
			pick = i
		}
		if expected == 0 {
			break
		}
	}
	if pick >= 0 {
		e.travel.add(t.Sub(q[pick]).Seconds())
		q = q[pick+1:]
	}
	e.queue[k] = q
}

// releaseGap is how long a key is let go before the same key is pressed for
// the next note.
const releaseGap = 12 * time.Millisecond

func (e *Engine) press(k int, t time.Time) []Action {
	at := t.Add(e.delay())
	var acts []Action
	if e.down[k] {
		// Still holding a sustain; lift the key just before the next note.
		up := at.Add(-releaseGap)
		if earliest := e.downAt[k].Add(e.p.MinPress); up.Before(earliest) {
			up = earliest
		}
		acts = append(acts, e.forceUp(k, up))
	}
	if earliest := e.lastUp[k].Add(releaseGap / 2); at.Before(earliest) {
		at = earliest
	}
	acts = append(acts, Action{At: at, Lane: k, Down: true})
	e.down[k], e.downAt[k] = true, at
	if !e.p.Hold {
		acts = append(acts, e.forceUp(k, at.Add(e.p.MinPress)))
	}
	return acts
}

func (e *Engine) release(k int, t time.Time) []Action {
	if !e.down[k] {
		return nil
	}
	up := t.Add(e.delay())
	if earliest := e.downAt[k].Add(e.p.MinPress); up.Before(earliest) {
		up = earliest
	}
	return []Action{e.forceUp(k, up)}
}

// estimate averages recent measurements, ignoring outliers from mismatched
// notes. If the value really changes (a new song on another difficulty),
// the outliers keep coming and it starts over.
type estimate struct {
	samples []float64
	rejects int
}

func (s *estimate) add(v float64) {
	if v <= 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return
	}
	if med, ok := s.value(); ok && math.Abs(v-med) > 0.3*med {
		s.rejects++
		if s.rejects >= 6 {
			s.samples, s.rejects = []float64{v}, 0
		}
		return
	}
	s.rejects = 0
	s.samples = append(s.samples, v)
	if len(s.samples) > 15 {
		s.samples = s.samples[1:]
	}
}

// value is the mean of the middle half of the samples: as robust as the
// median, but it also averages out the rounding to whole screen frames.
func (s *estimate) value() (float64, bool) {
	n := len(s.samples)
	if n == 0 {
		return 0, false
	}
	sorted := append([]float64(nil), s.samples...)
	sort.Float64s(sorted)
	lo, hi := n/4, n-n/4
	sum := 0.0
	for _, v := range sorted[lo:hi] {
		sum += v
	}
	return sum / float64(hi-lo), true
}

func seconds(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func maxDur(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
