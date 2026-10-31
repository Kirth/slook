package chat

import "github.com/gammons/slk/gui/backend"

// ReadGate keeps a window from marking its conversation (or thread)
// read again right after the user marked part of it unread. A window
// with the keyboard marks what it shows read; without the gate, "Mark
// unread from here" would be undone on the next message or focus
// change. Hold closes it at the newest message shown; it opens when the
// window loses the keyboard (Blur: the user went elsewhere and coming
// back is reading it again) or when a message newer than that arrives
// (Arrived: there is something new to read, so reading resumes).
//
// The zero ReadGate is open.
type ReadGate struct {
	held  bool
	since string // the newest message when it was held
}

// Hold closes the gate; newest is the newest message the window shows.
func (g *ReadGate) Hold(newest string) { g.held, g.since = true, newest }

// Blur opens the gate: the window lost the keyboard.
func (g *ReadGate) Blur() { *g = ReadGate{} }

// Arrived opens the gate when ts is newer than the message it was held
// at. Callers pass only messages the window would mark read (not
// ephemerals, not replies another view shows).
func (g *ReadGate) Arrived(ts string) {
	if g.held && Later(ts, g.since) {
		*g = ReadGate{}
	}
}

// Open reports whether the window may mark read.
func (g ReadGate) Open() bool { return !g.held }

// UnreadBoundary is where "mark unread from ts" puts a conversation's
// read mark: the message before ts that Slack keeps (ephemerals are
// not in its history), or "0" when ts is the oldest, which makes all
// of it unread. It is false when ts is not among msgs or is itself an
// ephemeral. msgs are oldest first.
func UnreadBoundary(msgs []backend.Message, ts string) (string, bool) {
	at := -1
	for i, m := range msgs {
		if m.TS == ts {
			at = i
			break
		}
	}
	if at < 0 || msgs[at].Ephemeral {
		return "", false
	}
	for i := at - 1; i >= 0; i-- {
		if !msgs[i].Ephemeral {
			return msgs[i].TS, true
		}
	}
	return "0", true
}

// ThreadUnreadBoundary is UnreadBoundary for a thread's replies: the
// reply before ts, or for the first reply the parent's ts (the whole
// thread unread). The parent itself is not a reply, so it is false, as
// for a ts not among replies or an ephemeral one.
func ThreadUnreadBoundary(parentTS string, replies []backend.Message, ts string) (string, bool) {
	if ts == parentTS {
		return "", false
	}
	b, ok := UnreadBoundary(replies, ts)
	if b == "0" {
		b = parentTS
	}
	return b, ok
}
