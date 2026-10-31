package blockkit

import (
	"encoding/json"
	"testing"

	"github.com/slack-go/slack"
)

// A copy of internal/blockkit's parse_test.go capture, for the render
// tests in attachments_test.go; it goes with this package in the TUI
// sunset's Phase 5.

// capturedMentionAttachments is the `attachments` array of Slackbot's
// "You mentioned @x, but they're not in this channel" ephemeral, as
// captured from the Slack web client (IDs replaced with placeholders).
// See docs/superpowers/specs/2026-10-09-interactive-ephemeral-messages-design.md.
const capturedMentionAttachments = `[{
  "callback_id":"consistentephemeralmentions_U0EXAMPLE01_1791541429559219_0",
  "fallback":"You may want to invite them.","id":1,
  "actions":[
    {"id":"1","name":"invite","text":"Add Them","type":"button","value":"invite","style":"",
     "confirm":{"text":"New members will be able to see all of the channel's history, including any files that have been shared in the channel.",
                "title":"Are you sure you want to add them?","ok_text":"Add","dismiss_text":"Cancel"}},
    {"id":"2","name":"ignore","text":"Dismiss","type":"button","value":"ignore","style":""},
    {"id":"3","name":"dont-show-again","text":"Don't Show Again","type":"button","value":"dont-show-again","style":""}]}]`

func decodeCapturedAttachments(t *testing.T) []slack.Attachment {
	t.Helper()
	var atts []slack.Attachment
	if err := json.Unmarshal([]byte(capturedMentionAttachments), &atts); err != nil {
		t.Fatalf("decoding captured attachments: %v", err)
	}
	return atts
}
