// Command guitarflash-autoplay plays Guitar Flash (guitarflash.com) by
// watching the screen and pressing the fret keys.
//
// It finds the fretboard on its own, so it works at any screen resolution,
// browser zoom or display scaling. Run it, open a song in the browser and
// click on the game so it gets the keyboard.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/engine"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/platform"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/vision"
)

var version = "dev"

type options struct {
	keys      [5]platform.Key
	params    engine.Params
	poll      time.Duration
	display   int
	frets     *vision.Board
	calibrate bool
	dryRun    bool
	debug     bool
}

func main() {
	code := run()
	if code != 0 {
		platform.PauseBeforeExit()
	}
	os.Exit(code)
}

func run() int {
	p := engine.DefaultParams()
	var (
		keys      = flag.String("keys", "a,s,j,k,l", "the five fret keys, green to orange, as set in the game's \"Setting Keys\"")
		offset    = flag.Duration("offset", 0, "move every key press later (e.g. 15ms) or earlier (e.g. -15ms)")
		noHold    = flag.Bool("no-hold", false, "only tap notes, do not hold keys through sustained notes")
		display   = flag.Int("display", -1, "only search this display (0 = main display); -1 searches all")
		frets     = flag.String("frets", "", "skip detection: screen position of the green and orange fret centres as x1,y1,x2,y2")
		calibrate = flag.Bool("calibrate", false, "point at the green and orange frets with the mouse instead of detecting them")
		poll      = flag.Duration("poll", 4*time.Millisecond, "how often to read the screen")
		dryRun    = flag.Bool("dry-run", false, "watch and report notes but do not press any keys")
		debug     = flag.Bool("debug", false, "log every key press and save a screenshot of what was detected")
		analyze   = flag.String("analyze", "", "detect the fretboard in a screenshot file (PNG or JPEG) and save an annotated copy")
		showVer   = flag.Bool("version", false, "print the version and exit")
	)
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Guitar Flash AutoPlay %s\n\n", version)
		fmt.Fprintf(out, "Usage: %s [flags]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(out, "Start it, open a song on guitarflash.com and click on the game.")
		fmt.Fprintln(out, "Stop it with Ctrl+C, or by moving the mouse to the top-left corner of the screen.")
		fmt.Fprintln(out)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVer {
		fmt.Println(version)
		return 0
	}
	p.Offset = *offset
	p.Hold = !*noHold
	if *analyze != "" {
		if err := analyzeFile(*analyze, p); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	}

	o := options{params: p, poll: *poll, display: *display, calibrate: *calibrate, dryRun: *dryRun, debug: *debug}
	names := strings.Split(*keys, ",")
	if len(names) != 5 {
		fmt.Fprintf(os.Stderr, "error: -keys needs exactly 5 keys, got %q\n", *keys)
		return 2
	}
	for i, n := range names {
		k, err := platform.ParseKey(n)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 2
		}
		o.keys[i] = k
	}
	if *frets != "" {
		b, err := parseFrets(*frets)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 2
		}
		o.frets = &b
	}

	if problems := platform.Check(true, !o.dryRun); len(problems) > 0 {
		for _, pr := range problems {
			fmt.Fprintf(os.Stderr, "%s\n  -> %s\n", pr.What, pr.Fix)
		}
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := play(ctx, o); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Println("Bye!")
	return 0
}

func parseFrets(s string) (vision.Board, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return vision.Board{}, fmt.Errorf("-frets needs x1,y1,x2,y2, got %q", s)
	}
	var v [4]int
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return vision.Board{}, fmt.Errorf("-frets: %q is not a number", p)
		}
		v[i] = n
	}
	if v[2]-v[0] < 20 {
		return vision.Board{}, fmt.Errorf("-frets: the orange fret must be to the right of the green one")
	}
	return vision.ManualBoard(image.Pt(v[0], v[1]), image.Pt(v[2], v[3])), nil
}

var errQuit = errors.New("stopped with the mouse")

func play(ctx context.Context, o options) error {
	fmt.Printf("Guitar Flash AutoPlay %s\n", version)
	fmt.Printf("Keys: %s  (change with -keys to match the game's \"Setting Keys\")\n", keyNames(o.keys))
	if o.dryRun {
		fmt.Println("Dry run: no keys will be pressed.")
	}
	fmt.Println("Stop with Ctrl+C, or move the mouse to the top-left corner of the screen.")

	manual := o.frets
	if o.calibrate {
		b, err := calibrate()
		if err != nil {
			return err
		}
		manual = &b
	}

	if manual == nil {
		// Give the user a moment to switch to the browser, so the keys do
		// not end up in this terminal.
		fmt.Println("Click on the game window now. Starting in 3 seconds...")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}

	eng := engine.New(o.params)
	for {
		var board vision.Board
		if manual != nil {
			board = *manual
		} else {
			fmt.Println("\nLooking for the Guitar Flash fretboard... open a song and click on the game.")
			var err error
			if board, err = findBoard(ctx, o); err != nil {
				return err
			}
		}
		fmt.Printf("Found the fretboard: %s\n", board)
		err := follow(ctx, o, eng, board, manual == nil)
		if err != nil {
			if errors.Is(err, errQuit) {
				fmt.Println("\nMouse in the top-left corner: stopping.")
				return nil
			}
			return err
		}
		fmt.Println("The fretboard is gone (song over, or the page moved).")
	}
}

// findBoard searches the displays until the fretboard shows up at the same
// place twice in a row, so a board that is still sliding in is not used.
func findBoard(ctx context.Context, o options) (vision.Board, error) {
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
			if o.display >= 0 && i != o.display {
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
			if o.debug {
				saveDebug(shot, *found, o.params)
			}
			return *found, nil
		}
		last = found
		if !hinted && time.Since(start) > 30*time.Second {
			hinted = true
			fmt.Println("Still looking. Make sure the whole fretboard is visible and not covered.")
			fmt.Println("If it never gets found, run with -calibrate to point at the frets yourself.")
		}
		select {
		case <-ctx.Done():
			return vision.Board{}, ctx.Err()
		case <-time.After(300 * time.Millisecond):
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

// follow plays until the board disappears or the context is cancelled.
func follow(ctx context.Context, o options, eng *engine.Engine, board vision.Board, checkBoard bool) error {
	p := o.params
	upper, lower := vision.NewLine(board, p.Upper), vision.NewLine(board, p.Lower)
	region := upper.Bounds().Union(lower.Bounds()).Inset(-2)
	rings := image.Rect(
		int(board.FretX(0)-board.RingW), int(board.Y-board.RingH),
		int(board.FretX(4)+board.RingW)+1, int(board.Y+board.RingH)+1)

	var notes int
	sched := engine.NewScheduler(keyboard{o.keys, o.dryRun}, func(a engine.Action, late time.Duration, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "key %s: %v\n", o.keys[a.Lane].Name, err)
			return
		}
		if o.debug && a.Down {
			fmt.Printf("  press %-6s %-3s late %v\n", vision.FretColors[a.Lane], o.keys[a.Lane].Name, late.Round(time.Millisecond))
		}
	})
	eng.Reset()
	// Stop drops pending presses and lifts every key that is still down.
	defer sched.Stop()

	ticker := time.NewTicker(o.poll)
	defer ticker.Stop()
	lastSeen, lastCheck, lastStatus := time.Now(), time.Now(), time.Now()
	var failures int
	measured := false
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
		up, flashU := upper.Read(img)
		low, flashL := lower.Read(img)
		acts := eng.Update(t, up, low, flashU || flashL)
		for _, a := range acts {
			if a.Down {
				notes++
			}
		}
		sched.Add(acts)

		if v, ok := eng.Speed(); ok && !measured {
			measured = true
			fmt.Printf("Note speed: %.1f fret spacings per second.\n", v)
		}
		if t.Sub(lastStatus) > 10*time.Second && notes > 0 {
			lastStatus = t
			v, _ := eng.Speed()
			fmt.Printf("%d notes played (speed %.1f).\n", notes, v)
		}

		if t.Sub(lastCheck) < 250*time.Millisecond {
			continue
		}
		lastCheck = t
		if c, err := platform.Cursor(); err == nil && c.X <= 2 && c.Y <= 2 {
			return errQuit
		}
		if !checkBoard {
			continue
		}
		if img, err := platform.Capture(rings); err == nil && board.Present(img) {
			lastSeen = t
		} else if t.Sub(lastSeen) > 3*time.Second {
			return nil
		}
	}
}

func calibrate() (vision.Board, error) {
	in := bufio.NewReader(os.Stdin)
	point := func(name string) (image.Point, error) {
		fmt.Printf("Hover the mouse over the centre of the %s fret, then press Enter here.", name)
		if _, err := in.ReadString('\n'); err != nil {
			return image.Point{}, err
		}
		return platform.Cursor()
	}
	g, err := point("GREEN (leftmost)")
	if err != nil {
		return vision.Board{}, err
	}
	or, err := point("ORANGE (rightmost)")
	if err != nil {
		return vision.Board{}, err
	}
	if or.X-g.X < 20 {
		return vision.Board{}, fmt.Errorf("the orange fret must be to the right of the green one")
	}
	fmt.Printf("Next time you can skip this step with: -frets %d,%d,%d,%d\n", g.X, g.Y, or.X, or.Y)
	fmt.Println("Now click on the game so it has the keyboard.")
	return vision.ManualBoard(g, or), nil
}

func analyzeFile(path string, p engine.Params) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Rect, src, src.Bounds().Min, draw.Src)

	board, err := vision.FindBoard(img)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	fmt.Printf("Found the fretboard (%d of 5 rings): %s\n", board.Found, board)
	for _, l := range []vision.Line{vision.NewLine(board, p.Upper), vision.NewLine(board, p.Lower)} {
		cov, flash := l.Read(img)
		fmt.Printf("  sensor %.1f spacings above the frets: ", l.Height)
		for k, c := range cov {
			fmt.Printf("%s %3.0f%%  ", vision.FretColors[k], c*100)
		}
		if flash {
			fmt.Print("(flash)")
		}
		fmt.Println()
	}
	out := strings.TrimSuffix(path, filepath.Ext(path)) + "-debug.png"
	if err := writeOverlay(out, img, board, p); err != nil {
		return err
	}
	fmt.Println("Saved", out)
	return nil
}

func saveDebug(img *image.RGBA, b vision.Board, p engine.Params) {
	out := "guitarflash-debug.png"
	if err := writeOverlay(out, img, b, p); err != nil {
		fmt.Fprintln(os.Stderr, "debug screenshot:", err)
		return
	}
	fmt.Println("Saved what was detected to", out)
}

func writeOverlay(path string, img *image.RGBA, b vision.Board, p engine.Params) error {
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

func keyNames(keys [5]platform.Key) string {
	var s []string
	for _, k := range keys {
		s = append(s, k.Name)
	}
	return strings.Join(s, " ")
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
