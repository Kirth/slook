// Package chat holds a chat window's state apart from drawing it, so it
// can be tested without a display.
package chat

import (
	"cmp"
	"slices"
	"strings"

	"github.com/gammons/slk/gui/backend"
)

// Timeline is one conversation's channel-level messages, oldest first.
// A window opens before its history arrives, so live messages and the
// history can land in either order; both go through the same upsert.
type Timeline struct {
	msgs sortedMsgs
	// counted is the replies already counted on their parent, so a
	// reply delivered twice (the copy Send queues, then the socket's
	// echo of it) counts once.
	counted map[string]bool
}

// Messages returns the messages to draw. The caller must not modify it.
func (t *Timeline) Messages() []backend.Message { return t.msgs }

// Merge folds the cache's state into the timeline: msgs replace what
// is shown by ts (the cache has every live change applied, so its row
// is at least as new as the window's), and deleted names rows to drop.
// Replies that live only in their thread are skipped: their parent's
// row carries the count. Ephemerals are never in the cache, so neither
// list names one.
func (t *Timeline) Merge(msgs []backend.Message, deleted []string) {
	t.merge(msgs, deleted, true)
}

// Refresh is Merge for one thread's fetch: it replaces the rows shown
// and drops deleted ones, but adds none, since the thread's parent may
// be far older than what the timeline has loaded.
func (t *Timeline) Refresh(msgs []backend.Message, deleted []string) {
	t.merge(msgs, deleted, false)
}

func (t *Timeline) merge(msgs []backend.Message, deleted []string, add bool) {
	for _, m := range msgs {
		if m.InThreadOnly {
			// The cache's parent row already counts it; a live copy
			// arriving later must not count it again.
			t.markCounted(m.TS)
			continue
		}
		if _, ok := t.msgs.find(m.TS); ok || add {
			t.msgs.upsert(m)
		}
	}
	for _, ts := range deleted {
		t.msgs.remove(ts)
	}
}

func (t *Timeline) markCounted(ts string) {
	if t.counted == nil {
		t.counted = map[string]bool{}
	}
	t.counted[ts] = true
}

// Apply folds a live event into the timeline and reports whether it
// changed. Events for other conversations are ignored.
func (t *Timeline) Apply(channelID string, ev backend.Event) bool {
	switch ev := ev.(type) {
	case backend.MessagePosted:
		m := ev.Message
		if m.ChannelID != channelID {
			return false
		}
		if m.InThreadOnly {
			// A new reply counts on its parent; an edited one, or one
			// only the user sees (a command's reply in a thread),
			// changes nothing the channel shows.
			if m.Edited || m.Ephemeral {
				return false
			}
			return t.count(m)
		}
		if i, ok := t.msgs.find(m.TS); ok {
			// An edit carries no reply count, nor (reliably) reactions,
			// which arrive as their own events; keep the ones we have.
			m.ReplyCount, m.Reactions = t.msgs[i].ReplyCount, t.msgs[i].Reactions
			t.msgs[i] = m
			return true
		}
		t.msgs.insert(m)
		if isReply(m) && !m.Ephemeral && !m.Edited {
			// A broadcast reply is in the channel and in its thread.
			t.count(m)
		}
		return true
	case backend.MessageDeleted:
		if ev.ChannelID != channelID {
			return false
		}
		return t.msgs.remove(ev.TS)
	case backend.ReactionChanged:
		if ev.ChannelID != channelID {
			return false
		}
		return t.msgs.react(ev)
	}
	return false
}

// count adds a new reply to its parent's count, once per reply.
func (t *Timeline) count(reply backend.Message) bool {
	if t.counted[reply.TS] {
		return false
	}
	i, ok := t.msgs.find(reply.ThreadTS)
	if !ok {
		return false
	}
	t.markCounted(reply.TS)
	t.msgs[i].ReplyCount++
	return true
}

// Index is the position of the message with ts, if shown.
func (t *Timeline) Index(ts string) (int, bool) { return t.msgs.find(ts) }

// isReply reports whether m is a reply in a thread rather than its
// parent or a plain message.
func isReply(m backend.Message) bool { return m.ThreadTS != "" && m.ThreadTS != m.TS }

// sortedMsgs is messages kept in ts order, the list a Timeline and a
// Thread both hold.
type sortedMsgs []backend.Message

func (s sortedMsgs) find(ts string) (int, bool) {
	return slices.BinarySearchFunc(s, ts, func(m backend.Message, ts string) int { return compareTS(m.TS, ts) })
}

func (s *sortedMsgs) insert(m backend.Message) {
	i, _ := s.find(m.TS)
	*s = slices.Insert(*s, i, m)
}

// upsert replaces the message with m's ts, or inserts m.
func (s *sortedMsgs) upsert(m backend.Message) {
	if i, ok := s.find(m.TS); ok {
		(*s)[i] = m
		return
	}
	s.insert(m)
}

// react applies a reaction change to the message it names, if shown,
// and reports whether anything changed.
func (s sortedMsgs) react(ev backend.ReactionChanged) bool {
	i, ok := s.find(ev.TS)
	if !ok {
		return false
	}
	before := s[i].Reactions
	after := ApplyReaction(before, ev.Name, ev.UserID, ev.Added, ev.Self)
	s[i].Reactions = after
	return len(before) != len(after) || !slices.EqualFunc(before, after, func(a, b backend.Reaction) bool {
		return a.Name == b.Name && a.Count == b.Count && a.Mine == b.Mine
	})
}

// remove drops the message with ts and reports whether there was one.
func (s *sortedMsgs) remove(ts string) bool {
	i, ok := s.find(ts)
	if ok {
		*s = slices.Delete(*s, i, i+1)
	}
	return ok
}

// Later reports whether Slack timestamp a is after b.
func Later(a, b string) bool { return compareTS(a, b) > 0 }

// compareTS orders Slack timestamps ("1700000000.000100"): by their
// seconds as numbers, then by the fraction.
func compareTS(a, b string) int {
	as, af, _ := strings.Cut(a, ".")
	bs, bf, _ := strings.Cut(b, ".")
	if c := cmp.Compare(len(as), len(bs)); c != 0 {
		return c
	}
	if c := strings.Compare(as, bs); c != 0 {
		return c
	}
	return strings.Compare(af, bf)
}
