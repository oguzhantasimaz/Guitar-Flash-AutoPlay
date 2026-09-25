package engine

import (
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu     sync.Mutex
	events []Action
}

func (r *recorder) Key(lane int, down bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, Action{At: time.Now(), Lane: lane, Down: down})
	return nil
}

func (r *recorder) get() []Action {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Action(nil), r.events...)
}

func TestSchedulerRunsActionsInOrderAndOnTime(t *testing.T) {
	var kb recorder
	s := NewScheduler(&kb, nil)
	now := time.Now()
	s.Add([]Action{
		{At: now.Add(60 * time.Millisecond), Lane: 1, Down: false},
		{At: now.Add(30 * time.Millisecond), Lane: 1, Down: true},
		{At: now.Add(-time.Second), Lane: 0, Down: true}, // already due
		{At: now.Add(40 * time.Millisecond), Lane: 0, Down: false},
	})
	time.Sleep(150 * time.Millisecond)
	s.Stop()

	got := kb.get()
	want := []struct {
		lane int
		down bool
		at   time.Duration
	}{{0, true, 0}, {1, true, 30 * time.Millisecond}, {0, false, 40 * time.Millisecond}, {1, false, 60 * time.Millisecond}}
	if len(got) != len(want) {
		t.Fatalf("got %d key events, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Lane != w.lane || got[i].Down != w.down {
			t.Errorf("event %d: lane %d down=%v, want lane %d down=%v", i, got[i].Lane, got[i].Down, w.lane, w.down)
		}
		// Generous: CI machines can be busy.
		if late := got[i].At.Sub(now.Add(w.at)); w.at > 0 && (late < 0 || late > 50*time.Millisecond) {
			t.Errorf("event %d ran %v late", i, late)
		}
	}
}

func TestSchedulerStopReleasesKeys(t *testing.T) {
	var kb recorder
	s := NewScheduler(&kb, nil)
	now := time.Now()
	s.Add([]Action{
		{At: now, Lane: 2, Down: true},
		{At: now.Add(time.Hour), Lane: 2, Down: false}, // never reached
		{At: now.Add(time.Hour), Lane: 3, Down: true},  // dropped
	})
	time.Sleep(30 * time.Millisecond)
	s.Stop()
	s.Stop() // safe to call twice
	s.Add([]Action{{At: now, Lane: 4, Down: true}})

	got := kb.get()
	if len(got) != 2 || got[0].Lane != 2 || !got[0].Down || got[1].Lane != 2 || got[1].Down {
		t.Fatalf("want lane 2 down then up, got %v", got)
	}
}
