package ui

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/oguzhantasimaz/Guitar-Flash-AutoPlay/internal/player"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	colBg      = rgb(0x14141b)
	colPanel   = rgb(0x1f1f2a)
	colField   = rgb(0x2a2a38)
	colText    = rgb(0xececf1)
	colMuted   = rgb(0x9a9aae)
	colAccent  = rgb(0xe53935)
	colStop    = rgb(0x44445a)
	colGood    = rgb(0x43c463)
	colWait    = rgb(0xffb300)
	colProblem = rgb(0x5a1f24)

	// The fret colours, as the game draws them.
	fretColors = [5]color.NRGBA{rgb(0x0b9712), rgb(0xee0e0d), rgb(0xe1e214), rgb(0x1192e3), rgb(0xee6310)}
	fretNames  = [5]string{"Green", "Red", "Yellow", "Blue", "Orange"}
)

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

// layout draws the whole window.
func (u *UI) layout(gtx C) D {
	paint.Fill(gtx.Ops, colBg)
	u.mu.Lock()
	st := u.st
	st.logs = append([]logLine(nil), u.st.logs...)
	frets := u.settings.Frets
	u.mu.Unlock()

	// The header, status and Start button stay put; the rest scrolls.
	var sections []layout.Widget
	for i, p := range st.problems {
		i, p := i, p
		sections = append(sections, func(gtx C) D { return u.problemCard(gtx, i, p.What, p.Fix, p.Settings != "") })
	}
	sections = append(sections,
		func(gtx C) D { return u.previewCard(gtx, st) },
		func(gtx C) D {
			if st.running || st.calibrating != "" {
				// Locked while playing, so the bot's own key presses
				// cannot type into the key fields.
				return dimmed(gtx, func(gtx C) D { return u.settingsCard(gtx.Disabled(), st, frets != nil) })
			}
			return u.settingsCard(gtx, st, frets != nil)
		},
		func(gtx C) D { return u.logCard(gtx, st) },
		func(gtx C) D {
			return label(gtx, u.th, 12, colMuted, "Stop any time: move the mouse to the top-left corner of the screen.")
		},
	)
	gap := layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout)
	return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D { return u.header(gtx) }),
			gap,
			layout.Rigid(func(gtx C) D { return u.statusCard(gtx, st) }),
			gap,
			layout.Rigid(func(gtx C) D { return u.startButton(gtx, st) }),
			gap,
			layout.Flexed(1, func(gtx C) D {
				return material.List(u.th, &u.page).Layout(gtx, len(sections), func(gtx C, i int) D {
					return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, sections[i])
				})
			}),
		)
	})
}

func (u *UI) header(gtx C) D {
	return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
		layout.Flexed(1, func(gtx C) D {
			l := material.Label(u.th, 22, "Guitar Flash AutoPlay")
			l.Font.Weight = font.Bold
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D { return label(gtx, u.th, 12, colMuted, u.version) }),
	)
}

func (u *UI) statusCard(gtx C, st status) D {
	dot, title, detail := colMuted, "Ready", "Open a song on guitarflash.com, then press Start."
	switch {
	case st.calibrating != "":
		dot, title, detail = colWait, "Pointing at the frets", st.calibrating
	case !st.running:
	case st.state == player.Starting:
		dot, title, detail = colWait, "Starting...", "Click on the game now so it gets the keyboard."
	case st.state == player.Searching:
		dot, title, detail = colWait, "Looking for the fretboard", "Start a song. The whole fretboard has to be visible."
	case st.state == player.Playing:
		dot, title = colGood, "Playing"
		detail = fmt.Sprintf("%d notes played", st.notes)
		if st.speed > 0 {
			detail += fmt.Sprintf(" · note speed %.1f", st.speed)
		}
	}
	return card(gtx, colPanel, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				d := gtx.Dp(12)
				paint.FillShape(gtx.Ops, dot, clip.Ellipse{Max: image.Pt(d, d)}.Op(gtx.Ops))
				return D{Size: image.Pt(d, d)}
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			layout.Flexed(1, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						l := material.Label(u.th, 17, title)
						l.Font.Weight = font.SemiBold
						return l.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D { return label(gtx, u.th, 13, colMuted, detail) }),
					layout.Rigid(func(gtx C) D {
						if st.notice == "" || st.running {
							return D{}
						}
						return label(gtx, u.th, 13, colWait, st.notice)
					}),
				)
			}),
		)
	})
}

func (u *UI) problemCard(gtx C, i int, what, fix string, hasSettings bool) D {
	return card(gtx, colProblem, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				l := material.Label(u.th, 15, what)
				l.Font.Weight = font.SemiBold
				return l.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D { return label(gtx, u.th, 13, colText, fix) }),
			layout.Rigid(func(gtx C) D {
				if !hasSettings || i >= len(u.settingsBtns) {
					return D{}
				}
				return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
					b := material.Button(u.th, &u.settingsBtns[i], "Open System Settings")
					b.Background = colAccent
					return b.Layout(gtx)
				})
			}),
		)
	})
}

// previewCard shows what the player sees and lights up the fret keys as
// they are pressed.
func (u *UI) previewCard(gtx C, st status) D {
	return card(gtx, colPanel, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				h := gtx.Dp(170)
				size := image.Pt(gtx.Constraints.Max.X, h)
				rect := image.Rectangle{Max: size}
				paint.FillShape(gtx.Ops, colBg, clip.UniformRRect(rect, gtx.Dp(8)).Op(gtx.Ops))
				gtx.Constraints = layout.Exact(size)
				if st.hasPreview {
					defer clip.UniformRRect(rect, gtx.Dp(8)).Push(gtx.Ops).Pop()
					return widget.Image{Src: st.preview, Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
				}
				msg := "The fretboard shows up here while a song is playing."
				if st.running && st.state == player.Searching {
					msg = "Looking for the fretboard..."
				}
				return layout.Center.Layout(gtx, func(gtx C) D {
					l := material.Label(u.th, 13, msg)
					l.Color, l.Alignment = colMuted, text.Middle
					return l.Layout(gtx)
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx C) D { return u.laneLights(gtx, st) }),
		)
	})
}

// laneLights draws one light per fret; it glows while the key is held and
// for a moment after each press, so quick taps are visible too.
func (u *UI) laneLights(gtx C, st status) D {
	const glow = 150 * time.Millisecond
	children := make([]layout.FlexChild, 5)
	for k := 0; k < 5; k++ {
		k := k
		children[k] = layout.Flexed(1, func(gtx C) D {
			return layout.Center.Layout(gtx, func(gtx C) D {
				d := gtx.Dp(46)
				since := gtx.Now.Sub(st.lastPress[k])
				lit := st.keyDown[k] || (!st.lastPress[k].IsZero() && since < glow)
				if !st.keyDown[k] && lit {
					gtx.Execute(op.InvalidateCmd{At: st.lastPress[k].Add(glow)})
				}
				r := image.Rectangle{Max: image.Pt(d, d)}
				fill := withAlpha(fretColors[k], 50)
				if lit {
					fill = fretColors[k]
				}
				paint.FillShape(gtx.Ops, fill, clip.Ellipse(r).Op(gtx.Ops))
				paint.FillShape(gtx.Ops, fretColors[k], clip.Stroke{Path: clip.Ellipse(r).Path(gtx.Ops), Width: float32(gtx.Dp(3))}.Op())
				name := u.keyEd[k].Text()
				gtx.Constraints = layout.Exact(r.Max)
				return layout.Center.Layout(gtx, func(gtx C) D {
					l := material.Label(u.th, 16, name)
					l.Font.Weight = font.Bold
					l.Color = colText
					if lit && k == 2 {
						l.Color = colBg // dark text on the bright yellow
					}
					return l.Layout(gtx)
				})
			})
		})
	}
	return layout.Flex{}.Layout(gtx, children...)
}

func (u *UI) settingsCard(gtx C, st status, manualFrets bool) D {
	return card(gtx, colPanel, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D { return heading(gtx, u.th, "Fret keys") }),
			layout.Rigid(func(gtx C) D {
				return label(gtx, u.th, 12, colMuted, "The keys set in the game's \"Setting Keys\", green to orange.")
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx C) D { return u.keyEditors(gtx) }),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

			layout.Rigid(func(gtx C) D { return heading(gtx, u.th, "Timing") }),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx C) D {
						s := material.Slider(u.th, &u.offset)
						s.Color = colAccent
						return s.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Dp(110)
						return label(gtx, u.th, 13, colText, offsetText(sliderToOffset(u.offset.Value)))
					}),
				)
			}),
			layout.Rigid(func(gtx C) D {
				return label(gtx, u.th, 12, colMuted, "Move this right if notes are hit too early, left if too late.")
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

			layout.Rigid(func(gtx C) D {
				c := material.CheckBox(u.th, &u.hold, "Hold keys through long notes")
				c.IconColor = colAccent
				return c.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				c := material.CheckBox(u.th, &u.practice, "Watch only (don't press any keys)")
				c.IconColor = colAccent
				return c.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

			layout.Rigid(func(gtx C) D { return heading(gtx, u.th, "Fretboard") }),
			layout.Rigid(func(gtx C) D {
				msg := "Found automatically on any screen."
				if manualFrets {
					msg = "Set by hand. Use this if it is never found automatically."
				}
				return label(gtx, u.th, 12, colMuted, msg)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx C) D {
				children := []layout.FlexChild{
					layout.Rigid(func(gtx C) D {
						return smallButton(gtx, u.th, &u.calibrateBtn, "Point at the frets")
					}),
				}
				if manualFrets {
					children = append(children,
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx C) D {
							return smallButton(gtx, u.th, &u.autoBtn, "Find automatically")
						}))
				}
				return layout.Flex{}.Layout(gtx, children...)
			}),
		)
	})
}

func (u *UI) keyEditors(gtx C) D {
	children := make([]layout.FlexChild, 0, 9)
	for k := 0; k < 5; k++ {
		k := k
		if k > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
		}
		children = append(children, layout.Flexed(1, func(gtx C) D {
			// A plain border with a solid colour bar, not a coloured outline:
			// five outlined boxes in the fret colours would look like fret
			// rings to the detector.
			return widget.Border{Color: colStop, CornerRadius: unit.Dp(6), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx C) D {
						size := gtx.Constraints.Min
						rect := image.Rectangle{Max: size}
						paint.FillShape(gtx.Ops, colField, clip.UniformRRect(rect, gtx.Dp(6)).Op(gtx.Ops))
						bar := image.Rect(gtx.Dp(6), size.Y-gtx.Dp(5), size.X-gtx.Dp(6), size.Y-gtx.Dp(2))
						paint.FillShape(gtx.Ops, fretColors[k], clip.UniformRRect(bar, gtx.Dp(1)).Op(gtx.Ops))
						return D{Size: size}
					}),
					layout.Stacked(func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
							e := material.Editor(u.th, &u.keyEd[k], fretNames[k])
							e.TextSize = 16
							e.Font.Weight = font.Bold
							e.HintColor = colMuted
							return e.Layout(gtx)
						})
					}),
				)
			})
		}))
	}
	return layout.Flex{}.Layout(gtx, children...)
}

func (u *UI) startButton(gtx C, st status) D {
	txt, bg := "Start", colAccent
	if st.running {
		txt, bg = "Stop", colStop
	}
	if st.calibrating != "" {
		gtx = gtx.Disabled()
	}
	b := material.Button(u.th, &u.startBtn, txt)
	b.Background = bg
	b.TextSize = 18
	b.CornerRadius = unit.Dp(10)
	b.Inset = layout.UniformInset(unit.Dp(14))
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return b.Layout(gtx)
}

func (u *UI) logCard(gtx C, st status) D {
	return card(gtx, colPanel, func(gtx C) D {
		gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(130)))
		return material.List(u.th, &u.logList).Layout(gtx, len(st.logs), func(gtx C, i int) D {
			l := st.logs[i]
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return label(gtx, u.th, 12, colMuted, l.at.Format("15:04:05")+"  ")
				}),
				layout.Flexed(1, func(gtx C) D { return label(gtx, u.th, 12, colText, l.msg) }),
			)
		})
	})
}

func offsetText(ms int) string {
	switch {
	case ms > 0:
		return fmt.Sprintf("+%d ms later", ms)
	case ms < 0:
		return fmt.Sprintf("%d ms earlier", ms)
	}
	return "on time"
}

// card draws w on a rounded panel spanning the full width.
func card(gtx C, bg color.NRGBA, w layout.Widget) D {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx C) D {
			rect := image.Rectangle{Max: gtx.Constraints.Min}
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(rect, gtx.Dp(10)).Op(gtx.Ops))
			return D{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.UniformInset(unit.Dp(14)).Layout(gtx, w)
		}),
	)
}

// dimmed draws w with a veil over it, for parts that cannot be used now.
func dimmed(gtx C, w layout.Widget) D {
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(w),
		layout.Expanded(func(gtx C) D {
			rect := image.Rectangle{Max: gtx.Constraints.Min}
			paint.FillShape(gtx.Ops, withAlpha(colBg, 140), clip.UniformRRect(rect, gtx.Dp(10)).Op(gtx.Ops))
			return D{Size: gtx.Constraints.Min}
		}),
	)
}

func label(gtx C, th *material.Theme, size unit.Sp, c color.NRGBA, txt string) D {
	l := material.Label(th, size, txt)
	l.Color = c
	return l.Layout(gtx)
}

func heading(gtx C, th *material.Theme, txt string) D {
	l := material.Label(th, 15, txt)
	l.Font.Weight = font.SemiBold
	return l.Layout(gtx)
}

func smallButton(gtx C, th *material.Theme, c *widget.Clickable, txt string) D {
	b := material.Button(th, c, txt)
	b.Background = colField
	b.TextSize = 14
	b.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}
	return b.Layout(gtx)
}
