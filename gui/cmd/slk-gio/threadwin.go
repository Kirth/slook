package main

import (
	"fmt"
	"image"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/key"
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

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/internal/slackfmt"
)

var (
	iconThread  = mustIcon(icons.CommunicationForum)
	iconReplies = mustIcon(icons.CommunicationChatBubbleOutline)
	iconChannel = mustIcon(icons.CommunicationChat)
)

// threadKey names a thread: its conversation and its parent's ts.
type threadKey struct{ channelID, ts string }

// focusKey is the key a thread window has the keyboard under in
// hub.focused. It is never a channel ID, so a focused thread does not
// make its channel count as being read.
func (k threadKey) focusKey() string { return k.channelID + "/" + k.ts }

// threadWindow is one thread in a window of its own: the parent, the
// replies, and a composer that replies.
type threadWindow struct {
	h    *hub
	conv backend.Conversation
	key  threadKey
	w    *app.Window
	box  inbox
	th   *material.Theme
	msgView
	composeBox

	thread    chat.Thread
	loading   bool
	loadErr   string
	connected bool
	list      widget.List
	focused   bool // the composer has been given the keyboard
	// winFocused is whether the window has the keyboard; while it does,
	// the thread is read. marked is the ts last marked read.
	winFocused bool
	marked     string
	cfg        app.Config
	header     headerBar
	sizeDp     image.Point
	moved      bool
	resynced   time.Time
	// jump is a reply to scroll to and light once it is shown;
	// jumpPx asks the next frame to leave some context above it.
	jump   string
	jumpPx bool
	// fits is whether the whole thread fit the window last frame: it
	// then reads from the top, and once it does not, the list follows
	// its end as a chat does.
	fits bool
	// cardH is the parent card's height last frame, so its surface can
	// be carried under the list's scrollbar gutter to the window edge.
	cardH int
}

// threadLoaded carries the thread as the backend first answered.
type threadLoaded struct {
	msgs []backend.Message
	err  error
}

// threadState is what a thread window hands to its replacement when it
// moves to the user's workspace.
type threadState struct {
	conv      backend.Conversation
	ts        string
	connected bool
	thread    chat.Thread
	loading   bool
	loadErr   string
	profiles  map[string]*profileEntry
	cmds      []backend.Command
	marked    string
	draft     string
	broadcast bool
	position  layout.Position
	sizeDp    image.Point
	jump      string
	upload    uploadState
}

func newThreadWindow(h *hub, conv backend.Conversation, ts string, connected bool) *threadWindow {
	th := newTheme()
	t := &threadWindow{
		h:         h,
		conv:      conv,
		key:       threadKey{conv.ID, ts},
		w:         new(app.Window),
		th:        th,
		thread:    chat.Thread{ChannelID: conv.ID, TS: ts},
		loading:   true,
		connected: connected,
		fits:      true,
	}
	t.box.w = t.w
	t.msgView = newMsgView(h, th, &t.box, func(a backend.Attachment) bool { return t.downloading[backend.AttachmentPath(a)] })
	t.composeBox = newComposeBox(h, th, &t.box, conv.ID, ts)
	t.composeBox.deliver = func(item any) { h.toThread(t.key, item) }
	t.onEdit, t.notify = t.composeBox.beginEdit, t.composeBox.note
	t.composeBox.people, t.composeBox.convs, t.composeBox.rv = h.people, h.cachedConversations, &t.msgView.view
	t.composeBox.groups = h.be.UserGroups
	t.onEditLast = func() (backend.Message, bool) {
		if m, ok := chat.LastOwn(t.thread.Replies()); ok {
			return m, true
		}
		if p, ok := t.thread.Parent(); ok && p.FromSelf {
			return p, true
		}
		return backend.Message{}, false
	}
	t.list.Axis = layout.Vertical
	t.w.Option(
		app.Title("Thread in "+slackfmt.ChannelGlyph(string(conv.Kind))+" "+conv.Name+" — "+h.be.Workspace()),
		app.Size(unit.Dp(520), unit.Dp(680)),
		app.Decorated(false), // the header bar is ours
	)
	return t
}

// newThreadWindowFrom opens a window that carries on where another
// left off, for moveHere.
func newThreadWindowFrom(h *hub, st threadState) *threadWindow {
	t := newThreadWindow(h, st.conv, st.ts, st.connected)
	t.thread, t.loading, t.loadErr = st.thread, st.loading, st.loadErr
	t.profiles, t.marked, t.jump = st.profiles, st.marked, st.jump
	t.popup.cmds = st.cmds
	t.composer.SetText(st.draft)
	t.broadcast.Value = st.broadcast
	t.list.Position = st.position
	t.adoptUpload(st.upload)
	if st.sizeDp.X > 0 && st.sizeDp.Y > 0 {
		t.w.Option(app.Size(unit.Dp(st.sizeDp.X), unit.Dp(st.sizeDp.Y)))
	}
	return t
}

// moveHere hands the window's state to a new window, which opens on the
// user's current workspace, and closes this one.
func (t *threadWindow) moveHere(text, jumpTS string) {
	if t.moved {
		return
	}
	t.moved = true
	draft := t.composer.Text()
	if draft == "" {
		draft = text
	}
	jump := t.jump
	if jumpTS != "" {
		jump = jumpTS
	}
	t.h.adoptThread(threadState{
		conv: t.conv, ts: t.key.ts, connected: t.connected, thread: t.thread, loading: t.loading,
		loadErr: t.loadErr, profiles: t.profiles, cmds: t.popup.cmds, marked: t.marked, draft: draft,
		broadcast: t.broadcast.Value, position: t.list.Position, sizeDp: t.sizeDp, jump: jump,
		upload: t.uploadState(),
	})
	t.w.Perform(system.ActionClose)
}

// load asks for the thread: the cache's copy now, and through a
// HistoryUpdated, whatever the fetch from Slack changes.
func (t *threadWindow) load() {
	msgs, err := t.h.be.Thread(t.h.ctx, t.conv.ID, t.key.ts)
	t.box.post(threadLoaded{msgs, err})
}

// castLabel is the broadcast box's label: where else the reply goes.
func (t *threadWindow) castLabel() string {
	switch t.conv.Kind {
	case backend.KindDM, backend.KindGroupDM:
		return "Also send as direct message"
	}
	return "Also send to " + slackfmt.ChannelGlyph(string(t.conv.Kind)) + t.conv.Name
}

func (t *threadWindow) headerSpec() headerSpec {
	spec := headerSpec{
		Seamless: true, // the parent card continues the bar
		Title:    "Thread",
		Subtitle: slackfmt.ChannelGlyph(string(t.conv.Kind)) + " " + t.conv.Name,
		Leading:  func(gtx C) D { return tile(gtx, t.th, leadingSize, tint(colAccent), iconThread, "") },
		Actions: []headerAction{{icon: iconChannel, onClick: func() {
			t.h.openChatAt(t.conv, t.key.ts)
		}}},
	}
	if !t.connected {
		spec.Subtitle, spec.SubColor = spec.Subtitle+" · offline, reconnecting…", colOffline
	}
	return spec
}

func (t *threadWindow) run() {
	var ops op.Ops
	for {
		switch e := t.w.Event().(type) {
		case app.DestroyEvent:
			t.h.closed(t, e.Err)
			return
		case app.ConfigEvent:
			t.cfg = e.Config
			if e.Config.Focused != t.winFocused {
				t.winFocused = e.Config.Focused
				t.h.setFocused(t.key.focusKey(), t.winFocused)
				t.markRead()
			}
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			t.apply()
			t.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (t *threadWindow) apply() {
	for _, item := range t.box.drain() {
		if ok, note := t.applyCommon(item); ok {
			if note != "" {
				t.sendErr = note
			}
			continue
		}
		if t.typingApply(item) || t.attachApply(item) {
			continue
		}
		switch v := item.(type) {
		case threadLoaded:
			t.loading = false
			t.loadErr = ""
			if v.err != nil {
				t.loadErr = v.err.Error()
			}
			t.thread.Merge(v.msgs, nil)
			t.markRead()
		case sendFailed:
			t.failed(v)
		case editFailed:
			t.failedEdit(v)
		case commandsLoaded:
			t.popup.cmds = v.cmds
		case composeText:
			if t.composer.Len() == 0 {
				t.composer.SetText(v.text)
			}
		case backend.ConnectionChanged:
			if v.Connected && !t.connected && !t.loading && time.Since(t.resynced) > resyncGate {
				// The socket replays nothing it missed: ask again.
				t.resynced = time.Now()
				go t.load()
			}
			t.connected = v.Connected
		case backend.UserGroupsChanged:
			if !t.loading {
				go t.load() // bodies again, with the groups' names
			}
		case backend.HistoryUpdated:
			t.thread.Merge(v.Messages, v.Deleted)
			for _, ts := range v.Deleted {
				delete(t.widgets, ts)
				t.deleted(ts)
			}
		case backend.MessageDeleted:
			t.thread.Apply(v)
			delete(t.widgets, v.TS)
			t.deleted(v.TS)
		case backend.Event:
			if t.thread.Apply(v) {
				t.markRead()
			}
		case raiseWindow:
			t.w.Perform(system.ActionRaise)
		case moveHere:
			t.moveHere(v.text, v.jumpTS)
		case jumpTo:
			t.jump = v.ts
		}
	}
	t.settleJump()
}

// settleJump scrolls to the reply a search asked for once the thread
// is loaded, and lights it.
func (t *threadWindow) settleJump() {
	if t.jump == "" || t.loading {
		return
	}
	row, ok := t.row(t.jump)
	if !ok {
		t.sendErr = "That reply is no longer in the thread"
		t.jump = ""
		return
	}
	t.list.ScrollTo(row)
	t.jumpPx = true
	t.light(t.jump, time.Now(), jumpHighlight)
	t.jump = ""
}

// The list's rows: the parent card (the parent, or a note that it is
// gone, and the thread's summary), then the replies.
const (
	rowParent  = 0
	rowReplies = 1
)

// row is the list row that shows the message with ts.
func (t *threadWindow) row(ts string) (int, bool) {
	if ts == t.key.ts {
		_, ok := t.thread.Parent()
		return rowParent, ok
	}
	for i, m := range t.thread.Replies() {
		if m.TS == ts {
			return rowReplies + i, true
		}
	}
	return 0, false
}

// markRead marks the thread read up to its newest reply while the
// window has the keyboard. Ephemeral replies are not in Slack's thread.
func (t *threadWindow) markRead() {
	if !t.winFocused {
		return
	}
	replies := t.thread.Replies()
	for i := len(replies) - 1; i >= 0; i-- {
		if replies[i].Ephemeral {
			continue
		}
		ts := replies[i].TS
		if t.marked == "" || chat.Later(ts, t.marked) {
			t.marked = ts
			go func() {
				if err := t.h.be.MarkThreadRead(t.h.ctx, t.conv.ID, t.key.ts, ts); err != nil {
					fmt.Fprintln(os.Stderr, "slk-gio: mark thread read:", err)
				}
			}()
		}
		return
	}
}

func (t *threadWindow) layout(gtx C) {
	fillBackground(gtx)
	t.sizeDp = image.Pt(int(float32(gtx.Constraints.Max.X)/gtx.Metric.PxPerDp), int(float32(gtx.Constraints.Max.Y)/gtx.Metric.PxPerDp))
	if t.pickerUpdate(gtx) || t.menuUpdate(gtx) {
		gtx.Execute(key.FocusCmd{Tag: &t.composer})
	}
	t.composeBox.update(gtx)
	t.handleWidgets(gtx, func() { t.sendErr = "" })
	defer t.trackPointer(gtx).Pop()
	t.authorHover, t.pillHover = "", nil
	if !t.focused {
		gtx.Execute(key.FocusCmd{Tag: &t.composer})
		t.focused = true
	}

	spec := t.headerSpec()
	t.header.update(gtx, t.w, t.cfg, spec)
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return t.header.layout(gtx, t.th, spec, t.cfg) }),
		layout.Flexed(1, t.messages),
		layout.Rigid(func(gtx C) D { return t.composeBox.layout(gtx, "Reply…", t.castLabel()) }),
	)
	t.pickerLayout(gtx)
	t.menuLayout(gtx)
	t.hoverCard(gtx)
}

func (t *threadWindow) messages(gtx C) D {
	parent, hasParent := t.thread.Parent()
	replies := t.thread.Replies()
	switch {
	case !hasParent && len(replies) == 0 && t.loading:
		return notice(gtx, t.th, "Loading thread…", colMuted)
	case !hasParent && len(replies) == 0 && t.loadErr != "":
		return notice(gtx, t.th, t.loadErr, colError)
	}
	if t.jumpPx {
		t.list.Position.Offset = -gtx.Dp(unit.Dp(120))
		t.jumpPx = false
	}
	t.list.ScrollToEnd = !t.fits
	defer func() {
		// The list knows its length only once laid out; a change shows
		// next frame, which is asked for now.
		if fits := t.list.Position.Length <= gtx.Constraints.Max.Y; fits != t.fits {
			t.fits = fits
			gtx.Execute(op.InvalidateCmd{})
		}
	}()
	// The list keeps a gutter for its scrollbar beside the rows; the
	// card's surface continues under it, so the header reads unbroken
	// to the window edge.
	if t.list.Position.First == rowParent && t.cardH > 0 {
		top := -t.list.Position.Offset
		bottom := top + t.cardH
		w := gtx.Constraints.Max.X
		paint.FillShape(gtx.Ops, colHeader, clip.Rect{Min: image.Pt(0, max(top, 0)), Max: image.Pt(w, bottom)}.Op())
		paint.FillShape(gtx.Ops, colBorder, clip.Rect{Min: image.Pt(0, bottom-gtx.Dp(1)), Max: image.Pt(w, bottom)}.Op())
	}
	return material.List(t.th, &t.list).Layout(gtx, rowReplies+len(replies), func(gtx C, i int) D {
		switch i {
		case rowParent:
			return t.parentCard(gtx, parent, hasParent)
		}
		var prev *backend.Message
		if j := i - rowReplies; j > 0 {
			prev = &replies[j-1]
		} else if hasParent {
			// The first reply dates against the parent, so the date line
			// is not repeated, but always shows its author: the divider
			// separates them.
			p := parent
			p.Author, p.AuthorID = "", ""
			prev = &p
		}
		return t.message(gtx, replies[i-rowReplies], prev)
	})
}

// Parent card geometry: the parent's avatar, and the gap from its
// column to the text.
const (
	cardAvatarDp = unit.Dp(44)
	cardGapDp    = unit.Dp(12)
)

// parentCard is the thread's head, on the header bar's own surface so
// the two read as one: the parent with a larger avatar, its author over
// the full date, its "⋯" actions always showing, its body and
// reactions, then the thread's summary (how many replies, when the
// last came) and a rule over the replies. A deleted parent leaves a
// note in its place.
func (t *threadWindow) parentCard(gtx C, parent backend.Message, ok bool) D {
	rec := op.Record(gtx.Ops)
	dims := layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(12), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		var rows []layout.FlexChild
		if ok {
			rows = append(rows, layout.Rigid(func(gtx C) D { return t.parentPost(gtx, parent) }))
		} else {
			rows = append(rows, layout.Rigid(func(gtx C) D {
				lbl := secondaryLabel(t.th, "The message this thread started from was deleted.")
				lbl.Color = colMuted
				return lbl.Layout(gtx)
			}))
		}
		rows = append(rows, layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, t.threadSummary)
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
	call := rec.Stop()
	paint.FillShape(gtx.Ops, colHeader, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	paint.FillShape(gtx.Ops, colBorder, clip.Rect{Min: image.Pt(0, dims.Size.Y-gtx.Dp(1)), Max: dims.Size}.Op())
	if dims.Size.Y != t.cardH {
		t.cardH = dims.Size.Y
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

// parentPost is the parent itself: avatar, author over the full date,
// the "⋯" menu, then the body indented under the name.
func (t *threadWindow) parentPost(gtx C, m backend.Message) D {
	w := t.widgets[m.TS]
	if w == nil {
		w = newMsgWidgets()
		t.widgets[m.TS] = w
	}
	w.begin()
	w.gallery = attachments(m.Body)
	if m.AuthorID != "" && (w.avatarHover.Hovered() || w.nameHover.Hovered()) {
		t.authorHover, t.hoverName = m.AuthorID, m.Author
	}
	if w.more.Clicked(gtx) {
		t.menu.show(m, t.pointer)
	}
	indent := cardAvatarDp + cardGapDp
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Right: cardGapDp}.Layout(gtx, func(gtx C) D {
						return w.avatarHover.Layout(gtx, func(gtx C) D {
							pointer.CursorPointer.Add(gtx.Ops)
							return t.view.avatar(gtx, m.AuthorID, m.AvatarURL, m.Author, cardAvatarDp)
						})
					})
				}),
				layout.Flexed(1, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return w.nameHover.Layout(gtx, func(gtx C) D {
								pointer.CursorPointer.Add(gtx.Ops)
								lbl := bodyLabel(t.th, m.Author)
								lbl.Font.Weight = font.SemiBold
								lbl.Color = authorColor(m.Author)
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx C) D {
							lbl := captionLabel(t.th, chat.FullStamp(m.Time))
							lbl.Color = colMuted
							return lbl.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx C) D { return actionChip(gtx, t.th, &w.more, "⋯") }),
			)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: unit.Dp(8), Left: indent}.Layout(gtx, func(gtx C) D {
				var rows []layout.FlexChild
				rows = append(rows, layout.Rigid(func(gtx C) D {
					return t.selectable(gtx, w, m, func(gtx C) D { return t.view.blocks(gtx, w, m.Body) })
				}))
				if m.Edited {
					rows = append(rows, layout.Rigid(func(gtx C) D {
						lbl := secondaryLabel(t.th, "(edited)")
						lbl.Color = colMuted
						return lbl.Layout(gtx)
					}))
				}
				if len(m.Reactions) > 0 {
					rows = append(rows, layout.Rigid(func(gtx C) D { return t.reactions(gtx, w, m) }))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
			})
		}),
	)
}

// threadSummary is the card's footer: a reply icon and how many
// replies, a hairline, and when the last one came.
func (t *threadWindow) threadSummary(gtx C) D {
	n, last := t.thread.Summary()
	label := "No replies yet"
	if n > 0 {
		label = replyCount(n)
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(unit.Dp(16)), gtx.Dp(unit.Dp(16))))
				return iconReplies.Layout(gtx, colMuted)
			})
		}),
		layout.Rigid(func(gtx C) D {
			lbl := secondaryLabel(t.th, label)
			lbl.Color = colMuted
			return lbl.Layout(gtx)
		}),
	}
	if n > 0 {
		children = append(children,
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
					size := image.Pt(gtx.Dp(unit.Dp(1)), gtx.Dp(unit.Dp(14)))
					paint.FillShape(gtx.Ops, colBorder, clip.Rect{Max: size}.Op())
					return D{Size: size}
				})
			}),
			layout.Rigid(func(gtx C) D {
				lbl := secondaryLabel(t.th, chat.LastReply(last, gtx.Now))
				lbl.Color = colMuted
				return lbl.Layout(gtx)
			}),
		)
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}
