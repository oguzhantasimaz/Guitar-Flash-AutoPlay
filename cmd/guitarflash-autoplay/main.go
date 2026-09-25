// Command guitarflash-autoplay plays Guitar Flash (guitarflash.com) by
// watching the screen and pressing the fret keys.
//
// Started without arguments (e.g. by double-clicking it) it opens a window.
// With flags it runs in the terminal; see -h.
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
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/engine"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/platform"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/player"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/ui"
	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/vision"
)

var version = "dev"

func main() {
	if wantWindow(os.Args[1:]) {
		ui.Run(version) // does not return
	}
	platform.UseParentConsole()
	code := run()
	if code != 0 {
		platform.PauseBeforeExit()
	}
	os.Exit(code)
}

// wantWindow reports whether to open the window: when there are no
// arguments, as when the app is double-clicked. (Old macOS versions pass a
// -psn_ argument to apps opened from the Finder.)
func wantWindow(args []string) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "-psn_") {
			return false
		}
	}
	return true
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

	cfg := player.DefaultConfig()
	cfg.Params, cfg.Poll, cfg.Display, cfg.DryRun = p, *poll, *display, *dryRun
	if *debug {
		cfg.DebugShot = "guitarflash-debug.png"
	}
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
		cfg.Keys[i] = k
	}
	if *frets != "" {
		b, err := parseFrets(*frets)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 2
		}
		cfg.Board = &b
	}

	if problems := platform.Check(true, !cfg.DryRun); len(problems) > 0 {
		for _, pr := range problems {
			fmt.Fprintf(os.Stderr, "%s\n  -> %s\n", pr.What, pr.Fix)
		}
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := play(ctx, cfg, *calibrate, *debug); err != nil && !errors.Is(err, context.Canceled) {
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

// cli prints what the player does to the terminal.
type cli struct {
	keys  [5]platform.Key
	debug bool
	mu    sync.Mutex
	state player.State
	notes int
}

func (c *cli) State(s player.State, b vision.Board) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == player.Playing && s != player.Playing && c.notes > 0 {
		fmt.Printf("%d notes played.\n", c.notes)
	}
	c.state = s
	switch s {
	case player.Starting:
		fmt.Println("Click on the game window now. Starting in 3 seconds...")
	case player.Searching:
		fmt.Println("\nLooking for the Guitar Flash fretboard... open a song and click on the game.")
	case player.Playing:
		c.notes = 0
		fmt.Printf("Found the fretboard: %s\n", b)
	}
}

func (c *cli) Log(msg string) { fmt.Println(msg) }

func (c *cli) Speed(v float64) { fmt.Printf("Note speed: %.1f fret spacings per second.\n", v) }

func (c *cli) Key(lane int, down bool) {
	if !down {
		return
	}
	c.mu.Lock()
	c.notes++
	c.mu.Unlock()
	if c.debug {
		fmt.Printf("  press %-6s %s\n", vision.FretColors[lane], c.keys[lane].Name)
	}
}

func (c *cli) Preview(*image.RGBA) {}

func play(ctx context.Context, cfg player.Config, calibrateFirst, debug bool) error {
	fmt.Printf("Guitar Flash AutoPlay %s\n", version)
	fmt.Printf("Keys: %s  (change with -keys to match the game's \"Setting Keys\")\n", keyNames(cfg.Keys))
	if cfg.DryRun {
		fmt.Println("Dry run: no keys will be pressed.")
	}
	fmt.Println("Stop with Ctrl+C, or move the mouse to the top-left corner of the screen.")
	if calibrateFirst {
		b, err := calibrate()
		if err != nil {
			return err
		}
		cfg.Board = &b
	}
	if cfg.Board != nil {
		cfg.StartDelay = 0 // calibrating already ends with a click on the game
	}
	err := player.Run(ctx, cfg, &cli{keys: cfg.Keys, debug: debug})
	if errors.Is(err, player.ErrMouseStop) {
		fmt.Println("\nMouse in the top-left corner: stopping.")
		return nil
	}
	return err
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
	if err := player.WriteOverlay(out, img, board, p); err != nil {
		return err
	}
	fmt.Println("Saved", out)
	return nil
}

func keyNames(keys [5]platform.Key) string {
	var s []string
	for _, k := range keys {
		s = append(s, k.Name)
	}
	return strings.Join(s, " ")
}
