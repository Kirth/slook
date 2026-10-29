package search

import (
	"strings"
	"testing"

	"github.com/gammons/slk/gui/backend"
)

func hits(ps []Piece) []string {
	var out []string
	for _, p := range ps {
		if p.Hit {
			out = append(out, p.Text)
		}
	}
	return out
}

func join(ps []Piece) string {
	var b strings.Builder
	for _, p := range ps {
		b.WriteString(p.Text)
	}
	return b.String()
}

func TestHighlight(t *testing.T) {
	cases := []struct {
		text, query string
		want        []string
	}{
		{"Deploying the queue, then the deploy", "deploy", []string{"Deploying", "deploy"}},
		{"Lunch at the Café?", "cafe", []string{"Café"}},
		{"queue backed up; restart the worker", "worker queue", []string{"queue", "worker"}},
		{"redeploy now", "deploy", nil}, // prefix of a word, as FTS5 matches
		{"nothing here", "zebra", nil},
		{"build 4821 failed", "48", []string{"4821"}},
		{"ship it", ":ship:", []string{"ship"}},
	}
	for _, c := range cases {
		ps := Highlight(c.text, c.query)
		if got := join(ps); got != c.text {
			t.Errorf("%q: pieces join to %q", c.text, got)
		}
		got := hits(ps)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("Highlight(%q, %q) hits %q, want %q", c.text, c.query, got, c.want)
		}
	}
}

func TestSnippet(t *testing.T) {
	if got := Snippet("short\nand sweet", "sweet", 80); got != "short and sweet" {
		t.Fatalf("short: %q", got)
	}
	long := strings.Repeat("filler words go here ", 20) + "the deploy failed again " + strings.Repeat("and then more words ", 20)
	got := Snippet(long, "deploy", 80)
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") || !strings.Contains(got, "deploy") {
		t.Fatalf("late match: %q", got)
	}
	if n := len([]rune(got)); n > 84 {
		t.Fatalf("snippet is %d runes", n)
	}
	if got := Snippet(long, "zebra", 40); !strings.HasPrefix(got, "filler") || !strings.HasSuffix(got, "…") {
		t.Fatalf("no match: %q", got)
	}
}

func TestBarState(t *testing.T) {
	if s := StateOf(backend.Coverage{}, 0.4); s != Filling {
		t.Errorf("filling: %v", s)
	}
	if s := StateOf(backend.Coverage{Complete: true}, 0.4); s != Complete {
		t.Errorf("complete: %v", s)
	}
	if s := StateOf(backend.Coverage{}, 1); s != Complete {
		t.Errorf("at the horizon: %v", s)
	}
	if s := StateOf(backend.Coverage{Complete: true, Holes: 2}, 1); s != Gaps {
		t.Errorf("complete with holes: %v", s)
	}
	cs := []backend.Coverage{{Complete: true}, {Holes: 1}}
	if s := OverallState(cs, 1); s != Gaps {
		t.Errorf("overall with a gap: %v", s)
	}
	if s := OverallState(cs[:1], 1); s != Complete {
		t.Errorf("overall complete: %v", s)
	}
}
