package search

import (
	"strings"
	"testing"
)

func TestSuggest(t *testing.T) {
	src := testSources()
	inserts := func(c Suggestion) string {
		var out []string
		for _, ch := range c.Choices {
			out = append(out, ch.Insert)
		}
		return strings.Join(out, " ")
	}
	for _, c := range []struct {
		q          string
		want       string
		start, end int
	}{
		{"deploy fr", "from:", 7, 9},
		{"f", "", 0, 0},                       // one letter is too few
		{"mo", "", 0, 0},                      // no filter starts so
		{"deploy is", "is:", 7, 9},            // a word that may be a filter
		{"from:ad", `"Ada Lovelace"`, 5, 7},   // a name with a space comes quoted
		{"from:m", "me", 5, 6},                // me
		{`from:"gra`, `"Grace Hopper"`, 5, 9}, // an open quote is replaced
		{"mentions:en", "@eng", 9, 11},        // usergroups for mentions
		{"from:en", "", 0, 0},                 // but not for from
		{"in:gen", "#general", 3, 6},          // a channel
		{"in:ada", `"@Ada Lovelace"`, 3, 6},   // a DM
		{"is:th", "thread", 3, 5},
		{"has:", "link file reaction", 4, 4},
		{"has::tad", ":tada: :stadium:", 4, 8},
		{"since:3", "3h 3d 3w 3m 3y", 6, 7},
		{"on:last", "last-week last-month last-year", 3, 7},
		{"is:thread", "", 0, 0},       // already complete
		{"from:ada deploy", "", 0, 0}, // the caret is in a plain word
	} {
		got := Suggest(c.q, len([]rune(c.q)), src, 8)
		if s := inserts(got); s != c.want || (c.want != "" && (got.Start != c.start || got.End != c.end)) {
			t.Errorf("Suggest(%q) = %q [%d,%d), want %q [%d,%d)", c.q, s, got.Start, got.End, c.want, c.start, c.end)
		}
	}
	if got := Suggest("from:ad deploy", 7, src, 8); inserts(got) != `"Ada Lovelace"` {
		t.Errorf("caret mid-query: %+v", got)
	}
	if got := Suggest("from:ada", 6, src, 8); got.Choices != nil {
		t.Errorf("caret inside a value: %+v", got)
	}
}
