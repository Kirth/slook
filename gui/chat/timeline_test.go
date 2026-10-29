package chat

import (
	"slices"
	"testing"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/rich"
)

func msg(ts, text string) backend.Message {
	return backend.Message{ChannelID: "C1", TS: ts, Body: []rich.Block{rich.Paragraph{Spans: []rich.Span{{Text: text}}}}}
}

// text is a message's body as plain text.
func text(m backend.Message) string {
	var out string
	for _, b := range m.Body {
		if p, ok := b.(rich.Paragraph); ok {
			out += rich.Plain(p.Spans)
		}
	}
	return out
}

func texts(t *Timeline) []string {
	var out []string
	for _, m := range t.Messages() {
		out = append(out, text(m))
	}
	return out
}

func TestLiveMessageBeforeHistoryIsKept(t *testing.T) {
	var tl Timeline
	tl.Apply("C1", backend.MessagePosted{Message: msg("1700000300.000100", "live")})
	tl.Merge([]backend.Message{msg("1700000100.000100", "a"), msg("1700000200.000100", "b")}, nil)
	if got, want := texts(&tl), []string{"a", "b", "live"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMergeReplacesByTSDeletesAndKeepsEphemerals(t *testing.T) {
	var tl Timeline
	tl.Merge([]backend.Message{msg("1700000100.000100", "a"), msg("1700000200.000100", "b"), msg("1700000300.000100", "c")}, nil)
	eph := msg("1700000250.000100", "only you")
	eph.Ephemeral = true
	tl.Apply("C1", backend.MessagePosted{Message: eph})
	reply := msg("1700000150.000100", "in the thread")
	reply.ThreadTS, reply.InThreadOnly = "1700000100.000100", true
	tl.Merge([]backend.Message{msg("1700000200.000100", "b, edited"), reply}, []string{"1700000300.000100", "1700000999.000100"})
	if got, want := texts(&tl), []string{"a", "b, edited", "only you"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEditKeepsPositionAndReplyCount(t *testing.T) {
	var tl Timeline
	parent := msg("1700000100.000100", "parent")
	parent.ReplyCount = 2
	tl.Merge([]backend.Message{parent, msg("1700000200.000100", "next")}, nil)
	edit := msg("1700000100.000100", "parent, edited")
	edit.Edited = true
	if !tl.Apply("C1", backend.MessagePosted{Message: edit}) {
		t.Fatal("edit reported no change")
	}
	got := tl.Messages()
	if text(got[0]) != "parent, edited" || got[0].ReplyCount != 2 || text(got[1]) != "next" {
		t.Fatalf("got %+v", got)
	}
}

func TestThreadOnlyReplyCountsOnParent(t *testing.T) {
	var tl Timeline
	tl.Merge([]backend.Message{msg("1700000100.000100", "parent")}, nil)
	reply := msg("1700000150.000100", "reply")
	reply.ThreadTS, reply.InThreadOnly = "1700000100.000100", true
	tl.Apply("C1", backend.MessagePosted{Message: reply})
	reply.Edited = true
	tl.Apply("C1", backend.MessagePosted{Message: reply}) // an edit must not count again
	got := tl.Messages()
	if len(got) != 1 || got[0].ReplyCount != 1 {
		t.Fatalf("got %+v, want only the parent with 1 reply", got)
	}
}

func TestBroadcastReplyIsShown(t *testing.T) {
	var tl Timeline
	reply := msg("1700000150.000100", "also sent to channel")
	reply.ThreadTS = "1700000100.000100"
	tl.Apply("C1", backend.MessagePosted{Message: reply})
	if got := texts(&tl); !slices.Equal(got, []string{"also sent to channel"}) {
		t.Fatalf("got %v", got)
	}
}

func TestDeleteAndOtherChannelsIgnored(t *testing.T) {
	var tl Timeline
	tl.Merge([]backend.Message{msg("1700000100.000100", "a"), msg("1700000200.000100", "b")}, nil)
	other := msg("1700000300.000100", "elsewhere")
	other.ChannelID = "C2"
	if tl.Apply("C1", backend.MessagePosted{Message: other}) {
		t.Fatal("a message for C2 changed C1's timeline")
	}
	tl.Apply("C1", backend.MessageDeleted{ChannelID: "C1", TS: "1700000100.000100"})
	if got := texts(&tl); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("got %v, want [b]", got)
	}
}

func TestCompareTSOrdersBySecondsNumerically(t *testing.T) {
	if compareTS("999999999.000001", "1000000000.000001") >= 0 {
		t.Fatal("a 9-digit second sorted after a 10-digit one")
	}
	if compareTS("1700000000.000002", "1700000000.000010") >= 0 {
		t.Fatal("fractions out of order")
	}
}

func TestReplyDeliveredTwiceCountsOnce(t *testing.T) {
	var tl Timeline
	tl.Merge([]backend.Message{msg("1700000100.000100", "parent")}, nil)
	reply := msg("1700000150.000100", "mine")
	reply.ThreadTS, reply.InThreadOnly = "1700000100.000100", true
	tl.Apply("C1", backend.MessagePosted{Message: reply}) // the copy SendReply queues
	tl.Apply("C1", backend.MessagePosted{Message: reply}) // the socket's echo
	if got := tl.Messages()[0].ReplyCount; got != 1 {
		t.Fatalf("reply count %d, want 1", got)
	}
}

func TestBroadcastReplyCountsOnParent(t *testing.T) {
	var tl Timeline
	tl.Merge([]backend.Message{msg("1700000100.000100", "parent")}, nil)
	reply := msg("1700000150.000100", "also sent to channel")
	reply.ThreadTS = "1700000100.000100"
	tl.Apply("C1", backend.MessagePosted{Message: reply})
	tl.Apply("C1", backend.MessagePosted{Message: reply})
	got := tl.Messages()
	if len(got) != 2 || got[0].ReplyCount != 1 {
		t.Fatalf("got %+v, want the parent with 1 reply and the broadcast", got)
	}
}

func TestRefreshUpdatesShownRowsOnly(t *testing.T) {
	var tl Timeline
	tl.Merge([]backend.Message{msg("1700000500.000100", "shown")}, nil)
	old := msg("1700000100.000100", "an old parent, not loaded")
	old.ReplyCount = 4
	shown := msg("1700000500.000100", "shown, refreshed")
	tl.Refresh([]backend.Message{old, shown}, nil)
	if got, want := texts(&tl), []string{"shown, refreshed"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFetchedReplyIsNotCountedAgainLive(t *testing.T) {
	var tl Timeline
	parent := msg("1700000100.000100", "parent")
	parent.ReplyCount = 1 // the cache counted the reply below
	reply := msg("1700000150.000100", "reply")
	reply.ThreadTS, reply.InThreadOnly = "1700000100.000100", true
	tl.Merge([]backend.Message{parent, reply}, nil)
	tl.Apply("C1", backend.MessagePosted{Message: reply}) // the socket, late
	if got := tl.Messages()[0].ReplyCount; got != 1 {
		t.Fatalf("reply count %d, want 1", got)
	}
}
