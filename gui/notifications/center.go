// Package notifications is slk-gio's desktop notifications: which new
// messages notify (the TUI's rules, internal/notify, plus "no window
// has the keyboard"), what the notification says, and the sinks that
// show it: the desktop's notification service over D-Bus, which can
// open the conversation on a click, or notify_command / beeep.
package notifications

import (
	"slices"
	"strings"
	"sync"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/mention"
	"github.com/gammons/slk/internal/notify"
	"github.com/gammons/slk/internal/ui/messages"
)

// Note is one notification: what it says, and where clicking it leads.
type Note struct {
	ChannelID string
	TS        string
	ThreadTS  string // for a thread reply, the thread to open on a click
	Title     string
	Body      string
}

// Sink shows notes. Show replaces the conversation's previous note, so
// one conversation shows one notification; Clear takes it away.
type Sink interface {
	Show(n Note) error
	Clear(channelID string)
}

// bodyRunes is how long a notification's text may be.
const bodyRunes = 140

// recent is how many notified messages are remembered, so a message
// delivered twice notifies once.
const recent = 512

// Center decides which messages notify and hands them to a sink. It is
// safe to use from several goroutines.
type Center struct {
	cfg  config.Notifications
	self string
	sink Sink

	mu   sync.Mutex
	seen map[string]bool
	// groups is the IDs of the user's own usergroups: a mention of one
	// is a mention of the user.
	groups []string
	ring   []string
}

// New is a Center notifying through sink by cfg's rules, for the user
// self.
func New(cfg config.Notifications, self string, sink Sink) *Center {
	return &Center{cfg: cfg, self: self, sink: sink, seen: map[string]bool{}}
}

// Message considers a new message in conv. focused says whether any
// slk-gio window has the keyboard: then the user is here and nothing
// notifies. conv.Notify overrides the configured rules: NotifyNone
// silences the conversation, NotifyAll notifies every message, muted
// in Slack or not. It reports whether the message notified.
func (c *Center) Message(m backend.Message, conv backend.Conversation, focused bool) bool {
	if !c.cfg.Enabled || focused || m.Ephemeral || m.Edited || m.FromSelf || conv.Notify == backend.NotifyNone {
		return false
	}
	author := m.AuthorID
	if author == "" {
		author = m.Author
	}
	if conv.Notify == backend.NotifyAll {
		// The user asked for every message here, in slk: it holds even
		// while the conversation is muted in Slack.
		if author == c.self {
			return false
		}
		return c.show(m, conv)
	}
	ctx := notify.NotifyContext{
		CurrentUserID: c.self,
		OnMention:     c.cfg.OnMention,
		OnDM:          c.cfg.OnDM,
		OnKeyword:     c.cfg.OnKeyword,
		IsMuted:       conv.Muted,
	}
	if notify.ShouldNotify(ctx, m.ChannelID, author, m.Text, string(kind(conv, m.ChannelID))) {
		return c.show(m, conv)
	}
	// A mention of one of the user's usergroups is a mention of them,
	// as in Slack; ShouldNotify cannot know their groups.
	if c.cfg.OnMention && !conv.Muted && author != c.self && mention.InGroups(m.Text, c.myGroups()) {
		return c.show(m, conv)
	}
	return false
}

// SetGroups records the user's own usergroups (their IDs).
func (c *Center) SetGroups(ids []string) {
	c.mu.Lock()
	c.groups = slices.Clone(ids)
	c.mu.Unlock()
}

func (c *Center) myGroups() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.groups
}

// show notifies of m, once.
func (c *Center) show(m backend.Message, conv backend.Conversation) bool {
	if !c.remember(m.ChannelID + "\x00" + m.TS) {
		return false
	}
	n := Note{ChannelID: m.ChannelID, TS: m.TS, Title: Title(m, conv), Body: Body(m)}
	if m.ThreadTS != "" && m.ThreadTS != m.TS {
		n.ThreadTS = m.ThreadTS
	}
	return c.sink.Show(n) == nil
}

// Read takes the conversation's notification away: it has been read.
func (c *Center) Read(channelID string) { c.sink.Clear(channelID) }

// remember records key, reporting false when it was already there.
func (c *Center) remember(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen[key] {
		return false
	}
	c.seen[key] = true
	c.ring = append(c.ring, key)
	if len(c.ring) > recent {
		delete(c.seen, c.ring[0])
		c.ring = c.ring[1:]
	}
	return true
}

// kind is conv's kind, or for a conversation the list has not loaded,
// a guess from its ID: D… is a direct message.
func kind(conv backend.Conversation, channelID string) backend.Kind {
	switch {
	case conv.Kind != "":
		return conv.Kind
	case strings.HasPrefix(channelID, "D"):
		return backend.KindDM
	}
	return backend.KindChannel
}

// Title is the notification's heading: the author for a direct
// message, "Author in #channel" or "Author in Group" otherwise, with
// "(thread)" for a reply in a thread.
func Title(m backend.Message, conv backend.Conversation) string {
	t := m.Author
	switch k := kind(conv, m.ChannelID); {
	case k == backend.KindDM:
	case conv.Name == "":
	case k == backend.KindGroupDM:
		t += " in " + conv.Name
	default:
		t += " in " + messages.ChannelGlyph(string(k)) + conv.Name
	}
	if m.ThreadTS != "" && m.ThreadTS != m.TS {
		t += " (thread)"
	}
	return t
}

// Body is the message as one line of plain text, names resolved, cut
// to bodyRunes.
func Body(m backend.Message) string {
	s := strings.Join(strings.Fields(rich.PlainBlocks(m.Body)), " ")
	if r := []rune(s); len(r) > bodyRunes {
		s = string(r[:bodyRunes-1]) + "…"
	}
	return s
}
