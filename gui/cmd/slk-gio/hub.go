package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"gioui.org/app"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/notifications"
)

// hub owns the set of open windows and routes backend events to them.
// Each window runs its own event loop on its own goroutine; everything
// reaches a window through its inbox, never by touching its state.
type hub struct {
	ctx    context.Context
	be     backend.Backend
	onLast func(error) // called when the last window closes, with its error

	images *imageStore
	// commands lists the workspace's slash commands, fetched once.
	commands func() ([]backend.Command, error)
	// customEmoji is the workspace's custom emoji, fetched once.
	customEmoji func() (map[string]string, error)

	// peopleList is who @ completes to, read from the backend at most
	// every peopleFresh (see people).
	peopleMu      sync.Mutex
	peopleList    []backend.Person
	peopleAt      time.Time
	peopleLoading bool

	mu    sync.Mutex
	convs []backend.Conversation // the last list loaded, for /open and /msg
	// focused is the windows that have the keyboard: chat windows by
	// channel ID, thread windows by threadKey.focusKey.
	focused map[string]bool
	opening map[string]bool // chat windows being built, by channel ID
	// unread and mentions summarise the list: conversations needing
	// reading, and how many of them mention the user.
	unread, mentions int
	// muted is the muted conversations, from the list and then the
	// socket's mute changes; notes decides and shows notifications (nil
	// when they are off).
	muted    map[string]bool
	levels   map[string]backend.NotifyLevel // slk's notify levels, by channel ID
	myGroups map[string]bool                // the user's own usergroups, by ID
	// askedConv is the conversations asked of Slack because a message
	// came from one the list did not know; each is asked once.
	askedConv map[string]bool
	// typingOn is [animations] typing_indicators: show who is typing,
	// and tell Slack when the user is. Set before any window opens.
	typingOn   bool
	notes      *notifications.Center
	list       *listWindow
	search     *searchWindow  // the one search window, or nil
	threadsWin *threadsWindow // the one Threads window, or nil
	// threadsUnread is how many followed threads have unread replies,
	// for the list's Threads row.
	threadsUnread  backend.ThreadsUnread
	chats          map[string]*chatWindow // by channel ID
	threads        map[threadKey]*threadWindow
	openingThreads map[threadKey]bool // thread windows being built
	windows        int
	connected      bool
}

func newHub(ctx context.Context, be backend.Backend, onLast func(error)) *hub {
	h := &hub{ctx: ctx, be: be, onLast: onLast, images: newImageStore(ctx, be.Images()),
		chats: map[string]*chatWindow{}, focused: map[string]bool{}, opening: map[string]bool{},
		threads: map[threadKey]*threadWindow{}, openingThreads: map[threadKey]bool{}}
	h.commands = sync.OnceValues(func() ([]backend.Command, error) {
		cmds, err := be.Commands(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: slash commands:", err)
		}
		return cmds, err
	})
	h.customEmoji = sync.OnceValues(func() (map[string]string, error) {
		customs, err := be.CustomEmoji(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: custom emoji:", err)
		}
		return customs, err
	})
	return h
}

// conversations is the conversation list, loading it if no window has.
func (h *hub) conversations() ([]backend.Conversation, error) {
	h.mu.Lock()
	convs := h.convs
	h.mu.Unlock()
	if convs != nil {
		return convs, nil
	}
	convs, err := h.be.Conversations(h.ctx)
	if err != nil {
		return nil, err
	}
	h.setConversations(convs)
	return convs, nil
}

// cachedConversations is the last conversation list the list window
// published, without fetching; nil before it has loaded.
func (h *hub) cachedConversations() []backend.Conversation {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.convs
}

func (h *hub) setConversations(convs []backend.Conversation) {
	h.mu.Lock()
	h.convs = convs
	h.muted = map[string]bool{}
	h.levels = map[string]backend.NotifyLevel{}
	for _, c := range convs {
		if c.Muted {
			h.muted[c.ID] = true
		}
		if c.Notify != backend.NotifyDefault {
			h.levels[c.ID] = c.Notify
		}
	}
	h.mu.Unlock()
}

// Things a window can be asked to do, besides apply a backend.Event.
type (
	raiseWindow struct{}
	// moveHere asks an open window to come to the user's current
	// workspace. Wayland lets no app raise or move its own window, but a
	// new one opens where the user is, so the window hands its state to
	// a new one (hub.adoptChat, adoptList) and closes. text, when set,
	// goes in the composer.
	moveHere struct {
		text string
		// jumpTS, for a chat window, is a message to scroll to once
		// the window is up (a search result).
		jumpTS string
	}
	// unreadChanged wakes a chat window to redraw its switcher badge.
	unreadChanged struct{}
	// activity tells the list a message arrived. unread is whether it
	// makes the conversation unread: not the user's own, not a reply
	// only in a thread, not an edit, and not in a window the user is
	// looking at.
	activity struct {
		channelID string
		at        time.Time
		unread    bool
	}
	// visited tells the list the user opened a conversation.
	visited struct {
		channelID string
		at        time.Time
	}
)

// setFocused records whether a window has the keyboard, under a chat
// window's channel ID or a thread window's focusKey.
func (h *hub) setFocused(key string, on bool) {
	h.mu.Lock()
	if on {
		h.focused[key] = true
	} else {
		delete(h.focused, key)
	}
	notes := h.notes
	h.mu.Unlock()
	if on && notes != nil {
		// Looking at a conversation (or one of its threads) reads it:
		// its notification goes. A thread's key is "channel/ts".
		channelID, _, _ := strings.Cut(key, "/")
		go notes.Read(channelID)
	}
}

// anyFocused reports whether a chat window has the keyboard: the user
// is busy, and background work stands aside.
func (h *hub) anyFocused() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.focused) > 0
}

// dispatch hands a backend event to the windows it concerns. It runs on
// the backend's goroutine.
func (h *hub) dispatch(ev backend.Event) {
	h.mu.Lock()
	var to []*inbox
	switch ev := ev.(type) {
	case backend.MessagePosted:
		m := ev.Message
		if c := h.chats[m.ChannelID]; c != nil {
			to = append(to, &c.box)
		}
		h.askUnknownLocked(m.ChannelID)
		if h.list != nil && !m.Ephemeral {
			unread := !m.FromSelf && !m.Edited && !m.InThreadOnly && !h.focused[m.ChannelID]
			h.list.box.post(activity{m.ChannelID, m.Time, unread})
		}
		if h.notes != nil {
			conv := h.conversationLocked(m.ChannelID)
			conv.Muted, conv.Notify = h.muted[m.ChannelID], h.levels[m.ChannelID]
			// Off the event goroutine: showing one talks to the desktop.
			go h.notes.Message(m, conv, len(h.focused) > 0)
		}
	case backend.ReadChanged:
		if h.list != nil {
			to = append(to, &h.list.box)
		}
		if h.notes != nil && !ev.Unread {
			go h.notes.Read(ev.ChannelID) // read here or elsewhere
		}
	case backend.MutesChanged:
		h.muted = ev.Muted
		if h.list != nil {
			to = append(to, &h.list.box)
		}
		for _, c := range h.chats {
			to = append(to, &c.box)
		}
	case backend.ThreadsUnread:
		h.threadsUnread = ev
		if h.list != nil {
			to = append(to, &h.list.box)
		}
	case backend.ThreadState:
		if c := h.chats[ev.ChannelID]; c != nil {
			to = append(to, &c.box)
		}
		if h.threadsWin != nil {
			to = append(to, &h.threadsWin.box)
		}
	case backend.ThreadsChanged:
		if h.threadsWin != nil {
			to = append(to, &h.threadsWin.box)
		}
	case backend.ProfileChanged:
		// Whoever shows them: any window's header or hover card.
		for _, c := range h.chats {
			to = append(to, &c.box)
		}
		for _, t := range h.threads {
			to = append(to, &t.box)
		}
	case backend.ConversationOpened:
		if h.list != nil {
			to = append(to, &h.list.box) // its publish brings h.convs along
		} else {
			h.convs = upsertConv(h.convs, ev.Conv)
		}
	case backend.Typing:
		if c := h.chats[ev.ChannelID]; c != nil && h.typingOn {
			to = append(to, &c.box)
		}
	case backend.AppStatus:
		if !h.typingOn {
			break
		}
		if c := h.chats[ev.ChannelID]; c != nil {
			to = append(to, &c.box)
		}
		for k, t := range h.threads {
			if k.channelID == ev.ChannelID && k.ts == ev.ThreadTS {
				to = append(to, &t.box)
			}
		}
	case backend.UserGroupsChanged:
		h.myGroups = map[string]bool{}
		for _, id := range ev.Mine {
			h.myGroups[id] = true
		}
		if h.notes != nil {
			h.notes.SetGroups(ev.Mine)
		}
		// Bodies are built when read: open windows read theirs again,
		// so mentions show the groups' names.
		for _, c := range h.chats {
			to = append(to, &c.box)
		}
		for _, t := range h.threads {
			to = append(to, &t.box)
		}
	case backend.NotifyChanged:
		if h.levels == nil {
			h.levels = map[string]backend.NotifyLevel{}
		}
		h.levels[ev.ChannelID] = ev.Level
		// A copy: the list window may share the slice it handed over.
		h.convs = slices.Clone(h.convs)
		for i := range h.convs {
			if h.convs[i].ID == ev.ChannelID {
				h.convs[i].Notify = ev.Level
			}
		}
		if h.list != nil {
			to = append(to, &h.list.box)
		}
		if c := h.chats[ev.ChannelID]; c != nil {
			to = append(to, &c.box)
		}
	case backend.SectionsChanged:
		if h.list != nil {
			to = append(to, &h.list.box)
		}
	case backend.PresenceChanged:
		for _, c := range h.chats {
			to = append(to, &c.box)
		}
		if h.list != nil {
			to = append(to, &h.list.box)
		}
	case backend.MessageDeleted:
		if c := h.chats[ev.ChannelID]; c != nil {
			to = append(to, &c.box)
		}
	case backend.ReactionChanged:
		// Not activity: only the windows showing the conversation.
		if c := h.chats[ev.ChannelID]; c != nil {
			to = append(to, &c.box)
		}
	case backend.HistoryUpdated:
		// The cache catching up is not activity: only the window on
		// the conversation hears of it, never the list.
		if c := h.chats[ev.ChannelID]; c != nil {
			to = append(to, &c.box)
		}
	case backend.SyncChanged:
		if h.search != nil {
			to = append(to, &h.search.box)
		}
	case backend.ConnectionChanged:
		h.connected = ev.Connected
		for _, c := range h.chats {
			to = append(to, &c.box)
		}
		if h.list != nil {
			to = append(to, &h.list.box)
		}
		if h.search != nil {
			to = append(to, &h.search.box)
		}
	}
	open := make([]threadKey, 0, len(h.threads))
	for k := range h.threads {
		open = append(open, k)
	}
	for _, k := range threadTargets(ev, open) {
		to = append(to, &h.threads[k].box)
	}
	h.mu.Unlock()
	for _, b := range to {
		b.post(ev)
	}
}

// threadTargets is which of the open threads an event concerns: a
// message in a thread (or the thread's parent, edited); a deletion, a
// catch-up or a reaction in a conversation, which may name any of its
// threads' rows; the connection.
func threadTargets(ev backend.Event, open []threadKey) []threadKey {
	var out []threadKey
	switch ev := ev.(type) {
	case backend.MessagePosted:
		m := ev.Message
		for _, k := range open {
			if k.channelID == m.ChannelID && (k.ts == m.TS || m.ThreadTS != "" && k.ts == m.ThreadTS) {
				out = append(out, k)
			}
		}
	case backend.MessageDeleted:
		for _, k := range open {
			if k.channelID == ev.ChannelID {
				out = append(out, k)
			}
		}
	case backend.HistoryUpdated:
		for _, k := range open {
			if k.channelID == ev.ChannelID {
				out = append(out, k)
			}
		}
	case backend.ReactionChanged:
		// The message may be any thread's parent or reply.
		for _, k := range open {
			if k.channelID == ev.ChannelID {
				out = append(out, k)
			}
		}
	case backend.ConnectionChanged:
		out = append(out, open...)
	}
	return out
}

// unreadSummary is how many conversations need reading, and how many
// of those mention the user.
func (h *hub) unreadSummary() (unread, mentions int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.unread, h.mentions
}

// setUnread records the list's unread summary; chat windows redraw
// their switcher badge when it changes.
func (h *hub) setUnread(unread, mentions int) {
	h.mu.Lock()
	changed := unread != h.unread || mentions != h.mentions
	h.unread, h.mentions = unread, mentions
	var to []*inbox
	if changed {
		for _, c := range h.chats {
			to = append(to, &c.box)
		}
	}
	h.mu.Unlock()
	for _, b := range to {
		b.post(unreadChanged{})
	}
}

// showList opens the conversation list, or brings it here if it is
// open (see moveHere).
func (h *hub) showList() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.list != nil {
		h.list.box.post(moveHere{})
		return
	}
	l := newListWindow(h, h.connected)
	h.list = l
	l.box.post(h.threadsUnread)
	h.windows++
	go l.run()
	go l.load()
}

// openChat opens a conversation's window, or brings it here if it is
// open (see moveHere).
func (h *hub) openChat(conv backend.Conversation) { h.openChatWith(conv, "") }

// openChatWith is openChat that also puts text in the window's composer
// when it is empty.
//
// The window is built outside the hub's lock: the caller is a window's
// event loop, and nothing should wait on another window being made.
func (h *hub) openChatWith(conv backend.Conversation, text string) {
	h.mu.Lock()
	c, busy := h.chats[conv.ID], h.opening[conv.ID]
	if c == nil && !busy {
		h.opening[conv.ID] = true
	}
	connected, list := h.connected, h.list
	h.mu.Unlock()

	switch {
	case c != nil:
		c.box.post(moveHere{text: text})
	case busy:
		// A click a moment ago is already opening it.
	default:
		c = newChatWindow(h, conv, connected)
		h.mu.Lock()
		delete(h.opening, conv.ID)
		h.chats[conv.ID] = c
		h.windows++
		h.mu.Unlock()
		go c.run()
		go c.load()
		go c.loadCommands()
		go c.loadCustomEmoji()
		if text != "" {
			c.box.post(composeText{text})
		}
	}
	go h.be.Visited(conv.ID)
	if list != nil {
		list.box.post(visited{conv.ID, time.Now()})
	}
}

// joinAndOpen joins a channel the user is not in and opens its window,
// with text in its composer when that is empty. It returns at once; a
// failure is reported to from (the window that asked) as a note.
func (h *hub) joinAndOpen(r backend.ChannelResult, text string, from *inbox) {
	go func() {
		conv, err := h.be.Join(h.ctx, r.ID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: join #"+r.Name+":", err)
			from.post(sendFailed{note: "Could not join #" + r.Name + ": " + err.Error()})
			return
		}
		h.openChatWith(conv, text)
	}()
}

// openDM opens the DM with one person, or the group DM with several,
// and its window, with text in its composer when that is empty (never
// over a draft). A DM the list has opens with no request; anything else
// goes through conversations.open. It returns at once; a failure is
// reported to from as a note.
func (h *hub) openDM(ids []string, text string, from *inbox) {
	if len(ids) == 1 {
		h.mu.Lock()
		var found *backend.Conversation
		for _, c := range h.convs {
			if c.Kind == backend.KindDM && c.PeerID == ids[0] {
				found = &c
				break
			}
		}
		h.mu.Unlock()
		if found != nil {
			h.openChatWith(*found, text)
			return
		}
	}
	go func() {
		conv, err := h.be.OpenConversation(h.ctx, ids)
		if err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: open conversation:", err)
			from.post(sendFailed{note: "Could not start the conversation: " + err.Error()})
			return
		}
		h.openChatWith(conv, text)
	}()
}

// channelsFound is Slack's answer to a filter's channel search (see
// chat.RemoteSearch).
type channelsFound struct {
	q   string
	res []backend.ChannelResult
	err error
}

// searchChannels asks Slack which channels match q, off the window's
// event loop, and posts the answer to box as a channelsFound.
func (h *hub) searchChannels(q string, box *inbox) {
	go func() {
		res, err := h.be.SearchChannels(h.ctx, q)
		if err == nil && res == nil {
			res = []backend.ChannelResult{} // answered: nothing matches
		}
		box.post(channelsFound{q: q, res: res, err: err})
	}()
}

// adoptChat opens a window that takes over an open one's state, for
// moveHere; the old window closes itself after.
func (h *hub) adoptChat(st chatState) {
	c := newChatWindowFrom(h, st)
	h.mu.Lock()
	h.chats[st.conv.ID] = c
	h.windows++
	h.mu.Unlock()
	go c.run()
	go c.loadCustomEmoji()
}

// adoptList is adoptChat for the list window.
func (h *hub) adoptList(st listState) {
	l := newListWindowFrom(h, st)
	h.mu.Lock()
	h.list = l
	l.box.post(h.threadsUnread)
	h.windows++
	h.mu.Unlock()
	go l.run()
}

// closed forgets a window whose event loop ended. err is why, when the
// window did not close normally (for example, it could not be created).
func (h *hub) closed(w any, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: window:", err)
	}
	h.mu.Lock()
	switch w := w.(type) {
	case *listWindow:
		if h.list == w { // not if a moved copy replaced it
			h.list = nil
		}
	case *chatWindow:
		if h.chats[w.conv.ID] == w {
			delete(h.chats, w.conv.ID)
			delete(h.focused, w.conv.ID)
		}
	case *searchWindow:
		if h.search == w {
			h.search = nil
		}
	case *threadsWindow:
		if h.threadsWin == w {
			h.threadsWin = nil
		}
	case *threadWindow:
		if h.threads[w.key] == w {
			delete(h.threads, w.key)
			delete(h.focused, w.key.focusKey())
		}
	}
	h.windows--
	last := h.windows == 0
	h.mu.Unlock()
	if last {
		h.onLast(err)
	}
}

// inbox is how other goroutines reach a window: post queues an item and
// wakes the window, which drains the queue at its next frame.
type inbox struct {
	w     *app.Window
	mu    sync.Mutex
	items []any
}

func (b *inbox) post(v any) {
	b.mu.Lock()
	b.items = append(b.items, v)
	b.mu.Unlock()
	b.w.Invalidate()
}

func (b *inbox) drain() []any {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := b.items
	b.items = nil
	return items
}

// showSearch opens the search window, or brings it here if it is open
// (see moveHere).
func (h *hub) showSearch() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.search != nil {
		h.search.box.post(moveHere{})
		return
	}
	s := newSearchWindow(h, h.connected)
	h.search = s
	h.windows++
	go s.run()
	go s.load()
}

// adoptSearch is adoptChat for the search window.
func (h *hub) adoptSearch(st searchState) {
	s := newSearchWindowFrom(h, st)
	h.mu.Lock()
	h.search = s
	h.windows++
	h.mu.Unlock()
	go s.run()
	go s.load()
}

// openChatAt opens a conversation's window showing the message at ts.
func (h *hub) openChatAt(conv backend.Conversation, ts string) {
	h.mu.Lock()
	c := h.chats[conv.ID]
	h.mu.Unlock()
	if c != nil {
		// The window moves here and carries the jump into its
		// replacement: posting jumpTo after moveHere would reach the
		// window that is closing.
		c.box.post(moveHere{jumpTS: ts})
		return
	}
	h.openChat(conv)
	h.toChat(conv.ID, jumpTo{ts})
}

// toChat posts an item to whichever window shows the conversation now:
// a result for a window that has since moved reaches its replacement.
func (h *hub) toChat(channelID string, item any) {
	h.mu.Lock()
	c := h.chats[channelID]
	h.mu.Unlock()
	if c != nil {
		c.box.post(item)
	}
}

// openThread opens a thread's window, or brings it here if it is open
// (see moveHere). jumpTS, when set, is a reply to scroll to and light.
func (h *hub) openThread(conv backend.Conversation, ts, jumpTS string) {
	k := threadKey{conv.ID, ts}
	h.mu.Lock()
	t, busy := h.threads[k], h.openingThreads[k]
	if t == nil && !busy {
		h.openingThreads[k] = true
	}
	connected := h.connected
	h.mu.Unlock()

	switch {
	case t != nil:
		t.box.post(moveHere{jumpTS: jumpTS})
	case busy:
		// A click a moment ago is already opening it.
	default:
		t = newThreadWindow(h, conv, ts, connected)
		h.mu.Lock()
		delete(h.openingThreads, k)
		h.threads[k] = t
		h.windows++
		h.mu.Unlock()
		go t.run()
		go t.load()
		go t.loadCommands()
		go t.loadCustomEmoji()
		if jumpTS != "" {
			t.box.post(jumpTo{jumpTS})
		}
	}
}

// adoptThread is adoptChat for a thread window.
func (h *hub) adoptThread(st threadState) {
	t := newThreadWindowFrom(h, st)
	h.mu.Lock()
	h.threads[t.key] = t
	h.windows++
	h.mu.Unlock()
	go t.run()
	go t.loadCustomEmoji()
}

// conversationLocked is the conversation by ID from the list, or for
// one the list does not hold (not loaded yet), its ID with a kind
// guessed from it. Callers hold mu.
func (h *hub) conversationLocked(channelID string) backend.Conversation {
	for _, c := range h.convs {
		if c.ID == channelID {
			return c
		}
	}
	conv := backend.Conversation{ID: channelID, Name: channelID, Kind: backend.KindChannel}
	if strings.HasPrefix(channelID, "D") {
		conv.Kind = backend.KindDM
	}
	return conv
}

// openNote opens what a clicked notification is about: its
// conversation at the message, and for a reply, its thread on top lit
// on the reply. The windows open on the current workspace.
func (h *hub) openNote(n notifications.Note) {
	h.mu.Lock()
	conv := h.conversationLocked(n.ChannelID)
	h.mu.Unlock()
	if n.ThreadTS != "" {
		h.openChatAt(conv, n.ThreadTS)
		h.openThread(conv, n.ThreadTS, n.TS)
		return
	}
	h.openChatAt(conv, n.TS)
}

// peopleFresh is how long the people list serves before it is read
// again: new people show up as messages arrive.
const peopleFresh = 5 * time.Minute

// askUnknownLocked asks Slack about a conversation a message came from
// that the list does not know (a DM someone just started, when the
// socket's "opened" event did not say so), once. h.mu is held.
func (h *hub) askUnknownLocked(channelID string) {
	if channelID == "" || len(h.convs) == 0 || h.askedConv[channelID] {
		return
	}
	for _, c := range h.convs {
		if c.ID == channelID {
			return
		}
	}
	if h.askedConv == nil {
		h.askedConv = map[string]bool{}
	}
	h.askedConv[channelID] = true
	go func() {
		if _, err := h.be.Conversation(h.ctx, channelID); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: new conversation", channelID+":", err)
		}
	}()
}

// upsertConv is convs with c in it: replacing the one with its ID, or
// added. convs is not changed (it may be shared).
func upsertConv(convs []backend.Conversation, c backend.Conversation) []backend.Conversation {
	out := slices.Clone(convs)
	for i := range out {
		if out[i].ID == c.ID {
			// Keep what the list knows of its activity and read state.
			c.Unread, c.Mentions = out[i].Unread || c.Unread, max(out[i].Mentions, c.Mentions)
			if out[i].Latest.After(c.Latest) {
				c.Latest = out[i].Latest
			}
			if out[i].Visited.After(c.Visited) {
				c.Visited = out[i].Visited
			}
			out[i] = c
			return out
		}
	}
	return append(out, c)
}

// seedGroups takes the user's own usergroups from what the backend
// already knows (its cache), before any UserGroupsChanged: one only
// comes when the groups change.
func (h *hub) seedGroups() {
	var mine []string
	h.mu.Lock()
	h.myGroups = map[string]bool{}
	for _, g := range h.be.UserGroups() {
		if g.Mine {
			h.myGroups[g.ID] = true
			mine = append(mine, g.ID)
		}
	}
	notes := h.notes
	h.mu.Unlock()
	if notes != nil {
		notes.SetGroups(mine)
	}
}

// isMyGroup reports whether a usergroup is one of the user's.
func (h *hub) isMyGroup(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.myGroups[id]
}

// people is who @ completes to: the backend's list, read in the
// background the first time and again once it is peopleFresh old. Until
// the first read lands it is empty.
func (h *hub) people() []backend.Person {
	h.peopleMu.Lock()
	defer h.peopleMu.Unlock()
	if !h.peopleLoading && time.Since(h.peopleAt) > peopleFresh {
		h.peopleLoading = true
		go func() {
			list, err := h.be.People(h.ctx)
			if err != nil {
				fmt.Fprintln(os.Stderr, "slk-gio: people:", err)
			}
			h.peopleMu.Lock()
			defer h.peopleMu.Unlock()
			h.peopleLoading = false
			h.peopleAt = time.Now()
			if err == nil {
				h.peopleList = list
			}
		}()
	}
	return h.peopleList
}
