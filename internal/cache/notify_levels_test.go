package cache

import (
	"maps"
	"testing"
)

func TestNotifyLevels(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	for _, c := range []struct {
		ch    string
		level int
	}{{"C1", 1}, {"C2", 2}, {"C1", 2}, {"C2", 0}} {
		if err := db.SetNotifyLevel("T1", c.ch, c.level); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetNotifyLevel("T2", "C3", 1); err != nil {
		t.Fatal(err)
	}
	got, err := db.NotifyLevels("T1")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]int{"C1": 2}; !maps.Equal(got, want) {
		t.Fatalf("levels %v, want %v (the default removes the row; other workspaces stay apart)", got, want)
	}
}
