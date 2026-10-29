package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/gammons/slk/gui/backend"
)

func TestThreadTargets(t *testing.T) {
	a := threadKey{"C1", "100.000001"}
	b := threadKey{"C1", "200.000001"}
	other := threadKey{"C2", "100.000001"}
	open := []threadKey{a, b, other}
	cases := []struct {
		name string
		ev   backend.Event
		want []threadKey
	}{
		{"a reply reaches its thread", backend.MessagePosted{Message: backend.Message{ChannelID: "C1", TS: "150.000001", ThreadTS: a.ts}}, []threadKey{a}},
		{"an edited parent reaches its thread", backend.MessagePosted{Message: backend.Message{ChannelID: "C1", TS: b.ts}}, []threadKey{b}},
		{"same ts in another channel is another thread", backend.MessagePosted{Message: backend.Message{ChannelID: "C2", TS: "150.000001", ThreadTS: a.ts}}, []threadKey{other}},
		{"a plain message reaches none", backend.MessagePosted{Message: backend.Message{ChannelID: "C1", TS: "300.000001"}}, nil},
		{"a deletion reaches the channel's threads", backend.MessageDeleted{ChannelID: "C1", TS: "150.000001"}, []threadKey{a, b}},
		{"a catch-up reaches the channel's threads", backend.HistoryUpdated{ChannelID: "C2"}, []threadKey{other}},
		{"the connection reaches all", backend.ConnectionChanged{Connected: true}, open},
		{"read state reaches none", backend.ReadChanged{ChannelID: "C1"}, nil},
		{"a reaction reaches the channel's threads", backend.ReactionChanged{ChannelID: "C1", TS: "150.000001", Name: "+1"}, []threadKey{a, b}},
	}
	for _, tc := range cases {
		got := threadTargets(tc.ev, open)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestThreadFocusKeyIsNeverAChannel(t *testing.T) {
	k := threadKey{"C1", "100.000001"}
	if k.focusKey() == k.channelID || !strings.HasPrefix(k.focusKey(), "C1/") {
		t.Fatalf("focus key %q", k.focusKey())
	}
}
