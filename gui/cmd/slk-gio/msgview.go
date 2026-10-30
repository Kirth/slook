package main

import (
	"context"
	"image"
	"os"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/sync/errgroup"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/gui/motion"
	"github.com/gammons/slk/internal/ui/messages"
)

// msgView draws a list of messages and keeps what that needs between
// frames: each message's widgets, the profiles the hover card shows,
// the attachments being downloaded, and a message being lit. The chat
// window and the thread window both embed one.
type msgView struct {
	h    *hub
	box  *inbox // the window's
	th   *material.Theme
	view richView

	widgets     map[string]*msgWidgets // by message ts
	profiles    map[string]*profileEntry
	downloading map[string]bool // attachments being fetched for the previewer, by path

	// Hover card state: where the pointer is, which person a mention
	// span or an author line is hovering.
	pointerTag  struct{}
	pointer     image.Point
	spanHover   string
	authorHover string
	hoverName   string

	// highlight is a message lit until a moment (a jump's target).
	highlight struct {
		ts           string
		start, until time.Time
	}

	// picker chooses a reaction for a message; menu is a message's
	// actions and confirm asks before deleting one; clip is text waiting
	// for the next frame to reach the clipboard.
	picker  reactPicker
	menu    msgMenu
	confirm confirmDelete
	clip    string
	// onEdit starts editing one of the user's messages in the window's
	// composer; notify shows a note under the messages (an error, or a
	// passing note like "Copied").
	onEdit func(backend.Message)
	notify func(text string, isErr bool)

	// onThread opens the thread under a parent's ts. When it is set,
	// messages offer their thread: a reply count, a "Reply" action on
	// hover, a caption on a broadcast reply. The thread window leaves
	// it nil.
	onThread func(threadTS string)
}

func newMsgView(h *hub, th *material.Theme, box *inbox, busy func(backend.Attachment) bool) msgView {
	v := msgView{
		h: h, box: box, th: th,
		widgets:     map[string]*msgWidgets{},
		profiles:    map[string]*profileEntry{},
		downloading: map[string]bool{},
	}
	v.view = richView{th: th, images: h.images, box: box, busy: busy, myGroup: h.isMyGroup}
	return v
}

// applyCommon handles the inbox items every message list gets. It
// reports whether item was one, and a note for the user when something
// failed.
func (v *msgView) applyCommon(item any) (handled bool, note string) {
	switch it := item.(type) {
	case profileLoaded:
		e := v.profiles[it.id]
		if e == nil {
			e = &profileEntry{}
			v.profiles[it.id] = e
		}
		if it.err != nil {
			e.err = it.err
		} else {
			p := it.p
			e.p = &p
		}
		return true, ""
	case previewDone:
		if it.path != "" {
			delete(v.downloading, it.path)
		}
		if it.err != nil {
			return true, "Could not open " + it.name + ": " + it.err.Error()
		}
		return true, ""
	case imageReady:
		return true, "" // the post already woke the window; it redraws
	case customEmojiLoaded:
		v.view.customs = it.customs
		return true, ""
	case reactFailed:
		return true, "Could not react: " + it.err.Error()
	case deleteFailed:
		return true, "Could not delete: " + it.err.Error()
	case permalinkReady:
		switch {
		case it.err != nil:
			return true, "Could not get the link: " + it.err.Error()
		case it.open:
			openURL(it.url)
		default:
			v.clip = it.url
			v.notify("Link copied", false)
		}
		return true, ""
	}
	return false, ""
}

// handleWidgets acts on the messages' link, mention and attachment
// events since the last frame. started is called when a preview starts.
func (v *msgView) handleWidgets(gtx C, started func()) {
	for _, w := range v.widgets {
		w.handle(gtx, v.setSpanHover, func(r previewReq) {
			if v.startPreview(r) && started != nil {
				started()
			}
		})
	}
}

// gutterTime is the avatar column of a line continuing a group: empty,
// or while the pointer is over the line, its time, right-aligned
// against the text and on the baseline of its first line.
func (v *msgView) gutterTime(gtx C, w *msgWidgets, m backend.Message) D {
	size := gtx.Constraints.Min
	if !w.hovered {
		return D{Size: size}
	}
	gtx.Constraints.Max.X = size.X
	return layout.Inset{Top: gutterTimeTop, Right: gutterDp}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.E.Layout(gtx, func(gtx C) D {
			lbl := captionLabel(v.th, m.Time.Format("15:04"))
			lbl.Color = colMuted
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		})
	})
}

// ruledCaption is a caption centered between two thin rules that fill
// the width: a date separator, the reply count under a thread's parent.
func ruledCaption(gtx C, th *material.Theme, text string) D {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, divider),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
				lbl := captionLabel(th, text)
				lbl.Color = colMuted
				return lbl.Layout(gtx)
			})
		}),
		layout.Flexed(1, divider),
	)
}

// threadOf is the thread m starts or is in: its parent's ts.
func threadOf(m backend.Message) string {
	if m.ThreadTS != "" {
		return m.ThreadTS
	}
	return m.TS
}

// hoverRow draws a message's row inside an area that notices the
// pointer and, while the pointer is over it, the message's actions at
// its top right: add a reaction, reply in its thread (when the window
// offers threads) and "⋯", the actions menu, which a right-click on the
// row opens too. The area encloses the row, so the row's links still
// get their clicks.
func (v *msgView) hoverRow(gtx C, w *msgWidgets, m backend.Message, row layout.Widget) D {
	if w.reply.Clicked(gtx) && v.onThread != nil {
		v.onThread(threadOf(m))
	}
	if w.react.Clicked(gtx) {
		v.picker.show(m)
	}
	if w.more.Clicked(gtx) {
		v.menu.show(m, v.pointer)
	}
	v.rowMenu(gtx, w, m)
	hovered := w.rowHover.Update(gtx.Source)
	// The row reads it to show a grouped line's time in its gutter; the
	// row's own actions count, so the time stays while they are used.
	w.hovered = hovered || w.reply.Hovered() || w.react.Hovered() || w.more.Hovered()
	rec := op.Record(gtx.Ops)
	dims := row(gtx)
	call := rec.Stop()
	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
	w.rowHover.Add(gtx.Ops)
	event.Op(gtx.Ops, w) // right-clicks, for the menu
	call.Add(gtx.Ops)
	if hovered || w.reply.Hovered() || w.react.Hovered() || w.more.Hovered() {
		gtx.Constraints = layout.Exact(dims.Size)
		layout.NE.Layout(gtx, func(gtx C) D {
			actions := []layout.FlexChild{layout.Rigid(func(gtx C) D { return actionChip(gtx, v.th, &w.react, "+ 🙂") })}
			if v.onThread != nil {
				actions = append(actions,
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx C) D { return actionChip(gtx, v.th, &w.reply, "Reply in thread") }))
			}
			actions = append(actions,
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx C) D { return actionChip(gtx, v.th, &w.more, "⋯") }))
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, actions...)
		})
	}
	return dims
}

// actionChip is a hover action on a message, such as replying in its
// thread.
func actionChip(gtx C, th *material.Theme, cl *widget.Clickable, label string) D {
	return cl.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		bg := colBg
		if cl.Hovered() {
			bg = colHover
		}
		return widget.Border{Color: colBorder, CornerRadius: cardRadius, Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
			return rounded(gtx, bg, cardRadius, func(gtx C) D {
				return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
					lbl := captionLabel(th, label)
					lbl.Color = colAccent
					return lbl.Layout(gtx)
				})
			})
		})
	})
}

// linkCaption is a small link that acts on a click, such as "3 replies".
func linkCaption(gtx C, th *material.Theme, cl *widget.Clickable, text string) D {
	return cl.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		lbl := secondaryLabel(th, text)
		lbl.Color = colAccent
		if cl.Hovered() {
			lbl.Font.Weight = font.SemiBold
		}
		return lbl.Layout(gtx)
	})
}

// profileEntry is a profile being or done loading.
type profileEntry struct {
	p   *backend.Profile
	err error
}

func (v *msgView) setSpanHover(id string, on bool) {
	switch {
	case on:
		v.spanHover = id
	case v.spanHover == id:
		v.spanHover = ""
	}
}

// profile returns what is known about id, starting a fetch the first
// time it is asked for.
func (v *msgView) profile(id string) *profileEntry {
	if e := v.profiles[id]; e != nil {
		return e
	}
	e := &profileEntry{}
	v.profiles[id] = e
	go func() {
		p, err := v.h.be.Profile(v.h.ctx, id)
		v.box.post(profileLoaded{id, p, err})
	}()
	return e
}

// hoverCard draws the profile card beside the pointer when a mention
// or an author line is hovered. It is drawn last, so it is on top.
func (v *msgView) hoverCard(gtx C) {
	if v.menu.open || v.confirm.open || v.picker.open {
		return // a popup is in front; the card would cover it
	}
	id := v.spanHover
	if id == "" {
		id = v.authorHover
	}
	if id == "" {
		return
	}
	e := v.profile(id)
	name := v.hoverName
	if v.spanHover != "" {
		name = ""
	}
	m := op.Record(gtx.Ops)
	dims := v.view.profileCard(gtx, name, e.p, e.err)
	call := m.Stop()
	pos := v.pointer.Add(image.Pt(gtx.Dp(14), gtx.Dp(18)))
	if pos.X+dims.Size.X > gtx.Constraints.Max.X {
		pos.X = max(0, gtx.Constraints.Max.X-dims.Size.X-gtx.Dp(4))
	}
	if pos.Y+dims.Size.Y > gtx.Constraints.Max.Y {
		pos.Y = max(0, v.pointer.Y-dims.Size.Y-gtx.Dp(8))
	}
	defer op.Offset(pos).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
}

// trackPointer remembers where the pointer is, so the hover card can
// be placed beside it. It opens a pointer area the caller closes once
// the window's content is laid out inside it.
func (v *msgView) trackPointer(gtx C) clip.Stack {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &v.pointerTag, Kinds: pointer.Move | pointer.Enter | pointer.Drag})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			v.pointer = image.Pt(int(e.Position.X), int(e.Position.Y))
		}
	}
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &v.pointerTag)
	return area
}

// lit draws w with a highlight behind it while ts is the message being
// lit (a jump's target): it comes up quickly, then fades.
func (v *msgView) lit(gtx C, ts string, w layout.Widget) D {
	h := v.highlight
	if ts != h.ts || h.until.IsZero() || !gtx.Now.Before(h.until) {
		return w(gtx)
	}
	// Painted behind the row, a little outside it, so the row keeps
	// its place as the highlight comes and goes.
	strength := motion.Pulse(h.start, gtx.Now, highlightIn, h.until.Sub(h.start))
	gtx.Execute(op.InvalidateCmd{})
	rec := op.Record(gtx.Ops)
	dims := w(gtx)
	call := rec.Stop()
	col := colControlHover
	col.A = uint8(255 * strength)
	pad := gtx.Dp(unit.Dp(4))
	paint.FillShape(gtx.Ops, col, clip.UniformRRect(image.Rect(-pad, -pad, dims.Size.X+pad, dims.Size.Y+pad), gtx.Dp(cardRadius)).Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

// highlightIn is how long a jump's highlight takes to come up.
const highlightIn = 120 * time.Millisecond

// light starts the highlight on the message at ts, for d.
func (v *msgView) light(ts string, now time.Time, d time.Duration) {
	v.highlight.ts, v.highlight.start, v.highlight.until = ts, now, now.Add(d)
}

// message draws one message: a date line when the day changes, then
// the avatar beside the author line (unless it continues the previous
// author's run), the body's blocks, and a reply count.
func (v *msgView) message(gtx C, m backend.Message, prev *backend.Message) D {
	newDay := chat.NewDay(prev, m)
	showAuthor := chat.StartsGroup(prev, m)

	w := v.widgets[m.TS]
	if w == nil {
		w = newMsgWidgets()
		v.widgets[m.TS] = w
	}
	w.begin()
	w.gallery = attachments(m.Body)
	if m.AuthorID != "" && (w.avatarHover.Hovered() || w.nameHover.Hovered()) {
		v.authorHover, v.hoverName = m.AuthorID, m.Author
	}

	var rows []layout.FlexChild
	if newDay {
		rows = append(rows, layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: dateTop, Bottom: dateBottom}.Layout(gtx, func(gtx C) D {
				return ruledCaption(gtx, v.th, messages.FormatDateSeparator(m.Time.Format("2006-01-02")))
			})
		}))
	}
	top := lineTop
	if showAuthor {
		top = groupTop
	}
	row := func(gtx C) D {
		return layout.Flex{}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Dp(avatarDp + gutterDp)
				if !showAuthor {
					return v.gutterTime(gtx, w, m)
				}
				return layout.Inset{Right: gutterDp}.Layout(gtx, func(gtx C) D {
					return w.avatarHover.Layout(gtx, func(gtx C) D {
						pointer.CursorPointer.Add(gtx.Ops)
						return v.view.avatar(gtx, m.AuthorID, m.AvatarURL, m.Author, avatarDp)
					})
				})
			}),
			layout.Flexed(1, func(gtx C) D { return v.body(gtx, w, m, showAuthor) }),
		)
	}
	rows = append(rows, layout.Rigid(func(gtx C) D {
		return layout.Inset{Top: top}.Layout(gtx, func(gtx C) D {
			if !m.Ephemeral {
				return v.lit(gtx, m.TS, func(gtx C) D { return v.hoverRow(gtx, w, m, row) })
			}
			// Only the user sees this: a tinted box with a label and an
			// accent rule on its left.
			return accentCard(gtx, colEphemeral, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return layout.Inset{Left: avatarDp + gutterDp, Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
								lbl := captionLabel(v.th, "Only visible to you")
								lbl.Color = colMuted
								return lbl.Layout(gtx)
							})
						}),
						layout.Rigid(row),
					)
				})
			})
		})
	}))
	return layout.Inset{Left: unit.Dp(14), Right: unit.Dp(14), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
}

// body is the author line, the blocks, and the trailing captions.
func (v *msgView) body(gtx C, w *msgWidgets, m backend.Message, showAuthor bool) D {
	var rows []layout.FlexChild
	if showAuthor {
		// The author line: the name in semibold and the time a caption
		// beside it on the same baseline, with a little room before the
		// body so the name does not sit on its first line.
		rows = append(rows, layout.Rigid(func(gtx C) D {
			return layout.Inset{Bottom: authorGap}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return w.nameHover.Layout(gtx, func(gtx C) D {
							pointer.CursorPointer.Add(gtx.Ops)
							lbl := bodyLabel(v.th, m.Author)
							lbl.Font.Weight = font.SemiBold
							lbl.Color = authorColor(m.Author)
							return lbl.Layout(gtx)
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx C) D {
						lbl := captionLabel(v.th, m.Time.Format("15:04"))
						lbl.Color = colMuted
						return lbl.Layout(gtx)
					}),
				)
			})
		}))
	}
	rows = append(rows, layout.Rigid(func(gtx C) D { return v.view.blocks(gtx, w, m.Body) }))
	if m.Edited {
		rows = append(rows, layout.Rigid(func(gtx C) D {
			lbl := secondaryLabel(v.th, "(edited)")
			lbl.Color = colMuted
			return lbl.Layout(gtx)
		}))
	}
	if len(m.Reactions) > 0 && !m.Ephemeral {
		rows = append(rows, layout.Rigid(func(gtx C) D { return v.reactions(gtx, w, m) }))
	}
	if v.onThread == nil {
		// The thread window: the thread is what it shows.
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	}
	if w.replies.Clicked(gtx) {
		v.onThread(threadOf(m))
	}
	switch {
	case m.ReplyCount > 0:
		rows = append(rows, layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
				return linkCaption(gtx, v.th, &w.replies, replyCount(m.ReplyCount))
			})
		}))
	case m.ThreadTS != "" && m.ThreadTS != m.TS && !m.Ephemeral:
		// A reply also sent to the conversation.
		rows = append(rows, layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
				return linkCaption(gtx, v.th, &w.replies, "Replied to a thread")
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

// startPreview opens an attachment in the previewer, downloading the
// message's attachments first. It reports whether it started; a second
// click while a download runs is ignored.
func (v *msgView) startPreview(req previewReq) bool {
	if req.index >= len(req.items) {
		return false
	}
	if v.downloading[backend.AttachmentPath(req.items[req.index])] {
		return false
	}
	for _, a := range req.items {
		v.downloading[backend.AttachmentPath(a)] = true
	}
	go v.preview(req)
	return true
}

// preview downloads a message's attachments (each only once; the paths
// are stable) and shows the clicked one in the previewer.
func (v *msgView) preview(req previewReq) {
	ctx, cancel := context.WithTimeout(v.h.ctx, previewTimeout)
	defer cancel()
	paths := make([]string, len(req.items))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for i, a := range req.items {
		g.Go(func() error {
			p, err := v.h.be.Download(gctx, a)
			paths[i] = p
			v.box.post(previewDone{path: backend.AttachmentPath(a), name: a.Name, err: err})
			if i == req.index {
				return err // the others failing does not stop this one
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return // reported through previewDone
	}
	// The previewer steps through what downloaded; the failures drop out.
	var ok []string
	index := 0
	for i, p := range paths {
		if _, err := os.Stat(p); p == "" || err != nil {
			continue
		}
		if i == req.index {
			index = len(ok)
		}
		ok = append(ok, p)
	}
	if err := showPreview(ok, index); err != nil {
		v.box.post(previewDone{name: req.items[req.index].Name, err: err})
	}
}
