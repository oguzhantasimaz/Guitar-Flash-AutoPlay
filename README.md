# Guitar Flash AutoPlay

An example app that plays [Guitar Flash](https://guitarflash.com) by itself on
**Windows** and **macOS**. It watches the screen, sees the notes coming down
the fretboard and presses the fret keys at the right moment.

It finds the fretboard on its own, so there are **no coordinates to edit**: it
works at any screen resolution, browser zoom level, Retina / HiDPI screen,
Windows display scaling (100% – 200%) and on multi-monitor setups. It also
measures how fast the notes scroll, so the timing is right on every
difficulty.

[Watch it play on YouTube](https://youtube.com/shorts/noo72JP1h3k?feature=share "Working Example")

> This is a fun project to show how screen-reading bots work. Please don't use
> it to submit scores to the rankings or in multiplayer duels.

## Quick start

### 1. Download

Get the zip for your system from the [Releases](../../releases) page (or from
the latest run on the [Actions](../../actions) tab, under *Artifacts*):

| System | File |
| --- | --- |
| Windows (most PCs) | `guitarflash-autoplay-windows-x64.zip` |
| Windows on ARM | `guitarflash-autoplay-windows-arm64.zip` |
| macOS (Apple Silicon and Intel) | `guitarflash-autoplay-macos.zip` |

### 2. Run it

**Windows**

1. Unzip and double-click `guitarflash-autoplay.exe`.
2. If Windows shows *"Windows protected your PC"*, click **More info →
   Run anyway**. The app is not signed, which is why Windows warns about it.

**macOS**

1. Unzip, open **Terminal** and run:

   ```sh
   cd ~/Downloads/guitarflash-autoplay-macos
   xattr -d com.apple.quarantine guitarflash-autoplay   # the app is not notarized
   ./guitarflash-autoplay
   ```

2. The first time, macOS asks for two permissions **for your terminal app**
   (Terminal, iTerm, …). Turn both on in **System Settings → Privacy &
   Security**, then quit and reopen the terminal:
   - **Screen & System Audio Recording**, so it can see the game
   - **Accessibility**, so it can press keys

### 3. Play

1. Open [guitarflash.com](https://guitarflash.com) and pick a song. Use
   the **Tap** game style (the default). Expert is the most fun to watch.
2. Start the app, then **click on the game** within 3 seconds so the browser
   gets the keyboard.
3. The app prints `Found the fretboard` and starts playing. Once the first
   note has gone by, it also prints the note speed it measured.

To stop, press **Ctrl+C** in the terminal, or **move the mouse to the
top-left corner** of the screen.

The app keeps running between songs: when the fretboard disappears it goes
back to searching and picks up the next song on its own.

## Options

Run `guitarflash-autoplay -h` to see them all.

| Option | What it does |
| --- | --- |
| `-keys a,s,j,k,l` | The five fret keys, green to orange. Change it if you changed *Setting Keys* in the game (e.g. `-keys 1,2,3,4,5`). |
| `-offset 15ms` | Press every note later (`15ms`) or earlier (`-15ms`). See [Timing](#timing). |
| `-no-hold` | Tap every note instead of holding keys through sustained notes. |
| `-display 1` | Only look at one display (0 is the main one). |
| `-calibrate` | Point at the green and orange frets with the mouse instead of detecting them. |
| `-frets x1,y1,x2,y2` | Give the green and orange fret centres directly (printed by `-calibrate`). |
| `-dry-run` | Watch and report, but don't press keys. |
| `-debug` | Log every key press and save `guitarflash-debug.png` showing what was detected. |
| `-analyze shot.png` | Run detection on a screenshot file and save `shot-debug.png`. Works on any OS. |

On macOS, keys are sent by their position on a US keyboard, so on other
layouts pick the key that sits where the US key would be.

## Timing

The app presses each key when the centre of the note reaches the centre of
the fret. If your hits are consistently a little early or late (it depends a
bit on the computer and the browser), nudge them with `-offset`:

```sh
guitarflash-autoplay -offset 10ms    # notes were hit too early
guitarflash-autoplay -offset -10ms   # notes were hit too late
```

## How it works

For everyone who asked "how do you do this?", here is the whole idea. The
code is small and commented, so it's a good place to start reading too.

**1. Find the fretboard.** The game draws into a fixed 822×580 canvas that
the browser scales up or down, so the layout never changes, only its size and
position. The app takes a screenshot and looks for the five fret rings:
blobs of green, red, yellow, blue and orange, shaped like flat ellipses,
evenly spaced on one line. That gives the position of every fret and the
**spacing** between them ([`internal/vision/board.go`](internal/vision/board.go)).

**2. Measure everything in "fret spacings".** From then on, every distance is
a multiple of the spacing, never a pixel count, and that's what makes it work
at every resolution. The highway is drawn in perspective: its edges meet 4.44
spacings above the frets, so a lane at height `h` is squeezed by
`1 - h / 4.44` towards the middle.

```
            \    \   |   /    /       the lanes meet 4.44 spacings up
         ....\....\..|../..../....    upper sensor, 2 spacings above the frets
              \    \ | /    /
          .....\....\|/..../.....     lower sensor, 1 spacing above the frets
               (G) (R) (Y) (B) (O)    fret rings, 1 spacing apart
```

**3. Watch two sensor lines.** Just above the frets the app reads two short
horizontal strips across each lane, over a hundred times a second. It
captures only those pixels, so each read is tiny and fast. A strip is "covered" when its
pixels are bright and saturated (the coloured gem) or white (its cap).
A gem covers about 90–100% of a strip, while the thin tail of a sustained note
covers only 10–30%, which is how notes and tails are told apart
([`internal/vision/sensor.go`](internal/vision/sensor.go)).

**4. Measure the speed.** Every note crosses the upper sensor first and the
lower one a moment later. Distance (1 spacing) divided by that time is the
scroll speed, averaged over recent notes, so it adapts to every difficulty
without any setting.

**5. Press on time.** When a note reaches the lower sensor, the app knows
exactly how far it still has to go, so it schedules the key press for the
moment the note reaches the fret. It holds the key while a sustain tail
passes, and ignores the white flash of special effects
([`internal/engine`](internal/engine)).

**Resolution and DPI details.** On Windows the app declares itself DPI aware,
so screenshots and coordinates are real pixels even at 150% scaling. On
macOS it works in points (the "looks like" resolution), so Retina screens
behave like any other.

## Troubleshooting

- **"Looking for the Guitar Flash fretboard…" forever**: make sure the
  whole fretboard (all five rings) is visible and not covered by another
  window. On macOS, check the Screen Recording permission. You can also run
  with `-debug`, or take a screenshot and run `-analyze` on it to see what the
  app sees. As a last resort, `-calibrate` lets you point at the frets.
- **It finds the board but no keys arrive**: click on the game so the
  browser has keyboard focus. On macOS, check the Accessibility permission.
  Make sure `-keys` matches the game's *Setting Keys*.
- **Notes are missed or hit early/late**: try `-offset`, close heavy
  programs, and keep the browser tab in front.

## Build from source

You need [Go](https://go.dev/dl/) 1.23 or newer.

```sh
git clone https://github.com/oguzhantasimaz/Guitar-Flash-AutoPlay
cd Guitar-Flash-AutoPlay
go build ./cmd/guitarflash-autoplay
```

- **Windows** needs nothing else; no C compiler is required. You can even
  build it from macOS or Linux with `GOOS=windows go build ./cmd/guitarflash-autoplay`.
- **macOS** needs the Xcode command line tools (`xcode-select --install`)
  because it calls CoreGraphics.

Run the tests with `go test ./...`. They run on any OS, and draw synthetic
fretboards at sizes from tiny to 4K.

To publish a release with ready-made downloads, push a tag and GitHub Actions
builds and uploads everything:

```sh
git tag v3.0.0 && git push origin v3.0.0
```

### Project layout

| Path | What's inside |
| --- | --- |
| `cmd/guitarflash-autoplay` | The app: options, search loop, play loop |
| `internal/vision` | Finding the fretboard and reading the lanes (pure Go, no OS code) |
| `internal/engine` | Note tracking, speed measurement and key-press timing (pure Go) |
| `internal/platform` | Screen capture, key presses and permissions for Windows and macOS |

## History

- **v3**: rewritten as a cross-platform app with automatic fretboard
  detection, resolution/DPI support and measured note speed.
- **v2**: Go version with hard-coded coordinates for one screen.
- **v1**: Python version.

Feel free to contribute!
