package chat

import (
	"testing"

	"github.com/gammons/slk/gui/backend"
)

func TestEditableText(t *testing.T) {
	cases := map[string]string{
		"a &lt; b &amp;&amp; c &gt; d":        "a < b && c > d",
		"hi <@U123> see <#C1|general>":        "hi <@U123> see <#C1|general>",
		"<https://example.com|docs> &amp;lt;": "<https://example.com|docs> &lt;",
	}
	for in, want := range cases {
		if got := EditableText(in); got != want {
			t.Errorf("EditableText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLastOwn(t *testing.T) {
	msgs := []backend.Message{
		{TS: "1", FromSelf: true},
		{TS: "2"},
		{TS: "3", FromSelf: true, Ephemeral: true},
		{TS: "4"},
	}
	if m, ok := LastOwn(msgs); !ok || m.TS != "1" {
		t.Fatalf("got %+v, %v; want ts 1", m, ok)
	}
	if _, ok := LastOwn(msgs[1:]); ok {
		t.Fatal("found an own message among others' and ephemerals")
	}
}

func TestForwardedLink(t *testing.T) {
	const link = "https://acme.slack.com/archives/C1/p1700000000000100"
	for _, tc := range []struct {
		text string
		want string
	}{
		{"<" + link + ">", link},
		{"  <" + link + ">\n", link},
		{"<" + link + "?thread_ts=1700000000.000000&cid=C1>", link + "?thread_ts=1700000000.000000&cid=C1"},
		{"<" + link + "|a message>", link},
		{"<" + link + "?thread_ts=1700000000.000000&amp;cid=C1>", link + "?thread_ts=1700000000.000000&cid=C1"},
		{"fyi\n<" + link + ">", ""},
		{"<" + link + "> <" + link + ">", ""},
		{"<https://example.com/x>", ""},
		{"<https://acme.slack.com/archives/C1>", ""},
		{link, ""},
		{"", ""},
	} {
		got, ok := ForwardedLink(tc.text)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("ForwardedLink(%q) = %q, %v; want %q", tc.text, got, ok, tc.want)
		}
	}
}

func TestButtonActionsSkipSelects(t *testing.T) {
	m := backend.Message{Actions: []backend.Action{
		{Index: 0, Label: "Pick", Type: "select"},
		{Index: 1, Label: "Send", Type: "button"},
		{Index: 2, Label: "Cancel", Type: "button"},
	}}
	got := ButtonActions(m)
	if len(got) != 2 || got[0].Label != "Send" || got[1].Label != "Cancel" {
		t.Fatalf("ButtonActions = %+v", got)
	}
	if !HasButtonActions(m) {
		t.Fatal("HasButtonActions = false")
	}
	m.Actions = m.Actions[:1]
	if HasButtonActions(m) || ButtonActions(m) != nil {
		t.Fatal("a menu alone counted as a button")
	}
	if HasButtonActions(backend.Message{}) {
		t.Fatal("a message without actions has buttons")
	}
}
