package backend

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/gammons/slk/internal/slack/slacktest"
)

// TestSelfStatus_ReadsPresenceAndDND asks users.getPresence and
// dnd.info for the user and keeps the answer.
func TestSelfStatus_ReadsPresenceAndDND(t *testing.T) {
	end := time.Now().Add(time.Hour).Unix()
	bodies := bootBodies()
	bodies["/api/users.getPresence"] = `{"ok":true,"presence":"away","manual_away":true}`
	bodies["/api/dnd.info"] = `{"ok":true,"dnd_enabled":false,"snooze_enabled":true,"snooze_endtime":` + strconv.FormatInt(end, 10) + `}`
	srv := slacktest.NewServer(t, bodies)
	s, _ := bootSlack(t, srv)
	st, err := s.SelfStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Away || !st.DND || st.DNDUntil.Unix() != end {
		t.Fatalf("got %+v, want away, DND until %d", st, end)
	}
	if got := srv.RequestTo(t, "/api/users.getPresence").Form.Get("user"); got != "U1" {
		t.Fatalf("users.getPresence user=%q, want U1", got)
	}
}

// TestSetPresence_TakesBackWhatSlackRefuses shows the change at once
// and the old status again when presence.set fails.
func TestSetPresence_TakesBackWhatSlackRefuses(t *testing.T) {
	bodies := bootBodies()
	bodies["/api/users.setPresence"] = `{"ok":false,"error":"invalid_presence"}`
	bodies["/api/dnd.setSnooze"] = `{"ok":true,"snooze_enabled":true}`
	srv := slacktest.NewServer(t, bodies)
	s, _ := bootSlack(t, srv)
	ctx := context.Background()

	if err := s.SetPresence(ctx, true); err == nil {
		t.Fatal("a refused presence.set succeeded")
	}
	if got := srv.RequestTo(t, "/api/users.setPresence").Form.Get("presence"); got != "away" {
		t.Fatalf("presence=%q, want away", got)
	}
	if st := s.applySelf(<-s.queue); !st.Away {
		t.Fatalf("not shown away at once: %+v", st)
	}
	if st := s.applySelf(<-s.queue); st.Away {
		t.Fatalf("not taken back: %+v", st)
	}

	if err := s.Snooze(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := srv.RequestTo(t, "/api/dnd.setSnooze").Form.Get("num_minutes"); got != "20" {
		t.Fatalf("num_minutes=%q, want 20", got)
	}
	if st := s.applySelf(<-s.queue); !st.DND || st.DNDUntil.IsZero() {
		t.Fatalf("snooze not shown: %+v", st)
	}
	// The socket's word wins: another client ended it.
	if st := s.applySelf(rawSelfDND{on: false}); st.DND {
		t.Fatalf("dnd_updated off left %+v", st)
	}
}
