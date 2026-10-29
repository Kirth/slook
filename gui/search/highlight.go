package search

import (
	"strings"
	"unicode"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/internal/text"
)

// Piece is a run of a result's text; Hit marks a word the query
// matched.
type Piece struct {
	Text string
	Hit  bool
}

// terms are query's words as the matcher compares them: folded (case-
// and accent-blind) and without the punctuation FTS5's tokenizer drops.
func terms(query string) []string {
	var out []string
	for _, f := range strings.Fields(query) {
		f = strings.TrimFunc(text.Fold(f), func(r rune) bool { return !isWordRune(r) })
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// isWordRune is a rune FTS5's unicode61 tokenizer keeps in a word.
func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// Highlight splits s into pieces, marking each word that starts with
// one of query's terms, case- and accent-blind: the rule the cache's
// full-text search matched it by. Concatenated, the pieces are s.
func Highlight(s, query string) []Piece {
	ts := terms(query)
	var out []Piece
	add := func(chunk string, hit bool) {
		if chunk == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].Hit == hit {
			out[n-1].Text += chunk
			return
		}
		out = append(out, Piece{Text: chunk, Hit: hit})
	}
	rs := []rune(s)
	for i := 0; i < len(rs); {
		j := i
		word := isWordRune(rs[i])
		for j < len(rs) && isWordRune(rs[j]) == word {
			j++
		}
		chunk := string(rs[i:j])
		hit := false
		if word {
			folded := text.Fold(chunk)
			for _, t := range ts {
				if strings.HasPrefix(folded, t) {
					hit = true
					break
				}
			}
		}
		add(chunk, hit)
		i = j
	}
	return out
}

// Snippet is s on one line, cut to about width runes around its first
// match with "…" where it was cut, so the match shows in a short row.
// Short text comes back whole; text with no match keeps its start.
func Snippet(s, query string, width int) string {
	s = strings.Join(strings.Fields(s), " ")
	rs := []rune(s)
	if len(rs) <= width {
		return s
	}
	first := -1
	pos := 0
	for _, p := range Highlight(s, query) {
		if p.Hit {
			first = pos
			break
		}
		pos += len([]rune(p.Text))
	}
	start := 0
	if first > width/3 {
		start = first - width/3
	}
	end := min(len(rs), start+width)
	start = max(0, end-width)
	// Cut at spaces, not inside words.
	if start > 0 {
		for start < first && rs[start-1] != ' ' {
			start++
		}
	}
	if end < len(rs) {
		for e := end; e > start+width/2; e-- {
			if rs[e] == ' ' {
				end = e
				break
			}
		}
	}
	out := strings.TrimSpace(string(rs[start:end]))
	if start > 0 {
		out = "…" + out
	}
	if end < len(rs) {
		out += "…"
	}
	return out
}

// BarState is what a coverage bar's colour says.
type BarState int

const (
	Filling  BarState = iota // still short of the horizon
	Complete                 // all of it, no gaps
	Gaps                     // holes between what is cached
)

// StateOf is a conversation's bar state at fraction f: gaps first (a
// conversation with holes is not complete, however far back it
// reaches), then complete when it reaches the start or the horizon.
func StateOf(c backend.Coverage, f float64) BarState {
	switch {
	case c.Holes > 0:
		return Gaps
	case c.Complete || f >= 0.999:
		return Complete
	}
	return Filling
}

// OverallState is the panel bar's state over the counted conversations
// at overall fraction f: gaps if any has them.
func OverallState(cs []backend.Coverage, f float64) BarState {
	for _, c := range cs {
		if c.Holes > 0 {
			return Gaps
		}
	}
	if f >= 0.999 {
		return Complete
	}
	return Filling
}
