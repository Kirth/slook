package backend

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gammons/slk/internal/slack/slacktest"
	"github.com/slack-go/slack"
)

// TestPressAction_EchoesTheEphemeralsButton builds an ephemeral as the
// socket delivers it (attachments plus the action ids slack-go drops)
// and presses its second button: chat.attachmentAction carries the
// action with its id, the attachment's id and callback, and the bot as
// service_id.
func TestPressAction_EchoesTheEphemeralsButton(t *testing.T) {
	bodies := bootBodies()
	srv := slacktest.NewServer(t, bodies)
	bodies["/api/chat.attachmentAction"] = `{"ok":true}`
	bodies["/api/bots.info"] = `{"ok":true,"bot":{"id":"B9","name":"standup"}}`
	s, _ := bootSlack(t, srv)
	ctx := context.Background()

	r := rawMessage{
		channelID: "C1", ts: "1700000000.000100", botID: "B9", username: "Standup Bot", ephemeral: true, text: "Send it?",
		attachments: []slack.Attachment{{ID: 1, CallbackID: "cb", Actions: []slack.AttachmentAction{
			{Name: "pick", Text: "Pick", Type: "select"},
			{Name: "send", Text: "Send", Type: "button", Value: "send", Style: "primary"},
		}}},
		actionIDs: [][]string{{"1", "2"}},
	}
	m := s.message(ctx, r)
	if len(m.Actions) != 2 || m.Actions[1].Label != "Send" || m.Actions[1].Index != 1 {
		t.Fatalf("actions %+v", m.Actions)
	}
	if err := s.PressAction(ctx, m, m.Actions[0]); err == nil {
		t.Fatal("pressed a menu")
	}
	if err := s.PressAction(ctx, m, m.Actions[1]); err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests("/api/chat.attachmentAction")
	if len(reqs) != 1 || reqs[0].Form.Get("service_id") != "B9" {
		t.Fatalf("chat.attachmentAction requests %+v", reqs)
	}
	var p struct {
		Actions      []struct{ ID, Name, Value string } `json:"actions"`
		AttachmentID string                             `json:"attachment_id"`
		CallbackID   string                             `json:"callback_id"`
		ChannelID    string                             `json:"channel_id"`
		IsEphemeral  bool                               `json:"is_ephemeral"`
		MessageTS    string                             `json:"message_ts"`
	}
	if err := json.Unmarshal([]byte(reqs[0].Form.Get("payload")), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Actions) != 1 || p.Actions[0].ID != "2" || p.Actions[0].Name != "send" || p.AttachmentID != "1" ||
		p.CallbackID != "cb" || p.ChannelID != "C1" || !p.IsEphemeral || p.MessageTS != "1700000000.000100" {
		t.Fatalf("payload %s", reqs[0].Form.Get("payload"))
	}

	r.ephemeral = false
	if m := s.message(ctx, r); m.Actions != nil {
		t.Fatalf("a message everyone sees has actions %+v", m.Actions)
	}
}
