package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
)

// MaxRecipients is how many people a group DM can be opened with,
// besides the user: Slack's limit for conversations.open.
const MaxRecipients = 8

// recipients is userIDs without self, repeats and empty IDs, or why it
// cannot be opened: no one left, or more than MaxRecipients.
func recipients(userIDs []string, self string) ([]string, error) {
	var ids []string
	for _, id := range userIDs {
		if id != "" && id != self && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	switch {
	case len(ids) == 0:
		return nil, errors.New("choose someone to message")
	case len(ids) > MaxRecipients:
		return nil, fmt.Errorf("at most %d people in a group message", MaxRecipients)
	}
	return ids, nil
}

// OpenConversation opens (or finds) the DM or group DM with userIDs
// through conversations.open, then describes it as Conversation does,
// which also reports it as a ConversationOpened.
func (s *Slack) OpenConversation(ctx context.Context, userIDs []string) (Conversation, error) {
	ids, err := recipients(userIDs, s.Self())
	if err != nil {
		return Conversation{}, err
	}
	id, _, err := s.client.OpenConversation(ctx, ids)
	if err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: dm:", err)
		return Conversation{}, err
	}
	return s.Conversation(ctx, id)
}
