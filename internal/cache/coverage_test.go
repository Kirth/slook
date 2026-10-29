package cache

import "testing"

func TestSearchMessages_WorkspaceWideWithReplies(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	reply := feedMsg("1700000150.000000", "the deploy went fine")
	reply.ThreadTS = "1700000100.000000"
	other := Message{TS: "1700000160.000000", ChannelID: "C2", WorkspaceID: "T1", Text: "deploy tomorrow", CreatedAt: 1}
	elsewhere := Message{TS: "1700000170.000000", ChannelID: "C9", WorkspaceID: "T2", Text: "deploy there", CreatedAt: 1}
	gone := feedMsg("1700000180.000000", "deploy, deleted")
	if err := db.UpsertMessages([]Message{feedMsg("1700000100.000000", "deploy?"), reply, other, elsewhere, gone, feedMsg("1700000200.000000", "lunch")}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteMessage("C1", "1700000180.000000"); err != nil {
		t.Fatal(err)
	}
	for _, fts := range []bool{true, false} {
		db.ftsDisabled = !fts
		got, err := db.SearchMessages("T1", "deploy", 10, SearchNewest)
		if err != nil {
			t.Fatal(err)
		}
		var ts []string
		for _, m := range got {
			ts = append(ts, m.TS)
		}
		want := []string{"1700000160.000000", "1700000150.000000", "1700000100.000000"}
		if len(ts) != 3 || ts[0] != want[0] || ts[1] != want[1] || ts[2] != want[2] {
			t.Fatalf("fts=%v: got %v, want %v (newest first, reply included, other workspace and deleted excluded)", fts, ts, want)
		}
		if got[1].ThreadTS != "1700000100.000000" || got[0].ChannelID != "C2" {
			t.Fatalf("fts=%v: rows lack their columns: %+v", fts, got)
		}
		if got, _ = db.SearchMessages("T1", "deploy", 2, SearchNewest); len(got) != 2 {
			t.Fatalf("fts=%v: limit ignored: %d rows", fts, len(got))
		}
		if got, _ = db.SearchMessages("T1", "   ", 2, SearchNewest); len(got) != 0 {
			t.Fatalf("fts=%v: blank query returned %d rows", fts, len(got))
		}
	}
}

func TestSearchMessagesRanksByRelevance(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	dense := feedMsg("1700000100.000000", "deploy deploy")
	sparse := feedMsg("1700000200.000000", "a long note about the week: lunch, the offsite, the plan, a deploy somewhere in it, and more")
	if err := db.UpsertMessages([]Message{dense, sparse, feedMsg("1700000300.000000", "lunch")}); err != nil {
		t.Fatal(err)
	}
	order := func(o SearchOrder) []string {
		t.Helper()
		got, err := db.SearchMessages("T1", "deploy", 10, o)
		if err != nil {
			t.Fatal(err)
		}
		var ts []string
		for _, m := range got {
			ts = append(ts, m.TS)
		}
		return ts
	}
	if got := order(SearchRelevance); len(got) != 2 || got[0] != dense.TS {
		t.Fatalf("relevance: %v, want the dense, older message first", got)
	}
	if got := order(SearchNewest); len(got) != 2 || got[0] != sparse.TS {
		t.Fatalf("newest: %v, want the newer message first", got)
	}
	db.ftsDisabled = true
	if got := order(SearchRelevance); len(got) != 2 || got[0] != sparse.TS {
		t.Fatalf("LIKE fallback: %v, want newest first (no rank)", got)
	}
}

func TestSpansForWorkspaceAndMessageCounts(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	for _, s := range []Span{
		{ChannelID: "C2", WorkspaceID: "T1", OldestTS: "5.0", LatestTS: "6.0", Live: true},
		{ChannelID: "C1", WorkspaceID: "T1", OldestTS: "1.0", LatestTS: "2.0", ReachesStart: true},
		{ChannelID: "C1", WorkspaceID: "T1", OldestTS: "4.0", LatestTS: "5.0"},
		{ChannelID: "C9", WorkspaceID: "T2", OldestTS: "1.0", LatestTS: "2.0"},
	} {
		if err := db.AddSpan(s); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.SpansForWorkspace("T1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ChannelID != "C1" || got[0].OldestTS != "1.0" || got[1].OldestTS != "4.0" || got[2].ChannelID != "C2" {
		t.Fatalf("spans = %+v, want C1 [1,2] [4,5] then C2", got)
	}

	reply := feedMsg("1700000150.000000", "reply")
	reply.ThreadTS = "1700000100.000000"
	rows := []Message{
		feedMsg("1700000100.000000", "a"), reply, feedMsg("1700000200.000000", "b"),
		{TS: "1700000300.000000", ChannelID: "C2", WorkspaceID: "T1", Text: "c", CreatedAt: 1},
		{TS: "1700000300.000000", ChannelID: "C9", WorkspaceID: "T2", Text: "d", CreatedAt: 1},
	}
	if err := db.UpsertMessages(rows); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteMessage("C1", "1700000200.000000"); err != nil {
		t.Fatal(err)
	}
	counts, err := db.MessageCounts("T1")
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 2 || counts["C1"] != 2 || counts["C2"] != 1 {
		t.Fatalf("counts = %v, want C1:2 (reply counted, deleted not) C2:1", counts)
	}
}
