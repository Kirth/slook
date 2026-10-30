package chat

import (
	"slices"
	"testing"

	"github.com/gammons/slk/gui/backend"
)

func TestTrigger(t *testing.T) {
	cases := []struct {
		text  string
		caret int // -1: the end
		sigil rune
		query string
		start int
		ok    bool
	}{
		{"@ad", -1, '@', "ad", 0, true},
		{"hi @", -1, '@', "", 3, true},
		{"hi @ada lov", -1, '@', "ada lov", 3, true},
		{"hi @ada lovelace and", -1, 0, "", 0, false}, // two spaces: done
		{"see #gen", -1, '#', "gen", 4, true},
		{"see #gen x", -1, 0, "", 0, false},
		{"party :ta", -1, ':', "ta", 6, true},
		{"party :t", -1, 0, "", 0, false}, // one character
		{"re: x", -1, 0, "", 0, false},
		{"foo@bar", -1, 0, "", 0, false},
		{"a:bc", -1, 0, "", 0, false},
		{"(@gr", -1, '@', "gr", 1, true},
		{"@ada and more", 4, '@', "ada", 0, true}, // caret mid-text
		{"line\n@x", -1, '@', "x", 5, true},
		{"@x\nmore", -1, 0, "", 0, false},
	}
	for _, c := range cases {
		caret := c.caret
		if caret < 0 {
			caret = len([]rune(c.text))
		}
		sigil, q, start, ok := Trigger(c.text, caret)
		if ok != c.ok || (ok && (sigil != c.sigil || q != c.query || start != c.start)) {
			t.Errorf("Trigger(%q, %d) = %q %q %d %v, want %q %q %d %v", c.text, caret, sigil, q, start, ok, c.sigil, c.query, c.start, c.ok)
		}
	}
}

var people = []backend.Person{
	{ID: "U1", Name: "Ada Lovelace", RealName: "Augusta Ada King", Handle: "ada.lovelace"},
	{ID: "U2", Name: "Grace Hopper", Handle: "grace"},
	{ID: "U3", Name: "Zoë Adams", Handle: "zoe"},
	{ID: "B1", Name: "Adabot", Handle: "adabot", Bot: true},
	{ID: "U1", Name: "Ada Lovelace"}, // a duplicate
}

func labels(cs []Choice) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Label)
	}
	return out
}

func TestMatchPeople(t *testing.T) {
	got := MatchPeople("ada", people, 10)
	if want := []string{"Ada Lovelace", "Zoë Adams", "Adabot"}; !slices.Equal(labels(got), want) {
		t.Fatalf("ada: %v, want %v", labels(got), want)
	}
	if got[0].Insert != "@Ada Lovelace" || got[0].Wire != "<@U1>" || got[0].ID != "U1" {
		t.Fatalf("choice %+v", got[0])
	}
	if got := MatchPeople("zoe", people, 10); len(got) != 1 || got[0].Label != "Zoë Adams" {
		t.Fatalf("accent-blind: %v", labels(got))
	}
	if got := MatchPeople("augusta", people, 10); len(got) != 1 || got[0].Label != "Ada Lovelace" {
		t.Fatalf("real name: %v", labels(got))
	}
	if got := MatchPeople("he", people, 10); len(got) == 0 || got[0].Wire != "<!here>" {
		t.Fatalf("special: %v", labels(got))
	}
	if got := MatchPeople("", people, 2); len(got) != 2 {
		t.Fatalf("limit: %v", labels(got))
	}
}

func TestMatchChannels(t *testing.T) {
	convs := []backend.Conversation{
		{ID: "C1", Name: "general", Kind: backend.KindChannel},
		{ID: "C2", Name: "eng-general", Kind: backend.KindPrivate},
		{ID: "D1", Name: "general bot", Kind: backend.KindDM},
	}
	got := MatchChannels("gen", convs, 10)
	if !slices.Equal(labels(got), []string{"general", "eng-general"}) {
		t.Fatalf("got %v", labels(got))
	}
	if got[0].Insert != "#general" || got[0].Wire != "<#C1|general>" || got[1].Detail != "private" {
		t.Fatalf("choices %+v", got)
	}
}

func TestMentionsWire(t *testing.T) {
	m := Mentions{"@Ada Lovelace": "<@U1>", "@Ada": "<@U9>", "#general": "<#C1|general>", "@here": "<!here>"}
	cases := map[string]string{
		"hi @Ada Lovelace, see #general": "hi <@U1>, see <#C1|general>",
		"@Ada and @Ada Lovelace":         "<@U9> and <@U1>",
		"mail me@Ada Lovelace":           "mail me@Ada Lovelace",
		"#generalist":                    "#generalist",
		"@here please":                   "<!here> please",
		"nothing":                        "nothing",
	}
	for in, want := range cases {
		if got := m.Wire(in); got != want {
			t.Errorf("Wire(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadableRoundTrip(t *testing.T) {
	names := func(id string) (string, bool) {
		n, ok := map[string]string{"U1": "Ada Lovelace", "C1": "general"}[id]
		return n, ok
	}
	m := Mentions{}
	wire := "hi <@U1> in <#C1|general>, <!here>; <@U77> unknown"
	read := Readable(wire, names, m)
	if read != "hi @Ada Lovelace in #general, @here; <@U77> unknown" {
		t.Fatalf("readable %q", read)
	}
	if back := m.Wire(read); back != wire {
		t.Fatalf("round trip %q, want %q", back, wire)
	}
}
