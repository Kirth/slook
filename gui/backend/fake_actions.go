package backend

import (
	"context"
	"errors"
	"strings"

	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/core/blocks"
)

// fakeStandup is a /standup preview: where it is and the notes.
type fakeStandup struct {
	channelID, threadTS, notes string
}

// fakeStandupBot is the Standup Bot's bot ID, the service a press names.
const fakeStandupBot = "B1"

// standupLocked is /standup's ephemeral: a preview of the notes with
// Send, Shuffle and Cancel (which asks first), remembered for
// PressAction. Callers hold mu.
func (f *Fake) standupLocked(channelID, threadTS, notes string) Message {
	m := f.nextLocked(channelID, "Standup Bot", "")
	m.AuthorID = fakeStandupBot
	m.Ephemeral = true
	m.ThreadTS, m.InThreadOnly = threadTS, threadTS != ""
	f.previewLocked(&m, notes)
	if f.standups == nil {
		f.standups = map[string]fakeStandup{}
	}
	f.standups[m.TS] = fakeStandup{channelID: channelID, threadTS: threadTS, notes: notes}
	return m
}

// previewLocked fills m with the preview of notes and its buttons.
func (f *Fake) previewLocked(m *Message, notes string) {
	text := "Here is your standup. Send it to the channel?"
	atts := []blocks.LegacyAttachment{{
		ID: 1, CallbackID: "standup_preview", Text: notes,
		Actions: []blocks.LegacyAction{
			{ID: "1", Name: "send", Text: "Send", Type: "button", Value: "send", Style: "primary"},
			{ID: "2", Name: "shuffle", Text: "Shuffle", Type: "button", Value: "shuffle"},
			{ID: "3", Name: "cancel", Text: "Cancel", Type: "button", Value: "cancel", Style: "danger",
				Confirm: &blocks.ActionConfirm{Title: "Cancel?", Text: "Discard the preview?"}},
		},
	}}
	m.Text = text
	m.Body = append(rich.FromMrkdwn(text, fakeNames), rich.FromAttachments(atts, fakeNames)...)
	m.Actions = messageActions(atts, fakeStandupBot)
}

// PressAction acts out the standup preview's buttons as its app would:
// Send posts the notes as Standup Bot and removes the preview, Shuffle
// replaces it with the notes reordered, Cancel removes it. Anything
// else is refused as Slack refuses it.
func (f *Fake) PressAction(ctx context.Context, m Message, a Action) error {
	if !pressable(m, a) {
		return errNotPressable
	}
	f.mu.Lock()
	st, ok := f.standups[m.TS]
	if !ok || m.ChannelID != st.channelID {
		f.mu.Unlock()
		return errors.New("invalid_action")
	}
	var evs []Event
	switch a.legacy.Value {
	case "send":
		var posted Message
		if st.threadTS != "" {
			posted = f.replyLocked(st.channelID, st.threadTS, "Standup Bot", st.notes, false)
		} else {
			posted = f.appendLocked(st.channelID, "Standup Bot", st.notes)
		}
		delete(f.standups, m.TS)
		evs = []Event{MessagePosted{Message: posted}, MessageDeleted{ChannelID: m.ChannelID, TS: m.TS}}
	case "shuffle":
		words := strings.Fields(st.notes)
		if len(words) > 1 {
			words = append(words[1:], words[0])
		}
		st.notes = strings.Join(words, " ")
		f.standups[m.TS] = st
		shuffled := m
		shuffled.Edited = true
		f.previewLocked(&shuffled, st.notes)
		evs = []Event{MessagePosted{Message: shuffled}}
	case "cancel":
		delete(f.standups, m.TS)
		evs = []Event{MessageDeleted{ChannelID: m.ChannelID, TS: m.TS}}
	default:
		f.mu.Unlock()
		return errors.New("invalid_action")
	}
	f.mu.Unlock()
	for _, ev := range evs {
		select {
		case f.events <- ev:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
