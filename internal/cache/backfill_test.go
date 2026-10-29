package cache

import "testing"

func TestThreadsNeedingReplies(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	// Fixed-width ts, as Slack emits them: the query orders as strings.
	full := feedMsg("1700000100.000000", "complete thread")
	full.ThreadTS, full.ReplyCount = "1700000100.000000", 1
	fullReply := feedMsg("1700000110.000000", "its reply")
	fullReply.ThreadTS = "1700000100.000000"
	partial := feedMsg("1700000200.000000", "under-cached thread")
	partial.ThreadTS, partial.ReplyCount = "1700000200.000000", 3
	partialReply := feedMsg("1700000210.000000", "one of three")
	partialReply.ThreadTS = "1700000200.000000"
	old := feedMsg("1700000050.000000", "old thread")
	old.ThreadTS, old.ReplyCount = "1700000050.000000", 2
	plain := feedMsg("1700000300.000000", "no thread")
	other := Message{TS: "1700000400.000000", ChannelID: "C2", WorkspaceID: "T2", ThreadTS: "1700000400.000000", ReplyCount: 1, CreatedAt: 1000}
	if err := db.UpsertMessages([]Message{full, fullReply, partial, partialReply, old, plain, other}); err != nil {
		t.Fatal(err)
	}

	got, err := db.ThreadsNeedingReplies("T1", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (ThreadRef{"C1", "1700000200.000000"}) || got[1] != (ThreadRef{"C1", "1700000050.000000"}) {
		t.Fatalf("got %+v, want the partial thread then the old one", got)
	}
	if got, _ = db.ThreadsNeedingReplies("T1", "1700000100.000000", 10); len(got) != 1 || got[0].TS != "1700000200.000000" {
		t.Fatalf("with a lower bound: %+v, want only 200.0", got)
	}
	if got, _ = db.ThreadsNeedingReplies("T1", "", 1); len(got) != 1 {
		t.Fatalf("limit ignored: %+v", got)
	}
}
