package msgsync

import (
	"slices"
	"testing"
)

func TestStale(t *testing.T) {
	heads := map[string]string{
		"C1": "1700000100.000100", // newer upstream: stale
		"C2": "1700000200.000100", // same: not stale
		"C3": "1700000300.000100", // ignored
		"C4": "1700000050.000100", // newer, not in order
		"C5": "1700000060.000100", // newer, not in order, newest
		"C6": "1700000070.000100", // no latest
	}
	latest := map[string]string{
		"C1": "1700000150.000100",
		"C2": "1700000200.000100",
		"C3": "1700000999.000100",
		"C4": "1700000400.000100",
		"C5": "1700000500.000100",
		"C9": "1700000999.000100", // very active, never followed
	}
	ignored := func(id string) bool { return id == "C3" }
	got := Stale([]string{"C9", "C2", "C1", "C3"}, heads, latest, ignored, 10)
	if want := []string{"C1", "C5", "C4"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := Stale([]string{"C1"}, heads, latest, ignored, 2); !slices.Equal(got, []string{"C1", "C5"}) {
		t.Fatalf("limit 2: %v", got)
	}
	if got := Stale(nil, heads, latest, nil, 0); got != nil {
		t.Fatalf("limit 0: %v", got)
	}
	if got := Stale(nil, map[string]string{"C1": ""}, latest, nil, 5); len(got) != 0 {
		t.Fatalf("an empty head: %v", got)
	}
}

func TestTSAfterComparesSecondsAsNumbers(t *testing.T) {
	if !tsAfter("1000000000.000001", "999999999.999999") {
		t.Fatal("a 10-digit second is after a 9-digit one")
	}
	if tsAfter("999999999.999999", "1000000000.000001") {
		t.Fatal("a 9-digit second is not after a 10-digit one")
	}
	if !tsAfter("1700000000.000200", "1700000000.000100") || tsAfter("1700000000.000100", "1700000000.000100") {
		t.Fatal("fractions")
	}
}
