package search

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/internal/searchq"
)

func testSources() Sources {
	return Sources{
		People: []backend.Person{
			{ID: "U0", Name: "Kirth"},
			{ID: "U1", Name: "Ada Lovelace", Handle: "ada"},
			{ID: "U2", Name: "Grace Hopper", Handle: "grace"},
		},
		Groups: []backend.UserGroup{{ID: "S1", Handle: "eng", Name: "Engineering"}},
		Convs: []backend.Conversation{
			{ID: "C1", Name: "general", Kind: backend.KindChannel},
			{ID: "C2", Name: "random", Kind: backend.KindChannel},
			{ID: "G1", Name: "secret", Kind: backend.KindPrivate},
			{ID: "D1", Name: "Ada Lovelace", Kind: backend.KindDM, PeerID: "U1"},
		},
		Self: "U0",
	}
}

func chipTexts(chips []Chip) string {
	var out []string
	for _, c := range chips {
		s := c.Text
		if c.Err {
			s = "!" + s
		}
		out = append(out, s)
	}
	return strings.Join(out, " | ")
}

// now is Wednesday 28 October 2026, 15:04 UTC.
var now = time.Date(2026, 10, 28, 15, 4, 0, 0, time.UTC)

func TestResolve(t *testing.T) {
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	for _, c := range []struct {
		in    string
		want  backend.SearchQuery
		chips string
	}{
		{"deploy staging", backend.SearchQuery{All: [][]string{{"deploy"}, {"staging"}}}, ""},
		{`"build failed" !staging`, backend.SearchQuery{All: [][]string{{"build failed"}}, None: []string{"staging"}}, "without “staging”"},
		{"red | green", backend.SearchQuery{All: [][]string{{"red", "green"}}}, "“red” or “green”"},
		{"from:adl", backend.SearchQuery{Users: []string{"U1"}}, "from Ada Lovelace"},
		{"from:ada from:grace", backend.SearchQuery{Users: []string{"U1", "U2"}}, "from Ada Lovelace | from Grace Hopper"},
		{"from:me", backend.SearchQuery{Users: []string{"U0"}}, "from Kirth"},
		{"!from:grace", backend.SearchQuery{NotUsers: []string{"U2"}}, "not from Grace Hopper"},
		{"from:zzz deploy", backend.SearchQuery{All: [][]string{{"deploy"}}, Nothing: true}, "!no one matches “zzz”"},
		{"from:", backend.SearchQuery{}, ""},
		{"in:#gen", backend.SearchQuery{Channels: []string{"C1"}}, "in #general"},
		{"in:@ada", backend.SearchQuery{Channels: []string{"D1"}}, "in your DM with Ada Lovelace"},
		{"!in:random", backend.SearchQuery{NotChannels: []string{"C2"}}, "not in #random"},
		{"in:general is:dm", backend.SearchQuery{Nothing: true}, "in #general | in direct messages"},
		{"is:channel", backend.SearchQuery{Channels: []string{"C1", "C2", "G1"}}, "in channels"},
		{"!is:dm", backend.SearchQuery{NotChannels: []string{"D1"}}, "not in direct messages"},
		{"mentions:me", backend.SearchQuery{Mentions: []string{"<@U0>"}}, "mentions you"},
		{"mentions:@eng", backend.SearchQuery{Mentions: []string{"<!subteam^S1"}}, "mentions @eng"},
		{"mentions:grace", backend.SearchQuery{Mentions: []string{"<@U2>"}}, "mentions Grace Hopper"},
		{"is:thread !is:edited is:wat", backend.SearchQuery{Thread: backend.Yes, Edited: backend.No, Nothing: true},
			"a thread | not edited | !is: takes dm, channel, thread, reply, edited"},
		{"has:link !has:reaction has::thumbsup:", backend.SearchQuery{Link: backend.Yes, Reaction: backend.No, Emoji: []string{"+1"}},
			"with a link | without a reaction | with :+1:"},
		{"on:monday", backend.SearchQuery{Since: day(10, 26), Until: day(10, 27)}, "on Mon 26 Oct"},
		{"during:october", backend.SearchQuery{Since: day(10, 1), Until: day(11, 1)}, "during October 2026"},
		{"after:2026-10-01 before:last-week", backend.SearchQuery{Since: day(10, 2), Until: day(10, 19)},
			"after Thu 1 Oct | before the week of Mon 19 Oct"},
		{"since:3d", backend.SearchQuery{Since: now.AddDate(0, 0, -3)}, "in the last 3 days"},
		{"before:2w", backend.SearchQuery{Until: now.AddDate(0, 0, -14)}, "before 2 weeks ago"},
		{"after:today before:yesterday", backend.SearchQuery{Since: day(10, 29), Until: day(10, 27), Nothing: true},
			"after Wed 28 Oct | before Tue 27 Oct"},
		{"on:soon", backend.SearchQuery{Nothing: true}, "!“soon” is not a time"},
	} {
		got, chips := Resolve(searchq.Parse(c.in), testSources(), now)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.in, got, c.want)
		}
		if s := chipTexts(chips); s != c.chips {
			t.Errorf("%s: chips %q, want %q", c.in, s, c.chips)
		}
	}
}

func TestResolveChipSpans(t *testing.T) {
	q := `deploy from:"ada l"`
	_, chips := Resolve(searchq.Parse(q), testSources(), now)
	if len(chips) != 1 || chips[0].Span != [2]int{7, 19} {
		t.Fatalf("chips %+v", chips)
	}
}

func TestHighlightIgnoresFilters(t *testing.T) {
	var hit []string
	for _, p := range Highlight("ada said from the deploy", `from:ada deploy !said`) {
		if p.Hit {
			hit = append(hit, p.Text)
		}
	}
	if !slices.Equal(hit, []string{"deploy"}) {
		t.Fatalf("hits %v", hit)
	}
}
