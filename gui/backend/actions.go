package backend

import (
	"context"
	"errors"
	"time"

	"github.com/gammons/slk/internal/core/blocks"
	slackclient "github.com/gammons/slk/internal/slack"
)

// pressTimeout bounds one button press.
const pressTimeout = 10 * time.Second

// messageActions is atts' actions in order (every action of every
// attachment, as rich.FromAttachments numbers them), each with what a
// press echoes: its attachment's id and callback, and serviceID, the
// posting app's bot ID.
func messageActions(atts []blocks.LegacyAttachment, serviceID string) []Action {
	var out []Action
	for _, att := range atts {
		for _, act := range att.Actions {
			out = append(out, Action{
				Index: len(out), Label: act.Text, Type: act.Type, Style: act.Style, URL: act.URL, Confirm: act.Confirm,
				legacy: act, attachmentID: att.ID, callbackID: att.CallbackID, serviceID: serviceID,
			})
		}
	}
	return out
}

// errNotPressable is a press of something that is not a button of an
// ephemeral message.
var errNotPressable = errors.New("only the buttons of a reply only you can see can be pressed")

// pressable reports whether a is one of m's buttons a press can send.
func pressable(m Message, a Action) bool {
	return m.Ephemeral && a.Type == "button" && a.URL == "" &&
		a.Index >= 0 && a.Index < len(m.Actions) && m.Actions[a.Index].legacy == a.legacy
}

// PressAction presses one of an ephemeral's legacy buttons through
// chat.attachmentAction, the request the web client sends
// (slackclient.AttachmentAction). Slack answers over the socket.
func (s *Slack) PressAction(ctx context.Context, m Message, a Action) error {
	if !pressable(m, a) {
		return errNotPressable
	}
	ctx, cancel := context.WithTimeout(ctx, pressTimeout)
	defer cancel()
	return s.client.AttachmentAction(ctx, slackclient.AttachmentActionRequest{
		ChannelID:    m.ChannelID,
		MessageTS:    m.TS,
		AttachmentID: a.attachmentID,
		CallbackID:   a.callbackID,
		ServiceID:    a.serviceID,
		Ephemeral:    true,
		Action:       a.legacy,
	})
}
