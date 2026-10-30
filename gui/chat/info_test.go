package chat

import (
	"slices"
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
)

func TestInfoTabs(t *testing.T) {
	for kind, want := range map[backend.Kind][]InfoTab{
		backend.KindChannel: {TabAbout, TabMembers, TabNotify},
		backend.KindPrivate: {TabAbout, TabMembers, TabNotify},
		backend.KindGroupDM: {TabMembers, TabNotify},
		backend.KindDM:      {TabAbout, TabNotify},
	} {
		if got := InfoTabs(kind); !slices.Equal(got, want) {
			t.Errorf("%v: %v, want %v", kind, got, want)
		}
	}
}

func TestFilterMembers(t *testing.T) {
	people := []backend.Person{
		{ID: "U9"},
		{ID: "U2", Name: "Édith Piaf", Handle: "edith"},
		{ID: "U1", Name: "ada", RealName: "Augusta Ada King"},
		{ID: "U3"},
		{ID: "U4", Name: "Grace Hopper", Handle: "amazing.grace"},
	}
	ids := func(ps []backend.Person) []string {
		var out []string
		for _, p := range ps {
			out = append(out, p.ID)
		}
		return out
	}
	for _, c := range []struct {
		q    string
		want []string
	}{
		{"", []string{"U1", "U2", "U4", "U3", "U9"}},
		{"edi", []string{"U2"}},
		{"@king", []string{"U1"}},
		{"amazing", []string{"U4"}},
		{"zzz", nil},
	} {
		if got := ids(FilterMembers(people, c.q)); !slices.Equal(got, c.want) {
			t.Errorf("%q: %v, want %v", c.q, got, c.want)
		}
	}
}

func TestCreatedLine(t *testing.T) {
	at := time.Date(2019, 3, 14, 9, 26, 0, 0, time.Local)
	for _, c := range []struct {
		name string
		at   time.Time
		want string
	}{
		{"Ada", at, "Created by Ada on March 14, 2019"},
		{"Ada", time.Time{}, "Created by Ada"},
		{"", at, "Created on March 14, 2019"},
		{"", time.Time{}, ""},
	} {
		if got := CreatedLine(c.name, c.at); got != c.want {
			t.Errorf("%q %v: %q, want %q", c.name, c.at, got, c.want)
		}
	}
}
