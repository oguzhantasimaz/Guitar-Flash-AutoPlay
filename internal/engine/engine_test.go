package engine

import (
	"math"
	"testing"
	"time"
)

// song simulates the game: notes scroll down at a constant speed and the
// screen only changes 60 times a second.
type song struct {
	speed float64 // fret spacings per second
	notes []note
	fps   float64
	flash []span // times at which the screen flashes white
}

type note struct {
	lane    int
	hit     time.Duration // when the gem centre reaches the frets
	sustain time.Duration // length of the tail, 0 for none
}

type span struct{ from, to time.Duration }

const gemHalf = 0.12 // half the gem height, in spacings

// coverage is what a lane sensor at height h reads at time t.
func (s song) coverage(lane int, h float64, t time.Duration) float64 {
	if s.flashing(t) {
		return 1
	}
	frame := time.Duration(math.Floor(t.Seconds()*s.fps) / s.fps * float64(time.Second))
	cov := 0.0
	for _, n := range s.notes {
		if n.lane != lane {
			continue
		}
		c := s.speed * (n.hit - frame).Seconds() // gem centre height
		switch {
		case h >= c-gemHalf && h <= c+gemHalf*0.4:
			cov = 1 // gem body
		case h > c+gemHalf*0.4 && h <= c+gemHalf:
			cov = math.Max(cov, 0.5) // white cap
		case n.sustain > 0 && h > c+gemHalf+0.03 && h <= c+s.speed*n.sustain.Seconds():
			cov = math.Max(cov, 0.22) // tail, after a small gap
		}
	}
	return cov
}

// sensor reads a lane like vision.Line does: the highest coverage over a
// short band of rows reaching down from h.
func (s song) sensor(lane int, h float64, t time.Duration) float64 {
	c := 0.0
	for i := 0; i < 4; i++ {
		c = math.Max(c, s.coverage(lane, h-0.05*float64(i), t))
	}
	return c
}

func (s song) flashing(t time.Duration) bool {
	for _, f := range s.flash {
		if t >= f.from && t < f.to {
			return true
		}
	}
	return false
}

// play feeds the engine a frame every poll and returns the actions it
// produced, with times relative to the song start.
func (s song) play(p Params, poll, length time.Duration) []timed {
	e := New(p)
	start := time.Unix(1000, 0)
	var out []timed
	for t := time.Duration(0); t < length; t += poll {
		var up, low [5]float64
		for k := 0; k < 5; k++ {
			up[k] = s.sensor(k, p.Upper, t)
			low[k] = s.sensor(k, p.Lower, t)
		}
		for _, a := range e.Update(start.Add(t), up, low, s.flashing(t)) {
			out = append(out, timed{a.At.Sub(start), a})
		}
	}
	return out
}

type timed struct {
	at time.Duration
	Action
}

func downs(acts []timed) []timed {
	var d []timed
	for _, a := range acts {
		if a.Down {
			d = append(d, a)
		}
	}
	return d
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

// at returns the time of the i-th note of a steady rhythm, nudged by a few
// milliseconds so notes do not all land at the same point of a screen
// frame, as in a real song.
func at(start, every, i int) time.Duration {
	return ms(start+every*i) + time.Duration((i*7919)%13)*time.Millisecond
}

// perfect is where a press lands when there is no real screen latency: the
// engine still subtracts Latency, and the 60fps screen makes it see notes
// up to a frame late.
func checkPresses(t *testing.T, s song, p Params, acts []timed) {
	t.Helper()
	d := downs(acts)
	if len(d) != len(s.notes) {
		t.Fatalf("%d presses for %d notes: %v", len(d), len(s.notes), d)
	}
	for i, n := range s.notes {
		a := d[i]
		if a.Lane != n.lane {
			t.Errorf("note %d: pressed lane %d, want %d", i, a.Lane, n.lane)
		}
		// A 60fps screen shows a note up to a frame late, the sensors add
		// up to one poll on top.
		err := a.at - (n.hit - p.Latency + p.Offset)
		if err < -ms(15) || err > ms(25) {
			t.Errorf("note %d (lane %d at %v): pressed at %v, off by %v", i, n.lane, n.hit, a.at, err)
		}
	}
}

func TestSingleNotesAndChords(t *testing.T) {
	s := song{speed: 8, fps: 60, notes: []note{
		{0, ms(1000), 0},
		{2, ms(1400), 0},
		{4, ms(1700), 0},
		{1, ms(2000), 0}, {3, ms(2000), 0}, // chord
		{0, ms(2300), 0}, {2, ms(2300), 0}, {4, ms(2300), 0},
	}}
	p := DefaultParams()
	acts := s.play(p, ms(4), ms(3000))
	checkPresses(t, s, p, sortByNote(acts, s))
}

// sortByNote orders presses like the song's notes (chords are emitted lane
// by lane within one frame).
func sortByNote(acts []timed, s song) []timed {
	d := downs(acts)
	out := make([]timed, 0, len(d))
	used := make([]bool, len(d))
	for _, n := range s.notes {
		best := -1
		for i, a := range d {
			if used[i] || a.Lane != n.lane {
				continue
			}
			if best < 0 || absDur(a.at-n.hit) < absDur(d[best].at-n.hit) {
				best = i
			}
		}
		if best >= 0 {
			used[best] = true
			out = append(out, d[best])
		}
	}
	return out
}

func TestMeasuresSpeed(t *testing.T) {
	for _, speed := range []float64{4, 7.5, 12} {
		s := song{speed: speed, fps: 60}
		for i := 0; i < 9; i++ {
			s.notes = append(s.notes, note{(i * 3) % 5, at(1000, 170, i), 0})
		}
		p := DefaultParams()
		e := New(p)
		start := time.Unix(0, 0)
		for tm := time.Duration(0); tm < ms(3000); tm += ms(4) {
			var up, low [5]float64
			for k := range up {
				up[k], low[k] = s.sensor(k, p.Upper, tm), s.sensor(k, p.Lower, tm)
			}
			e.Update(start.Add(tm), up, low, false)
		}
		v, ok := e.Speed()
		if !ok || math.Abs(v-speed) > 0.05*speed {
			t.Errorf("speed %.1f: measured %.2f (ok=%v)", speed, v, ok)
		}
		checkPresses(t, s, p, s.play(p, ms(4), ms(3000)))
	}
}

func TestFastStreamOnOneLane(t *testing.T) {
	s := song{speed: 9, fps: 60}
	for i := 0; i < 16; i++ {
		s.notes = append(s.notes, note{3, at(1000, 90, i), 0})
	}
	p := DefaultParams()
	checkPresses(t, s, p, s.play(p, ms(4), ms(3000)))
}

func TestHoldsSustains(t *testing.T) {
	s := song{speed: 8, fps: 60, notes: []note{
		{1, ms(1000), ms(600)},
		{1, ms(1800), 0},
		// A note straight after the end of a tail must still be pressed
		// again, not swallowed by the hold.
		{4, ms(2200), ms(400)},
		{4, ms(2660), 0},
	}}
	p := DefaultParams()
	acts := s.play(p, ms(4), ms(3500))
	checkPresses(t, s, p, acts)

	// The key must stay down for the whole tail.
	var downAt, upAt time.Duration
	for _, a := range acts {
		if a.Lane == 1 && a.Down && downAt == 0 {
			downAt = a.at
		}
		if a.Lane == 1 && !a.Down && upAt == 0 {
			upAt = a.at
		}
	}
	if held := upAt - downAt; held < ms(550) || held > ms(700) {
		t.Errorf("sustain held for %v, want about 600ms", held)
	}

	// Every press must be followed by a release before the next press.
	var down [5]bool
	for _, a := range acts {
		if a.Down == down[a.Lane] {
			t.Fatalf("lane %d: key %v twice in a row at %v", a.Lane, map[bool]string{true: "down", false: "up"}[a.Down], a.at)
		}
		down[a.Lane] = a.Down
	}
}

func TestTapOnlyReleasesQuickly(t *testing.T) {
	s := song{speed: 8, fps: 60, notes: []note{{2, ms(1000), ms(800)}}}
	p := DefaultParams()
	p.Hold = false
	acts := s.play(p, ms(4), ms(2500))
	checkPresses(t, s, p, acts)
	if len(acts) != 2 || acts[1].Down || acts[1].at-acts[0].at != p.MinPress {
		t.Errorf("want a single tap of %v, got %v", p.MinPress, acts)
	}
}

func TestOffsetAndLatency(t *testing.T) {
	s := song{speed: 8, fps: 60, notes: []note{{0, ms(1000), 0}, {4, ms(1500), 0}}}
	p := DefaultParams()
	p.Offset = ms(30)
	p.Latency = ms(50)
	checkPresses(t, s, p, s.play(p, ms(4), ms(2500)))
}

func TestIgnoresFlashes(t *testing.T) {
	// During the flash every lane reads as fully covered; none of that may
	// turn into a press.
	s := song{speed: 8, fps: 60,
		notes: []note{{0, ms(1000), 0}, {3, ms(1600), 0}},
		flash: []span{{ms(1100), ms(1250)}},
	}
	p := DefaultParams()
	checkPresses(t, s, p, s.play(p, ms(4), ms(2500)))
}

func TestAdaptsToNewSpeed(t *testing.T) {
	// Speed changes from one song to the next without a reset.
	slow := song{speed: 5, fps: 60}
	for i := 0; i < 10; i++ {
		slow.notes = append(slow.notes, note{i % 5, at(1000, 300, i), 0})
	}
	fast := song{speed: 11, fps: 60}
	for i := 0; i < 14; i++ {
		fast.notes = append(fast.notes, note{(i * 2) % 5, at(5000, 250, i), 0})
	}
	p := DefaultParams()
	e := New(p)
	start := time.Unix(0, 0)
	var acts []timed
	for tm := time.Duration(0); tm < ms(9000); tm += ms(4) {
		cur := slow
		if tm >= ms(4500) {
			cur = fast
		}
		var up, low [5]float64
		for k := range up {
			up[k], low[k] = cur.sensor(k, p.Upper, tm), cur.sensor(k, p.Lower, tm)
		}
		for _, a := range e.Update(start.Add(tm), up, low, false) {
			acts = append(acts, timed{a.At.Sub(start), a})
		}
	}
	if v, _ := e.Speed(); math.Abs(v-11) > 0.55 {
		t.Errorf("speed after the change: %.2f, want 11", v)
	}
	// The last notes of the fast song must be on time again.
	d := downs(acts)
	for i := len(d) - 5; i < len(d); i++ {
		n := fast.notes[len(fast.notes)-(len(d)-i)]
		if err := d[i].at - (n.hit - p.Latency); err < -ms(15) || err > ms(25) {
			t.Errorf("note at %v pressed at %v (off by %v)", n.hit, d[i].at, err)
		}
	}
}

func TestReleaseAll(t *testing.T) {
	e := New(DefaultParams())
	now := time.Unix(0, 0)
	full := [5]float64{1, 0, 1, 0, 0}
	e.Update(now, [5]float64{}, full, false)
	acts := e.ReleaseAll(now.Add(time.Second))
	if len(acts) != 2 || acts[0].Down || acts[1].Down {
		t.Fatalf("want two releases, got %v", acts)
	}
	if len(e.ReleaseAll(now)) != 0 {
		t.Error("keys released twice")
	}
}
