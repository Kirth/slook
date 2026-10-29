package main

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	"golang.org/x/image/math/fixed"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/motion"
	"github.com/gammons/slk/gui/palette"
	"github.com/gammons/slk/gui/sidebar"
	"github.com/gammons/slk/internal/ui/styles"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

// The colours everything is drawn with. They are the palette config
// chooses (see gui/palette), set by applyPalette before the first
// window; until then, the default palette's.
var (
	colBg           color.NRGBA
	colFg           color.NRGBA
	colMuted        color.NRGBA
	colAccent       color.NRGBA
	colOnAccent     color.NRGBA // text on an accent or badge fill
	colBorder       color.NRGBA
	colError        color.NRGBA
	colHover        color.NRGBA
	colOffline      color.NRGBA
	colCodeFg       color.NRGBA // inline code
	colCodeBg       color.NRGBA
	colBroadcast    color.NRGBA // @here, @channel
	colMention      color.NRGBA
	colEphemeral    color.NRGBA // behind a message only the user sees
	colHeader       color.NRGBA // window header bars and panels: the surface
	colOverlay      color.NRGBA // overlays, a step above the surface
	colShadow       color.NRGBA // under overlays
	colPresent      color.NRGBA // a person who is active
	colBadge        color.NRGBA // unread mentions elsewhere
	colControl      color.NRGBA // window control buttons
	colControlHover color.NRGBA
)

func init() { applyPalette(palette.Presets[palette.Default]) }

// applyPalette sets the colours from p. Call it before any window or
// theme is built: windows read the colours as they draw, and themes
// take theirs when built.
func applyPalette(p palette.Palette) {
	colBg, colHeader, colOverlay = p.Background, p.Surface, p.Overlay
	colFg, colMuted, colOnAccent = p.Text, p.TextMuted, p.OnAccent
	colAccent, colMention, colBroadcast = p.Accent, p.Mention, p.Broadcast
	colBorder, colHover, colControl, colControlHover = p.Border, p.Hover, p.Control, p.ControlHover
	colError, colOffline, colPresent, colBadge = p.Error, p.Warning, p.Success, p.Badge
	colCodeFg, colCodeBg, colEphemeral = p.CodeText, p.CodeBackground, p.Ephemeral
	colShadow = p.Shadow
}

// Text sizes, in sp: the type scale. Every label is one of these, drawn
// through bodyLabel, secondaryLabel or captionLabel (a test holds the
// package to it); only text sized from its box (initials, emoji in a
// cell, a badge) is not.
const (
	textSize      = unit.Sp(15) // message text, names: anything read
	secondarySize = unit.Sp(13) // metadata and controls
	captionSize   = unit.Sp(12) // timestamps, counts, labels
	headingSize   = unit.Sp(18)
)

// bodyLabel is text to read, at textSize.
func bodyLabel(th *material.Theme, s string) material.LabelStyle {
	return material.Label(th, textSize, s)
}

// secondaryLabel is metadata or a control's label, at secondarySize.
func secondaryLabel(th *material.Theme, s string) material.LabelStyle {
	return material.Label(th, secondarySize, s)
}

// captionLabel is a timestamp, count or small label, at captionSize.
func captionLabel(th *material.Theme, s string) material.LabelStyle {
	return material.Label(th, captionSize, s)
}

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
	faces := systemFaces()
	mono, extra := monoFaces(faces)
	if len(faces) > 0 {
		// The fontconfig faces cover what is drawn (see fontPatterns), so
		// the shaper skips its own scan of the system's fonts. Go Mono
		// joins them when fontconfig has no monospace face.
		th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(append(collection(faces), extra...)))
		th.Face = faceList(faces, "sans-serif", fontRoleSans, fontRoleCJK, fontRoleScripts, fontRoleEmoji)
	}
	// Without fontconfig the theme's own collection is Gio's Go fonts,
	// Go Mono among them.
	monoOnce.Do(func() { fontMono.Typeface = mono })
	th.Palette = material.Palette{Bg: colBg, Fg: colFg, ContrastBg: colAccent, ContrastFg: colOnAccent}
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
		{Typeface: th.Face, Weight: font.SemiBold},
		{Typeface: th.Face, Style: font.Italic},
		fontMono,
	} {
		for _, size := range []unit.Sp{captionSize, secondarySize, textSize} {
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
// An overlay's entrance: it slides down enterRise into place and fades
// in over enterDur.
const (
	enterDur  = 200 * time.Millisecond
	enterRise = unit.Dp(6)
)

// enter draws w as an overlay that opened at since: sliding down into
// place and fading in, then still. It asks for frames only while it
// moves.
func enter(gtx C, since time.Time, w layout.Widget) D {
	p, moving := motion.Enter(since, gtx.Now, enterDur)
	if moving {
		gtx.Execute(op.InvalidateCmd{})
	}
	defer op.Offset(image.Pt(0, -int((1-p)*float64(gtx.Dp(enterRise))))).Push(gtx.Ops).Pop()
	defer paint.PushOpacity(gtx.Ops, float32(p)).Pop()
	return w(gtx)
}

// accentBarDp is the width of the accent rule on the left of a block
// that stands apart: a message only the user sees.
const accentBarDp = unit.Dp(2)

// accentCard draws w on bg in a rounded card with an accent rule down
// its left edge.
func accentCard(gtx C, bg color.NRGBA, w layout.Widget) D {
	dims := rounded(gtx, bg, cardRadius, w)
	r := gtx.Dp(cardRadius)
	defer clip.UniformRRect(image.Rectangle{Max: dims.Size}, r).Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, colAccent, clip.Rect{Max: image.Pt(gtx.Dp(accentBarDp), dims.Size.Y)}.Op())
	return dims
}

// cardRadius is the corner radius every card and overlay shares: code
// blocks, file chips, images, the hover card, menus, dialogs.
const cardRadius = unit.Dp(6)

// overlayFrame is the panel an overlay is drawn in: the lifted overlay
// surface, a 1 dp edge, cardRadius corners and a faint shadow, so it
// reads as a layer above the window. Rows inside highlight with
// colControl, a step above colOverlay.
func overlayFrame(gtx C, content layout.Widget) D {
	return lifted(gtx, func(gtx C) D {
		return widget.Border{Color: colBorder, CornerRadius: cardRadius, Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
			return rounded(gtx, colOverlay, cardRadius, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return content(gtx)
			})
		})
	})
}

// shadowLayers are the soft shadow's rings, from the edge out: how far
// each spreads (and drops) and its share of colShadow's alpha. Gio has
// no blur; stacked translucent rounded rects stand in for one.
var shadowLayers = []struct {
	spread unit.Dp
	share  int // of 10
}{{2, 5}, {6, 3}, {12, 2}} // a soft 12 dp spread

// lifted draws w over a soft shadow cast downward, the shape of its
// bounds at cardRadius.
func lifted(gtx C, w layout.Widget) D {
	rec := op.Record(gtx.Ops)
	dims := w(gtx)
	call := rec.Stop()
	r := gtx.Dp(cardRadius)
	for i := len(shadowLayers) - 1; i >= 0; i-- {
		l := shadowLayers[i]
		s := gtx.Dp(l.spread)
		c := colShadow
		c.A = uint8(int(colShadow.A) * l.share / 10)
		rect := image.Rect(-s, -s/2, dims.Size.X+s, dims.Size.Y+s+s/2)
		paint.FillShape(gtx.Ops, c, clip.UniformRRect(rect, r+s).Op(gtx.Ops))
	}
	call.Add(gtx.Ops)
	return dims
}

func rounded(gtx C, bg color.NRGBA, radius unit.Dp, w layout.Widget) D {
	m := op.Record(gtx.Ops)
	dims := w(gtx)
	call := m.Stop()
	paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(radius)).Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

// tint is c at a low opacity over the header, for tiles.
func tint(c color.NRGBA) color.NRGBA {
	mix := func(a, b uint8) uint8 { return uint8((int(a)*35 + int(b)*65) / 100) }
	return color.NRGBA{R: mix(c.R, colHeader.R), G: mix(c.G, colHeader.G), B: mix(c.B, colHeader.B), A: 0xff}
}

// textOn is a colour for text over bg that is not a palette colour (a
// person's avatar tile): white or near-black, whichever contrasts more,
// whatever the theme.
func textOn(bg color.NRGBA) color.NRGBA {
	light, dark := rgb(0xffffff), rgb(0x1d1c1d)
	if palette.Contrast(light, bg) >= palette.Contrast(dark, bg) {
		return light
	}
	return dark
}

// convNameColor is a conversation's name in the list and the switcher:
// full text while it needs reading, slate once read, and sunk further
// when muted. Unread is told by light, not weight.
func convNameColor(c backend.Conversation) color.NRGBA {
	switch {
	case c.Muted:
		return sunk(colMuted)
	case sidebar.NeedsReading(c):
		return colFg
	}
	return colMuted
}

// sunk is c at 60 %, sinking into the background: what is there but
// asks for nothing (a muted conversation).
func sunk(c color.NRGBA) color.NRGBA {
	m := func(a, b uint8) uint8 { return uint8((int(a)*60 + int(b)*40) / 100) }
	return color.NRGBA{R: m(c.R, colBg.R), G: m(c.G, colBg.G), B: m(c.B, colBg.B), A: 0xff}
}
