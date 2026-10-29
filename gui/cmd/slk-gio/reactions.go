package main

import (
	"image"
	"strconv"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/gui/rich"
)

// reactFailed is Slack refusing a reaction.
type reactFailed struct{ err error }

// loadCustomEmoji asks the hub for the workspace's custom emoji
// (fetched once for every window) and hands them to the window.
func (v *msgView) loadCustomEmoji() {
	if customs, err := v.h.customEmoji(); err == nil {
		v.box.post(customEmojiLoaded{customs})
	}
}

// react adds the user's reaction name to m, or removes it.
func (v *msgView) react(m backend.Message, name string, add bool) {
	go func() {
		if err := v.h.be.React(v.h.ctx, m.ChannelID, m.TS, name, add); err != nil {
			v.box.post(reactFailed{err})
		}
	}()
}

// reactions draws m's reaction pills and a "+" that opens the picker;
// clicking a pill toggles the user's reaction.
func (v *msgView) reactions(gtx C, w *msgWidgets, m backend.Message) D {
	for len(w.pills) < len(m.Reactions) {
		w.pills = append(w.pills, new(widget.Clickable))
	}
	for i, r := range m.Reactions {
		if w.pills[i].Clicked(gtx) {
			v.react(m, r.Name, !r.Mine)
		}
	}
	if w.addPill.Clicked(gtx) {
		v.picker.show(m)
	}
	items := make([]layout.Widget, 0, len(m.Reactions)+1)
	for i, r := range m.Reactions {
		items = append(items, func(gtx C) D { return v.pill(gtx, w.pills[i], r) })
	}
	items = append(items, func(gtx C) D {
		return pillFrame(gtx, &w.addPill, false, func(gtx C) D {
			lbl := material.Body2(v.th, "+")
			lbl.Color = colMuted
			return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, lbl.Layout)
		})
	})
	return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D { return wrapRow(gtx, unit.Dp(4), items) })
}

// pill is one reaction: its emoji and how many reacted, lit when the
// user is among them.
func (v *msgView) pill(gtx C, cl *widget.Clickable, r backend.Reaction) D {
	return pillFrame(gtx, cl, r.Mine, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D { return v.emoji(gtx, r.Name, unit.Dp(16)) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx C) D {
				lbl := material.Caption(v.th, strconv.Itoa(r.Count))
				lbl.Color = colMuted
				if r.Mine {
					lbl.Color = colAccent
				}
				return lbl.Layout(gtx)
			}),
		)
	})
}

// emoji draws an emoji by name at about size: its character, its
// picture for a custom one, or ":name:" while that is loading.
func (v *msgView) emoji(gtx C, name string, size unit.Dp) D {
	if g, ok := rich.EmojiGlyph(name); ok {
		lbl := material.Label(v.th, unit.Sp(float32(size)*0.85), g)
		return lbl.Layout(gtx)
	}
	if img, ok := v.view.customEmoji(name); ok {
		gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(size), gtx.Dp(size)))
		return emojiPicture(gtx, img)
	}
	lbl := material.Caption(v.th, ":"+name+":")
	lbl.Color = colMuted
	return lbl.Layout(gtx)
}

// pillFrame is the rounded, outlined button a reaction pill is drawn
// in; lit pills are tinted with the accent.
func pillFrame(gtx C, cl *widget.Clickable, lit bool, content layout.Widget) D {
	return cl.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		bg, border := colBg, colBorder
		switch {
		case lit:
			bg, border = tint(colAccent), colAccent
		case cl.Hovered():
			bg = colHover
		}
		return widget.Border{Color: border, CornerRadius: unit.Dp(10), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
			return rounded(gtx, bg, unit.Dp(10), func(gtx C) D {
				return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(7), Right: unit.Dp(8)}.Layout(gtx, content)
			})
		})
	})
}

// wrapRow lays items out left to right, gap apart, starting a new line
// when the next one does not fit.
func wrapRow(gtx C, gap unit.Dp, items []layout.Widget) D {
	g := gtx.Dp(gap)
	maxW := gtx.Constraints.Max.X
	cgtx := gtx
	cgtx.Constraints.Min = image.Point{}
	var x, y, lineH, width int
	for _, it := range items {
		rec := op.Record(gtx.Ops)
		d := it(cgtx)
		call := rec.Stop()
		if x > 0 && x+d.Size.X > maxW {
			x, y, lineH = 0, y+lineH+g, 0
		}
		st := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		st.Pop()
		width = max(width, x+d.Size.X)
		x += d.Size.X + g
		lineH = max(lineH, d.Size.Y)
	}
	return D{Size: image.Pt(width, y+lineH)}
}

// Reaction picker geometry: how many choices it shows, in rows of
// pickerCols.
const (
	pickerMax  = 40
	pickerCols = 8
)

// reactPicker is a panel for choosing any reaction for one message: a
// filter over the emoji names and a grid of matches, the common ones
// before anything is typed. Arrows move, Enter or a click picks,
// Escape or a click outside closes.
type reactPicker struct {
	open    bool
	focus   bool // move the keyboard to the filter next frame
	target  backend.Message
	ed      widget.Editor
	rl      readline
	sel     int
	choices []chat.EmojiChoice
	cells   [pickerMax]widget.Clickable
	// scrim takes clicks outside the panel; panel takes the ones on it
	// that hit no choice, so they do not reach the messages beneath.
	scrim, panel struct{}
}

func (p *reactPicker) show(m backend.Message) {
	p.open, p.focus, p.target, p.sel = true, true, m, 0
	p.ed.SingleLine, p.ed.Submit = true, true
	p.ed.SetText("")
}

// pickerUpdate handles the picker's keys and clicks before it is drawn.
// It reports whether the picker closed, so the window can give the
// keyboard back to its composer.
func (v *msgView) pickerUpdate(gtx C) (closed bool) {
	p := &v.picker
	if !p.open {
		return false
	}
	pick := -1
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &p.ed, Name: key.NameUpArrow},
			key.Filter{Focus: &p.ed, Name: key.NameDownArrow},
			key.Filter{Focus: &p.ed, Name: key.NameLeftArrow},
			key.Filter{Focus: &p.ed, Name: key.NameRightArrow},
			key.Filter{Focus: &p.ed, Name: key.NameEscape},
		)
		if !ok {
			break
		}
		k, ok := ev.(key.Event)
		if !ok || k.State != key.Press {
			continue
		}
		switch k.Name {
		case key.NameUpArrow:
			p.sel -= pickerCols
		case key.NameDownArrow:
			p.sel += pickerCols
		case key.NameLeftArrow:
			p.sel--
		case key.NameRightArrow:
			p.sel++
		case key.NameEscape:
			p.open = false
			return true
		}
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &p.scrim, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok {
			p.open = false
			return true
		}
	}
	p.rl.keys(gtx, &p.ed)
	for {
		ev, ok := p.ed.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			p.sel = 0
		case widget.SubmitEvent:
			pick = p.sel
		}
	}
	p.choices = chat.MatchEmoji(p.ed.Text(), v.view.customs, pickerMax)
	if n := len(p.choices); n > 0 {
		p.sel = min(max(p.sel, 0), n-1)
	} else {
		p.sel = 0
	}
	for i := range p.cells {
		if p.cells[i].Clicked(gtx) {
			pick = i
		}
	}
	if pick < 0 || pick >= len(p.choices) {
		return false
	}
	name := p.choices[pick].Name
	add := true
	for _, r := range p.target.Reactions {
		if r.Name == name && r.Mine {
			add = false // picking one the user has takes it back, as a pill click does
		}
	}
	v.react(p.target, name, add)
	p.open = false
	return true
}

// pickerLayout draws the picker over the window, under its header.
func (v *msgView) pickerLayout(gtx C) {
	p := &v.picker
	if !p.open {
		return
	}
	if p.focus {
		gtx.Execute(key.FocusCmd{Tag: &p.ed})
		p.focus = false
	}
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &p.scrim)
	area.Pop()

	width := min(gtx.Dp(360), gtx.Constraints.Max.X-gtx.Dp(24))
	x := (gtx.Constraints.Max.X - width) / 2
	defer op.Offset(image.Pt(x, gtx.Dp(headerHeight)+gtx.Dp(6))).Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, gtx.Constraints.Max.Y)}
	rec := op.Record(gtx.Ops)
	dims := widget.Border{Color: colBorder, CornerRadius: unit.Dp(10), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
		return rounded(gtx, colHeader, unit.Dp(10), func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(framedEditor(v.th, &p.ed, "Add a reaction")),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(v.pickerGrid),
				)
			})
		})
	})
	call := rec.Stop()
	// The panel's own area, under the choices, keeps clicks on it from
	// reaching the scrim (which would close it) or the messages.
	panel := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &p.panel)
	call.Add(gtx.Ops)
	panel.Pop()
}

// pickerGrid is the picker's choices, pickerCols to a row.
func (v *msgView) pickerGrid(gtx C) D {
	p := &v.picker
	if len(p.choices) == 0 {
		lbl := material.Body2(v.th, "No emoji match")
		lbl.Color = colMuted
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, lbl.Layout)
	}
	cell := gtx.Constraints.Max.X / pickerCols
	var rows []layout.FlexChild
	for start := 0; start < len(p.choices); start += pickerCols {
		end := min(start+pickerCols, len(p.choices))
		rows = append(rows, layout.Rigid(func(gtx C) D {
			var cells []layout.FlexChild
			for i := start; i < end; i++ {
				cells = append(cells, layout.Rigid(func(gtx C) D {
					gtx.Constraints = layout.Exact(image.Pt(cell, cell))
					return p.cells[i].Layout(gtx, func(gtx C) D {
						pointer.CursorPointer.Add(gtx.Ops)
						bg := colHeader
						if i == p.sel || p.cells[i].Hovered() {
							bg = colHover
						}
						return rounded(gtx, bg, unit.Dp(6), func(gtx C) D {
							return layout.Center.Layout(gtx, func(gtx C) D {
								return v.emoji(gtx, p.choices[i].Name, unit.Dp(22))
							})
						})
					})
				}))
			}
			return layout.Flex{}.Layout(gtx, cells...)
		}))
	}
	// The highlighted choice's name, under the grid.
	name := ":" + p.choices[p.sel].Name + ":"
	rows = append(rows, layout.Rigid(func(gtx C) D {
		return layout.Inset{Top: unit.Dp(4), Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
			lbl := material.Caption(v.th, name)
			lbl.Color = colMuted
			return lbl.Layout(gtx)
		})
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}
