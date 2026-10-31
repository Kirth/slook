package chat

import (
	"strconv"
	"time"

	"github.com/gammons/slk/gui/backend"
)

// ThreadMeta is a followed thread's line under its preview: how many
// replies and when the last came ("3 replies · Last reply 14:05").
func ThreadMeta(t backend.ThreadSummary, now time.Time) string {
	n := "No replies"
	switch {
	case t.Replies == 1:
		n = "1 reply"
	case t.Replies > 1:
		n = strconv.Itoa(t.Replies) + " replies"
	}
	if t.LastReply.IsZero() {
		return n
	}
	return n + " · " + LastReply(t.LastReply, now)
}

// UnreadThreads counts the followed threads with unread replies.
func UnreadThreads(ts []backend.ThreadSummary) int {
	n := 0
	for _, t := range ts {
		if t.Unread {
			n++
		}
	}
	return n
}
