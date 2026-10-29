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
