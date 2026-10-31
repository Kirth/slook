package backend

import (
	"context"
	"testing"

	"github.com/gammons/slk/internal/slack/slacktest"
)

// TestForward_QueuesThePostedMessage forwards with a comment: Slack is
// asked for the permalink, the comment and the permalink are posted on
// two lines, and the posted message is queued for Run as Send's is.
func TestForward_QueuesThePostedMessage(t *testing.T) {
	const link = "https://acme.slack.com/archives/C1/p1700000000000100"
	bodies := bootBodies()
	srv := slacktest.NewServer(t, bodies)
	bodies["/api/chat.getPermalink"] = `{"ok":true,"channel":"C1","permalink":"` + link + `"}`
	bodies["/api/chat.postMessage"] = `{"ok":true,"channel":"D1","ts":"1700000001.000100"}`
	s, _ := bootSlack(t, srv)

	m := Message{ChannelID: "C1", TS: "1700000000.000100"}
	if err := s.Forward(context.Background(), m, "D1", "fyi"); err != nil {
		t.Fatal(err)
	}
	posts := srv.Requests("/api/chat.postMessage")
	if len(posts) != 1 || posts[0].Form.Get("channel") != "D1" || posts[0].Form.Get("text") != "fyi\n"+link {
		t.Fatalf("chat.postMessage requests %+v", posts)
	}
	r, ok := (<-s.queue).(rawMessage)
	if !ok || r.channelID != "D1" || r.ts != "1700000001.000100" || r.userID != "U1" || r.text != "fyi\n<"+link+">" {
		t.Fatalf("queued %#v", r)
	}
}
