package backend

import (
	"context"
	"testing"
	"time"
)

// fakeThreadParent is C1's seeded thread with plain replies.
func fakeThreadParent(t *testing.T, f *Fake) Message {
	t.Helper()
	hist, err := f.History(context.Background(), "C1")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range hist {
		if m.InThreadOnly {
			t.Fatalf("History returned a reply that lives only in its thread: %+v", m)
		}
	}
	for _, m := range hist {
		if m.ReplyCount == 3 {
			return m
		}
	}
	t.Fatal("no seeded thread with 3 replies in C1")
	return Message{}
}

func TestFakeThreadIsParentThenReplies(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	parent := fakeThreadParent(t, f)
	msgs, err := f.Thread(context.Background(), "C1", parent.TS)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 || msgs[0].TS != parent.TS {
		t.Fatalf("got %d messages starting %s, want the parent and 3 replies", len(msgs), msgs[0].TS)
	}
	for _, m := range msgs[1:] {
		if m.ThreadTS != parent.TS || !m.InThreadOnly {
			t.Fatalf("reply %+v is not in the thread", m)
		}
	}
}

func TestFakeSendReplyComesBackAndBroadcastsReachHistory(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Event, 8)
	go f.Run(ctx, func(ev Event) { got <- ev })
	<-got // connected
	parent := fakeThreadParent(t, f)

	if err := f.SendReply(ctx, "C1", parent.TS, "on it", false); err != nil {
		t.Fatal(err)
	}
	ev := nextPosted(t, got)
	if m := ev.Message; m.ThreadTS != parent.TS || !m.InThreadOnly || !m.FromSelf {
		t.Fatalf("got %+v, want the user's reply in the thread", m)
	}
	if err := f.SendReply(ctx, "C1", parent.TS, "everyone should see this", true); err != nil {
		t.Fatal(err)
	}
	cast := nextPosted(t, got).Message
	if cast.InThreadOnly {
		t.Fatal("a broadcast reply is only in its thread")
	}

	thread, _ := f.Thread(ctx, "C1", parent.TS)
	if len(thread) != 6 || thread[0].ReplyCount != 5 {
		t.Fatalf("thread has %d messages, parent counts %d; want 6 and 5", len(thread), thread[0].ReplyCount)
	}
	hist, _ := f.History(ctx, "C1")
	if last := hist[len(hist)-1]; last.TS != cast.TS {
		t.Fatalf("history ends at %s, want the broadcast %s", last.TS, cast.TS)
	}
}

func TestFakeCommandInThreadRepliesThere(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Event, 8)
	go f.Run(ctx, func(ev Event) { got <- ev })
	<-got // connected
	parent := fakeThreadParent(t, f)
	if err := f.RunCommand(ctx, "C1", parent.TS, "/remind", "me later"); err != nil {
		t.Fatal(err)
	}
	if m := nextPosted(t, got).Message; !m.Ephemeral || m.ThreadTS != parent.TS || !m.InThreadOnly {
		t.Fatalf("got %+v, want an ephemeral reply in the thread", m)
	}
}
