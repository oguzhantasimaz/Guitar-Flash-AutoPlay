package engine

import (
	"container/heap"
	"sync"
	"time"
)

// Keyboard presses and releases the key of a lane.
type Keyboard interface {
	Key(lane int, down bool) error
}

// Scheduler performs actions at their time on its own goroutine, so the
// capture loop never waits for a key press. Actions that are already due
// run immediately.
type Scheduler struct {
	kb     Keyboard
	onDone func(a Action, late time.Duration, err error)
	in     chan []Action
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

// NewScheduler starts a scheduler. onDone, if not nil, is called after each
// action with how late it ran.
func NewScheduler(kb Keyboard, onDone func(a Action, late time.Duration, err error)) *Scheduler {
	s := &Scheduler{
		kb:     kb,
		onDone: onDone,
		in:     make(chan []Action, 64),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go s.run()
	return s
}

// Add queues actions.
func (s *Scheduler) Add(acts []Action) {
	if len(acts) == 0 {
		return
	}
	select {
	case s.in <- acts:
	case <-s.done:
	}
}

// Stop drops pending actions, releases every key that is still down and
// waits for the scheduler to finish.
func (s *Scheduler) Stop() {
	s.once.Do(func() { close(s.stop) })
	<-s.done
}

func (s *Scheduler) run() {
	defer close(s.done)
	var q actionQueue
	var seq int
	var held [5]bool
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()

	do := func(a Action) {
		err := s.kb.Key(a.Lane, a.Down)
		if err == nil {
			held[a.Lane] = a.Down
		}
		if s.onDone != nil {
			s.onDone(a, time.Since(a.At), err)
		}
	}
	for {
		for q.Len() > 0 && !time.Now().Before(q[0].At) {
			do(heap.Pop(&q).(queued).Action)
		}
		var wake <-chan time.Time
		if q.Len() > 0 {
			timer.Reset(time.Until(q[0].At))
			wake = timer.C
		}
		select {
		case acts := <-s.in:
			for _, a := range acts {
				seq++
				heap.Push(&q, queued{a, seq})
			}
		case <-wake:
		case <-s.stop:
			for lane, down := range held {
				if down {
					do(Action{At: time.Now(), Lane: lane, Down: false})
				}
			}
			return
		}
	}
}

// queued keeps actions with the same time in the order they were added.
type queued struct {
	Action
	seq int
}

type actionQueue []queued

func (q actionQueue) Len() int { return len(q) }
func (q actionQueue) Less(i, j int) bool {
	if q[i].At.Equal(q[j].At) {
		return q[i].seq < q[j].seq
	}
	return q[i].At.Before(q[j].At)
}
func (q actionQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *actionQueue) Push(x any)   { *q = append(*q, x.(queued)) }
func (q *actionQueue) Pop() any {
	old := *q
	x := old[len(old)-1]
	*q = old[:len(old)-1]
	return x
}
