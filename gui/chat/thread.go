package chat

import "github.com/gammons/slk/gui/backend"

// Thread is one thread's messages, oldest first: its parent and every
// reply, broadcast or not, including replies only the user can see.
// Like a Timeline it takes the history and live messages in either
// order.
type Thread struct {
	ChannelID string
	TS        string // the parent's ts

	msgs    sortedMsgs
	counted map[string]bool // replies counted on the parent, as Timeline
}

// Messages returns the messages to draw, the parent first when it is
// known. The caller must not modify it.
func (t *Thread) Messages() []backend.Message { return t.msgs }

// Parent is the thread's parent message, when it is shown.
func (t *Thread) Parent() (backend.Message, bool) {
	if i, ok := t.msgs.find(t.TS); ok {
		return t.msgs[i], true
	}
	return backend.Message{}, false
}

// Replies is the replies shown, without the parent; ephemerals included.
func (t *Thread) Replies() []backend.Message {
	out := t.msgs
	if len(out) > 0 && out[0].TS == t.TS {
		out = out[1:]
	}
	return out
}

// Index is the position of the message with ts in Messages, if shown.
func (t *Thread) Index(ts string) (int, bool) { return t.msgs.find(ts) }

// in reports whether m belongs to this thread.
func (t *Thread) in(m backend.Message) bool {
	return m.ChannelID == t.ChannelID && (m.TS == t.TS || m.ThreadTS == t.TS)
}

// Merge folds the cache's (or a fetch's) state into the thread: msgs
// of this thread replace what is shown by ts, the rest are ignored, and
// deleted names rows to drop.
func (t *Thread) Merge(msgs []backend.Message, deleted []string) {
	for _, m := range msgs {
		if !t.in(m) {
			continue
		}
		t.msgs.upsert(m)
		if m.TS != t.TS {
			// The fetched parent's count already includes it.
			t.markCounted(m.TS)
		}
	}
	for _, ts := range deleted {
		t.msgs.remove(ts)
	}
}

// Apply folds a live event into the thread and reports whether it
// changed.
func (t *Thread) Apply(ev backend.Event) bool {
	switch ev := ev.(type) {
	case backend.MessagePosted:
		m := ev.Message
		if !t.in(m) {
			return false
		}
		if i, ok := t.msgs.find(m.TS); ok {
			if m.TS == t.TS {
				// An edit carries no reply count; keep the one we have.
				m.ReplyCount = t.msgs[i].ReplyCount
			}
			// Reactions arrive as their own events.
			m.Reactions = t.msgs[i].Reactions
			t.msgs[i] = m
			return true
		}
		t.msgs.insert(m)
		if m.TS != t.TS && !m.Ephemeral && !m.Edited && !t.counted[m.TS] {
			t.markCounted(m.TS)
			if i, ok := t.msgs.find(t.TS); ok {
				t.msgs[i].ReplyCount++
			}
		}
		return true
	case backend.MessageDeleted:
		if ev.ChannelID != t.ChannelID {
			return false
		}
		return t.msgs.remove(ev.TS)
	case backend.ReactionChanged:
		if ev.ChannelID != t.ChannelID {
			return false
		}
		return t.msgs.react(ev)
	}
	return false
}

func (t *Thread) markCounted(ts string) {
	if t.counted == nil {
		t.counted = map[string]bool{}
	}
	t.counted[ts] = true
}
