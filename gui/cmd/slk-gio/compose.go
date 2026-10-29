package main

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/internal/slashcmd"
)

// composeBox is a window's composer: the editor, its slash-command
// popup and readline keys, and a note under the messages (a send that
// failed, a command refused). It posts to channelID, or in the thread
// under threadTS when that is set, where broadcast is Slack's "Also
// send to #channel".
type composeBox struct {
	h   *hub
	box *inbox // the window's
	th  *material.Theme

	channelID string
	threadTS  string

	composer  widget.Editor
	popup     commandPopup
	rl        readline
	sendErr   string
	broadcast widget.Bool

	// info is a passing note under the messages ("Copied"), shown until
	// infoUntil; sendErr, when set, is shown instead.
	info      string
	infoUntil time.Time

	// editing is the user's message being edited, nil when the composer
	// writes a new one; stash is the draft set aside meanwhile.
	editing    *backend.Message
	stash      string
	cancelEdit widget.Clickable
	focusNext  bool // give the composer the keyboard next frame
	// onEditLast is the message Up in an empty composer edits.
	onEditLast func() (backend.Message, bool)
}

// editFailed is Slack refusing an edit: the composer goes back to
// editing m with text, so Enter tries again rather than posting it.
type editFailed struct {
	m    backend.Message
	text string
	err  error
}

// infoFor is how long a passing note stays.
const infoFor = 3 * time.Second

// note shows text under the messages: an error until the next send, or
// a passing note.
func (b *composeBox) note(text string, isErr bool) {
	if isErr {
		b.sendErr, b.info = text, ""
		return
	}
	b.sendErr, b.info, b.infoUntil = "", text, time.Now().Add(infoFor)
}

// beginEdit puts one of the user's messages in the composer to edit,
// setting aside what was being written.
func (b *composeBox) beginEdit(m backend.Message) {
	if b.editing == nil {
		b.stash = b.composer.Text()
	}
	b.editing = &m
	b.setText(chat.EditableText(m.Text))
	b.popup.matches = nil
	b.sendErr = ""
	b.focusNext = true
}

// endEdit leaves editing and brings back the draft set aside.
func (b *composeBox) endEdit() {
	if b.editing == nil {
		return
	}
	b.editing = nil
	b.setText(b.stash)
	b.stash = ""
	b.focusNext = true
}

// editFailed goes back to editing after Slack refused the edit.
func (b *composeBox) failedEdit(v editFailed) {
	b.beginEdit(v.m)
	b.setText(v.text)
	b.note("Not saved: "+v.err.Error(), true)
}

// deleted ends an edit of the message at ts, which is gone.
func (b *composeBox) deleted(ts string) {
	if b.editing != nil && b.editing.TS == ts {
		b.endEdit()
		b.note("The message you were editing was deleted", true)
	}
}

// setText replaces the composer's text, the caret at its end.
func (b *composeBox) setText(t string) {
	b.composer.SetText(t)
	n := utf8.RuneCountInString(t)
	b.composer.SetCaret(n, n)
}

// submitEdit saves the edit: nothing to do when unchanged; an emptied
// message is refused (deleting is its own action).
func (b *composeBox) submitEdit(raw string) {
	m := *b.editing
	text := strings.TrimSpace(raw)
	switch {
	case text == "":
		b.note("Delete the message instead of emptying it", true)
		return
	case text == strings.TrimSpace(chat.EditableText(m.Text)):
		b.endEdit()
		return
	}
	b.endEdit()
	b.sendErr = ""
	go func() {
		if err := b.h.be.Edit(b.h.ctx, m, text); err != nil {
			b.box.post(editFailed{m: m, text: text, err: err})
		}
	}()
}

// editKeys handles Escape while editing and Up in an empty composer,
// before the editor sees them.
func (b *composeBox) editKeys(gtx C) {
	var filters []event.Filter
	if b.editing != nil {
		filters = append(filters, key.Filter{Focus: &b.composer, Name: key.NameEscape})
	} else if b.composer.Len() == 0 && !b.popup.open() && b.onEditLast != nil {
		filters = append(filters, key.Filter{Focus: &b.composer, Name: key.NameUpArrow})
	}
	if len(filters) == 0 {
		return
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		k, ok := ev.(key.Event)
		if !ok || k.State != key.Press {
			continue
		}
		switch k.Name {
		case key.NameEscape:
			b.endEdit()
		case key.NameUpArrow:
			if m, ok := b.onEditLast(); ok {
				b.beginEdit(m)
			}
		}
		return
	}
}

func newComposeBox(h *hub, th *material.Theme, box *inbox, channelID, threadTS string) composeBox {
	b := composeBox{h: h, box: box, th: th, channelID: channelID, threadTS: threadTS}
	b.composer.Submit = true // Enter sends; Shift+Enter is a new line
	return b
}

// send posts text, as a reply when the composer is a thread's.
func (b *composeBox) send(text string, broadcast bool) {
	var err error
	if b.threadTS != "" {
		err = b.h.be.SendReply(b.h.ctx, b.channelID, b.threadTS, text, broadcast)
	} else {
		err = b.h.be.Send(b.h.ctx, b.channelID, text)
	}
	if err != nil {
		b.box.post(sendFailed{text: text, err: err})
	}
}

// failed puts a message that was not sent back in the composer, with a
// note under it.
func (b *composeBox) failed(v sendFailed) {
	b.sendErr = v.note
	if b.sendErr == "" {
		b.sendErr = "Not sent: " + v.err.Error()
	}
	if b.composer.Len() == 0 {
		b.composer.SetText(v.text)
	}
}

// update handles the composer's keys and Enter, before it is drawn.
func (b *composeBox) update(gtx C) {
	if b.cancelEdit.Clicked(gtx) {
		b.endEdit()
	}
	b.editKeys(gtx)
	if b.focusNext {
		gtx.Execute(key.FocusCmd{Tag: &b.composer})
		b.focusNext = false
	}
	b.popupKeys(gtx)
	b.rl.keys(gtx, &b.composer)
	for {
		ev, ok := b.composer.Update(gtx)
		if !ok {
			break
		}
		if s, ok := ev.(widget.SubmitEvent); ok {
			b.submit(s.Text)
		}
	}
	b.popup.update(b.composer.Text())
	if b.editing != nil {
		b.popup.matches = nil // an edit is text, never a command
	}
	b.broadcast.Update(gtx)
}

// layout draws the note, the command popup, the editor with hint as
// its placeholder, and in a thread the broadcast box labelled castLabel.
func (b *composeBox) layout(gtx C, hint, castLabel string) D {
	if b.editing != nil {
		hint = "Editing message — Enter to save, Esc to cancel"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			text, col := b.sendErr, colError
			if text == "" && b.info != "" && gtx.Now.Before(b.infoUntil) {
				text, col = b.info, colMuted
				gtx.Execute(op.InvalidateCmd{At: b.infoUntil})
			}
			if text == "" {
				return D{}
			}
			return layout.Inset{Left: unit.Dp(14), Right: unit.Dp(14), Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
				lbl := material.Caption(b.th, text)
				lbl.Color = col
				return lbl.Layout(gtx)
			})
		}),
		layout.Rigid(b.commandList),
		layout.Rigid(func(gtx C) D {
			if b.editing == nil {
				return D{}
			}
			return layout.Inset{Left: unit.Dp(14), Right: unit.Dp(14), Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						lbl := material.Caption(b.th, "Editing message")
						lbl.Color = colAccent
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx C) D { return linkCaption(gtx, b.th, &b.cancelEdit, "Cancel") }),
				)
			})
		}),
		layout.Rigid(func(gtx C) D {
			in := layout.UniformInset(unit.Dp(12))
			if b.editing != nil {
				in.Top = unit.Dp(4)
			}
			if b.threadTS != "" && b.editing == nil {
				in.Bottom = unit.Dp(4)
			}
			return in.Layout(gtx, framedEditor(b.th, &b.composer, hint))
		}),
		layout.Rigid(func(gtx C) D {
			if b.threadTS == "" || b.editing != nil {
				return D{}
			}
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				cb := material.CheckBox(b.th, &b.broadcast, castLabel)
				cb.Color, cb.IconColor = colMuted, colAccent
				cb.TextSize = b.th.TextSize * 0.85
				return cb.Layout(gtx)
			})
		}),
	)
}

// loadCommands asks the hub for the workspace's commands.
func (b *composeBox) loadCommands() {
	if cmds, err := b.h.commands(); err == nil {
		b.box.post(commandsLoaded{cmds})
	}
}

// popupKeys handles the keys the popup takes from the composer while it
// is open: Up and Down move the highlight, Tab completes, Escape hides
// it. They are read before the editor sees them.
func (b *composeBox) popupKeys(gtx C) {
	if !b.popup.open() {
		return
	}
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &b.composer, Name: key.NameUpArrow},
			key.Filter{Focus: &b.composer, Name: key.NameDownArrow},
			key.Filter{Focus: &b.composer, Name: key.NameTab},
			key.Filter{Focus: &b.composer, Name: key.NameEscape},
		)
		if !ok {
			break
		}
		k, ok := ev.(key.Event)
		if !ok || k.State != key.Press {
			continue
		}
		n := len(b.popup.matches)
		switch k.Name {
		case key.NameUpArrow:
			b.popup.sel = (b.popup.sel + n - 1) % n
		case key.NameDownArrow:
			b.popup.sel = (b.popup.sel + 1) % n
		case key.NameTab:
			b.complete(b.popup.matches[b.popup.sel])
		case key.NameEscape:
			b.popup.dismissed = b.composer.Text()
			b.popup.matches = nil
		}
	}
}

// complete replaces the typed name with a command, ready for arguments.
func (b *composeBox) complete(cmd backend.Command) {
	t := cmd.Name + " "
	b.composer.SetText(t)
	n := utf8.RuneCountInString(t)
	b.composer.SetCaret(n, n)
	b.popup.matches = nil
}

// submit handles Enter in the composer: it completes a command name
// still being typed, runs a command, or sends a message.
func (b *composeBox) submit(raw string) {
	if b.editing != nil {
		b.submitEdit(raw)
		return
	}
	if b.popup.open() && !chat.IsCommand(b.popup.cmds, strings.ToLower(strings.TrimSpace(raw))) {
		b.complete(b.popup.matches[b.popup.sel])
		return
	}
	text := strings.TrimRightFunc(raw, unicode.IsSpace)
	if strings.TrimSpace(text) == "" {
		return
	}
	name, args, ok := slashcmd.Parse(text)
	if !ok {
		b.composer.SetText("")
		b.sendErr = ""
		broadcast := b.threadTS != "" && b.broadcast.Value
		b.broadcast.Value = false // Slack's box resets after each reply too
		go b.send(strings.TrimSpace(text), broadcast)
		return
	}
	b.sendErr = ""
	switch name {
	case "/open", "/join":
		b.composer.SetText("")
		go b.openNamed(text, args, "")
	case "/msg", "/dm":
		b.composer.SetText("")
		go b.openNamed(text, args, "dm")
	case "/leave", "/close", "/part":
		// Slack's web client does these itself; chat.command refuses them.
		b.sendErr = name + " isn't supported in slk-gio yet"
	default:
		b.composer.SetText("")
		go b.runCommand(text, name, args)
	}
}

func (b *composeBox) runCommand(typed, name, args string) {
	if err := b.h.be.RunCommand(b.h.ctx, b.channelID, b.threadTS, name, args); err != nil {
		b.box.post(sendFailed{text: typed, note: slashcmd.FailureText(name, err)})
	}
}

// openNamed is /open and /msg: it opens the window of a conversation
// the user is in, by name. For /msg (kind "dm") the target is a direct
// message and the rest of args is put in its composer, not sent.
func (b *composeBox) openNamed(typed, args, kind string) {
	convs, err := b.h.conversations()
	if err != nil {
		b.box.post(sendFailed{text: typed, err: err})
		return
	}
	target := strings.TrimSpace(args)
	if target == "" {
		b.box.post(sendFailed{text: typed, note: "Name a conversation to open"})
		return
	}
	conv, rest, ok := findConversation(convs, target, kind == "dm")
	if !ok {
		b.box.post(sendFailed{text: typed, note: "No conversation named " + strings.Fields(target)[0]})
		return
	}
	b.h.openChatWith(conv, strings.TrimSpace(rest))
}

// commandList draws the popup's rows.
func (b *composeBox) commandList(gtx C) D {
	p := &b.popup
	if !p.open() {
		return D{}
	}
	for i := range p.matches {
		if p.rows[i].Clicked(gtx) {
			b.complete(p.matches[i])
			gtx.Execute(key.FocusCmd{Tag: &b.composer})
			return D{}
		}
	}
	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
		return widget.Border{Color: colBorder, CornerRadius: unit.Dp(6), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
			rows := make([]layout.FlexChild, 0, len(p.matches))
			for i, cmd := range p.matches {
				rows = append(rows, layout.Rigid(func(gtx C) D {
					return p.rows[i].Layout(gtx, func(gtx C) D {
						pointer.CursorPointer.Add(gtx.Ops)
						bg := colBg
						if i == p.sel {
							bg = colHover
						}
						return rounded(gtx, bg, unit.Dp(4), func(gtx C) D {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
								return b.commandRow(gtx, cmd)
							})
						})
					})
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		})
	})
}

func (b *composeBox) commandRow(gtx C, cmd backend.Command) D {
	desc := cmd.Desc
	if cmd.App != "" {
		if desc != "" {
			desc += " · "
		}
		desc += cmd.App
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					lbl := material.Body1(b.th, cmd.Name)
					lbl.Font.Weight = 600
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Flexed(1, func(gtx C) D {
					lbl := material.Body2(b.th, cmd.Usage)
					lbl.Color = colMuted
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx C) D {
			if desc == "" {
				return D{}
			}
			lbl := material.Caption(b.th, desc)
			lbl.Color = colMuted
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		}),
	)
}
