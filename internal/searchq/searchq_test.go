package searchq

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// brief writes a query's clauses compactly, for comparing.
func brief(q Query) string {
	var parts []string
	var one func(c Clause) string
	one = func(c Clause) string {
		s := ""
		if c.Neg {
			s = "!"
		}
		switch c.Kind {
		case Word:
			s += "w(" + c.Value + ")"
		case Phrase:
			s += "p(" + c.Value + ")"
		case Or:
			var alts []string
			for _, a := range c.Alts {
				alts = append(alts, one(a))
			}
			s += "or[" + strings.Join(alts, ",") + "]"
		default:
			s += c.Key + "(" + c.Value + ")"
			if c.Quoted {
				s += "q"
			}
		}
		return s
	}
	for _, c := range q.Clauses {
		parts = append(parts, one(c))
	}
	return strings.Join(parts, " ")
}

func TestParse(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"deploy", "w(deploy)"},
		{"deploy staging", "w(deploy) w(staging)"},
		{`"the build broke"`, "p(the build broke)"},
		{`"unclosed phrase`, "p(unclosed phrase)"},
		{"!staging -prod", "!w(staging) !w(prod)"},
		{`!"not this"`, "!p(not this)"},
		{"- alone", "w(-) w(alone)"},
		{"a | b", "or[w(a),w(b)]"},
		{"a OR b OR c", "or[w(a),w(b),w(c)]"},
		{"a or b", "w(a) w(or) w(b)"},
		{`a | "two words"`, "or[w(a),p(two words)]"},
		{"a|b", "or[w(a),w(b)]"},
		{"| a", "w(a)"},
		{"a |", "w(a)"},
		{"!a | b", "!w(a) w(b)"},
		{"from:ada", "from(ada)"},
		{"From:Ada", "from(Ada)"},
		{`from:"ada lovelace" in:#general`, "from(ada lovelace)q in(#general)"},
		{"!from:bob", "!from(bob)"},
		{"from:", "from()"},
		{"during:october on:monday", "during(october) on(monday)"},
		{"has::tada: is:thread", "has(:tada:) is(thread)"},
		{"http://example.com", "w(http://example.com)"},
		{"note:this", "w(note:this)"},
		{`after:"last week"`, "after(last week)q"},
		{"from:ada deploy !staging since:3d", "from(ada) w(deploy) !w(staging) since(3d)"},
	} {
		if got := brief(Parse(c.in)); got != c.want {
			t.Errorf("Parse(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestParseSpans(t *testing.T) {
	q := Parse(`deploy !from:"ada l" a | b`)
	want := [][2]int{{0, 6}, {7, 20}, {21, 26}}
	var got [][2]int
	for _, c := range q.Clauses {
		got = append(got, c.Span)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans %v, want %v", got, want)
	}
	if v := q.Clauses[1].ValueAt; v != 14 {
		t.Fatalf("ValueAt %d, want 14 (inside the quote)", v)
	}
	if c, ok := q.ClauseAt(26); !ok || c.Kind != Or {
		t.Fatalf("ClauseAt(26) = %+v, %v", c, ok)
	}
	if _, ok := Parse("a  b").ClauseAt(2); ok {
		t.Fatal("between clauses is in none")
	}
}

func TestTextTerms(t *testing.T) {
	got := Parse(`deploy "the build" !staging from:ada x | y`).TextTerms()
	want := []string{"deploy", "the", "build", "x", "y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TextTerms %v, want %v", got, want)
	}
	if Parse("from:ada since:2d").HasText() {
		t.Fatal("filters alone have no text")
	}
	if !Parse("!staging").HasText() {
		t.Fatal("an exclusion is text")
	}
}

func TestParseTime(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	// Wednesday 2026-10-28, 15:04 local.
	now := time.Date(2026, 10, 28, 15, 4, 0, 0, loc)
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, loc) }
	for _, c := range []struct {
		in         string
		start, end time.Time
	}{
		{"today", day(2026, 10, 28), day(2026, 10, 29)},
		{"Yesterday", day(2026, 10, 27), day(2026, 10, 28)},
		{"wednesday", day(2026, 10, 28), day(2026, 10, 29)},
		{"thu", day(2026, 10, 22), day(2026, 10, 23)},
		{"monday", day(2026, 10, 26), day(2026, 10, 27)},
		{"this-week", day(2026, 10, 26), day(2026, 11, 2)},
		{"last week", day(2026, 10, 19), day(2026, 10, 26)},
		{"this-month", day(2026, 10, 1), day(2026, 11, 1)},
		{"last-month", day(2026, 9, 1), day(2026, 10, 1)},
		{"this-year", day(2026, 1, 1), day(2027, 1, 1)},
		{"last-year", day(2025, 1, 1), day(2026, 1, 1)},
		{"october", day(2026, 10, 1), day(2026, 11, 1)},
		{"dec", day(2025, 12, 1), day(2026, 1, 1)},
		{"2026-02-28", day(2026, 2, 28), day(2026, 3, 1)},
		{"2025-12", day(2025, 12, 1), day(2026, 1, 1)},
		{"2024", day(2024, 1, 1), day(2025, 1, 1)},
	} {
		s, err := ParseTime(c.in, now)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if !s.Start.Equal(c.start) || !s.End.Equal(c.end) || s.Relative {
			t.Errorf("%s: [%v, %v) relative=%v, want [%v, %v)", c.in, s.Start, s.End, s.Relative, c.start, c.end)
		}
	}
	for _, c := range []struct {
		in    string
		start time.Time
	}{
		{"4h", now.Add(-4 * time.Hour)},
		{"3d", now.AddDate(0, 0, -3)},
		{"2w", now.AddDate(0, 0, -14)},
		{"6m", now.AddDate(0, -6, 0)},
		{"1y", now.AddDate(-1, 0, 0)},
	} {
		s, err := ParseTime(c.in, now)
		if err != nil || !s.Start.Equal(c.start) || !s.End.Equal(now) || !s.Relative {
			t.Errorf("%s: %+v %v, want from %v to now, relative", c.in, s, err, c.start)
		}
	}
	for _, bad := range []string{"", "0d", "soon", "2026-13", "2026-1-1", "3x"} {
		if _, err := ParseTime(bad, now); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

func TestParseTimeMondayAndJanuary(t *testing.T) {
	// A Sunday in January: this week started the Monday before, last
	// month was in the year before.
	now := time.Date(2027, 1, 3, 9, 0, 0, 0, time.UTC)
	s, _ := ParseTime("this-week", now)
	if want := time.Date(2026, 12, 28, 0, 0, 0, 0, time.UTC); !s.Start.Equal(want) {
		t.Fatalf("this-week starts %v, want %v", s.Start, want)
	}
	s, _ = ParseTime("last-month", now)
	if want := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC); !s.Start.Equal(want) {
		t.Fatalf("last-month starts %v, want %v", s.Start, want)
	}
}
