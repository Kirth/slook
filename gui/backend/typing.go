package backend

import slackclient "github.com/gammons/slk/internal/slack"

type (
	// rawTyping is the socket's user_typing.
	rawTyping struct{ channelID, userID string }
	// rawAppStatus is the socket's ai_assistant_status.
	rawAppStatus slackclient.AssistantStatus
)

// Typing tells Slack the user is typing in channelID, over the socket,
// as Slack's clients do. Best-effort: with no socket it is dropped.
func (s *Slack) Typing(channelID string) {
	go func() { _ = s.client.SendTyping(channelID) }()
}
