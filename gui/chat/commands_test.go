package chat

import (
	"reflect"
	"testing"

	"github.com/gammons/slk/gui/backend"
)

func names(cmds []backend.Command) []string {
	var out []string
	for _, c := range cmds {
		out = append(out, c.Name)
	}
	return out
}

func TestMatchCommands(t *testing.T) {
	cmds := []backend.Command{{Name: "/me"}, {Name: "/remind"}, {Name: "/remove"}, {Name: "/re"}, {Name: "/shrug"}}
	cases := []struct {
		text string
		want []string
	}{
		{"/", []string{"/me", "/remind", "/remove", "/re", "/shrug"}},
		{"/re", []string{"/re", "/remind", "/remove"}}, // exact match first
		{"/REM", []string{"/remind", "/remove"}},       // case-insensitive
		{"/remind ", nil},                              // arguments started: popup closes
		{"/zzz", nil},
		{"hello", nil},
		{"", nil},
		{" /re", nil},
	}
	for _, tc := range cases {
		if got := names(MatchCommands(cmds, tc.text)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("MatchCommands(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestEphemeralThreadReplyDoesNotCount(t *testing.T) {
	var tl Timeline
	tl.Merge([]backend.Message{msg("1700000100.000100", "parent")}, nil)
	reply := msg("1700000150.000100", "only you can see this")
	reply.ThreadTS, reply.InThreadOnly, reply.Ephemeral = "1700000100.000100", true, true
	if tl.Apply("C1", backend.MessagePosted{Message: reply}) {
		t.Fatal("an ephemeral thread reply changed the channel")
	}
	if got := tl.Messages()[0].ReplyCount; got != 0 {
		t.Fatalf("reply count %d, want 0", got)
	}
}
