package cache

import (
	"testing"
)

// Tests for the write paths a message cache fed by history pages and
// WebSocket events needs: the batch upsert, edits that touch only what
// an edit is about, reply counting, and range pruning.

func feedMsg(ts, text string) Message {
	return Message{TS: ts, ChannelID: "C1", WorkspaceID: "T1", UserID: "U1", Text: text, CreatedAt: 1000}
}

func TestUpsertMessages_WritesVersionAndEditedAtAndKeepsVersionWhenEmpty(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	m := feedMsg("100.0", "hello")
	m.Version = "v1"
	m.EditedAt = "150.0"
	if err := db.UpsertMessages([]Message{m, feedMsg("200.0", "world")}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetMessage("C1", "100.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "v1" || got.EditedAt != "150.0" {
		t.Fatalf("got version=%q edited_at=%q, want v1 / 150.0", got.Version, got.EditedAt)
	}

	// A socket delivery of the same message carries no version: the
	// stored one stays. A later page with a new version replaces it.
	unversioned := feedMsg("100.0", "hello again")
	if err := db.UpsertMessage(unversioned); err != nil {
		t.Fatal(err)
	}
	if got, _ = db.GetMessage("C1", "100.0"); got.Version != "v1" || got.Text != "hello again" {
		t.Fatalf("after unversioned upsert: version=%q text=%q", got.Version, got.Text)
	}
	m.Version = "v2"
	if err := db.UpsertMessages([]Message{m}); err != nil {
		t.Fatal(err)
	}
	if got, _ = db.GetMessage("C1", "100.0"); got.Version != "v2" {
		t.Fatalf("after versioned upsert: version=%q, want v2", got.Version)
	}
	versions, err := db.MessageVersions("C1", "100.0", "200.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions["100.0"] != "v2" {
		t.Fatalf("MessageVersions = %v, want only 100.0 → v2", versions)
	}
}

func TestUpsertMessages_ResurrectsWhatTheServerStillReturns(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	if err := db.UpsertMessages([]Message{feedMsg("100.0", "a")}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteMessage("C1", "100.0"); err != nil {
		t.Fatal(err)
	}
	// The server still returns it: it is not deleted.
	if err := db.UpsertMessages([]Message{feedMsg("100.0", "a")}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetMessage("C1", "100.0"); got.IsDeleted {
		t.Fatal("a message the server returned stayed deleted")
	}
	if err := db.UpsertMessages(nil); err != nil {
		t.Fatalf("empty batch: %v", err)
	}
}

func TestApplyEdit_TouchesOnlyWhatAnEditIsAbout(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	parent := feedMsg("100.0", "parent")
	parent.Subtype = "thread_broadcast"
	parent.Version = "v1"
	parent.ReplyCount = 2
	if err := db.UpsertMessages([]Message{parent}); err != nil {
		t.Fatal(err)
	}

	// A bare text edit: subtype, version, reply_count and thread_ts stay.
	ok, err := db.ApplyEdit("C1", "100.0", Edit{Text: "parent, edited", RawJSON: `{"text":"parent, edited"}`, EditedAt: "160.0"})
	if err != nil || !ok {
		t.Fatalf("ApplyEdit = %v, %v; want applied", ok, err)
	}
	got, _ := db.GetMessage("C1", "100.0")
	if got.Text != "parent, edited" || got.RawJSON != `{"text":"parent, edited"}` || got.EditedAt != "160.0" {
		t.Errorf("edit not applied: %+v", got)
	}
	if got.Subtype != "thread_broadcast" || got.Version != "v1" || got.ReplyCount != 2 || got.ThreadTS != "" {
		t.Errorf("edit touched more than the edit: %+v", got)
	}

	// The server reporting a parent's first reply: thread_ts and
	// reply_count arrive, edited_at does not.
	ok, err = db.ApplyEdit("C1", "100.0", Edit{Text: "parent, edited", RawJSON: "{}", ThreadTS: "100.0", ReplyCount: 1})
	if err != nil || !ok {
		t.Fatalf("ApplyEdit = %v, %v; want applied", ok, err)
	}
	got, _ = db.GetMessage("C1", "100.0")
	if got.ThreadTS != "100.0" || got.EditedAt != "160.0" {
		t.Errorf("thread change: %+v", got)
	}
	// reply_count is a maximum: 1 does not lower the stored 2.
	if got.ReplyCount != 2 {
		t.Errorf("reply_count = %d, want 2 (MAX)", got.ReplyCount)
	}
	if ok, err = db.ApplyEdit("C1", "100.0", Edit{Text: "parent, edited", RawJSON: "{}", ReplyCount: 5}); err != nil || !ok {
		t.Fatal(err)
	}
	if got, _ = db.GetMessage("C1", "100.0"); got.ReplyCount != 5 {
		t.Errorf("reply_count = %d, want 5", got.ReplyCount)
	}

	// An edit of a message the cache does not hold writes nothing.
	ok, err = db.ApplyEdit("C1", "999.0", Edit{Text: "ghost", RawJSON: "{}"})
	if err != nil || ok {
		t.Fatalf("ApplyEdit on a missing row = %v, %v; want false", ok, err)
	}
	if _, err := db.GetMessage("C1", "999.0"); err == nil {
		t.Fatal("an edit created a row")
	}
}

func TestApplyEdit_UpdatesTheSearchIndex(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	if db.ftsDisabled {
		t.Skip("FTS5 unavailable")
	}
	if err := db.UpsertMessages([]Message{feedMsg("100.0", "original wording")}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ApplyEdit("C1", "100.0", Edit{Text: "replacement wording", RawJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	hits, err := db.SearchChannelMessages("C1", "T1", "replacement")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("search for the new text found %v, want 1 hit", hits)
	}
	if hits, _ = db.SearchChannelMessages("C1", "T1", "original"); len(hits) != 0 {
		t.Fatalf("search for the old text found %v, want none", hits)
	}
}

func TestIncrementReplyCount(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	if err := db.UpsertMessages([]Message{feedMsg("100.0", "parent")}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := db.IncrementReplyCount("C1", "100.0"); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := db.GetMessage("C1", "100.0"); got.ReplyCount != 2 {
		t.Fatalf("reply_count = %d, want 2", got.ReplyCount)
	}
	// A missing parent is left alone, not created.
	if err := db.IncrementReplyCount("C1", "555.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetMessage("C1", "555.0"); err == nil {
		t.Fatal("incrementing a missing parent created a row")
	}
}

func TestPruneChannelRange(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	reply := feedMsg("150.0", "reply")
	reply.ThreadTS = "100.0"
	broadcast := feedMsg("160.0", "broadcast")
	broadcast.ThreadTS, broadcast.Subtype = "100.0", "thread_broadcast"
	fresh := feedMsg("250.0", "arrived mid-fetch")
	fresh.CreatedAt = 2000
	rows := []Message{
		feedMsg("100.0", "a"), reply, broadcast,
		feedMsg("200.0", "b"), fresh, feedMsg("300.0", "c"), feedMsg("400.0", "outside"),
	}
	if err := db.UpsertMessages(rows); err != nil {
		t.Fatal(err)
	}

	// The page covering [100, 300] returned a and c (b was deleted
	// meanwhile). The reply, the broadcast, the row cached after the
	// request started and the row outside the bounds all survive.
	pruned, err := db.PruneChannelRange("C1", "100.0", "300.0", []string{"100.0", "300.0"}, 1500)
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned) != 1 || pruned[0] != "200.0" {
		t.Fatalf("pruned %v, want [200.0]", pruned)
	}
	for _, c := range []struct {
		ts      string
		deleted bool
	}{
		{"100.0", false}, {"150.0", false}, {"160.0", false},
		{"200.0", true}, {"250.0", false}, {"300.0", false}, {"400.0", false},
	} {
		got, err := db.GetMessage("C1", c.ts)
		if err != nil {
			t.Fatal(err)
		}
		if got.IsDeleted != c.deleted {
			t.Errorf("%s: deleted=%v, want %v", c.ts, got.IsDeleted, c.deleted)
		}
	}
	if _, err := db.PruneChannelRange("C1", "", "300.0", nil, 1500); err == nil {
		t.Fatal("an open bound was accepted")
	}
}

func TestGetMessagesBetweenAndOldestCachedTS(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	reply := feedMsg("150.0", "reply")
	reply.ThreadTS = "100.0"
	if err := db.UpsertMessages([]Message{feedMsg("100.0", "a"), reply, feedMsg("200.0", "b"), feedMsg("300.0", "c")}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetMessagesBetween("C1", "100.0", "200.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].TS != "100.0" || got[1].TS != "200.0" {
		t.Fatalf("between = %+v, want a and b (no reply)", got)
	}
	if got, _ = db.GetMessagesBetween("C1", "200.0", ""); len(got) != 2 || got[1].TS != "300.0" {
		t.Fatalf("open latest = %+v, want b and c", got)
	}
	oldest, err := db.OldestCachedTS("C1")
	if err != nil || oldest != "100.0" {
		t.Fatalf("OldestCachedTS = %q, %v; want 100.0", oldest, err)
	}
	if err := db.DeleteMessage("C1", "100.0"); err != nil {
		t.Fatal(err)
	}
	if oldest, _ = db.OldestCachedTS("C1"); oldest != "200.0" {
		t.Fatalf("OldestCachedTS after delete = %q, want 200.0", oldest)
	}
	if oldest, _ = db.OldestCachedTS("C9"); oldest != "" {
		t.Fatalf("OldestCachedTS of an unknown channel = %q, want empty", oldest)
	}
}
