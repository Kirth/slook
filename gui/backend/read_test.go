package backend

import (
	"context"
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/slack/slacktest"
)

// TestMarkUnread_CountsMentionsAfterTheBoundary marks a channel unread
// through conversations.mark and reports the mentions of the user after
// the boundary, from the cache (never the boundary itself, never the
// user's own).
func TestMarkUnread_CountsMentionsAfterTheBoundary(t *testing.T) {
	bodies := bootBodies()
	bodies["/api/conversations.mark"] = `{"ok":true}`
	bodies["/api/subscriptions.thread.mark"] = `{"ok":true}`
	srv := slacktest.NewServer(t, bodies)
	s, _ := bootSlack(t, srv) // the user is U1
	for _, m := range []cache.Message{
		{TS: "1700000001.000100", UserID: "U2", Text: "hi <@U1>"}, // the boundary: stays read
		{TS: "1700000002.000100", UserID: "U2", Text: "<@U1> look"},
		{TS: "1700000003.000100", UserID: "U1", Text: "<!here> mine"},
		{TS: "1700000004.000100", UserID: "U2", Text: "no mention"},
		{TS: "1700000005.000100", UserID: "U2", Text: "<!channel> all"},
	} {
		m.ChannelID, m.WorkspaceID = "C1", "T1"
		if err := s.db.UpsertMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if err := s.MarkUnread(ctx, "C1", "", "1700000001.000100"); err != nil {
		t.Fatal(err)
	}
	form := srv.RequestTo(t, "/api/conversations.mark").Form
	if form.Get("channel") != "C1" || form.Get("ts") != "1700000001.000100" {
		t.Fatalf("conversations.mark %v", form)
	}
	got := (<-s.queue).(rawMarked)
	if got.channelID != "C1" || got.ts != "1700000001.000100" || got.unread == 0 || got.mentions != 2 {
		t.Fatalf("queued %#v, want C1 unread with 2 mentions", got)
	}

	if err := s.MarkUnread(ctx, "C1", "1700000001.000100", "1700000001.000100"); err != nil {
		t.Fatal(err)
	}
	form = srv.RequestTo(t, "/api/subscriptions.thread.mark").Form
	if form.Get("thread_ts") != "1700000001.000100" || form.Get("read") != "0" {
		t.Fatalf("subscriptions.thread.mark %v", form)
	}
	if _, ok := (<-s.queue).(rawThreadUnread); !ok {
		t.Fatal("the thread's unread state was not queued")
	}
}

// TestThreadMarkedEchoOfAnUnreadStaysUnread applies thread_marked with
// a read mark before the newest reply (another client's mark unread, or
// the echo of this one's) as unread.
func TestThreadMarkedEchoOfAnUnreadStaysUnread(t *testing.T) {
	srv := slacktest.NewServer(t, bootBodies())
	s, _ := bootSlack(t, srv)
	parent := "1700000001.000100"
	for _, m := range []cache.Message{
		{TS: parent, UserID: "U2", Text: "q", ThreadTS: parent, ReplyCount: 2},
		{TS: "1700000002.000100", UserID: "U2", Text: "a", ThreadTS: parent},
		{TS: "1700000003.000100", UserID: "U1", Text: "mine", ThreadTS: parent},
	} {
		m.ChannelID, m.WorkspaceID = "C1", "T1"
		if err := s.db.UpsertMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	var states []ThreadState
	emit := func(ev Event) {
		if st, ok := ev.(ThreadState); ok {
			states = append(states, st)
		}
	}
	s.threadMarked(rawThreadMarked{"C1", parent, parent}, emit)
	s.threadMarked(rawThreadMarked{"C1", parent, "1700000002.000100"}, emit) // only the user's own after
	if len(states) != 2 || !states[0].Unread || states[1].Unread {
		t.Fatalf("states %+v, want unread then read", states)
	}
}
