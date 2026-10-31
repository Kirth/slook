package main

import (
	"image"
	"strings"
	"time"

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
)

// newMessagePicker chooses who a new message goes to: a filter over the
// people slk knows (no apps, not the user), the ones chosen as pills
// above it, at most chat.MaxRecipients. Up/Down move, Space or Tab adds
// or takes out the highlighted person, Backspace in an empty filter
// takes out the last, Enter opens the DM (one person) or group DM (with
// nobody chosen, the highlighted person's DM); Escape or a click
// outside closes it. The chat window opens it from the switcher's "New
// message…", the list from its header.
type newMessagePicker struct {
	open    bool
	focus   bool // move the keyboard to the filter next frame
	ed      widget.Editor
	rl      readline
	sel     int
	to      chat.Recipients
	choices []chat.Choice
	rows    [popupRows]widget.Clickable
	pills   [chat.MaxRecipients]widget.Clickable
	// scrim takes clicks outside the panel, panel the ones on it; bytes
	// so their tags differ (TestPopupTagsAreDistinct).
	scrim, panel byte
	openedAt     time.Time
}

func (p *newMessagePicker) show() {
	p.open, p.focus, p.sel, p.openedAt = true, true, 0, time.Now()
	p.to = chat.Recipients{}
	p.ed.SingleLine, p.ed.Submit = true, true
	p.ed.SetText("")
}

// update handles the picker's keys and clicks before it is drawn. ids
// is who to open a conversation with when the user chose; closed is
// whether the picker closed (the window then takes the keyboard back).
func (p *newMessagePicker) update(gtx C, people []backend.Person, self string) (ids []string, closed bool) {
	if !p.open {
		return nil, false
	}
	if dismissed(gtx, &p.scrim, &p.ed) {
		p.open = false
		return nil, true
	}
	filters := []event.Filter{
		key.Filter{Focus: &p.ed, Name: key.NameUpArrow},
		key.Filter{Focus: &p.ed, Name: key.NameDownArrow},
		key.Filter{Focus: &p.ed, Name: key.NameTab},
	}
	if p.ed.Len() == 0 {
		filters = append(filters, key.Filter{Focus: &p.ed, Name: key.NameDeleteBackward})
	}
	toggle := false
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		k, ok := ev.(key.Event)
		if !ok || k.State != key.Press {
			continue
		}
		n := max(len(p.choices), 1)
		switch k.Name {
		case key.NameUpArrow:
			p.sel = (p.sel + n - 1) % n
		case key.NameDownArrow:
			p.sel = (p.sel + 1) % n
		case key.NameTab:
			toggle = true
		case key.NameDeleteBackward:
			p.to.Pop()
		}
	}
	p.rl.keys(gtx, &p.ed)
	submit := false
	for {
		ev, ok := p.ed.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			p.sel = 0
			// Space adds: names are found by their start, so the filter
			// never needs one.
			if t := p.ed.Text(); strings.Contains(t, " ") {
				p.ed.SetText(strings.ReplaceAll(t, " ", ""))
				p.ed.SetCaret(p.ed.Len(), p.ed.Len())
				toggle = true
			}
		case widget.SubmitEvent:
			submit = true
		}
	}
	others := make([]backend.Person, 0, len(people))
	for _, q := range people {
		if q.ID != self {
			others = append(others, q)
		}
	}
	p.choices = chat.PeopleChoices(p.ed.Text(), others, popupRows)
	if p.sel >= len(p.choices) {
		p.sel = 0
	}
	for i := range p.rows {
		if p.rows[i].Clicked(gtx) && i < len(p.choices) {
			p.sel, toggle = i, true
		}
	}
	for i := range p.pills {
		if p.pills[i].Clicked(gtx) && i < len(p.to.People) {
			p.to.Toggle(p.to.People[i])
		}
	}
	if toggle && p.sel < len(p.choices) {
		c := p.choices[p.sel]
		if p.to.Toggle(backend.Person{ID: c.ID, Name: c.Label, AvatarURL: c.Avatar}) {
			p.ed.SetText("")
		}
	}
	if !submit {
		return nil, false
	}
	ids = p.to.IDs()
	if len(ids) == 0 && p.sel < len(p.choices) {
		ids = []string{p.choices[p.sel].ID}
	}
	if len(ids) == 0 {
		return nil, false
	}
	p.open = false
	return ids, true
}

// layout draws the picker over the window, under its header.
func (p *newMessagePicker) layout(gtx C, th *material.Theme, rv *richView) {
	if !p.open {
		return
	}
	if p.focus {
		gtx.Execute(key.FocusCmd{Tag: &p.ed})
		p.focus = false
	}
	popupScrim(gtx, &p.scrim)
	width := min(gtx.Dp(440), gtx.Constraints.Max.X-gtx.Dp(24))
	x := (gtx.Constraints.Max.X - width) / 2
	defer op.Offset(image.Pt(x, gtx.Dp(headerHeight)+gtx.Dp(6))).Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, gtx.Constraints.Max.Y)}
	rec := op.Record(gtx.Ops)
	dims := enter(gtx, p.openedAt, func(gtx C) D { return p.panelLayout(gtx, th, rv) })
	call := rec.Stop()
	// The panel's own area keeps clicks on it from reaching the scrim
	// (which would close it) or the window beneath.
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &p.panel)
	call.Add(gtx.Ops)
	area.Pop()
}

// panelLayout is the picker's box: the filter, the people chosen, the
// matches and the count.
func (p *newMessagePicker) panelLayout(gtx C, th *material.Theme, rv *richView) D {
	return overlayFrame(gtx, func(gtx C) D {
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					lbl := secondaryLabel(th, "New message")
					lbl.Color = colMuted
					return layout.Inset{Left: unit.Dp(2), Bottom: unit.Dp(6)}.Layout(gtx, lbl.Layout)
				}),
				layout.Rigid(framedEditor(th, &p.ed, "To: a name")),
				layout.Rigid(func(gtx C) D { return p.pillRow(gtx, th) }),
				layout.Rigid(func(gtx C) D {
					if len(p.choices) == 0 {
						lbl := secondaryLabel(th, "No one matches")
						lbl.Color = colMuted
						return layout.UniformInset(unit.Dp(8)).Layout(gtx, lbl.Layout)
					}
					return popupList(gtx, &p.rows, len(p.choices), p.sel, func(gtx C, i int) D {
						c := p.choices[i]
						if p.to.Has(c.ID) {
							c.Detail = "✓ " + c.Detail
						}
						return choiceRow(gtx, th, rv, c)
					})
				}),
				layout.Rigid(func(gtx C) D {
					text := "Space adds · Enter opens · " + p.to.Counter()
					if p.to.Full() {
						text = "Group limit reached · " + p.to.Counter()
					}
					lbl := captionLabel(th, text)
					lbl.Color = colMuted
					return layout.Inset{Top: unit.Dp(6), Left: unit.Dp(2)}.Layout(gtx, lbl.Layout)
				}),
			)
		})
	})
}

// pillRow is the people chosen, each a pill; clicking one takes them
// out.
func (p *newMessagePicker) pillRow(gtx C, th *material.Theme) D {
	if len(p.to.People) == 0 {
		return D{}
	}
	items := make([]layout.Widget, len(p.to.People))
	for i, person := range p.to.People {
		cl := &p.pills[i]
		items[i] = func(gtx C) D {
			return cl.Layout(gtx, func(gtx C) D {
				pointer.CursorPointer.Add(gtx.Ops)
				bg := colControl
				if cl.Hovered() {
					bg = colHover
				}
				return rounded(gtx, bg, unit.Dp(10), func(gtx C) D {
					return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
						return secondaryLabel(th, person.Name+"  ×").Layout(gtx)
					})
				})
			})
		}
	}
	return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
		return wrapRow(gtx, unit.Dp(4), items)
	})
}
