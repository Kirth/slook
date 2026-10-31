package backend

import (
	"context"
	"testing"

	"github.com/gammons/slk/internal/slack/slacktest"
)

// TestOpenConversation_LeavesOutSelfAndRepeats opens a group DM through
// conversations.open with the user and a repeat taken out, and
// describes it with conversations.info.
func TestOpenConversation_LeavesOutSelfAndRepeats(t *testing.T) {
	bodies := bootBodies()
	bodies["/api/conversations.open"] = `{"ok":true,"channel":{"id":"G9"}}`
	bodies["/api/conversations.info"] = `{"ok":true,"channel":{"id":"G9","name":"mpdm-pat--sam-1","is_mpim":true,"is_open":true}}`
	srv := slacktest.NewServer(t, bodies)
	s, _ := bootSlack(t, srv)
	ctx := context.Background()

	conv, err := s.OpenConversation(ctx, []string{"U2", "U1", "U3", "U2"})
	if err != nil {
		t.Fatal(err)
	}
	if conv.ID != "G9" || conv.Kind != KindGroupDM {
		t.Fatalf("opened %#v, want group DM G9", conv)
	}
	if got := srv.RequestTo(t, "/api/conversations.open").Form.Get("users"); got != "U2,U3" {
		t.Fatalf("conversations.open users=%q, want U2,U3", got)
	}

	if _, err := s.OpenConversation(ctx, []string{"U1"}); err == nil {
		t.Fatal("a DM with only the user opened")
	}
	nine := []string{"U2", "U3", "U4", "U5", "U6", "U7", "U8", "U9", "U10"}
	if _, err := s.OpenConversation(ctx, nine); err == nil {
		t.Fatal("nine people opened a group DM")
	}
	if n := srv.Count("/api/conversations.open"); n != 1 {
		t.Fatalf("%d conversations.open requests, want 1", n)
	}
}
