package main

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/image/math/fixed"

	"github.com/gammons/slk/internal/ui/styles"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	colBg           = rgb(0x1a1d21)
	colFg           = rgb(0xd1d2d3)
	colMuted        = rgb(0x8b8d91)
	colAccent       = rgb(0x1d9bd1)
	colBorder       = rgb(0x35373b)
	colError        = rgb(0xe0526b)
	colHover        = rgb(0x27292d)
	colOffline      = rgb(0xe8a33d)
	colCodeFg       = rgb(0xf0a3a3) // inline code, like Slack's red
	colCodeBg       = rgb(0x222529)
	colBroadcast    = rgb(0xf2c744) // @here, @channel
	colMention      = rgb(0x5fb3e8)
	colEphemeral    = rgb(0x1f262d) // behind a message only the user sees
	colHeader       = rgb(0x222529) // window header bars
	colPresent      = rgb(0x2bac76) // a person who is active
	colBadge        = rgb(0xe0526b) // unread mentions elsewhere
	colControl      = rgb(0x2f3236) // window control buttons
	colControlHover = rgb(0x3d4045)
)

// Text sizes, in sp.
const (
	textSize    = unit.Sp(15)
	captionSize = unit.Sp(12)
	headingSize = unit.Sp(18)
)

// fontMono is for code: the system's monospace font (see fonts.go),
// else whatever Gio resolves "monospace" to.
var fontMono = font.Font{Typeface: "monospace"}

// monoOnce sets fontMono from the first window's fonts; every window
// gets the same files, so the names are the same.
var monoOnce sync.Once

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}

// newTheme builds a window's theme. Each window needs its own: a
// text.Shaper is not safe to share between windows' goroutines. Its
// fonts are the ones fontconfig picks (see fonts.go): the text face,
// then CJK and emoji faces for the glyphs it lacks.
//
// It takes the theme prepared ahead of time when there is one. The
// window calls refillThemes once its first frame is out, so preparing
// the next does not compete with drawing this one.
func newTheme() *material.Theme {
	select {
	case th := <-readyThemes:
		return th
	default:
	}
	return buildTheme()
}

// readyThemes holds a theme built and warmed up ahead of time, so a new
// window does not wait for one. prepareTheme fills it.
var readyThemes = make(chan *material.Theme, 1)

// preparing is set while prepareTheme runs, so one runs at a time.
var preparing atomic.Bool

// refillThemes prepares the next window's theme in the background.
func refillThemes() {
	if len(readyThemes) == 0 && preparing.CompareAndSwap(false, true) {
		go func() {
			defer preparing.Store(false)
			prepareTheme()
		}()
	}
}

// prepareTheme builds a theme, warms up its shaper, and keeps it for
// the next window. If one is already waiting, this one is dropped.
func prepareTheme() {
	th := buildTheme()
	warmUp(th)
	select {
	case readyThemes <- th:
	default:
	}
}

// buildTheme is a theme with its own shaper over the shared faces.
func buildTheme() *material.Theme {
	th := material.NewTheme()
	if faces := systemFaces(); len(faces) > 0 {
		// The fontconfig faces cover what is drawn (see fontPatterns), so
		// the shaper skips its own scan of the system's fonts.
		th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(collection(faces)))
		th.Face = faceList(faces, "sans-serif", fontRoleSans, fontRoleCJK, fontRoleScripts, fontRoleEmoji)
		monoOnce.Do(func() {
			fontMono.Typeface = faceList(faces, "monospace", fontRoleMono, fontRoleCJK, fontRoleScripts, fontRoleEmoji)
		})
	}
	th.Palette = material.Palette{Bg: colBg, Fg: colFg, ContrastBg: colAccent, ContrastFg: rgb(0xffffff)}
	th.TextSize = textSize
	return th
}

// warmUp shapes a little text in each style a window draws, so the
// shaper's lazy setup and first shaping happen before a window is
// waiting for them. Only for a theme no window has yet.
func warmUp(th *material.Theme) {
	const sample = "Ag é 15:04 # 👍 ツ"
	for _, f := range []font.Font{
		{Typeface: th.Face},
		{Typeface: th.Face, Weight: font.Bold},
		{Typeface: th.Face, Weight: font.SemiBold},
		{Typeface: th.Face, Style: font.Italic},
		fontMono,
	} {
		for _, size := range []unit.Sp{captionSize, textSize, unit.Sp(16)} {
			th.Shaper.LayoutString(text.Parameters{Font: f, PxPerEm: fixed.I(int(size)), MaxWidth: 1000}, sample)
			for {
				if _, ok := th.Shaper.NextGlyph(); !ok {
					break
				}
			}
		}
	}
}

// authorColor is the TUI's per-user color, keyed by name since that is
// what the prototype's messages carry.
func authorColor(name string) color.NRGBA {
	return color.NRGBAModel.Convert(styles.UserColor(name)).(color.NRGBA)
}

// cardColor is the stripe color of a legacy attachment: Slack's named
// colors, a hex color, or the border color when unset or unknown.
func cardColor(s string) color.NRGBA {
	switch strings.ToLower(s) {
	case "good":
		return rgb(0x2eb67d)
	case "warning":
		return rgb(0xecb22e)
	case "danger":
		return rgb(0xe01e5a)
	case "":
		return colBorder
	}
	if v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32); err == nil && len(strings.TrimPrefix(s, "#")) == 6 {
		return rgb(uint32(v))
	}
	return colBorder
}

func fillBackground(gtx C) { paint.Fill(gtx.Ops, colBg) }

// divider is a one-pixel horizontal rule.
func divider(gtx C) D {
	size := gtx.Constraints.Min
	size.X = gtx.Constraints.Max.X
	size.Y = gtx.Dp(unit.Dp(1))
	paint.FillShape(gtx.Ops, colBorder, clip.Rect{Max: size}.Op())
	return D{Size: size}
}

// framedEditor draws an editor inside a rounded border.
func framedEditor(th *material.Theme, ed *widget.Editor, hint string) layout.Widget {
	return func(gtx C) D {
		border := widget.Border{Color: colBorder, CornerRadius: unit.Dp(6), Width: unit.Dp(1)}
		return border.Layout(gtx, func(gtx C) D {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
				e := material.Editor(th, ed, hint)
				e.HintColor = colMuted
				return e.Layout(gtx)
			})
		})
	}
}

// rounded fills a rounded rectangle of the widget's size behind it.
func rounded(gtx C, bg color.NRGBA, radius unit.Dp, w layout.Widget) D {
	m := op.Record(gtx.Ops)
	dims := w(gtx)
	call := m.Stop()
	paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(radius)).Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}
