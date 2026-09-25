// Package player runs the whole bot: it waits for the fretboard to show up,
// follows the notes and presses the keys, and tells a Listener what it is
// doing. The command line and the window both drive it.
package player

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"time"

	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/engine"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/platform"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/vision"
)

// Config is everything the player needs to run.
type Config struct {
	Keys    [5]platform.Key
	Params  engine.Params
	Poll    time.Duration // how often to read the screen
	Display int           // only search this display; -1 searches all
	Board   *vision.Board // use this board instead of detecting one
	DryRun  bool          // follow the notes but do not press keys
	// StartDelay gives the user time to click on the game before anything
	// happens, so no keys end up in the wrong window.
	StartDelay time.Duration
	// DebugShot, if set, is where a screenshot of each detection is saved.
	DebugShot string
	// Preview asks for Listener.Preview pictures while playing.
	Preview bool
}

// DefaultConfig returns the settings used when nothing is changed.
func DefaultConfig() Config {
	var keys [5]platform.Key
	for i, n := range []string{"a", "s", "j", "k", "l"} {
		keys[i], _ = platform.ParseKey(n)
	}
	return Config{
		Keys:       keys,
		Params:     engine.DefaultParams(),
		Poll:       4 * time.Millisecond,
		Display:    -1,
		StartDelay: 3 * time.Second,
	}
}

// State is what the player is busy with.
type State int

const (
	Starting  State = iota // waiting for StartDelay
	Searching              // looking for the fretboard
	Playing                // following the notes
)

func (s State) String() string {
	switch s {
	case Starting:
		return "starting"
	case Searching:
		return "searching"
	case Playing:
		return "playing"
	}
	return "unknown"
}

// Listener hears what the player is doing. The methods are called from the
// player's goroutines and must return quickly.
type Listener interface {
	// State reports a new state; board is set while Playing.
	State(s State, board vision.Board)
	// Log is a message for the user.
	Log(msg string)
	// Speed reports the measured note speed, in fret spacings per second.
	Speed(v float64)
	// Key reports a key going down or up.
	Key(lane int, down bool)
	// Preview is a picture of the fretboard with the sensors drawn on it,
	// sent a few times per second while playing if Config.Preview is set.
	// The player does not touch img afterwards.
	Preview(img *image.RGBA)
}

// ErrMouseStop is returned when the user stopped the player by moving the
// mouse to the top-left corner of the screen.
var ErrMouseStop = errors.New("stopped with the mouse in the top-left corner")

// Run plays until ctx is cancelled (it then returns ctx.Err()), the mouse is
// moved to the top-left corner (ErrMouseStop) or something breaks.
func Run(ctx context.Context, cfg Config, l Listener) error {
	if cfg.StartDelay > 0 {
		l.State(Starting, vision.Board{})
		if err := sleep(ctx, cfg.StartDelay); err != nil {
			return err
		}
	}
	eng := engine.New(cfg.Params)
	for {
		board := vision.Board{}
		if cfg.Board != nil {
			board = *cfg.Board
		} else {
			l.State(Searching, vision.Board{})
			var err error
			if board, err = findBoard(ctx, cfg, l); err != nil {
				return err
			}
		}
		l.State(Playing, board)
		if err := follow(ctx, cfg, l, eng, board); err != nil {
			return err
		}
		l.Log("The fretboard is gone (song over, or the page moved).")
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// findBoard searches the displays until the fretboard shows up at the same
// place twice in a row, so a board that is still sliding in is not used.
func findBoard(ctx context.Context, cfg Config, l Listener) (vision.Board, error) {
	var last *vision.Board
	start := time.Now()
	hinted := false
	failures := 0
	for {
		displays, err := platform.Displays()
		if err != nil {
			return vision.Board{}, err
		}
		var found *vision.Board
		var shot *image.RGBA
		for i, d := range displays {
			if cfg.Display >= 0 && i != cfg.Display {
				continue
			}
			img, err := platform.Capture(d)
			if err != nil {
				// Displays can come and go (sleep, unplugging); only give up
				// when capturing keeps failing.
				if failures++; failures > 20 {
					return vision.Board{}, fmt.Errorf("capturing display %d: %w", i, err)
				}
				continue
			}
			failures = 0
			if b, err := vision.FindBoard(img); err == nil {
				found, shot = &b, img
				break
			}
		}
		if found != nil && last != nil && same(*found, *last) {
			if cfg.DebugShot != "" {
				saveDebug(l, cfg.DebugShot, shot, *found, cfg.Params)
			}
			return *found, nil
		}
		last = found
		if !hinted && time.Since(start) > 30*time.Second {
			hinted = true
			l.Log("Still looking. Make sure the whole fretboard is visible and not covered by another window.")
			l.Log("If it is never found, point at the frets with the mouse instead (calibrate).")
		}
		if err := sleep(ctx, 300*time.Millisecond); err != nil {
			return vision.Board{}, err
		}
	}
}

func same(a, b vision.Board) bool {
	tol := 0.03 * a.Spacing
	return abs(a.X0-b.X0) < tol && abs(a.Y-b.Y) < tol && abs(a.Spacing-b.Spacing) < tol
}

type keyboard struct {
	keys   [5]platform.Key
	dryRun bool
}

func (k keyboard) Key(lane int, down bool) error {
	if k.dryRun {
		return nil
	}
	if down {
		return platform.KeyDown(k.keys[lane])
	}
	return platform.KeyUp(k.keys[lane])
}

// follow plays until the board disappears (returns nil) or it has to stop.
func follow(ctx context.Context, cfg Config, l Listener, eng *engine.Engine, board vision.Board) error {
	p := cfg.Params
	upper, lower := vision.NewColumn(board, p.Upper), vision.NewColumn(board, p.Lower)
	tails := vision.NewLine(board, p.Lower)
	region := upper.Bounds().Union(lower.Bounds()).Union(tails.Bounds()).Inset(-2)
	rings := image.Rect(
		int(board.FretX(0)-board.RingW), int(board.Y-board.RingH),
		int(board.FretX(4)+board.RingW)+1, int(board.Y+board.RingH)+1)
	// The preview also shows the rings, so it doubles as the check that
	// the board is still there.
	check := rings
	if cfg.Preview {
		check = check.Union(region).Inset(-int(board.Spacing / 2))
	}

	sched := engine.NewScheduler(keyboard{cfg.Keys, cfg.DryRun}, func(a engine.Action, late time.Duration, err error) {
		if err != nil {
			l.Log(fmt.Sprintf("Pressing %s failed: %v", cfg.Keys[a.Lane].Name, err))
			return
		}
		l.Key(a.Lane, a.Down)
	})
	eng.Reset()
	// Stop drops pending presses and lifts every key that is still down.
	defer sched.Stop()

	ticker := time.NewTicker(cfg.Poll)
	defer ticker.Stop()
	lastSeen, lastCheck := time.Now(), time.Now()
	var failures int
	reported := 0.0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		t := time.Now()
		img, err := platform.Capture(region)
		if err != nil {
			if failures++; failures > 50 {
				return fmt.Errorf("screen capture keeps failing: %w", err)
			}
			continue
		}
		failures = 0
		r := engine.Reading{Upper: upper.Read(img), Lower: lower.Read(img)}
		r.Tail, r.Flash = tails.Read(img)
		sched.Add(eng.Update(t, r))

		if v, ok := eng.Speed(); ok && abs(v-reported) > 0.05*v {
			reported = v
			l.Speed(v)
		}

		if t.Sub(lastCheck) < 200*time.Millisecond {
			continue
		}
		lastCheck = t
		if c, err := platform.Cursor(); err == nil && c.X <= 2 && c.Y <= 2 {
			return ErrMouseStop
		}
		shot, err := platform.Capture(check)
		if err != nil {
			continue
		}
		if cfg.Board != nil || board.Present(shot) {
			lastSeen = t
		} else if t.Sub(lastSeen) > 3*time.Second {
			return nil
		}
		if cfg.Preview {
			vision.DrawOverlay(shot, board, vision.NewLine(board, p.Upper), tails)
			l.Preview(shot)
		}
	}
}

func saveDebug(l Listener, path string, img *image.RGBA, b vision.Board, p engine.Params) {
	if err := WriteOverlay(path, img, b, p); err != nil {
		l.Log(fmt.Sprintf("Saving the debug screenshot failed: %v", err))
		return
	}
	l.Log("Saved what was detected to " + path)
}

// WriteOverlay draws what the player looks at onto img and saves it as PNG.
func WriteOverlay(path string, img *image.RGBA, b vision.Board, p engine.Params) error {
	vision.DrawOverlay(img, b, vision.NewLine(b, p.Upper), vision.NewLine(b, p.Lower))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
