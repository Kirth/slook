package chat

import (
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
)

func TestThreadMetaAndUnread(t *testing.T) {
	now := time.Date(2026, 10, 30, 15, 0, 0, 0, time.Local)
	at := time.Date(2026, 10, 30, 14, 5, 0, 0, time.Local)
	for _, c := range []struct {
		t    backend.ThreadSummary
		want string
	}{
		{backend.ThreadSummary{Replies: 3, LastReply: at}, "3 replies · Last reply 14:05"},
		{backend.ThreadSummary{Replies: 1, LastReply: at}, "1 reply · Last reply 14:05"},
		{backend.ThreadSummary{}, "No replies"},
	} {
		if got := ThreadMeta(c.t, now); got != c.want {
			t.Errorf("ThreadMeta(%+v) = %q, want %q", c.t, got, c.want)
		}
	}
	if n := UnreadThreads([]backend.ThreadSummary{{Unread: true}, {}, {Unread: true}}); n != 2 {
		t.Fatalf("UnreadThreads = %d", n)
	}
}
