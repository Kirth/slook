package chat

import (
	"testing"

	"github.com/gammons/slk/gui/backend"
)

func TestReadGateReleasesOnBlur(t *testing.T) {
	var g ReadGate
	if !g.Open() {
		t.Fatal("the zero gate is closed")
	}
	g.Hold("1700000300.000100")
	if g.Open() {
		t.Fatal("held gate is open")
	}
	g.Arrived("1700000300.000100") // the newest again (an edit): still held
	g.Arrived("1700000200.000100") // older: still held
	if g.Open() {
		t.Fatal("an older message opened the gate")
	}
	g.Blur()
	if !g.Open() {
		t.Fatal("losing the keyboard did not open the gate")
	}
}

func TestReadGateReleasesOnNewerMessage(t *testing.T) {
	var g ReadGate
	g.Hold("1700000300.000100")
	g.Arrived("1700000300.000200")
	if !g.Open() {
		t.Fatal("a newer message did not open the gate")
	}
	g.Arrived("1700000400.000100") // open stays open
	if !g.Open() {
		t.Fatal("gate closed by itself")
	}
}

func msgs(tss ...string) []backend.Message {
	out := make([]backend.Message, len(tss))
	for i, ts := range tss {
		out[i] = backend.Message{TS: ts}
	}
	return out
}

func TestUnreadBoundary(t *testing.T) {
	ms := msgs("1.000100", "2.000100", "3.000100", "4.000100")
	ms[2].Ephemeral = true
	cases := []struct {
		ts, want string
		ok       bool
	}{
		{"1.000100", "0", true},        // the oldest: everything unread
		{"2.000100", "1.000100", true}, // the one before
		{"4.000100", "2.000100", true}, // the ephemeral between is skipped
		{"3.000100", "", false},        // an ephemeral is not Slack's
		{"9.000100", "", false},        // not shown
	}
	for _, c := range cases {
		got, ok := UnreadBoundary(ms, c.ts)
		if got != c.want || ok != c.ok {
			t.Errorf("UnreadBoundary(%s) = %q, %v; want %q, %v", c.ts, got, ok, c.want, c.ok)
		}
	}
	// Only ephemerals before it: from the start.
	ms[0].Ephemeral, ms[1].Ephemeral = true, true
	if got, ok := UnreadBoundary(ms, "4.000100"); got != "0" || !ok {
		t.Errorf("after ephemerals only: %q, %v", got, ok)
	}
}

func TestThreadUnreadBoundary(t *testing.T) {
	replies := msgs("2.000100", "3.000100")
	if got, ok := ThreadUnreadBoundary("1.000100", replies, "2.000100"); got != "1.000100" || !ok {
		t.Errorf("first reply: %q, %v; want the parent", got, ok)
	}
	if got, ok := ThreadUnreadBoundary("1.000100", replies, "3.000100"); got != "2.000100" || !ok {
		t.Errorf("second reply: %q, %v", got, ok)
	}
	if _, ok := ThreadUnreadBoundary("1.000100", replies, "1.000100"); ok {
		t.Error("the parent can be marked unread from")
	}
	if _, ok := ThreadUnreadBoundary("1.000100", replies, "5.000100"); ok {
		t.Error("a reply not shown can be marked unread from")
	}
}
