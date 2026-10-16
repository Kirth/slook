package main

import (
	"testing"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ui"
)

// The Activity closures must resolve the workspace they were asked for,
// not whichever is active when they run. A workspace switch can land
// between dispatch and execution; resolving via the active workspace
// then fetches the wrong feed and labels it with the requested team.
//
// T1 is active and its Client is nil, so any call through it panics;
// T2 is requested and not connected, so the only correct result is a
// nil Msg without touching T1.
func TestActivityClosures_ResolveRequestedTeamNotActive(t *testing.T) {
	router := newWorkspaceRouter()
	active := &WorkspaceContext{TeamID: "T1"}
	router.Add(active)
	router.Set(active)

	cases := []struct {
		name string
		call func() core.Msg
	}{
		{"fetch", func() core.Msg { return activityFetchFunc(router)("T2", 50, false) }},
		{"hydrate", func() core.Msg {
			return activityHydrateFunc(router)("T2", map[string][]string{"C1": {"1.1"}})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s for T2 went through the active workspace T1: %v", c.name, r)
				}
			}()
			if msg := c.call(); msg != nil {
				t.Fatalf("%s for unconnected T2 = %#v, want nil", c.name, msg)
			}
		})
	}
}

// SetRead must always answer, even for a workspace that is not
// connected, so the UI can undo its optimistic change. Like the other
// closures it resolves the requested team: T1 is active with a nil
// Client, so going through it would panic.
func TestActivitySetRead_UnconnectedTeamReportsError(t *testing.T) {
	router := newWorkspaceRouter()
	active := &WorkspaceContext{TeamID: "T1"}
	router.Add(active)
	router.Set(active)

	msg := activitySetReadFunc(router)("T2", core.ActivityItem{Key: "k1", Type: "at_user", FeedTS: "1.1"}, true)
	got, ok := msg.(ui.ActivityReadSetMsg)
	if !ok {
		t.Fatalf("SetRead returned %#v, want ui.ActivityReadSetMsg", msg)
	}
	if got.TeamID != "T2" || got.Key != "k1" || !got.Read || got.Err == nil {
		t.Fatalf("SetRead for unconnected T2 = %+v, want TeamID T2, Key k1, Read true and an error", got)
	}
}
