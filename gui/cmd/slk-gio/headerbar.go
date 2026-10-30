package main

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// Icons used across the windows (Material Design, drawn by Gio).
var (
	iconClose    = mustIcon(icons.NavigationClose)
	iconMinimize = mustIcon(icons.ContentRemove)
	iconMaximize = mustIcon(icons.ImageCropSquare)
	iconRestore  = mustIcon(icons.ImageFilterNone)
	iconOpenWeb  = mustIcon(icons.ActionOpenInNew)
	iconMenu     = mustIcon(icons.NavigationMenu)
	iconPeople   = mustIcon(icons.SocialPeople)
	iconLock     = mustIcon(icons.ActionLock)
	iconClock    = mustIcon(icons.ActionSchedule)
	iconNight    = mustIcon(icons.ImageBrightness3)
	iconMuted    = mustIcon(icons.SocialNotificationsOff)
	iconSearch   = mustIcon(icons.ActionSearch)
	iconExpand   = mustIcon(icons.NavigationExpandMore)
	iconCopy     = mustIcon(icons.ContentContentCopy)
)

func mustIcon(data []byte) *widget.Icon {
	ic, err := widget.NewIcon(data)
	if err != nil {
		panic(err)
	}
	return ic
}

// badgeSize is the text of the count on an action's corner, sized to
// its dot rather than by role.
const badgeSize = unit.Sp(9)

// Header bar geometry.
const (
	headerHeight = unit.Dp(52)
	leadingSize  = unit.Dp(32)
	actionSize   = unit.Dp(34)
	controlSize  = unit.Dp(24)
)

// headerAction is a button in the header bar, before the window
// controls. badge, when set, is a small count or dot on its corner in
// badgeCol.
type headerAction struct {
	icon     *widget.Icon
	onClick  func()
	active   bool // drawn pressed, e.g. while its panel is open
	badge    string
	badgeCol color.NRGBA
}

// headerChip is one item of the header's second line: an optional icon
// and text in a colour. grow takes the spare width and is cut short.
type headerChip struct {
	icon *widget.Icon
	text string
	col  color.NRGBA
	grow bool
}

// headerSpec is what a window puts in its header bar.
type headerSpec struct {
	Leading  layout.Widget // avatar or tile, 32dp; may be nil
	Title    string
	Status   string      // after the title, e.g. a DM's status emoji
	Subtitle string      // shown instead of Chips when set
	SubColor color.NRGBA // zero means muted
	Chips    []headerChip
	Actions  []headerAction
	// Seamless leaves out the rule under the bar, for a window whose
	// content continues the bar's surface (the thread's parent card).
	Seamless bool
	// OnTitle, when set, makes the title and subtitle a button with a
	// chevron after the title, turned by Expanded (0 closed, 1 open).
	// The rest of the bar still moves the window: on Wayland a press in
	// the move area goes to the compositor, so the button sits beside
	// it rather than in it.
	OnTitle  func()
	Expanded float32
}

// headerBar is a window's title bar, drawn by the app: the window's
// title and details, its actions, and minimize, maximize and close
// unless the compositor draws those itself. Dragging it moves the
// window. The window must be created with app.Decorated(false).
type headerBar struct {
	deco    widget.Decorations
	actions []widget.Clickable
	title   widget.Clickable
}

// update performs what was clicked since the last frame: window
// actions through w, header actions and the title through their
// callbacks. Call it before layout, with the spec layout will draw.
func (h *headerBar) update(gtx C, w *app.Window, cfg app.Config, spec headerSpec) {
	h.deco.Maximized = cfg.Mode == app.Maximized
	if a := h.deco.Update(gtx); a != 0 {
		w.Perform(a)
	}
	if h.title.Clicked(gtx) && spec.OnTitle != nil {
		spec.OnTitle()
	}
	actions := spec.Actions
	for i := range actions {
		if i < len(h.actions) && h.actions[i].Clicked(gtx) && actions[i].onClick != nil {
			actions[i].onClick()
		}
	}
}

func (h *headerBar) layout(gtx C, th *material.Theme, spec headerSpec, cfg app.Config) D {
	for len(h.actions) < len(spec.Actions) {
		h.actions = append(h.actions, widget.Clickable{})
	}
	fg, sub := colFg, spec.SubColor
	if sub == (color.NRGBA{}) {
		sub = colMuted
	}
	if !cfg.Focused {
		fg = colMuted // as GTK dims an inactive window's header
	}
	height := gtx.Dp(headerHeight)
	gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, height)
	gtx.Constraints.Max.Y = height
	size := gtx.Constraints.Min
	paint.FillShape(gtx.Ops, colHeader, clip.Rect{Max: size}.Op())
	if !spec.Seamless {
		paint.FillShape(gtx.Ops, colBorder, clip.Rect{Min: image.Pt(0, size.Y-gtx.Dp(1)), Max: size}.Op())
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx C) D {
			if spec.Leading == nil {
				return D{Size: image.Pt(gtx.Dp(14), 0)}
			}
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(10)}.Layout(gtx, spec.Leading)
		}),
		// The title area moves the window when dragged.
		layout.Flexed(1, func(gtx C) D {
			gtx.Constraints.Min = gtx.Constraints.Max
			if spec.OnTitle == nil {
				return h.deco.LayoutMove(gtx, func(gtx C) D {
					return layout.W.Layout(gtx, func(gtx C) D {
						return h.titles(gtx, th, spec, fg, sub)
					})
				})
			}
			full := gtx.Constraints.Max
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					gtx.Constraints.Min = image.Point{}
					return h.titleButton(gtx, th, spec, fg, sub)
				}),
				layout.Flexed(1, func(gtx C) D {
					gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, full.Y)
					return h.deco.LayoutMove(gtx, func(gtx C) D { return D{Size: gtx.Constraints.Min} })
				}),
			)
		}),
	}
	for i, a := range spec.Actions {
		children = append(children, layout.Rigid(func(gtx C) D {
			bg := color.NRGBA{}
			if a.active {
				bg = colControlHover
			}
			dims := iconButton(gtx, &h.actions[i], a.icon, actionSize, unit.Dp(20), fg, bg, colHover)
			if a.badge != "" {
				badge(gtx, th, dims.Size, a.badge, a.badgeCol)
			}
			return dims
		}))
	}
	if !cfg.Decorated {
		children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
		max := iconMaximize
		if h.deco.Maximized {
			max = iconRestore
		}
		for _, c := range []struct {
			action system.Action
			icon   *widget.Icon
			hover  color.NRGBA
		}{
			{system.ActionMinimize, iconMinimize, colControlHover},
			{system.ActionMaximize, max, colControlHover},
			{system.ActionClose, iconClose, colError},
		} {
			children = append(children, layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
					cl := h.deco.Clickable(c.action)
					return iconButton(gtx, cl, c.icon, controlSize, unit.Dp(16), fg, colControl, c.hover, c.action)
				})
			}))
		}
	}
	children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout))
	layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
	return D{Size: size}
}

// titleButton is the title and subtitle as a button that opens the
// window's details, with a chevron after the title that turns as they
// open, and a soft highlight under the pointer.
func (h *headerBar) titleButton(gtx C, th *material.Theme, spec headerSpec, fg, sub color.NRGBA) D {
	return h.title.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		m := op.Record(gtx.Ops)
		dims := layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(6), Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
			return h.titles(gtx, th, spec, fg, sub)
		})
		call := m.Stop()
		if h.title.Hovered() || spec.Expanded > 0.5 {
			paint.FillShape(gtx.Ops, colHover, clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(6)).Op(gtx.Ops))
		}
		call.Add(gtx.Ops)
		return dims
	})
}

// chevron is the title button's turn-down arrow, rotated half a turn
// as turn goes from 0 to 1.
func chevron(gtx C, col color.NRGBA, turn float32) D {
	px := gtx.Dp(16)
	c := f32.Pt(float32(px)/2, float32(px)/2)
	defer op.Affine(f32.AffineId().Rotate(c, turn*math.Pi)).Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Exact(image.Pt(px, px))
	return iconExpand.Layout(gtx, col)
}

// titles is the title (with its status) over the subtitle.
func (h *headerBar) titles(gtx C, th *material.Theme, spec headerSpec, fg, sub color.NRGBA) D {
	gtx.Constraints.Min = image.Point{} // its own height, centred by the caller
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			title := spec.Title
			if spec.Status != "" {
				title += "  " + spec.Status
			}
			lbl := bodyLabel(th, title)
			lbl.Font.Weight = font.SemiBold
			lbl.Color = fg
			lbl.MaxLines = 1
			if spec.OnTitle == nil {
				return lbl.Layout(gtx)
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(lbl.Layout),
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						return chevron(gtx, sub, spec.Expanded)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx C) D {
			if spec.Subtitle != "" {
				lbl := secondaryLabel(th, spec.Subtitle)
				lbl.Color = sub
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}
			return chips(gtx, th, spec.Chips)
		}),
	)
}

// chips lays out header chips in a row, each at its natural width.
// When they do not fit, the grow chip (a topic, a status) gives up its
// width and is cut short with an ellipsis; the others stay whole.
func chips(gtx C, th *material.Theme, cs []headerChip) D {
	if len(cs) == 0 {
		return D{}
	}
	gap := gtx.Dp(12)
	chip := func(gtx C, c headerChip) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				if c.icon == nil {
					return D{}
				}
				return layout.Inset{Right: unit.Dp(3)}.Layout(gtx, func(gtx C) D {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(13), gtx.Dp(13)))
					return c.icon.Layout(gtx, c.col)
				})
			}),
			layout.Rigid(func(gtx C) D {
				lbl := secondaryLabel(th, c.text)
				lbl.Color = c.col
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
		)
	}
	// Measure the fixed chips first; the grow chip gets what is left.
	free := gtx.Constraints.Max.X
	calls := make([]op.CallOp, len(cs))
	sizes := make([]image.Point, len(cs))
	measure := func(i int, maxX int) {
		g := gtx
		g.Constraints = layout.Constraints{Max: image.Pt(max(0, maxX), gtx.Constraints.Max.Y)}
		m := op.Record(gtx.Ops)
		d := chip(g, cs[i])
		calls[i], sizes[i] = m.Stop(), d.Size
	}
	for i, c := range cs {
		if !c.grow {
			measure(i, free)
			free -= sizes[i].X
		}
		if i > 0 {
			free -= gap
		}
	}
	for i, c := range cs {
		if c.grow {
			measure(i, free)
		}
	}
	x, height := 0, 0
	for i := range cs {
		if i > 0 {
			x += gap
		}
		st := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		calls[i].Add(gtx.Ops)
		st.Pop()
		x += sizes[i].X
		height = max(height, sizes[i].Y)
	}
	return D{Size: image.Pt(x, height)}
}

// badge draws a count (or, for "•", a dot) on the top-right corner of
// a button of the given size.
func badge(gtx C, th *material.Theme, button image.Point, text string, col color.NRGBA) {
	if text == "•" {
		d := gtx.Dp(9)
		at := image.Pt(button.X-d-gtx.Dp(3), gtx.Dp(3))
		ring := gtx.Dp(2)
		paint.FillShape(gtx.Ops, colHeader, clip.Ellipse{Min: at.Sub(image.Pt(ring, ring)), Max: at.Add(image.Pt(d+ring, d+ring))}.Op(gtx.Ops))
		paint.FillShape(gtx.Ops, col, clip.Ellipse{Min: at, Max: at.Add(image.Pt(d, d))}.Op(gtx.Ops))
		return
	}
	m := op.Record(gtx.Ops)
	lbl := material.Label(th, badgeSize, text)
	lbl.Color = colOnAccent
	lbl.Font.Weight = font.SemiBold
	gtx.Constraints.Min = image.Point{}
	dims := lbl.Layout(gtx)
	call := m.Stop()
	h := gtx.Dp(14)
	w := max(h, dims.Size.X+gtx.Dp(8))
	at := image.Pt(button.X-w+gtx.Dp(2), 0)
	rr := image.Rectangle{Min: at, Max: at.Add(image.Pt(w, h))}
	ring := gtx.Dp(2)
	paint.FillShape(gtx.Ops, colHeader, clip.UniformRRect(rr.Inset(-ring), (h+2*ring)/2).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, col, clip.UniformRRect(rr, h/2).Op(gtx.Ops))
	defer op.Offset(at.Add(image.Pt((w-dims.Size.X)/2, (h-dims.Size.Y)/2))).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
}

// presence is what is known of whether a person is around.
type presence int

const (
	presenceUnknown presence = iota
	presenceActive
	presenceAway
)

// withPresence draws an avatar with a presence dot on its lower right
// corner: filled green when active, a grey ring when away, none when
// unknown. ring is the colour behind the avatar, cut around the dot.
func withPresence(gtx C, avatar layout.Widget, p presence, ring color.NRGBA) D {
	dims := avatar(gtx)
	if p == presenceUnknown {
		return dims
	}
	d := max(gtx.Dp(8), dims.Size.X*3/10)
	at := dims.Size.Sub(image.Pt(d-gtx.Dp(2), d-gtx.Dp(2)))
	r := gtx.Dp(2)
	outer := image.Rectangle{Min: at.Sub(image.Pt(r, r)), Max: at.Add(image.Pt(d+r, d+r))}
	paint.FillShape(gtx.Ops, ring, clip.Ellipse(outer).Op(gtx.Ops))
	dot := image.Rectangle{Min: at, Max: at.Add(image.Pt(d, d))}
	if p == presenceActive {
		paint.FillShape(gtx.Ops, colPresent, clip.Ellipse(dot).Op(gtx.Ops))
	} else {
		paint.FillShape(gtx.Ops, colMuted, clip.Ellipse(dot).Op(gtx.Ops))
		in := gtx.Dp(2)
		paint.FillShape(gtx.Ops, ring, clip.Ellipse(dot.Inset(in)).Op(gtx.Ops))
	}
	return dims
}

// iconButton is a round button with an icon: bg behind it (transparent
// when zero), hover when the pointer is over it. A system action, when
// given, also marks the button as that window control for the platform.
func iconButton(gtx C, cl *widget.Clickable, ic *widget.Icon, size, iconSize unit.Dp, fg, bg, hover color.NRGBA, action ...system.Action) D {
	return cl.Layout(gtx, func(gtx C) D {
		px := gtx.Dp(size)
		box := image.Pt(px, px)
		pointer.CursorPointer.Add(gtx.Ops)
		for _, a := range action {
			system.ActionInputOp(a).Add(gtx.Ops)
		}
		fill := bg
		if cl.Hovered() {
			fill = hover
		}
		if fill != (color.NRGBA{}) {
			paint.FillShape(gtx.Ops, fill, clip.Ellipse{Max: box}.Op(gtx.Ops))
		}
		gtx.Constraints = layout.Exact(box)
		return layout.Center.Layout(gtx, func(gtx C) D {
			gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(iconSize), gtx.Dp(iconSize)))
			return ic.Layout(gtx, fg)
		})
	})
}

// tile is a rounded square with an icon or a letter: a channel's or
// workspace's picture in the header.
func tile(gtx C, th *material.Theme, size unit.Dp, bg color.NRGBA, ic *widget.Icon, letter string) D {
	px := gtx.Dp(size)
	box := image.Pt(px, px)
	paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: box}, gtx.Dp(unit.Dp(8))).Op(gtx.Ops))
	gtx.Constraints = layout.Exact(box)
	return layout.Center.Layout(gtx, func(gtx C) D {
		if ic != nil {
			gtx.Constraints = layout.Exact(image.Pt(px*5/8, px*5/8))
			return ic.Layout(gtx, colFg)
		}
		lbl := material.Label(th, unit.Sp(float32(size)*0.5), letter)
		lbl.Font.Weight = font.SemiBold
		lbl.Color = colFg
		return lbl.Layout(gtx)
	})
}
