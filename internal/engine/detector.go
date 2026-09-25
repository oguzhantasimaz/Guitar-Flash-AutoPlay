package engine

import "time"

// Coverage levels of a lane sensor (see vision.Line). A gem covers the
// sensor almost completely (90-100%); the white cap on top of it about half;
// the tail of a sustained note 10-30%, up to 45% on a tiny game. The cap is
// the last part of a gem to pass, so dropping to cap level and rising again
// always means a new gem.
const (
	gemLevel  = 0.70 // a new gem starts
	weakLevel = 0.55 // below this the gem has passed
	tailLevel = 0.08 // at least this much is a sustain tail
)

type phase uint8

const (
	idle phase = iota
	gem
	tail
)

type detectorConfig struct {
	// grace bridges the small gap between a gem and its tail, and short
	// dropouts inside a tail.
	grace time.Duration
	// minGap ignores a second gem start that comes too soon to be a
	// different note.
	minGap time.Duration
}

// event is a note starting (its leading edge reaching the sensor) or ending
// (the gem, or the end of its tail, leaving the sensor).
type event struct {
	start bool
	at    time.Time
}

// detector follows one lane on one sensor line.
type detector struct {
	phase      phase
	lastStart  time.Time
	pendingEnd bool
	endAt      time.Time
}

func (d *detector) update(t time.Time, cov float64, cfg detectorConfig) []event {
	var evs []event
	start := func() {
		if !d.lastStart.IsZero() && t.Sub(d.lastStart) < cfg.minGap {
			d.phase = gem
			return
		}
		d.phase, d.lastStart = gem, t
		evs = append(evs, event{start: true, at: t})
	}
	flush := func() {
		if d.pendingEnd {
			d.pendingEnd = false
			evs = append(evs, event{at: d.endAt})
		}
	}

	switch d.phase {
	case idle:
		switch {
		case cov >= gemLevel:
			flush()
			start()
		case d.pendingEnd && t.Sub(d.endAt) > cfg.grace:
			flush()
		case d.pendingEnd && cov >= tailLevel:
			// The tail continues after a short gap.
			d.phase, d.pendingEnd = tail, false
		}
	case gem:
		if cov < weakLevel {
			d.leave(t, cov)
		}
	case tail:
		switch {
		case cov >= gemLevel:
			// The next note follows straight after a tail.
			evs = append(evs, event{at: t})
			start()
		case cov < tailLevel:
			d.leave(t, cov)
		}
	}
	return evs
}

func (d *detector) leave(t time.Time, cov float64) {
	if cov >= tailLevel {
		d.phase = tail
		return
	}
	d.phase, d.pendingEnd, d.endAt = idle, true, t
}
