package main

import (
	"image"
	"io"
	"strings"

	"gioui.org/io/clipboard"
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
	"github.com/gammons/slk/gui/rich"
)

type (
	// permalinkReady is a message's link, fetched for Copy link (open
	// false) or Open in Slack.
	permalinkReady struct {
		url  string
		open bool
		err  error
	}
	// deleteFailed is Slack refusing a deletion.
	deleteFailed struct{ err error }
)

// msgMenu is the actions menu of one message, opened from its "⋯"
// hover action or a right-click: copy its text or link, open it in
// Slack, and for the user's own, edit or delete it.
type msgMenu struct {
	open   bool
	focus  bool // take the keyboard next frame, for Escape
	target backend.Message
	at     image.Point // where it was opened, in window pixels
	items  [8]widget.Clickable
	// scrim takes clicks outside the menu (closing it), panel the ones
	// on it between items.
	// They are bytes, not struct{}: two zero-size fields side by side
	// share an address, so their tags would be equal and a click on the
	// panel would close it (TestPopupTagsAreDistinct).
	scrim, panel byte
}

// menuItem is one line of the menu.
type menuItem struct {
	label  string
	danger bool
	act    func()
}

func (mm *msgMenu) show(m backend.Message, at image.Point) {
	mm.open, mm.focus, mm.target, mm.at = true, true, m, at
}

// confirmDelete asks before deleting a message.
type confirmDelete struct {
	open            bool
	focus           bool
	target          backend.Message
	cancel, confirm widget.Clickable
	// scrim and panel are bytes so their tags differ; see msgMenu.
	scrim, panel byte
}

// menuItems is what the menu offers for m.
func (v *msgView) menuItems(m backend.Message) []menuItem {
	var items []menuItem
	if sel := v.selectionOf(m.TS); strings.TrimSpace(sel) != "" {
		items = append(items, menuItem{label: "Copy selection", act: func() {
			v.clip = sel
			v.notify("Copied", false)
		}})
	}
	items = append(items, []menuItem{
		{label: "Copy text", act: func() {
			v.clip = rich.PlainBlocks(m.Body)
			v.notify("Copied", false)
		}},
		{label: "Copy link", act: func() { v.permalink(m, false) }},
		{label: "Open in Slack", act: func() { v.permalink(m, true) }},
	}...)
	if v.markUnread != nil && !m.Ephemeral {
		if act := v.markUnread(m); act != nil {
			items = append(items, menuItem{label: "Mark unread from here", act: act})
		}
	}
	if m.FromSelf && v.onEdit != nil {
		items = append(items,
			menuItem{label: "Edit message", act: func() { v.onEdit(m) }},
			menuItem{label: "Delete message…", danger: true, act: func() {
				v.confirm = confirmDelete{open: true, focus: true, target: m}
			}})
	}
	return items
}

// permalink fetches m's link, to copy or to open.
func (v *msgView) permalink(m backend.Message, open bool) {
	go func() {
		url, err := v.h.be.Permalink(v.h.ctx, m.ChannelID, m.TS)
		v.box.post(permalinkReady{url: url, open: open, err: err})
	}()
}

// rowMenu opens the menu on a right-click anywhere on a message's row;
// the row's hover area is registered under w for it.
func (v *msgView) rowMenu(gtx C, w *msgWidgets, m backend.Message) {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: w, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Buttons.Contain(pointer.ButtonSecondary) {
			v.menu.show(m, v.pointer)
		}
	}
}

// writeClipboard puts pending text on the clipboard; the command needs
// a frame.
func (v *msgView) writeClipboard(gtx C) {
	if v.clip == "" {
		return
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(v.clip))})
	v.clip = ""
}

// menuUpdate handles the menu's and the delete confirmation's input
// before they are drawn. It reports whether one closed, so the window
// can give the keyboard back to its composer.
func (v *msgView) menuUpdate(gtx C) (closed bool) {
	v.writeClipboard(gtx)
	if v.menu.open {
		mm := &v.menu
		items := v.menuItems(mm.target)
		for i := range items {
			if mm.items[i].Clicked(gtx) {
				mm.open = false
				items[i].act()
				// Delete opens the confirmation, which takes the keyboard.
				return !v.confirm.open
			}
		}
		if dismissed(gtx, &mm.scrim, &mm.panel) {
			mm.open = false
			return true
		}
	}
	if v.confirm.open {
		d := &v.confirm
		yes := d.confirm.Clicked(gtx)
		no := d.cancel.Clicked(gtx) || dismissed(gtx, &d.scrim, &d.panel)
		for {
			ev, ok := gtx.Event(key.Filter{Focus: &d.panel, Name: key.NameReturn}, key.Filter{Focus: &d.panel, Name: key.NameEnter})
			if !ok {
				break
			}
			if k, ok := ev.(key.Event); ok && k.State == key.Press {
				yes = true
			}
		}
		switch {
		case yes:
			d.open = false
			m := d.target
			go func() {
				if err := v.h.be.Delete(v.h.ctx, m.ChannelID, m.TS); err != nil {
					v.box.post(deleteFailed{err})
				}
			}()
			return true
		case no:
			d.open = false
			return true
		}
	}
	return false
}

// dismissed reports a click on scrim (outside a popup) or Escape while
// the popup's panel has the keyboard.
func dismissed(gtx C, scrim, panel event.Tag) bool {
	out := false
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: scrim, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok {
			out = true
		}
	}
	for {
		ev, ok := gtx.Event(key.Filter{Focus: panel, Name: key.NameEscape})
		if !ok {
			break
		}
		if k, ok := ev.(key.Event); ok && k.State == key.Press {
			out = true
		}
	}
	return out
}

// popupScrim registers scrim over the whole window: a click there,
// outside the popup drawn after it, closes the popup.
func popupScrim(gtx C, scrim any) {
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, scrim)
	area.Pop()
}

// popupPanel draws a recorded popup at off inside an area of its own
// under tag panel, which takes the clicks between its controls and,
// when focus is set, the keyboard.
func popupPanel(gtx C, off image.Point, dims D, call op.CallOp, panel any, focus *bool) {
	defer op.Offset(off).Push(gtx.Ops).Pop()
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	event.Op(gtx.Ops, panel)
	call.Add(gtx.Ops)
	area.Pop()
	// Gio keeps the keyboard only on a tag that asks to be focusable
	// every frame.
	for {
		if _, ok := gtx.Event(key.FocusFilter{Target: panel}); !ok {
			break
		}
	}
	if *focus {
		// The popup reads its keys from the next frame on; ask for them
		// now too (a popup's first frame has none to lose).
		for {
			if _, ok := gtx.Event(
				key.Filter{Focus: panel, Name: key.NameEscape},
				key.Filter{Focus: panel, Name: key.NameReturn},
				key.Filter{Focus: panel, Name: key.NameEnter},
			); !ok {
				break
			}
		}
		gtx.Execute(key.FocusCmd{Tag: panel})
		*focus = false
	}
}

// menuLayout draws the menu at where it was opened, kept inside the
// window, and the delete confirmation centered.
func (v *msgView) menuLayout(gtx C) {
	if v.menu.open {
		mm := &v.menu
		popupScrim(gtx, &mm.scrim)
		items := v.menuItems(mm.target)
		cgtx := gtx
		cgtx.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(220), gtx.Constraints.Max.Y)}
		cgtx.Constraints.Min.X = cgtx.Constraints.Max.X
		rec := op.Record(gtx.Ops)
		dims := popupFrame(cgtx, func(gtx C) D {
			rows := make([]layout.FlexChild, 0, len(items))
			for i, it := range items {
				rows = append(rows, layout.Rigid(func(gtx C) D { return menuRowAt(gtx, v.th, &mm.items[i], it.label, it.danger, false) }))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		})
		call := rec.Stop()
		off := mm.at
		off.X = max(0, min(off.X, gtx.Constraints.Max.X-dims.Size.X))
		off.Y = max(0, min(off.Y, gtx.Constraints.Max.Y-dims.Size.Y))
		popupPanel(gtx, off, dims, call, &mm.panel, &mm.focus)
	}
	if v.confirm.open {
		v.confirmLayout(gtx)
	}
}

// popupFrame is the overlay panel a menu or dialog is drawn in.
func popupFrame(gtx C, content layout.Widget) D {
	return overlayFrame(gtx, func(gtx C) D {
		return layout.UniformInset(unit.Dp(4)).Layout(gtx, content)
	})
}

// menuRowAt is one line of a popup menu: label, red when danger, with
// a check mark after it when current (the state the user is in).
func menuRowAt(gtx C, th *material.Theme, cl *widget.Clickable, label string, danger, current bool) D {
	return cl.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		bg := colOverlay
		if cl.Hovered() {
			bg = colControl
		}
		return rounded(gtx, bg, unit.Dp(5), func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
				lbl := secondaryLabel(th, label)
				if danger {
					lbl.Color = colError
				}
				if !current {
					return lbl.Layout(gtx)
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, lbl.Layout),
					layout.Rigid(func(gtx C) D {
						mark := secondaryLabel(th, "✓")
						mark.Color = colAccent
						return mark.Layout(gtx)
					}),
				)
			})
		})
	})
}

// confirmLayout draws the delete confirmation in the middle of the
// window: what will be deleted, Cancel and Delete.
func (v *msgView) confirmLayout(gtx C) {
	d := &v.confirm
	popupScrim(gtx, &d.scrim)
	preview := strings.Join(strings.Fields(rich.PlainBlocks(d.target.Body)), " ")
	if r := []rune(preview); len(r) > 120 {
		preview = string(r[:120]) + "…"
	}
	width := min(gtx.Dp(380), gtx.Constraints.Max.X-gtx.Dp(24))
	cgtx := gtx
	cgtx.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, gtx.Constraints.Max.Y)}
	rec := op.Record(gtx.Ops)
	dims := popupFrame(cgtx, func(gtx C) D {
		return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					lbl := bodyLabel(v.th, "Delete this message? This can't be undone.")
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx C) D {
					if preview == "" {
						return D{}
					}
					return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
						lbl := secondaryLabel(v.th, preview)
						lbl.Color = colMuted
						lbl.MaxLines = 3
						return lbl.Layout(gtx)
					})
				}),
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
						return layout.Flex{}.Layout(gtx,
							layout.Flexed(1, func(gtx C) D { return D{Size: image.Pt(gtx.Constraints.Min.X, 0)} }),
							layout.Rigid(func(gtx C) D { return dialogButton(gtx, v.th, &d.cancel, "Cancel", false) }),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx C) D { return dialogButton(gtx, v.th, &d.confirm, "Delete", true) }),
						)
					})
				}),
			)
		})
	})
	call := rec.Stop()
	off := image.Pt((gtx.Constraints.Max.X-dims.Size.X)/2, max(gtx.Dp(headerHeight)+gtx.Dp(40), (gtx.Constraints.Max.Y-dims.Size.Y)/3))
	popupPanel(gtx, off, dims, call, &d.panel, &d.focus)
}

// dialogButton is a dialog's button; the dangerous one is filled red.
func dialogButton(gtx C, th *material.Theme, cl *widget.Clickable, label string, danger bool) D {
	return cl.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		bg, fg := colHover, th.Fg
		if danger {
			bg, fg = colError, colBg
		}
		return rounded(gtx, bg, unit.Dp(6), func(gtx C) D {
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx C) D {
				lbl := secondaryLabel(th, label)
				lbl.Color = fg
				return lbl.Layout(gtx)
			})
		})
	})
}
