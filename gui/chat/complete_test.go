package chat

import (
	"slices"
	"testing"
	"time"

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
	got := MatchPeople("ada", people, nil, 10)
	if want := []string{"Ada Lovelace", "Zoë Adams", "Adabot"}; !slices.Equal(labels(got), want) {
		t.Fatalf("ada: %v, want %v", labels(got), want)
	}
	if got[0].Insert != "@Ada Lovelace" || got[0].Wire != "<@U1>" || got[0].ID != "U1" {
		t.Fatalf("choice %+v", got[0])
	}
	if got := MatchPeople("zoe", people, nil, 10); len(got) != 1 || got[0].Label != "Zoë Adams" {
		t.Fatalf("accent-blind: %v", labels(got))
	}
	if got := MatchPeople("augusta", people, nil, 10); len(got) != 1 || got[0].Label != "Ada Lovelace" {
		t.Fatalf("real name: %v", labels(got))
	}
	if got := MatchPeople("he", people, nil, 10); len(got) == 0 || got[0].Wire != "<!here>" {
		t.Fatalf("special: %v", labels(got))
	}
	if got := MatchPeople("", people, nil, 2); len(got) != 2 {
		t.Fatalf("limit: %v", labels(got))
	}
}

func TestMatchPeopleOffersGroups(t *testing.T) {
	groups := []backend.UserGroup{
		{ID: "S1", Handle: "ada-fans", Name: "Ada's fans", Members: []string{"U1", "U2"}},
		{ID: "S2", Handle: "design", Name: "Design"},
		{ID: "S3", Handle: "ops", Name: "Platform and Ada support"},
	}
	got := MatchPeople("ada", people, groups, 10)
	if want := []string{"@ada-fans", "Ada Lovelace", "Zoë Adams", "Adabot", "@ops"}; !slices.Equal(labels(got), want) {
		t.Fatalf("ada: %v, want %v", labels(got), want)
	}
	if c := got[0]; c.Insert != "@ada-fans" || c.Wire != "<!subteam^S1|@ada-fans>" || c.Detail != "Ada's fans · 2 members" {
		t.Fatalf("group choice %+v", c)
	}
	m := Mentions{c0(got).Insert: c0(got).Wire}
	if w := m.Wire("cc @ada-fans please"); w != "cc <!subteam^S1|@ada-fans> please" {
		t.Fatalf("wire %q", w)
	}
}

func c0(cs []Choice) Choice { return cs[0] }

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
		n, ok := map[string]string{"U1": "Ada Lovelace", "C1": "general", "S2": "design"}[id]
		return n, ok
	}
	m := Mentions{}
	wire := "hi <@U1> in <#C1|general>, <!here>; <@U77> unknown, <!subteam^S1|@eng> and <!subteam^S2>, <!subteam^S9>"
	read := Readable(wire, names, m)
	if read != "hi @Ada Lovelace in #general, @here; <@U77> unknown, @eng and @design, <!subteam^S9>" {
		t.Fatalf("readable %q", read)
	}
	if back := m.Wire(read); back != wire {
		t.Fatalf("round trip %q, want %q", back, wire)
	}
}

func TestBestPerson(t *testing.T) {
	people := []backend.Person{
		{ID: "U1", Name: "Ada Lovelace", Handle: "ada"},
		{ID: "U2", Name: "Adam", Handle: "adam"},
		{ID: "U3", Name: "Grace Hopper", Handle: "grace"},
		{ID: "B1", Name: "Adabot", Bot: true},
		{ID: "U4", Name: "Linus", RealName: "Linus Torvalds"},
	}
	for _, c := range []struct{ q, want string }{
		{"ada", "U2"},      // both start with it: the shorter name
		{"@ada lov", "U1"}, // the full name
		{"lovelace", "U1"}, // a word of it
		{"adlv", "U1"},     // letters in order
		{"hopp", "U3"},     // a word
		{"torv", "U4"},     // the real name
		{"ADABOT", "B1"},   // a bot when only it matches
		{"race", "U3"},     // contains
		{"lnt", "U4"},      // three letters in order
		{"lt", ""},         // two in order are too few to mean anything
		{"zzz", ""},
		{"", ""},
	} {
		p, ok := BestPerson(c.q, people)
		if ok != (c.want != "") || p.ID != c.want {
			t.Errorf("BestPerson(%q) = %q %v, want %q", c.q, p.ID, ok, c.want)
		}
	}
}

func TestBestConversation(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	convs := []backend.Conversation{
		{ID: "C1", Name: "general", Kind: backend.KindChannel, Latest: now.Add(-time.Hour)},
		{ID: "C2", Name: "gen-ai", Kind: backend.KindChannel, Latest: now},
		{ID: "C3", Name: "eng-platform", Kind: backend.KindPrivate},
	}
	for _, c := range []struct{ q, want string }{
		{"#gen", "C2"}, // both start with it: the more recently active
		{"genera", "C1"},
		{"platform", "C3"}, // a word
		{"engpl", "C3"},    // letters in order
		{"nope", ""},
	} {
		got, ok := BestConversation(c.q, convs)
		if ok != (c.want != "") || got.ID != c.want {
			t.Errorf("BestConversation(%q) = %q %v, want %q", c.q, got.ID, ok, c.want)
		}
	}
	g, ok := BestGroup("@eng", []backend.UserGroup{{ID: "S1", Handle: "engineering"}, {ID: "S2", Handle: "design"}})
	if !ok || g.ID != "S1" {
		t.Fatalf("BestGroup: %v %v", g, ok)
	}
}
