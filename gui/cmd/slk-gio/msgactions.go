package main

import (
	"fmt"
	"image"
	"io"
	"os"
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
	// forwarded is a forward done (err nil) or refused, to name.
	forwarded struct {
		to  string
		err error
	}
)

// msgMenu is the actions menu of one message, opened from its "⋯"
// hover action or a right-click: copy its text or link, open it in
// Slack, and for the user's own, edit or delete it.
type msgMenu struct {
	open   bool
	focus  bool // take the keyboard next frame, for Escape
	target backend.Message
	at     image.Point // where it was opened, in window pixels
	items  [9]widget.Clickable
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

// confirmDialog asks before doing something: a title, an optional
// muted body, an optional one-line editor (a note to send along),
// Cancel and the OK button, filled red when danger. onOK gets the
// editor's text ("" without one); onCancel, when set, runs on Cancel,
// Escape or a click outside. Deleting a message is one
// (deleteDialog), forwarding one another.
type confirmDialog struct {
	open        bool
	focus       bool
	title, body string
	okLabel     string
	cancelLabel string // "Cancel" when empty
	danger      bool
	ed          *widget.Editor
	hint        string // the editor's placeholder
	onOK        func(text string)
	onCancel    func()

	cancel, confirm widget.Clickable
	// scrim and panel are bytes so their tags differ; see msgMenu.
	scrim, panel byte
}

// deleteDialog asks before deleting m, quoting its start.
func (v *msgView) deleteDialog(m backend.Message) confirmDialog {
	return confirmDialog{
		open: true, focus: true,
		title:   "Delete this message? This can't be undone.",
		body:    messagePreview(m),
		okLabel: "Delete", danger: true,
		onOK: func(string) {
			go func() {
				if err := v.h.be.Delete(v.h.ctx, m.ChannelID, m.TS); err != nil {
					v.box.post(deleteFailed{err})
				}
			}()
		},
	}
}

// forwardDialog asks for a note to send with m to conversation to, and
// forwards it on OK.
func (v *msgView) forwardDialog(m backend.Message, to backend.Conversation) confirmDialog {
	name := to.Name
	if to.Kind == backend.KindChannel || to.Kind == backend.KindPrivate {
		name = "#" + name
	}
	ed := new(widget.Editor)
	ed.SingleLine, ed.Submit = true, true
	body := messagePreview(m)
	if m.Author != "" && body != "" {
		body = m.Author + ": " + body
	}
	return confirmDialog{
		open: true, focus: true,
		title: "Forward to " + name, body: body,
		ed: ed, hint: "Add a message (optional)",
		okLabel: "Forward",
		onOK: func(comment string) {
			go func() {
				err := v.h.be.Forward(v.h.ctx, m, to.ID, comment)
				if err != nil {
					fmt.Fprintln(os.Stderr, "slk-gio: forward:", err)
				}
				v.box.post(forwarded{to: name, err: err})
			}()
		},
	}
}

// messagePreview is m's text on one line, cut at 120 characters.
func messagePreview(m backend.Message) string {
	preview := strings.Join(strings.Fields(rich.PlainBlocks(m.Body)), " ")
	if r := []rune(preview); len(r) > 120 {
		preview = string(r[:120]) + "…"
	}
	return preview
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
	if v.onForward != nil && !m.Ephemeral {
		items = append(items, menuItem{label: "Forward…", act: func() { v.onForward(m) }})
	}
	if v.markUnread != nil && !m.Ephemeral {
		if act := v.markUnread(m); act != nil {
			items = append(items, menuItem{label: "Mark unread from here", act: act})
		}
	}
	if m.FromSelf && v.onEdit != nil {
		items = append(items,
			menuItem{label: "Edit message", act: func() { v.onEdit(m) }},
			menuItem{label: "Delete message…", danger: true, act: func() {
				v.confirm = v.deleteDialog(m)
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
		return v.confirm.update(gtx)
	}
	return false
}

// update handles the dialog's buttons and keys: Enter (in the editor
// too) is OK, Escape or a click outside is Cancel. It reports whether
// the dialog closed.
func (d *confirmDialog) update(gtx C) (closed bool) {
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
	if d.ed != nil {
		for {
			ev, ok := gtx.Event(key.Filter{Focus: d.ed, Name: key.NameEscape})
			if !ok {
				break
			}
			if k, ok := ev.(key.Event); ok && k.State == key.Press {
				no = true
			}
		}
		for {
			ev, ok := d.ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				yes = true
			}
		}
	}
	switch {
	case yes:
		d.open = false
		text := ""
		if d.ed != nil {
			text = strings.TrimSpace(d.ed.Text())
		}
		if d.onOK != nil {
			d.onOK(text)
		}
		return true
	case no:
		d.open = false
		if d.onCancel != nil {
			d.onCancel()
		}
		return true
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

// confirmLayout draws the confirmation dialog in the middle of the
// window.
func (v *msgView) confirmLayout(gtx C) { v.confirm.layout(gtx, v.th) }

// layout draws the dialog in the middle of the window: its title, body
// and editor, Cancel and OK.
func (d *confirmDialog) layout(gtx C, th *material.Theme) {
	popupScrim(gtx, &d.scrim)
	width := min(gtx.Dp(380), gtx.Constraints.Max.X-gtx.Dp(24))
	cgtx := gtx
	cgtx.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, gtx.Constraints.Max.Y)}
	cancel := d.cancelLabel
	if cancel == "" {
		cancel = "Cancel"
	}
	rec := op.Record(gtx.Ops)
	dims := popupFrame(cgtx, func(gtx C) D {
		return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(bodyLabel(th, d.title).Layout),
				layout.Rigid(func(gtx C) D {
					if d.body == "" {
						return D{}
					}
					return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
						lbl := secondaryLabel(th, d.body)
						lbl.Color = colMuted
						lbl.MaxLines = 3
						return lbl.Layout(gtx)
					})
				}),
				layout.Rigid(func(gtx C) D {
					if d.ed == nil {
						return D{}
					}
					return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, framedEditor(th, d.ed, d.hint))
				}),
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
						return layout.Flex{}.Layout(gtx,
							layout.Flexed(1, func(gtx C) D { return D{Size: image.Pt(gtx.Constraints.Min.X, 0)} }),
							layout.Rigid(func(gtx C) D { return dialogButton(gtx, th, &d.cancel, cancel, false) }),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx C) D { return dialogButton(gtx, th, &d.confirm, d.okLabel, d.danger) }),
						)
					})
				}),
			)
		})
	})
	call := rec.Stop()
	off := image.Pt((gtx.Constraints.Max.X-dims.Size.X)/2, max(gtx.Dp(headerHeight)+gtx.Dp(40), (gtx.Constraints.Max.Y-dims.Size.Y)/3))
	if d.ed == nil {
		popupPanel(gtx, off, dims, call, &d.panel, &d.focus)
		return
	}
	// The editor takes the keyboard; Escape and Enter are read from it.
	focus := false
	popupPanel(gtx, off, dims, call, &d.panel, &focus)
	if d.focus {
		gtx.Execute(key.FocusCmd{Tag: d.ed})
		d.focus = false
	}
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
