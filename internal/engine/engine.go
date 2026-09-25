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

// Reading is what the sensors saw in one captured frame.
type Reading struct {
	// Upper and Lower are, per lane, the heights of the leading edges of the
	// gems in view around each sensor line, lowest first (vision.Column).
	Upper, Lower [5][]float64
	// Tail is how much of each lane the lower line covers, gems or the tails
	// of sustained notes (vision.Line); it tells how long to hold a key.
	Tail [5]float64
	// Flash is set for frames washed out by a full-screen flash.
	Flash bool
}

// Engine consumes one reading per captured frame. It is not safe for
// concurrent use.
type Engine struct {
	p       Params
	upper   [5]tracker
	lower   [5]tracker
	tails   [5]detector
	queue   [5][]time.Time // upper-line crossings waiting for their lower one
	travel  estimate       // upper-to-lower travel time, in seconds
	misfits []float64      // recent travel times that did not fit it
	down    [5]bool
	downAt  [5]time.Time
	lastUp  [5]time.Time
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

// Update processes one frame captured at time t. Frames that are flashing
// white are skipped.
func (e *Engine) Update(t time.Time, r Reading) []Action {
	if r.Flash {
		return nil
	}
	v, _ := e.Speed()
	cfg := detectorConfig{
		grace:  maxDur(35*time.Millisecond, seconds(0.25/v)),
		minGap: seconds(0.12 / v),
	}
	var acts []Action
	for k := 0; k < 5; k++ {
		for _, at := range e.upper[k].update(t, r.Upper[k], e.p.Upper, v) {
			if len(e.queue[k]) >= 16 {
				e.queue[k] = e.queue[k][1:] // the lower line missed these
			}
			e.queue[k] = append(e.queue[k], at)
		}
		for _, at := range e.lower[k].update(t, r.Lower[k], e.p.Lower, v) {
			e.measure(k, at)
			acts = append(acts, e.press(k, at)...)
		}
		// The tail sensor only decides when to let go; the trackers decide
		// when to press.
		for _, ev := range e.tails[k].update(t, r.Tail[k], cfg) {
			if !ev.start {
				acts = append(acts, e.release(k, ev.at)...)
			}
		}
		if e.down[k] && t.Sub(e.downAt[k]) > maxHold {
			acts = append(acts, e.forceUp(k, t))
			e.tails[k] = detector{}
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
//
// Once the speed is known, a crossing is only paired (and used up) when its
// travel time fits. If the upper sensor lost this note's gem in a fast run,
// the best candidate is the next note's; using it up would shift every
// pairing after it by one note, which once made the measured speed run away
// to twice the real one. Left alone, the next note pairs correctly again. A
// real change of speed (another song without the board ever leaving the
// screen) shows up as misfits that keep coming and agree with each other,
// and the estimate starts over from them.
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
	defer func() { e.queue[k] = q }()
	if len(q) == 0 || t.Sub(q[0]) < minTravel {
		return
	}
	dist := e.p.Upper - e.p.Lower
	possible := func(d time.Duration) bool {
		return d.Seconds() >= dist/maxSpeed && d.Seconds() <= dist
	}
	if expected == 0 {
		// Without a speed yet, the oldest crossing is this note's. If that
		// is wrong, the misfits that follow put it right.
		d := t.Sub(q[0])
		q = q[1:]
		if possible(d) {
			e.travel.add(d.Seconds())
		}
		return
	}
	pick, near := -1, 0
	for i, tu := range q {
		d := t.Sub(tu)
		if d < minTravel {
			break
		}
		if absDur(d-expected) <= expected*2/5 {
			near++
		}
		if pick < 0 || absDur(d-expected) < absDur(t.Sub(q[pick])-expected) {
			pick = i
		}
	}
	d := t.Sub(q[pick])
	if absDur(d-expected) > expected/4 {
		e.misfit(d)
		return
	}
	e.misfits = e.misfits[:0]
	q = q[pick+1:]
	// With another crossing almost as likely, the pairing is kept but not
	// measured.
	if near == 1 && possible(d) {
		e.travel.add(d.Seconds())
	}
}

// misfit notes a travel time that does not fit the estimate. Six in a row
// that agree with each other mean the speed really changed.
func (e *Engine) misfit(d time.Duration) {
	e.misfits = append(e.misfits, d.Seconds())
	if len(e.misfits) < 6 {
		return
	}
	m := estimate{samples: e.misfits}
	if v, _ := m.value(); spread(e.misfits, v) <= 0.1*v {
		e.travel = estimate{samples: append([]float64(nil), e.misfits...)}
		for k := range e.queue {
			e.queue[k] = nil
		}
	}
	e.misfits = e.misfits[:0]
}

// spread is the largest distance of a value from m.
func spread(vs []float64, m float64) float64 {
	d := 0.0
	for _, v := range vs {
		d = math.Max(d, math.Abs(v-m))
	}
	return d
}

// releaseGap is how long a key is let go before the same key is pressed for
// the next note: about a frame of the game, so it sees the key come up even
// if it only looks at the keys once per frame.
const releaseGap = 20 * time.Millisecond

func (e *Engine) press(k int, t time.Time) []Action {
	at := t.Add(e.delay())
	var acts []Action
	// Lift the key a moment before this press. It may still be held for a
	// sustain, or its release may be due too late: in a fast run the gems
	// almost touch, and the release that follows one gem out of the sensor
	// would leave the key up for only a few milliseconds. That later
	// release still happens, before this press, and does nothing.
	up := at.Add(-releaseGap)
	if earliest := e.downAt[k].Add(e.p.MinPress); up.Before(earliest) {
		up = earliest
	}
	if e.down[k] || e.lastUp[k].After(up) {
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

// estimate averages the most recent measurements.
type estimate struct {
	samples []float64
}

func (s *estimate) add(v float64) {
	if v <= 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return
	}
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
