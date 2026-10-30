package cache

import (
	"reflect"
	"testing"
)

func TestUserGroupsReplaceAndList(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	first := []UserGroup{
		{ID: "S1", Handle: "eng", Name: "Engineering", Description: "Builds it", Members: []string{"U1", "U2"}},
		{ID: "S2", Handle: "design", Name: "Design"},
	}
	if err := db.ReplaceUserGroups("T1", first); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceUserGroups("T2", []UserGroup{{ID: "S9", Handle: "other"}}); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListUserGroups("T1")
	if err != nil {
		t.Fatal(err)
	}
	want := []UserGroup{first[1], first[0]} // by handle
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	if err := db.ReplaceUserGroups("T1", []UserGroup{{ID: "S1", Handle: "eng", Members: []string{"U3"}}}); err != nil {
		t.Fatal(err)
	}
	got, _ = db.ListUserGroups("T1")
	if len(got) != 1 || got[0].ID != "S1" || !reflect.DeepEqual(got[0].Members, []string{"U3"}) {
		t.Fatalf("after replace: %+v (S2 should be gone, S1's members replaced)", got)
	}
}
