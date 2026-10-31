package main

import (
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/internal/emoji"
	"github.com/gammons/slk/internal/slackfmt"
)

// Message layout: the avatar column and the gap to the text, and the
// list's rhythm.
const (
	avatarDp = unit.Dp(36)
	gutterDp = unit.Dp(10)
	// authorGap is the room between an author line and the body under it.
	authorGap = unit.Dp(2)
	// groupTop is the room above a group's first message (one with its
	// author line); lineTop above a message continuing a group.
	groupTop = unit.Dp(16)
	lineTop  = unit.Dp(2)
	// dateTop and dateBottom are the room around a date separator.
	dateTop    = unit.Dp(16)
	dateBottom = unit.Dp(8)
	// gutterTimeTop lowers a grouped line's hover time in the gutter so
	// its baseline meets the body's first line (caption beside body).
	gutterTimeTop = unit.Dp(3)
)

// chatWindow is one conversation in a window of its own.
type chatWindow struct {
	h    *hub
	conv backend.Conversation
	w    *app.Window
	box  inbox
	th   *material.Theme
	// msgView draws the messages; composeBox is the composer and the
	// note above it.
	msgView
	composeBox

	tl        chat.Timeline
	loading   bool
	loadErr   string
	connected bool
	list      widget.List
	focused   bool
	// winFocused is whether the window has the keyboard; while it does,
	// what it shows is read. marked is the ts last marked read.
	winFocused bool
	marked     string
	// cfg is the window's last configuration (mode, focus, whether the
	// compositor decorates it); header draws the title bar.
	cfg     app.Config
	header  headerBar
	details *backend.Details
	// sw is the quick switcher; peer is the DM partner's presence;
	// sizeDp is the window's size, for a moved copy.
	sw     switcher
	peer   presence
	sizeDp image.Point
	moved  bool // handed to a new window; closing
	// opened is when the user asked for the window, for SLK_GIO_TRACE;
	// shown is set once its first frame is out.
	opened time.Time
	shown  bool
	// jump is a message the window is scrolling to (a search result);
	// jumpPx asks the next frame to leave some context above it.
	jump   jumpState
	jumpPx bool
	// pager asks for older pages as the user scrolls back.
	pager chat.Pager
	// resynced is when the window last caught up after a reconnect.
	resynced time.Time
	// info is the details panel the title opens.
	info infoPanel
}

// jumpState is a message the window is getting to: loading older pages
// until it is there, then scrolling to it and lighting it up briefly.
type jumpState struct {
	ts      string // the target; "" when there is none
	pages   int    // older pages fetched for this jump
	before  string // where the next older page starts
	loading bool   // an older page is in flight
	until   time.Time
	gen     int // tells a stale olderLoaded from the current one
}

// jumpMaxPages bounds how far back a jump will page.
const jumpMaxPages = 30

// jumpHighlight is how long the target stays lit.
const jumpHighlight = 2 * time.Second

type (
	// jumpTo asks the window to show the message at ts.
	jumpTo struct{ ts string }
	// olderLoaded is a page before what was shown: for a jump (gen), or
	// for scrolling back (scroll).
	olderLoaded struct {
		gen    int
		scroll bool
		msgs   []backend.Message
		err    error
	}
	historyLoaded struct {
		msgs []backend.Message
		err  error
	}
	// sendFailed puts text back in the composer with a note under it:
	// note when set, else "Not sent: <err>".
	sendFailed struct {
		text string
		err  error
		note string
	}
	profileLoaded struct {
		id  string
		p   backend.Profile
		err error
	}
	presenceLoaded struct{ active bool }
)

// chatState is what a chat window hands to its replacement when it
// moves to the user's workspace.
type chatState struct {
	conv      backend.Conversation
	connected bool
	tl        chat.Timeline
	loading   bool
	loadErr   string
	details   *backend.Details
	profiles  map[string]*profileEntry
	cmds      []backend.Command
	marked    string
	draft     string
	position  layout.Position
	peer      presence
	sizeDp    image.Point
	jump      jumpState
	pager     chat.Pager
}

// newChatWindowFrom opens a window that carries on where another left
// off: same messages, scroll position, draft and size.
func newChatWindowFrom(h *hub, st chatState) *chatWindow {
	c := newChatWindow(h, st.conv, st.connected)
	c.tl, c.loading, c.loadErr = st.tl, st.loading, st.loadErr
	c.details, c.profiles, c.marked, c.peer = st.details, st.profiles, st.marked, st.peer
	c.popup.cmds = st.cmds
	c.composer.SetText(st.draft)
	c.list.Position = st.position
	// A page the old window asked for reaches this one through
	// hub.toChat; if it was lost, settleJump asks again.
	c.jump = st.jump
	c.jump.loading = false
	c.pager = st.pager
	c.pager.Loading = false
	if st.sizeDp.X > 0 && st.sizeDp.Y > 0 {
		c.w.Option(app.Size(unit.Dp(st.sizeDp.X), unit.Dp(st.sizeDp.Y)))
	}
	return c
}

// moveHere hands the window's state to a new window, which opens on
// the user's current workspace, and closes this one.
func (c *chatWindow) moveHere(text, jumpTS string) {
	if c.moved {
		return
	}
	c.moved = true
	draft := c.composer.Text()
	if draft == "" {
		draft = text
	}
	jump := c.jump
	if jumpTS != "" {
		jump = jumpState{ts: jumpTS, gen: c.jump.gen + 1}
	}
	c.h.adoptChat(chatState{
		conv: c.conv, connected: c.connected, tl: c.tl, loading: c.loading, loadErr: c.loadErr,
		details: c.details, profiles: c.profiles, cmds: c.popup.cmds, marked: c.marked,
		draft: draft, position: c.list.Position, peer: c.peer, sizeDp: c.sizeDp, jump: jump,
		pager: c.pager,
	})
	c.w.Perform(system.ActionClose)
}

func newChatWindow(h *hub, conv backend.Conversation, connected bool) *chatWindow {
	opened := time.Now()
	th := newTheme()
	tracef(opened, "%s: theme ready", conv.Name)
	c := &chatWindow{
		opened:    opened,
		h:         h,
		conv:      conv,
		w:         new(app.Window),
		th:        th,
		loading:   true,
		connected: connected,
	}
	c.box.w = c.w
	c.msgView = newMsgView(h, th, &c.box, func(a backend.Attachment) bool { return c.downloading[backend.AttachmentPath(a)] })
	c.onThread = func(ts string) { h.openThread(c.conv, ts, "") }
	c.composeBox = newComposeBox(h, th, &c.box, conv.ID, "")
	c.onEdit, c.notify = c.composeBox.beginEdit, c.composeBox.note
	c.composeBox.people, c.composeBox.convs, c.composeBox.rv = h.people, h.cachedConversations, &c.msgView.view
	c.composeBox.groups = h.be.UserGroups
	c.onEditLast = func() (backend.Message, bool) { return chat.LastOwn(c.tl.Messages()) }
	c.list.Axis = layout.Vertical
	c.list.ScrollToEnd = true
	c.w.Option(
		app.Title(c.title()+" — "+h.be.Workspace()),
		app.Size(unit.Dp(600), unit.Dp(720)),
		app.Decorated(false), // the header bar is ours
	)
	return c
}

func (c *chatWindow) title() string {
	return slackfmt.ChannelGlyph(string(c.conv.Kind)) + " " + c.conv.Name
}

// hint is the composer's placeholder: "Message #general", or the
// person's name for a direct message.
func (c *chatWindow) hint() string {
	if c.conv.Kind == backend.KindChannel {
		return "Message #" + c.conv.Name
	}
	return "Message " + c.conv.Name
}

func (c *chatWindow) load() {
	go func() {
		if d, err := c.h.be.Details(c.h.ctx, c.conv.ID); err == nil {
			c.box.post(detailsLoaded{d})
		}
	}()
	if c.conv.PeerID != "" {
		c.h.be.WatchDND(c.conv.PeerID) // the header's do-not-disturb, once
		go func() {
			if active, err := c.h.be.Presence(c.h.ctx, c.conv.PeerID); err == nil {
				c.box.post(presenceLoaded{active})
			}
		}()
	}
	msgs, err := c.h.be.History(c.h.ctx, c.conv.ID)
	c.box.post(historyLoaded{msgs, err})
}

// resyncGate is the least time between two reconnect catch-ups of one
// window, so a flapping socket does not refetch on every flap.
const resyncGate = 30 * time.Second

// resync catches the window up after a reconnect: the cache's current
// rows now, and through the backend's catch-up page, whatever was
// posted or deleted while the socket was down.
func (c *chatWindow) resync() {
	msgs, err := c.h.be.History(c.h.ctx, c.conv.ID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: catching up", c.conv.Name+":", err)
		return
	}
	c.box.post(historyLoaded{msgs, nil})
}

// detailsLoaded carries the conversation's topic and size.
type detailsLoaded struct{ d backend.Details }

// headerSpec is the window's header: who or what the conversation is,
// a line about it, and its actions.
func (c *chatWindow) headerSpec(gtx C) headerSpec {
	spec := headerSpec{Title: c.conv.Name}
	var profile *backend.Profile
	if c.conv.Kind == backend.KindDM && c.conv.PeerID != "" {
		profile = c.profile(c.conv.PeerID).p
		if profile != nil && profile.StatusEmoji != "" {
			spec.Status = strings.TrimSpace(emoji.Sprint(profile.StatusEmoji))
		}
	}
	for _, ch := range chat.Chips(c.conv, c.details, profile, gtx.Now) {
		hc := headerChip{text: ch.Text, col: colMuted, grow: ch.Grow}
		switch ch.Icon {
		case chat.ChipPeople:
			hc.icon = iconPeople
		case chat.ChipClock:
			hc.icon = iconClock
		case chat.ChipNight:
			hc.icon = iconNight
		case chat.ChipMuted:
			hc.icon = iconMuted
		case chat.ChipDND:
			hc.icon = iconDND
		}
		if ch.Tone == chat.ToneWarn {
			hc.col = colOffline
		}
		spec.Chips = append(spec.Chips, hc)
	}
	if !c.connected {
		spec.Subtitle, spec.SubColor = "Offline · reconnecting…", colOffline
	}
	switch c.conv.Kind {
	case backend.KindDM:
		id := c.conv.PeerID
		if id == "" {
			id = c.conv.ID
		}
		spec.Leading = func(gtx C) D {
			return withPresence(gtx, func(gtx C) D {
				return c.view.avatar(gtx, id, c.conv.AvatarURL, c.conv.Name, leadingSize)
			}, c.peer, colHeader)
		}
	case backend.KindGroupDM:
		spec.Leading = func(gtx C) D { return tile(gtx, c.th, leadingSize, tint(colAccent), iconPeople, "") }
	case backend.KindPrivate:
		spec.Leading = func(gtx C) D { return tile(gtx, c.th, leadingSize, tint(authorColor(c.conv.Name)), iconLock, "") }
	default:
		spec.Leading = func(gtx C) D { return tile(gtx, c.th, leadingSize, tint(authorColor(c.conv.Name)), nil, "#") }
	}
	// The switcher, with what is waiting elsewhere on its corner: a
	// count of mentions in red, or a dot for other unread.
	sw := headerAction{icon: iconMenu, onClick: c.sw.toggle, active: c.sw.open}
	if unread, mentions := c.h.unreadSummary(); mentions > 0 {
		sw.badge, sw.badgeCol = strconv.Itoa(mentions), colBadge
	} else if unread > 0 {
		sw.badge, sw.badgeCol = "•", colAccent
	}
	spec.Actions = append(spec.Actions, sw)
	if url := c.h.be.WebURL(c.conv.ID); url != "" {
		spec.Actions = append(spec.Actions, headerAction{icon: iconOpenWeb, onClick: func() { openURL(url) }})
	}
	return spec
}

func (c *chatWindow) run() {
	var ops op.Ops
	for {
		switch e := c.w.Event().(type) {
		case app.DestroyEvent:
			c.h.closed(c, e.Err)
			return
		case app.ConfigEvent:
			if !c.shown && c.cfg == (app.Config{}) {
				tracef(c.opened, "%s: first configure", c.conv.Name)
			}
			c.cfg = e.Config
			if e.Config.Focused != c.winFocused {
				c.winFocused = e.Config.Focused
				c.h.setFocused(c.conv.ID, c.winFocused)
				c.markRead()
			}
		case app.FrameEvent:
			if !c.shown {
				tracef(c.opened, "%s: first frame event", c.conv.Name)
			}
			gtx := app.NewContext(&ops, e)
			c.apply()
			c.layout(gtx)
			e.Frame(gtx.Ops)
			if !c.shown {
				c.shown = true
				tracef(c.opened, "%s: first frame drawn", c.conv.Name)
				refillThemes()
			}
		}
	}
}

func (c *chatWindow) apply() {
	for _, item := range c.box.drain() {
		if ok, note := c.applyCommon(item); ok {
			if note != "" {
				c.sendErr = note
			}
			continue
		}
		if c.infoApply(item) || c.typingApply(item) {
			continue
		}
		switch v := item.(type) {
		case backend.NotifyChanged:
			c.conv.Notify = v.Level
		case backend.ThreadState:
			c.tl.Apply(c.conv.ID, v) // not activity: nothing to mark read
		case backend.UserGroupsChanged:
			if !c.loading {
				go c.resync() // bodies again, with the groups' names
			}
		case backend.MutesChanged:
			c.conv.Muted = v.Muted[c.conv.ID]
		case historyLoaded:
			c.loading = false
			if v.err != nil {
				c.loadErr = v.err.Error()
			}
			c.tl.Merge(v.msgs, nil)
			c.markRead()
		case sendFailed:
			c.failed(v)
		case editFailed:
			c.failedEdit(v)
		case commandsLoaded:
			c.popup.cmds = v.cmds
		case channelsFound:
			c.sw.found(v)
		case composeText:
			if c.composer.Len() == 0 {
				c.composer.SetText(v.text)
			}
		case backend.ConnectionChanged:
			if v.Connected && !c.connected && !c.loading && time.Since(c.resynced) > resyncGate {
				// The socket replays nothing it missed: read the
				// cache again, which also asks Slack for what is new.
				c.resynced = time.Now()
				go c.resync()
			}
			c.connected = v.Connected
		case detailsLoaded:
			d := v.d
			c.details = &d
		case backend.MessageDeleted:
			c.tl.Apply(c.conv.ID, v)
			delete(c.widgets, v.TS)
			c.deleted(v.TS)
		case backend.HistoryUpdated:
			// Before the Event case: catching up is not something to
			// mark read, and it ends the loading state a cache-less
			// first open was in. A thread's fetch only refreshes what
			// is shown.
			if v.ThreadTS != "" {
				c.tl.Refresh(v.Messages, v.Deleted)
			} else {
				c.loading = false
				c.tl.Merge(v.Messages, v.Deleted)
			}
			for _, ts := range v.Deleted {
				delete(c.widgets, ts)
				c.deleted(ts)
			}
		case backend.Event:
			c.tl.Apply(c.conv.ID, v)
			c.markRead()
		case raiseWindow:
			c.w.Perform(system.ActionRaise)
		case moveHere:
			c.moveHere(v.text, v.jumpTS)
		case jumpTo:
			c.jump = jumpState{ts: v.ts, gen: c.jump.gen + 1}
		case olderLoaded:
			if v.err == nil {
				c.keepInPlace(func() { c.tl.Merge(v.msgs, nil) }) // more history is never wrong
			}
			if v.scroll {
				c.pager.Done(len(v.msgs), v.err, time.Now())
				if v.err != nil {
					c.sendErr = "Could not load older messages: " + v.err.Error()
				}
				break
			}
			if v.err == nil && len(v.msgs) == 0 {
				c.pager.AtStart = true
			}
			if v.gen == c.jump.gen {
				c.jump.loading = false
				switch {
				case v.err != nil:
					c.sendErr = "Could not load older messages: " + v.err.Error()
					c.jump = jumpState{gen: c.jump.gen}
				case len(v.msgs) == 0:
					c.sendErr = "Message not found: it is older than what Slack returns"
					c.jump = jumpState{gen: c.jump.gen}
				default:
					c.jump.before = v.msgs[0].TS
				}
			}
		case presenceLoaded:
			c.peer = presenceAway
			if v.active {
				c.peer = presenceActive
			}
		case backend.PresenceChanged:
			if v.UserID == c.conv.PeerID {
				c.peer = presenceAway
				if v.Active {
					c.peer = presenceActive
				}
			}
		case unreadChanged:
			// The post already woke the window; it redraws.
		}
	}
	if c.jump.ts != "" {
		c.settleJump()
	}
}

// settleJump moves a jump along: scroll to the target once it is shown
// (and keep it in place while older rows arrive and shift indices), or
// ask for the page before what is shown, or give up.
func (c *chatWindow) settleJump() {
	if c.loading {
		return // history first
	}
	now := time.Now()
	if i, ok := c.tl.Index(c.jump.ts); ok {
		switch {
		case c.jump.until.IsZero():
			c.list.ScrollTo(i + rowMessages)
			c.jumpPx = true
			c.jump.until = now.Add(jumpHighlight)
			c.light(c.jump.ts, now, jumpHighlight)
		case now.Before(c.jump.until):
			c.list.Position.First = i + rowMessages
		default:
			c.jump = jumpState{gen: c.jump.gen}
		}
		return
	}
	if !c.jump.until.IsZero() {
		// It was shown and is gone (deleted meanwhile): done.
		c.jump = jumpState{gen: c.jump.gen}
		return
	}
	oldest := c.oldestTS()
	if oldest != "" && !chat.Later(oldest, c.jump.ts) {
		// Older rows are shown and the target is not among them: it is
		// not in this view (deleted, or a reply only its thread shows).
		c.sendErr = "That message is not in this conversation's history"
		c.jump = jumpState{gen: c.jump.gen}
		return
	}
	if c.jump.loading {
		return
	}
	if c.jump.pages >= jumpMaxPages {
		c.sendErr = "Message not found within " + strconv.Itoa(jumpMaxPages) + " pages of history"
		c.jump = jumpState{gen: c.jump.gen}
		return
	}
	if c.jump.before == "" {
		c.jump.before = oldest
	}
	if c.jump.before == "" {
		return // nothing shown yet; the history will come
	}
	c.jump.loading = true
	c.jump.pages++
	go func(gen int, before string) {
		msgs, err := c.h.be.Older(c.h.ctx, c.conv.ID, before)
		c.h.toChat(c.conv.ID, olderLoaded{gen: gen, msgs: msgs, err: err})
	}(c.jump.gen, c.jump.before)
}

func (c *chatWindow) layout(gtx C) {
	fillBackground(gtx)
	c.sizeDp = image.Pt(int(float32(gtx.Constraints.Max.X)/gtx.Metric.PxPerDp), int(float32(gtx.Constraints.Max.Y)/gtx.Metric.PxPerDp))
	convs := c.h.cachedConversations()
	search := func(q string) { c.h.searchChannels(q, &c.box) }
	if pick := c.sw.update(gtx, convs, c.conv.ID, search); pick.closed {
		gtx.Execute(key.FocusCmd{Tag: &c.composer})
		switch {
		case pick.conv != nil:
			c.h.openChat(*pick.conv)
		case pick.join != nil:
			c.h.joinAndOpen(*pick.join, "", &c.box)
		case pick.all:
			c.h.showList()
		}
	}
	if c.pickerUpdate(gtx) || c.menuUpdate(gtx) {
		gtx.Execute(key.FocusCmd{Tag: &c.composer})
	}
	c.composeBox.update(gtx)
	// Ctrl+K (Cmd+K on macOS) opens the quick switcher.
	for {
		ev, ok := gtx.Event(key.Filter{Name: "K", Required: key.ModShortcut})
		if !ok {
			break
		}
		if k, ok := ev.(key.Event); ok && k.State == key.Press {
			c.sw.toggle()
			if !c.sw.open {
				gtx.Execute(key.FocusCmd{Tag: &c.composer})
			}
		}
	}
	c.handleWidgets(gtx, func() { c.sendErr = "" })
	// The pointer area encloses everything below, so pointer moves over
	// any widget also reach it (Gio routes events to the hit widget and
	// its enclosing areas, not to areas beside it).
	defer c.trackPointer(gtx).Pop()
	c.authorHover, c.pillHover = "", nil
	c.infoUpdate(gtx)
	if !c.focused {
		gtx.Execute(key.FocusCmd{Tag: &c.composer})
		c.focused = true
	}

	spec := c.headerSpec(gtx)
	spec.OnTitle = c.toggleInfo
	if c.info.visible(gtx.Now) {
		v, _ := c.info.prog.Value(gtx.Now)
		spec.Expanded, spec.Seamless = float32(v), true
	}
	c.header.update(gtx, c.w, c.cfg, spec)

	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return c.header.layout(gtx, c.th, spec, c.cfg) }),
		layout.Flexed(1, c.timeline),
		layout.Rigid(func(gtx C) D { return c.composeBox.layout(gtx, c.hint(), "") }),
	)
	c.infoLayout(gtx, gtx.Dp(headerHeight))
	c.sw.layout(gtx, c.th, &c.view)
	c.pickerLayout(gtx)
	c.menuLayout(gtx)
	c.hoverCard(gtx)
}

func (c *chatWindow) timeline(gtx C) D {
	msgs := c.tl.Messages()
	switch {
	case len(msgs) == 0 && c.loading:
		return notice(gtx, c.th, "Loading messages…", colMuted)
	case len(msgs) == 0 && c.loadErr != "":
		return notice(gtx, c.th, c.loadErr, colError)
	case len(msgs) == 0:
		return notice(gtx, c.th, "No messages yet.", colMuted)
	}
	if c.jumpPx {
		// Some of what came before the target above it; layout/list
		// lays earlier rows into a negative offset.
		c.list.Position.Offset = -gtx.Dp(unit.Dp(120))
		c.jumpPx = false
	}
	dims := material.List(c.th, &c.list).Layout(gtx, rowMessages+len(msgs), func(gtx C, i int) D {
		if i < rowMessages {
			return c.historyEdge(gtx)
		}
		i -= rowMessages
		var prev *backend.Message
		if i > 0 {
			prev = &msgs[i-1]
		}
		return c.message(gtx, msgs[i], prev)
	})
	// Near the top, ask for the page before; a jump does its own paging.
	if !c.loading && c.jump.ts == "" && c.pager.Want(c.list.Position.First-rowMessages, len(msgs), gtx.Now) {
		c.loadOlder(c.oldestTS())
	}
	return dims
}

// rowMessages is the list row of the first message: row 0 is the edge
// of the history, which says what is above it.
const rowMessages = 1

// historyEdge is the row above the oldest message shown: older
// messages loading, the conversation's beginning, or nothing.
func (c *chatWindow) historyEdge(gtx C) D {
	var text string
	switch {
	case c.pager.Loading:
		text = "Loading older messages…"
	case c.pager.AtStart:
		text = "This is the beginning of " + slackfmt.ChannelGlyph(string(c.conv.Kind)) + c.conv.Name
		if c.conv.Kind == backend.KindDM || c.conv.Kind == backend.KindGroupDM {
			text = "This is the beginning of your conversation with " + c.conv.Name
		}
	default:
		return D{Size: image.Pt(gtx.Constraints.Max.X, 0)}
	}
	return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Center.Layout(gtx, func(gtx C) D {
			lbl := captionLabel(c.th, text)
			lbl.Color = colMuted
			return lbl.Layout(gtx)
		})
	})
}

// oldestTS is the oldest message shown that Slack keeps (ephemerals
// are not in its history), where the page before starts; "" when none.
func (c *chatWindow) oldestTS() string {
	for _, m := range c.tl.Messages() {
		if !m.Ephemeral {
			return m.TS
		}
	}
	return ""
}

// loadOlder asks for the page before before, for scrolling back. The
// pager has marked it in flight.
func (c *chatWindow) loadOlder(before string) {
	if before == "" {
		c.pager.Done(0, nil, time.Now()) // only ephemerals: nothing to page from
		return
	}
	go func() {
		msgs, err := c.h.be.Older(c.h.ctx, c.conv.ID, before)
		c.h.toChat(c.conv.ID, olderLoaded{scroll: true, msgs: msgs, err: err})
	}()
}

// keepInPlace runs merge, which adds rows above what is shown, and
// moves the list so the message at the top stays where it was.
func (c *chatWindow) keepInPlace(merge func()) {
	msgs := c.tl.Messages()
	pos := c.list.Position
	anchor, offset := anchorOf(msgs, pos.First-rowMessages, pos.Offset)
	merge()
	if anchor == "" {
		return
	}
	if i, ok := c.tl.Index(anchor); ok {
		c.list.Position.First, c.list.Position.Offset = i+rowMessages, offset
	}
}

// anchorOf is the message a list keeps in place while rows are added
// above it, and its offset: the first visible one, or when the history
// edge is visible (first < 0), the oldest message at the top.
func anchorOf(msgs []backend.Message, first, offset int) (string, int) {
	switch {
	case len(msgs) == 0:
		return "", 0
	case first < 0:
		return msgs[0].TS, 0
	case first < len(msgs):
		return msgs[first].TS, offset
	}
	return "", 0
}

func replyCount(n int) string {
	if n == 1 {
		return "1 reply"
	}
	return fmt.Sprintf("%d replies", n)
}

// markRead marks the conversation read up to its newest message while
// the window has the keyboard, as viewing it in Slack does. Ephemeral
// messages are not in Slack's history, so they are not marked.
func (c *chatWindow) markRead() {
	if !c.winFocused {
		return
	}
	msgs := c.tl.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Ephemeral {
			continue
		}
		ts := msgs[i].TS
		if c.marked == "" || chat.Later(ts, c.marked) {
			c.marked = ts
			go func() {
				if err := c.h.be.MarkRead(c.h.ctx, c.conv.ID, ts); err != nil {
					fmt.Fprintln(os.Stderr, "slk-gio: mark read:", err)
				}
			}()
		}
		return
	}
}
