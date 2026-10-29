package cache

import (
	"testing"
)

func TestUpsertAndGetReactions(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	err := db.UpsertReaction("1234.5678", "C123", "thumbsup", []string{"U001", "U002"}, 2)
	if err != nil {
		t.Fatalf("UpsertReaction: %v", err)
	}

	rows, err := db.GetReactions("1234.5678", "C123")
	if err != nil {
		t.Fatalf("GetReactions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 reaction, got %d", len(rows))
	}
	if rows[0].Emoji != "thumbsup" {
		t.Errorf("expected emoji thumbsup, got %s", rows[0].Emoji)
	}
	if rows[0].Count != 2 {
		t.Errorf("expected count 2, got %d", rows[0].Count)
	}
	if len(rows[0].UserIDs) != 2 || rows[0].UserIDs[0] != "U001" {
		t.Errorf("unexpected userIDs: %v", rows[0].UserIDs)
	}
}

func TestUpsertReactionUpdatesExisting(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	err := db.UpsertReaction("1234.5678", "C123", "thumbsup", []string{"U001"}, 1)
	if err != nil {
		t.Fatalf("UpsertReaction: %v", err)
	}

	err = db.UpsertReaction("1234.5678", "C123", "thumbsup", []string{"U001", "U002"}, 2)
	if err != nil {
		t.Fatalf("UpsertReaction update: %v", err)
	}

	rows, err := db.GetReactions("1234.5678", "C123")
	if err != nil {
		t.Fatalf("GetReactions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 reaction, got %d", len(rows))
	}
	if rows[0].Count != 2 {
		t.Errorf("expected count 2, got %d", rows[0].Count)
	}
}

func TestDeleteReaction(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	err := db.UpsertReaction("1234.5678", "C123", "thumbsup", []string{"U001"}, 1)
	if err != nil {
		t.Fatalf("UpsertReaction: %v", err)
	}

	err = db.DeleteReaction("1234.5678", "C123", "thumbsup")
	if err != nil {
		t.Fatalf("DeleteReaction: %v", err)
	}

	rows, err := db.GetReactions("1234.5678", "C123")
	if err != nil {
		t.Fatalf("GetReactions: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 reactions, got %d", len(rows))
	}
}

func TestGetReactionsEmpty(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	rows, err := db.GetReactions("nonexistent", "C123")
	if err != nil {
		t.Fatalf("GetReactions: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 reactions, got %d", len(rows))
	}
}

func TestMultipleReactionsOnMessage(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	if err := db.UpsertReaction("1234.5678", "C123", "thumbsup", []string{"U001"}, 1); err != nil {
		t.Fatalf("UpsertReaction thumbsup: %v", err)
	}
	if err := db.UpsertReaction("1234.5678", "C123", "rocket", []string{"U002", "U003"}, 2); err != nil {
		t.Fatalf("UpsertReaction rocket: %v", err)
	}
	if err := db.UpsertReaction("1234.5678", "C123", "heart", []string{"U001"}, 1); err != nil {
		t.Fatalf("UpsertReaction heart: %v", err)
	}

	rows, err := db.GetReactions("1234.5678", "C123")
	if err != nil {
		t.Fatalf("GetReactions: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 reactions, got %d", len(rows))
	}
}

func TestReactionsForReadsAPageInFirstAddedOrder(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.UpsertReaction("1.0", "C1", "tada", []string{"U1"}, 1))
	must(db.UpsertReaction("1.0", "C1", "+1", []string{"U1", "U2"}, 2))
	must(db.UpsertReaction("2.0", "C1", "eyes", []string{"U3"}, 1))
	must(db.UpsertReaction("1.0", "C2", "fire", []string{"U1"}, 1)) // another channel
	must(db.UpsertReaction("3.0", "C1", "fire", []string{"U1"}, 1)) // not asked for
	// An update keeps tada first.
	must(db.UpsertReaction("1.0", "C1", "tada", []string{"U1", "U4"}, 2))

	got, err := db.ReactionsFor("C1", []string{"1.0", "2.0", "9.0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got reactions for %d messages, want 2: %v", len(got), got)
	}
	first := got["1.0"]
	if len(first) != 2 || first[0].Emoji != "tada" || first[0].Count != 2 || len(first[0].UserIDs) != 2 || first[1].Emoji != "+1" {
		t.Fatalf("1.0: %+v", first)
	}
	if e := got["2.0"]; len(e) != 1 || e[0].Emoji != "eyes" {
		t.Fatalf("2.0: %+v", e)
	}
	if empty, err := db.ReactionsFor("C1", nil); err != nil || len(empty) != 0 {
		t.Fatalf("no ts: %v, %v", empty, err)
	}
}
