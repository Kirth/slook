package main

import (
	"context"
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
)

// markRecorder is the fake with its read marks recorded instead of
// made.
type markRecorder struct {
	backend.Backend
	marks chan string
}

func (r markRecorder) MarkRead(_ context.Context, channelID, ts string) error {
	r.marks <- "read " + channelID + " " + ts
	return nil
}

func (r markRecorder) MarkThreadRead(_ context.Context, channelID, threadTS, ts string) error {
	r.marks <- "read " + channelID + "/" + threadTS + " " + ts
	return nil
}

func (r markRecorder) MarkUnread(_ context.Context, channelID, threadTS, boundaryTS string) error {
	r.marks <- "unread " + channelID + "/" + threadTS + " " + boundaryTS
	return nil
}

func gateTestHub(t *testing.T) (*hub, *backend.Fake, chan string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f := backend.NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	rec := markRecorder{Backend: f, marks: make(chan string, 16)}
	return newHub(ctx, rec, func(error) {}), f, rec.marks
}

// wantMark reads the next mark the window made; "" expects none.
func wantMark(t *testing.T, marks chan string, want string) {
	t.Helper()
	wait := 2 * time.Second
	if want == "" {
		wait = 100 * time.Millisecond
	}
	select {
	case got := <-marks:
		if got != want {
			t.Fatalf("marked %q, want %q", got, want)
		}
	case <-time.After(wait):
		if want != "" {
			t.Fatalf("no mark, want %q", want)
		}
	}
}

func hasMenuItem(items []menuItem, label string) bool {
	for _, it := range items {
		if it.label == label {
			return true
		}
	}
	return false
}

func TestChatWindowKeepsAMarkUnreadUntilBlurOrNewMessage(t *testing.T) {
	h, f, marks := gateTestHub(t)
	msgs, err := f.History(h.ctx, "C1")
	if err != nil {
		t.Fatal(err)
	}
	c := newChatWindow(h, backend.Conversation{ID: "C1", Name: "general", Kind: backend.KindChannel}, true)
	c.box.post(historyLoaded{msgs: msgs})
	c.apply()
	wantMark(t, marks, "") // not focused: nothing read
	newest := msgs[len(msgs)-1].TS
	c.focusChanged(true)
	wantMark(t, marks, "read C1 "+newest)

	target := msgs[len(msgs)-3]
	if !hasMenuItem(c.menuItems(target), "Mark unread from here") {
		t.Fatal("the menu has no Mark unread")
	}
	c.markUnread(target)()
	wantMark(t, marks, "unread C1/ "+msgs[len(msgs)-4].TS)

	// Focused, the window reads again, re-delivers its history and
	// hears an edit: none of it marks read.
	c.box.post(historyLoaded{msgs: msgs})
	edited := msgs[len(msgs)-1]
	edited.Edited = true
	c.box.post(backend.MessagePosted{Message: edited})
	c.apply()
	wantMark(t, marks, "")

	// Leaving and coming back reads it.
	c.focusChanged(false)
	c.focusChanged(true)
	wantMark(t, marks, "read C1 "+newest)

	// A newer message ends the hold too.
	c.markUnread(target)()
	wantMark(t, marks, "unread C1/ "+msgs[len(msgs)-4].TS)
	fresh := backend.Message{ChannelID: "C1", TS: "1800000000.000100", Author: "Ada Lovelace"}
	c.box.post(backend.MessagePosted{Message: fresh})
	c.apply()
	wantMark(t, marks, "read C1 "+fresh.TS)

	// An ephemeral is not offered.
	eph := backend.Message{ChannelID: "C1", TS: "1800000001.000100", Ephemeral: true}
	c.box.post(backend.MessagePosted{Message: eph})
	c.apply()
	if hasMenuItem(c.menuItems(eph), "Mark unread from here") {
		t.Fatal("an ephemeral offers Mark unread")
	}
}

func TestThreadWindowKeepsAMarkUnreadUntilBlurOrNewReply(t *testing.T) {
	h, f, marks := gateTestHub(t)
	threads, err := f.Threads(h.ctx)
	if err != nil || len(threads) == 0 {
		t.Fatal("no thread", err)
	}
	var parent string
	for _, th := range threads {
		if th.Replies > 1 {
			parent = th.ThreadTS
		}
	}
	msgs, err := f.Thread(h.ctx, "C1", parent)
	if err != nil {
		t.Fatal(err)
	}
	tw := newThreadWindow(h, backend.Conversation{ID: "C1", Name: "general", Kind: backend.KindChannel}, parent, true)
	tw.box.post(threadLoaded{msgs: msgs})
	tw.apply()
	replies := tw.thread.Replies()
	newest := replies[len(replies)-1].TS
	tw.focusChanged(true)
	wantMark(t, marks, "read C1/"+parent+" "+newest)

	p, _ := tw.thread.Parent()
	if hasMenuItem(tw.menuItems(p), "Mark unread from here") {
		t.Fatal("the parent offers Mark unread")
	}
	tw.markUnread(replies[0])()
	wantMark(t, marks, "unread C1/"+parent+" "+parent)
	tw.box.post(threadLoaded{msgs: msgs})
	tw.apply()
	wantMark(t, marks, "")

	reply := backend.Message{ChannelID: "C1", TS: "1800000000.000100", ThreadTS: parent, InThreadOnly: true, Author: "Ada Lovelace"}
	tw.box.post(backend.MessagePosted{Message: reply})
	tw.apply()
	wantMark(t, marks, "read C1/"+parent+" "+reply.TS)

	tw.markUnread(replies[1])()
	wantMark(t, marks, "unread C1/"+parent+" "+replies[0].TS)
	tw.focusChanged(false)
	wantMark(t, marks, "")
	tw.focusChanged(true)
	wantMark(t, marks, "read C1/"+parent+" "+reply.TS)
}
