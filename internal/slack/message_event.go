package slackclient

import (
	"encoding/json"

	"github.com/slack-go/slack"
)

// MessageEventKind says what a MessageEvent reports.
type MessageEventKind int

const (
	// MessageNew is a message posted (subtype "", bot_message,
	// thread_broadcast, file_share, me_message).
	MessageNew MessageEventKind = iota
	// MessageChanged is a message_changed: an edit, or the server
	// updating a thread parent's reply_count / thread_ts.
	MessageChanged
	// MessageDeleted is a message_deleted.
	MessageDeleted
)

// MessageEvent is a message frame decoded whole, for receivers that
// cache messages. The base EventHandler.OnMessage flattens a frame to a
// few fields and drops the rest; a message_changed carries, among what
// it drops, the parent's new reply_count and thread_ts and the edit's
// timestamp, which is exactly what a cache must apply.
type MessageEvent struct {
	ChannelID string
	Kind      MessageEventKind
	// Message is the message object: the frame itself for MessageNew,
	// its `message` sub-object for MessageChanged (with Channel filled
	// in). For MessageDeleted only Channel and Timestamp (deleted_ts)
	// are set.
	Message slack.Message
	// Raw is that message object's JSON exactly as received, so a cache
	// can keep the server's own representation rather than rebuild one.
	// Nil for MessageDeleted.
	Raw json.RawMessage
	// EventTS is the frame's event_ts, when present.
	EventTS string
}

// MessageEventHandler is an EventHandler that wants message frames
// whole. When the handler given to StartWebSocket implements it, the
// dispatcher calls OnMessageEvent for new, changed and deleted messages
// instead of OnMessage / OnMessageDeleted. Ephemeral messages still go
// to OnEphemeralMessage: Slack never returns them from history, so a
// cache must never see them.
type MessageEventHandler interface {
	OnMessageEvent(ev MessageEvent)
}

// decodeMessageEvent builds the MessageEvent for a message frame already
// decoded as msg. false means the frame could not be decoded whole; the
// dispatcher then falls back to the flattened OnMessage delivery.
func decodeMessageEvent(data []byte, msg wsMessageEvent) (MessageEvent, bool) {
	ev := MessageEvent{ChannelID: msg.Channel, EventTS: msg.EventTS}
	switch msg.SubType {
	case "message_changed":
		var frame struct {
			Message json.RawMessage `json:"message"`
		}
		if err := json.Unmarshal(data, &frame); err != nil || len(frame.Message) == 0 {
			return ev, false
		}
		var inner slack.Msg
		if err := json.Unmarshal(frame.Message, &inner); err != nil {
			return ev, false
		}
		inner.Channel = msg.Channel
		ev.Kind = MessageChanged
		ev.Message = slack.Message{Msg: inner}
		ev.Raw = frame.Message
	case "message_deleted":
		ev.Kind = MessageDeleted
		ev.Message = slack.Message{Msg: slack.Msg{Channel: msg.Channel, Timestamp: msg.DeletedTS}}
	default:
		var m slack.Message
		if err := json.Unmarshal(data, &m); err != nil {
			return ev, false
		}
		ev.Kind = MessageNew
		ev.Message = m
		ev.Raw = append(json.RawMessage(nil), data...)
	}
	return ev, true
}
