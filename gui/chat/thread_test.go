package chat

import (
	"slices"
	"testing"

	"github.com/gammons/slk/gui/backend"
)

const root = "1700000100.000100"

func reply(ts, text string) backend.Message {
	m := msg(ts, text)
	m.ThreadTS, m.InThreadOnly = root, true
	return m
}

func threadTexts(th *Thread) []string {
	var out []string
	for _, m := range th.Messages() {
		out = append(out, text(m))
	}
	return out
}

func newThread() *Thread { return &Thread{ChannelID: "C1", TS: root} }

func TestThreadMergeKeepsOnlyItsRows(t *testing.T) {
	th := newThread()
	parent := msg(root, "parent")
	parent.ThreadTS, parent.ReplyCount = root, 2
	other := msg("1700000120.000100", "another thread's reply")
	other.ThreadTS, other.InThreadOnly = "1700000050.000100", true
	elsewhere := reply("1700000130.000100", "same ts, other channel")
	elsewhere.ChannelID = "C2"
	th.Merge([]backend.Message{
		parent, msg("1700000110.000100", "plain channel message"), other, elsewhere,
		reply("1700000140.000100", "r1"), reply("1700000150.000100", "r2"),
	}, nil)
	if got, want := threadTexts(th), []string{"parent", "r1", "r2"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if p, ok := th.Parent(); !ok || p.ReplyCount != 2 {
		t.Fatalf("parent %+v, %v", p, ok)
	}
	if got := len(th.Replies()); got != 2 {
		t.Fatalf("%d replies, want 2", got)
	}
}

func TestThreadLiveReplyBeforeHistoryIsKept(t *testing.T) {
	th := newThread()
	th.Apply(backend.MessagePosted{Message: reply("1700000300.000100", "live")})
	th.Merge([]backend.Message{msg(root, "parent"), reply("1700000200.000100", "r1")}, nil)
	if got, want := threadTexts(th), []string{"parent", "r1", "live"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestThreadReplyCountsOnceAndEditsKeepCount(t *testing.T) {
	th := newThread()
	parent := msg(root, "parent")
	parent.ReplyCount = 1
	th.Merge([]backend.Message{parent, reply("1700000200.000100", "r1")}, nil)

	// The copy SendReply queues, then the socket's echo: one reply.
	r := reply("1700000300.000100", "mine")
	th.Apply(backend.MessagePosted{Message: r})
	th.Apply(backend.MessagePosted{Message: r})
	// A reply the fetch already brought is not counted again live.
	th.Apply(backend.MessagePosted{Message: reply("1700000200.000100", "r1")})
	edit := msg(root, "parent, edited")
	edit.Edited = true
	th.Apply(backend.MessagePosted{Message: edit})

	p, _ := th.Parent()
	if text(p) != "parent, edited" || p.ReplyCount != 2 {
		t.Fatalf("parent %q with %d replies, want the edit with 2", text(p), p.ReplyCount)
	}
	if got := len(th.Replies()); got != 2 {
		t.Fatalf("%d replies shown, want 2", got)
	}
}

func TestThreadShowsEphemeralAndBroadcastReplies(t *testing.T) {
	th := newThread()
	th.Merge([]backend.Message{msg(root, "parent")}, nil)
	eph := reply("1700000200.000100", "only you")
	eph.Ephemeral = true
	cast := reply("1700000300.000100", "also sent to the channel")
	cast.InThreadOnly = false
	th.Apply(backend.MessagePosted{Message: eph})
	th.Apply(backend.MessagePosted{Message: cast})
	if got, want := threadTexts(th), []string{"parent", "only you", "also sent to the channel"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if p, _ := th.Parent(); p.ReplyCount != 1 {
		t.Fatalf("reply count %d, want 1 (the ephemeral does not count)", p.ReplyCount)
	}
}

func TestThreadDeletes(t *testing.T) {
	th := newThread()
	th.Merge([]backend.Message{msg(root, "parent"), reply("1700000200.000100", "r1"), reply("1700000300.000100", "r2")}, nil)
	th.Merge(nil, []string{"1700000200.000100"})
	if !th.Apply(backend.MessageDeleted{ChannelID: "C1", TS: root}) {
		t.Fatal("deleting the parent reported no change")
	}
	if th.Apply(backend.MessageDeleted{ChannelID: "C2", TS: "1700000300.000100"}) {
		t.Fatal("a delete in C2 changed the thread")
	}
	if got, want := threadTexts(th), []string{"r2"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if _, ok := th.Parent(); ok {
		t.Fatal("parent still shown")
	}
	if got := len(th.Replies()); got != 1 {
		t.Fatalf("%d replies, want 1", got)
	}
}
