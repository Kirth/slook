package usergroups

import (
	"testing"

	"github.com/slack-go/slack"
)

func TestHandlesPrefersHandle(t *testing.T) {
	byID := Handles([]slack.UserGroup{
		{ID: "S1", Handle: "platform-team", Name: "Platform Team"},
	})
	if byID["S1"] != "platform-team" {
		t.Errorf("handle = %q, want platform-team", byID["S1"])
	}
}

func TestHandlesSlugifiesNameFallback(t *testing.T) {
	byID := Handles([]slack.UserGroup{
		{ID: "S1", Name: "Platform Team"},
	})
	if byID["S1"] != "platform-team" {
		t.Errorf("name fallback = %q, want platform-team (no spaces)", byID["S1"])
	}
}

func TestHandlesSkipsUnmentionableGroups(t *testing.T) {
	byID := Handles([]slack.UserGroup{
		{ID: "S1"},
		{ID: "S2", Name: "   "},
		{ID: "S3", Name: "!!!"},
	})
	if len(byID) != 0 {
		t.Errorf("byID = %v, want empty (nothing mentionable)", byID)
	}
}

func TestSlugifyHandle(t *testing.T) {
	cases := map[string]string{
		"Platform Team":     "platform-team",
		"  Data   Eng  ":    "data-eng",
		"Design/Research":   "design-research",
		"on-call_rota.v2":   "on-call_rota.v2",
		"Team (EU) — North": "team-eu-north",
		"":                  "",
		"???":               "",
	}
	for in, want := range cases {
		if got := slugifyHandle(in); got != want {
			t.Errorf("slugifyHandle(%q) = %q, want %q", in, got, want)
		}
	}
}
