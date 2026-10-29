package slackclient

import (
	"bytes"
	"encoding/json"
	"testing"
)

// messageEventRecorder is a mockEventHandler that also wants message
// frames whole.
type messageEventRecorder struct {
	mockEventHandler
	events []MessageEvent
}

func (r *messageEventRecorder) OnMessageEvent(ev MessageEvent) { r.events = append(r.events, ev) }

var _ MessageEventHandler = (*messageEventRecorder)(nil)

func TestMessageEvent_NewCarriesTheWholeFrame(t *testing.T) {
	h := &messageEventRecorder{}
	data := []byte(`{"type":"message","channel":"C1","user":"U1","text":"hi","ts":"200.0","thread_ts":"100.0","subtype":"thread_broadcast","reply_count":0,"event_ts":"200.0","files":[{"id":"F1","name":"a.png"}]}`)
	dispatchWebSocketEvent(data, h)

	if len(h.messages) != 0 {
		t.Fatalf("flattened OnMessage was also called: %v", h.messages)
	}
	if len(h.events) != 1 {
		t.Fatalf("got %d events, want 1", len(h.events))
	}
	ev := h.events[0]
	if ev.Kind != MessageNew || ev.ChannelID != "C1" || ev.EventTS != "200.0" {
		t.Errorf("event header = %+v", ev)
	}
	m := ev.Message
	if m.Timestamp != "200.0" || m.ThreadTimestamp != "100.0" || m.SubType != "thread_broadcast" || m.User != "U1" || len(m.Files) != 1 || m.Files[0].ID != "F1" {
		t.Errorf("message = %+v", m.Msg)
	}
	if !bytes.Equal(ev.Raw, data) {
		t.Errorf("Raw is not the frame: %s", ev.Raw)
	}
}

func TestMessageEvent_ChangedCarriesTheInnerMessage(t *testing.T) {
	h := &messageEventRecorder{}
	inner := `{"user":"U1","text":"edited","ts":"100.0","thread_ts":"100.0","reply_count":3,"latest_reply":"150.0","subtype":"bot_message","edited":{"user":"U1","ts":"160.0"}}`
	data := []byte(`{"type":"message","subtype":"message_changed","channel":"C1","event_ts":"160.1","message":` + inner + `,"previous_message":{"text":"original","ts":"100.0"}}`)
	dispatchWebSocketEvent(data, h)

	if len(h.messages) != 0 {
		t.Fatalf("flattened OnMessage was also called: %v", h.messages)
	}
	if len(h.events) != 1 {
		t.Fatalf("got %d events, want 1", len(h.events))
	}
	ev := h.events[0]
	if ev.Kind != MessageChanged || ev.ChannelID != "C1" || ev.EventTS != "160.1" {
		t.Errorf("event header = %+v", ev)
	}
	m := ev.Message
	if m.Channel != "C1" || m.Timestamp != "100.0" || m.Text != "edited" || m.ReplyCount != 3 || m.LatestReply != "150.0" || m.SubType != "bot_message" || m.ThreadTimestamp != "100.0" {
		t.Errorf("inner message = %+v", m.Msg)
	}
	if m.Edited == nil || m.Edited.Timestamp != "160.0" {
		t.Errorf("edited = %+v, want ts 160.0", m.Edited)
	}
	var want, got bytes.Buffer
	if err := json.Compact(&want, []byte(inner)); err != nil {
		t.Fatal(err)
	}
	if err := json.Compact(&got, ev.Raw); err != nil {
		t.Fatalf("Raw is not JSON: %v", err)
	}
	if !bytes.Equal(got.Bytes(), want.Bytes()) {
		t.Errorf("Raw = %s, want the inner message %s", got.Bytes(), want.Bytes())
	}
}

func TestMessageEvent_Deleted(t *testing.T) {
	h := &messageEventRecorder{}
	dispatchWebSocketEvent([]byte(`{"type":"message","subtype":"message_deleted","channel":"C1","deleted_ts":"123.456","event_ts":"124.0"}`), h)

	if len(h.deletedMessages) != 0 {
		t.Fatalf("OnMessageDeleted was also called: %v", h.deletedMessages)
	}
	if len(h.events) != 1 {
		t.Fatalf("got %d events, want 1", len(h.events))
	}
	ev := h.events[0]
	if ev.Kind != MessageDeleted || ev.ChannelID != "C1" || ev.Message.Timestamp != "123.456" || ev.Message.Channel != "C1" || ev.Raw != nil {
		t.Errorf("event = %+v", ev)
	}
}

func TestMessageEvent_EphemeralStillGoesToOnEphemeralMessage(t *testing.T) {
	h := &messageEventRecorder{}
	dispatchWebSocketEvent([]byte(`{"type":"message","channel":"C1","user":"USLACKBOT","text":"only you","ts":"300.0","is_ephemeral":true}`), h)
	dispatchWebSocketEvent([]byte(`{"type":"message","subtype":"message_changed","channel":"C1","message":{"text":"shuffled","ts":"300.0","is_ephemeral":true}}`), h)

	if len(h.events) != 0 {
		t.Fatalf("an ephemeral reached OnMessageEvent: %+v", h.events)
	}
	if len(h.ephemerals) != 1 || h.ephemerals[0].Text != "only you" {
		t.Errorf("ephemerals = %+v, want the one notice", h.ephemerals)
	}
}

func TestMessageEvent_PlainHandlerKeepsFlattenedDelivery(t *testing.T) {
	h := &mockEventHandler{}
	dispatchWebSocketEvent([]byte(`{"type":"message","channel":"C1","user":"U1","text":"hi","ts":"200.0"}`), h)
	if len(h.messages) != 1 || h.messages[0] != "hi" {
		t.Fatalf("messages = %v, want [hi]", h.messages)
	}
}
