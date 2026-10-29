package cache

import "testing"

func span(oldest, latest string) Span {
	return Span{ChannelID: "C1", WorkspaceID: "T1", OldestTS: oldest, LatestTS: latest}
}

func mustSpans(t *testing.T, db *DB, channelID string) []Span {
	t.Helper()
	spans, err := db.Spans(channelID)
	if err != nil {
		t.Fatal(err)
	}
	return spans
}

func TestAddSpan_MergesOverlapping(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	if err := db.AddSpan(span("100.0", "200.0")); err != nil {
		t.Fatal(err)
	}
	// Shares 200.0 with the first: one span.
	if err := db.AddSpan(span("200.0", "300.0")); err != nil {
		t.Fatal(err)
	}
	got := mustSpans(t, db, "C1")
	if len(got) != 1 || got[0].OldestTS != "100.0" || got[0].LatestTS != "300.0" {
		t.Fatalf("spans = %+v, want one [100.0, 300.0]", got)
	}
	// Contained entirely: no change.
	if err := db.AddSpan(span("150.0", "250.0")); err != nil {
		t.Fatal(err)
	}
	if got = mustSpans(t, db, "C1"); len(got) != 1 || got[0].OldestTS != "100.0" || got[0].LatestTS != "300.0" {
		t.Fatalf("spans = %+v, want one [100.0, 300.0]", got)
	}
}

func TestAddSpan_KeepsGapsSeparateAndBridgesThem(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	for _, s := range []Span{span("100.0", "200.0"), span("500.0", "600.0")} {
		if err := db.AddSpan(s); err != nil {
			t.Fatal(err)
		}
	}
	got := mustSpans(t, db, "C1")
	if len(got) != 2 || got[0].OldestTS != "100.0" || got[1].OldestTS != "500.0" {
		t.Fatalf("spans = %+v, want two, oldest first", got)
	}
	covered, err := db.Covers("C1", "150.0", "550.0")
	if err != nil || covered {
		t.Fatalf("Covers across the hole = %v, %v; want false", covered, err)
	}
	// A page reaching from inside one into the other joins them.
	if err := db.AddSpan(span("200.0", "500.0")); err != nil {
		t.Fatal(err)
	}
	got = mustSpans(t, db, "C1")
	if len(got) != 1 || got[0].OldestTS != "100.0" || got[0].LatestTS != "600.0" {
		t.Fatalf("spans = %+v, want one [100.0, 600.0]", got)
	}
	if covered, _ = db.Covers("C1", "150.0", "550.0"); !covered {
		t.Fatal("Covers after the bridge = false, want true")
	}
}

func TestAddSpan_FlagsSurviveMerge(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	s := span("100.0", "200.0")
	s.ReachesStart = true
	if err := db.AddSpan(s); err != nil {
		t.Fatal(err)
	}
	live := span("200.0", "300.0")
	live.Live = true
	if err := db.AddSpan(live); err != nil {
		t.Fatal(err)
	}
	got := mustSpans(t, db, "C1")
	if len(got) != 1 || !got[0].ReachesStart || !got[0].Live {
		t.Fatalf("spans = %+v, want one that reaches the start and is live", got)
	}
}

func TestAddSpan_RejectsBadBounds(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	if err := db.AddSpan(span("300.0", "200.0")); err == nil {
		t.Fatal("inverted bounds accepted")
	}
	if err := db.AddSpan(span("", "200.0")); err == nil {
		t.Fatal("empty bound accepted")
	}
}

func TestLiveSpans_ExtendAndClear(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	// No live span yet: nothing to extend.
	ok, err := db.ExtendLiveSpan("C1", "150.0")
	if err != nil || ok {
		t.Fatalf("ExtendLiveSpan without a live span = %v, %v; want false", ok, err)
	}
	live := span("100.0", "200.0")
	live.Live = true
	if err := db.AddSpan(live); err != nil {
		t.Fatal(err)
	}
	if ok, err = db.ExtendLiveSpan("C1", "250.0"); err != nil || !ok {
		t.Fatalf("ExtendLiveSpan = %v, %v; want true", ok, err)
	}
	// An older ts (a delayed duplicate) never moves it back.
	if _, err = db.ExtendLiveSpan("C1", "120.0"); err != nil {
		t.Fatal(err)
	}
	got := mustSpans(t, db, "C1")
	if len(got) != 1 || got[0].LatestTS != "250.0" || !got[0].Live {
		t.Fatalf("spans = %+v, want one live to 250.0", got)
	}
	// Another workspace's spans are untouched by a clear.
	other := Span{ChannelID: "C9", WorkspaceID: "T2", OldestTS: "1.0", LatestTS: "2.0", Live: true}
	if err := db.AddSpan(other); err != nil {
		t.Fatal(err)
	}
	if err := db.ClearLiveSpans("T1"); err != nil {
		t.Fatal(err)
	}
	if got = mustSpans(t, db, "C1"); got[0].Live {
		t.Fatal("C1's span is still live after the clear")
	}
	if got = mustSpans(t, db, "C9"); !got[0].Live {
		t.Fatal("T2's span lost its live flag to T1's clear")
	}
	if ok, _ = db.ExtendLiveSpan("C1", "300.0"); ok {
		t.Fatal("a cleared span was extended")
	}
}
