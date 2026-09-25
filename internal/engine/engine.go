// Package engine turns lane readings into timed key presses.
//
// Two sensor lines sit above the frets. Every note crosses the upper line
// first and the lower line a little later; the time between the two gives
// the scroll speed, so the engine knows exactly when the note will reach the
// frets and can press the key at that moment. Measuring the speed instead of
// hard-coding a delay makes it work on every difficulty, screen size and
// computer speed.
package engine

import (
	"math"
	"sort"
	"time"
)

// Params tune the engine. Heights are in fret spacings above the fret line.
type Params struct {
	Upper, Lower float64
	// HitLead is the distance from a gem's leading edge (what the sensor
	// sees first) to its centre, which is what has to meet the fret.
	HitLead float64
	// Latency is how old a screenshot is by the time we act on it; presses
	// are moved earlier by this much.
	Latency time.Duration
	// Offset is a manual correction: positive presses later.
	Offset time.Duration
	// DefaultSpeed (spacings per second) is used until the first note has
	// been measured.
	DefaultSpeed float64
	// MinPress is the shortest time a key is held down.
	MinPress time.Duration
	// Hold keeps keys down through the tail of sustained notes.
	Hold bool
}

// DefaultParams are tuned on the HTML5 version of guitarflash.com.
func DefaultParams() Params {
	return Params{
		Upper:        2.0,
		Lower:        1.0,
		HitLead:      0.13,
		Latency:      20 * time.Millisecond,
		DefaultSpeed: 8,
		MinPress:     25 * time.Millisecond,
		Hold:         true,
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
	speed  speedEstimate
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

// Speed returns the scroll speed in spacings per second and whether it has
// been measured yet.
func (e *Engine) Speed() (float64, bool) {
	if v, ok := e.speed.value(); ok {
		return v, true
	}
	return e.p.DefaultSpeed, false
}

// delay is the time from a note's leading edge crossing the lower line
// until the key should go down.
func (e *Engine) delay() time.Duration {
	v, _ := e.Speed()
	travel := (e.p.Lower + e.p.HitLead) / v
	return time.Duration(travel*float64(time.Second)) - e.p.Latency + e.p.Offset
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
// same note and records the speed.
func (e *Engine) measure(k int, t time.Time) {
	dist := e.p.Upper - e.p.Lower
	const minTravel = 15 * time.Millisecond
	maxTravel := 1500 * time.Millisecond
	expected := time.Duration(0)
	if v, ok := e.speed.value(); ok {
		expected = seconds(dist / v)
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
		e.speed.add(dist / t.Sub(q[pick]).Seconds())
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

// speedEstimate averages recent speed measurements, ignoring outliers from
// mismatched notes. If the speed really changes (a new song
// on another difficulty), the outliers keep coming and it starts over.
type speedEstimate struct {
	samples []float64
	rejects int
}

func (s *speedEstimate) add(v float64) {
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
func (s *speedEstimate) value() (float64, bool) {
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
