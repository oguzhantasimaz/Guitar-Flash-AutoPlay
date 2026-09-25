// Package ui is the app's window: start and stop the player, see what it
// sees, and change the settings without touching the command line.
package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/platform"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/player"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/vision"
)

// Run opens the window and never returns; the program exits when the window
// is closed. It must be called from the main goroutine.
func Run(version string) {
	u := newUI(version)
	go func() {
		u.w = new(app.Window)
		u.w.Option(
			app.Title("Guitar Flash AutoPlay"),
			app.Size(unit.Dp(480), unit.Dp(720)),
			app.MinSize(unit.Dp(400), unit.Dp(480)),
		)
		u.loop()
		u.shutdown()
		os.Exit(0)
	}()
	app.Main()
}

// status is everything the player reports, shared with the window.
type status struct {
	running     bool
	state       player.State
	board       vision.Board
	speed       float64
	notes       int
	keyDown     [5]bool
	lastPress   [5]time.Time
	preview     paint.ImageOp
	hasPreview  bool
	logs        []logLine
	problems    []platform.Problem
	calibrating string
	notice      string // why the last Start did not start
}

type logLine struct {
	at  time.Time
	msg string
}

// UI holds the window state. Fields below mu are shared with the player's
// goroutines; the widgets are only used by the window goroutine.
type UI struct {
	version  string
	w        *app.Window
	th       *material.Theme
	settings Settings

	mu     sync.Mutex
	st     status
	cancel context.CancelFunc
	done   chan struct{}

	startBtn     widget.Clickable
	calibrateBtn widget.Clickable
	autoBtn      widget.Clickable
	settingsBtns [4]widget.Clickable
	keyEd        [5]widget.Editor
	offset       widget.Float
	hold         widget.Bool
	practice     widget.Bool
	page         widget.List
	logList      widget.List
}

func newUI(version string) *UI {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{Bg: colBg, Fg: colText, ContrastBg: colAccent, ContrastFg: colText}
	th.TextSize = 15

	u := &UI{version: version, th: th, settings: loadSettings()}
	for i := range u.keyEd {
		u.keyEd[i] = widget.Editor{SingleLine: true, MaxLen: 10, Alignment: text.Middle}
		u.keyEd[i].SetText(u.settings.Keys[i])
	}
	u.offset.Value = offsetToSlider(u.settings.OffsetMs)
	u.hold.Value = u.settings.Hold
	u.practice.Value = u.settings.Practice
	u.page.Axis = layout.Vertical
	u.logList.Axis = layout.Vertical
	u.logList.ScrollToEnd = true
	u.st.problems = platform.Check(false, !u.settings.Practice)
	u.log("Open a song on guitarflash.com, then press Start and click on the game.")
	return u
}

func (u *UI) loop() {
	var ops op.Ops
	for {
		switch e := u.w.Event().(type) {
		case app.DestroyEvent:
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			u.handle(gtx)
			u.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// shutdown stops the player (which lets go of every key) and saves the
// settings before the program exits.
func (u *UI) shutdown() {
	u.stop()
	u.mu.Lock()
	done := u.done
	u.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	u.readSettings()
	u.saveSettings()
}

func (u *UI) invalidate() {
	if u.w != nil {
		u.w.Invalidate()
	}
}

// handle reacts to clicks and edits made since the last frame.
func (u *UI) handle(gtx C) {
	if u.startBtn.Clicked(gtx) {
		if u.isRunning() {
			u.stop()
		} else {
			u.start()
		}
	}
	if u.calibrateBtn.Clicked(gtx) {
		go u.calibrate()
	}
	if u.autoBtn.Clicked(gtx) {
		u.mu.Lock()
		u.settings.Frets = nil
		u.mu.Unlock()
		u.saveSettings()
		u.log("The fretboard will be found automatically again.")
	}
	if u.practice.Update(gtx) {
		u.mu.Lock()
		u.st.problems = platform.Check(false, !u.practice.Value)
		u.mu.Unlock()
	}
	u.mu.Lock()
	problems := u.st.problems
	u.mu.Unlock()
	for i := range problems {
		if i < len(u.settingsBtns) && u.settingsBtns[i].Clicked(gtx) {
			if err := platform.OpenSettings(problems[i]); err != nil {
				u.log(err.Error())
			}
		}
	}
}

func (u *UI) isRunning() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.st.running
}

// readSettings copies the widgets into u.settings.
func (u *UI) readSettings() {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i := range u.keyEd {
		u.settings.Keys[i] = strings.TrimSpace(u.keyEd[i].Text())
	}
	u.settings.OffsetMs = sliderToOffset(u.offset.Value)
	u.settings.Hold = u.hold.Value
	u.settings.Practice = u.practice.Value
}

// saveSettings writes the settings to disk. The calibration goroutine
// changes them too, hence the lock.
func (u *UI) saveSettings() {
	u.mu.Lock()
	s := u.settings
	u.mu.Unlock()
	if err := s.save(); err != nil {
		u.log("Saving the settings failed: " + err.Error())
	}
}

func (u *UI) start() {
	u.readSettings()
	u.saveSettings()
	u.mu.Lock()
	set := u.settings
	u.mu.Unlock()

	cfg := player.DefaultConfig()
	for i, name := range set.Keys {
		k, err := platform.ParseKey(name)
		if err != nil {
			u.notify(fmt.Sprintf("The %s fret key: %v.", strings.ToLower(fretNames[i]), firstLine(err)))
			return
		}
		cfg.Keys[i] = k
	}
	cfg.Params.Offset = time.Duration(set.OffsetMs) * time.Millisecond
	cfg.Params.Hold = set.Hold
	cfg.DryRun = set.Practice
	cfg.Preview = true
	if f := set.Frets; f != nil {
		b := vision.ManualBoard(image.Pt(f[0], f[1]), image.Pt(f[2], f[3]))
		cfg.Board = &b
	}

	problems := platform.Check(true, !cfg.DryRun)
	u.mu.Lock()
	u.st.problems = problems
	u.mu.Unlock()
	if len(problems) > 0 {
		u.notify("Fix the problem below first.")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	u.mu.Lock()
	u.st.running, u.st.notes, u.st.speed, u.st.hasPreview = true, 0, 0, false
	u.st.notice = ""
	u.cancel, u.done = cancel, done
	u.mu.Unlock()
	if cfg.DryRun {
		u.log("Practice mode: watching only, no keys will be pressed.")
	}
	go func() {
		defer close(done)
		err := player.Run(ctx, cfg, u)
		switch {
		case errors.Is(err, player.ErrMouseStop):
			u.log("Stopped: the mouse is in the top-left corner.")
		case err != nil && !errors.Is(err, context.Canceled):
			u.log("Stopped: " + err.Error())
		default:
			u.log("Stopped.")
		}
		u.mu.Lock()
		u.st.running, u.st.hasPreview = false, false
		u.st.keyDown = [5]bool{}
		u.cancel = nil
		u.mu.Unlock()
		u.invalidate()
	}()
}

func (u *UI) stop() {
	u.mu.Lock()
	cancel := u.cancel
	u.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// calibrate asks the user to hold the mouse over the green and then the
// orange fret, and remembers where they are.
func (u *UI) calibrate() {
	var pts [2]image.Point
	for i, name := range []string{"GREEN (leftmost)", "ORANGE (rightmost)"} {
		for n := 3; n > 0; n-- {
			u.mu.Lock()
			u.st.calibrating = fmt.Sprintf("Hold the mouse over the centre of the %s fret... %d", name, n)
			u.mu.Unlock()
			u.invalidate()
			time.Sleep(time.Second)
		}
		p, err := platform.Cursor()
		if err != nil {
			u.mu.Lock()
			u.st.calibrating = ""
			u.mu.Unlock()
			u.log("Reading the mouse position failed: " + err.Error())
			return
		}
		pts[i] = p
	}
	u.mu.Lock()
	u.st.calibrating = ""
	u.mu.Unlock()
	if pts[1].X-pts[0].X < 20 {
		u.log("The orange fret must be to the right of the green one. Try again.")
		return
	}
	u.mu.Lock()
	u.settings.Frets = &[4]int{pts[0].X, pts[0].Y, pts[1].X, pts[1].Y}
	u.mu.Unlock()
	u.saveSettings()
	u.invalidate()
	u.log(fmt.Sprintf("Frets set by hand: green at %v, orange at %v.", pts[0], pts[1]))
}

// notify shows msg under the status, where it cannot be missed, and logs it.
func (u *UI) notify(msg string) {
	u.mu.Lock()
	u.st.notice = msg
	u.mu.Unlock()
	u.log(msg)
}

func (u *UI) log(msg string) {
	u.mu.Lock()
	u.st.logs = append(u.st.logs, logLine{time.Now(), msg})
	if len(u.st.logs) > 200 {
		u.st.logs = u.st.logs[len(u.st.logs)-200:]
	}
	u.mu.Unlock()
	u.invalidate()
}

// The player.Listener methods.

func (u *UI) State(s player.State, b vision.Board) {
	u.mu.Lock()
	u.st.state, u.st.board = s, b
	if s == player.Playing {
		u.st.notes = 0
	} else {
		u.st.hasPreview = false
	}
	u.mu.Unlock()
	switch s {
	case player.Searching:
		u.log("Looking for the fretboard...")
	case player.Playing:
		u.log("Found the fretboard: " + b.String())
	}
	u.invalidate()
}

func (u *UI) Log(msg string) { u.log(msg) }

func (u *UI) Speed(v float64) {
	u.mu.Lock()
	u.st.speed = v
	u.mu.Unlock()
	u.invalidate()
}

func (u *UI) Key(lane int, down bool) {
	u.mu.Lock()
	u.st.keyDown[lane] = down
	if down {
		u.st.notes++
		u.st.lastPress[lane] = time.Now()
	}
	u.mu.Unlock()
	u.invalidate()
}

func (u *UI) Preview(img *image.RGBA) {
	op := paint.NewImageOp(img)
	u.mu.Lock()
	u.st.preview, u.st.hasPreview = op, true
	u.mu.Unlock()
	u.invalidate()
}

// The timing slider covers -100 ms to +100 ms in 5 ms steps.
const maxOffsetMs = 100

func sliderToOffset(v float32) int {
	return int(math.Round(float64(v*2-1)*maxOffsetMs/5)) * 5
}

func offsetToSlider(ms int) float32 {
	return (float32(ms)/maxOffsetMs + 1) / 2
}

func firstLine(err error) string {
	s := err.Error()
	if i := strings.Index(s, " (known keys"); i > 0 {
		return s[:i]
	}
	return s
}
