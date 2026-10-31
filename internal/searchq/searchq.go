// Package searchq reads a message search query: words, "phrases",
// alternatives (a | b), exclusions (!word, -word) and filters
// (from:, in:, mentions:, is:, has:, on:, during:, after:, before:,
// since:). It only reads: which people, conversations and messages a
// query means is the caller's to decide.
package searchq

import (
	"strings"
	"unicode"
)

// Kind is what a clause asks for.
type Kind int

const (
	// Word is a word the message must have (a prefix of one).
	Word Kind = iota
	// Phrase is words in a row ("…").
	Phrase
	// Or is any of Alts, each a Word or Phrase.
	Or
	// From is the message's author.
	From
	// In is the conversation.
	In
	// Mentions is someone (or a usergroup) the text mentions.
	Mentions
	// Is is the message's shape: dm, channel, thread, reply, edited.
	Is
	// Has is something the message carries: link, file, reaction, :emoji:.
	Has
	// On is a time the message falls in (on: and during:).
	On
	// After is a time the message comes after.
	After
	// Before is a time the message comes before.
	Before
	// Since is a time the message comes at or after the start of.
	Since
)

// keys are the filters by the word typed before the colon.
var keys = map[string]Kind{
	"from": From, "in": In, "mentions": Mentions, "is": Is, "has": Has,
	"on": On, "during": On, "after": After, "before": Before, "since": Since,
}

// Keys are the filter words, in the order to offer them.
var Keys = []string{"from", "in", "mentions", "is", "has", "on", "during", "after", "before", "since"}

// IsValues and HasValues are what is: and has: understand (has: also
// takes an :emoji:).
var (
	IsValues  = []string{"dm", "channel", "thread", "reply", "edited"}
	HasValues = []string{"link", "file", "reaction"}
)

// Clause is one part of a query.
type Clause struct {
	Kind Kind
	// Key is the filter word as typed, lowercased ("during"); "" for
	// text.
	Key string
	// Neg excludes what the clause matches (a leading ! or -).
	Neg bool
	// Value is the word, the phrase or the filter's value, unquoted.
	Value string
	// Quoted is whether Value was in quotes.
	Quoted bool
	// Alts are an Or's alternatives.
	Alts []Clause
	// Span is the clause's runes in the query, [start, end).
	Span [2]int
	// ValueAt is the rune where Value starts (after the colon, and the
	// opening quote if any).
	ValueAt int
}

// IsText is whether the clause is about the message's words.
func (c Clause) IsText() bool { return c.Kind == Word || c.Kind == Phrase || c.Kind == Or }

// Query is a parsed query, its clauses in the order typed.
type Query struct {
	Clauses []Clause
}

// Parse reads s. It never fails: what is not a filter is text, and an
// unclosed quote runs to the end.
func Parse(s string) Query {
	rs := []rune(s)
	var raw []Clause
	for i := 0; i < len(rs); {
		if unicode.IsSpace(rs[i]) {
			i++
			continue
		}
		c, next := readClause(rs, i)
		raw = append(raw, c)
		i = next
	}
	return Query{Clauses: joinOrs(raw)}
}

// orMark is the separator clause between alternatives; joinOrs removes it.
const orMark = Kind(-1)

// readClause reads the clause starting at rs[i] and returns it and
// where the next one may start.
func readClause(rs []rune, i int) (Clause, int) {
	start := i
	c := Clause{}
	if (rs[i] == '!' || rs[i] == '-') && i+1 < len(rs) && !unicode.IsSpace(rs[i+1]) {
		c.Neg = true
		i++
	}
	if rs[i] == '"' {
		val, end := readQuoted(rs, i)
		c.Kind, c.Value, c.Quoted, c.ValueAt = Phrase, val, true, i+1
		c.Span = [2]int{start, end}
		return c, end
	}
	// A bare token, which may be key:value or key:"value".
	j := i
	for j < len(rs) && !unicode.IsSpace(rs[j]) && rs[j] != ':' {
		j++
	}
	if j < len(rs) && rs[j] == ':' {
		if k, ok := keys[strings.ToLower(string(rs[i:j]))]; ok {
			c.Kind, c.Key, c.ValueAt = k, strings.ToLower(string(rs[i:j])), j+1
			if j+1 < len(rs) && rs[j+1] == '"' {
				val, end := readQuoted(rs, j+1)
				c.Value, c.Quoted, c.ValueAt = val, true, j+2
				c.Span = [2]int{start, end}
				return c, end
			}
			end := j + 1
			for end < len(rs) && !unicode.IsSpace(rs[end]) {
				end++
			}
			c.Value = string(rs[j+1 : end])
			c.Span = [2]int{start, end}
			return c, end
		}
	}
	end := i
	for end < len(rs) && !unicode.IsSpace(rs[end]) {
		end++
	}
	word := string(rs[i:end])
	c.Kind, c.Value, c.ValueAt, c.Span = Word, word, i, [2]int{start, end}
	if !c.Neg && (word == "|" || word == "OR") {
		c.Kind = orMark
		return c, end
	}
	if strings.Contains(word, "|") && !strings.Contains(word, "://") {
		// a|b written without spaces.
		var alts []Clause
		at := i
		for _, part := range strings.Split(word, "|") {
			if part != "" {
				alts = append(alts, Clause{Kind: Word, Value: part, ValueAt: at, Span: [2]int{at, at + len([]rune(part))}})
			}
			at += len([]rune(part)) + 1
		}
		switch len(alts) {
		case 0:
			c.Kind = orMark
		case 1:
			alts[0].Neg, alts[0].Span[0] = c.Neg, start
			c = alts[0]
		default:
			c.Kind, c.Value, c.Alts = Or, "", alts
		}
	}
	return c, end
}

// readQuoted reads a quoted string whose opening quote is rs[i]: the
// text inside and the rune after the closing quote (or the end).
func readQuoted(rs []rune, i int) (string, int) {
	j := i + 1
	for j < len(rs) && rs[j] != '"' {
		j++
	}
	val := string(rs[i+1 : j])
	if j < len(rs) {
		j++
	}
	return val, j
}

// joinOrs folds "a | b | c" (and OR) into one Or clause. A separator
// with no text clause on either side is dropped.
func joinOrs(raw []Clause) []Clause {
	var out []Clause
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c.Kind == orMark {
			continue
		}
		for i+2 < len(raw) && raw[i+1].Kind == orMark && joinable(c) && joinable(raw[i+2]) {
			next := raw[i+2]
			if c.Kind != Or {
				c = Clause{Kind: Or, Alts: []Clause{c}, Span: c.Span, ValueAt: c.ValueAt}
			}
			if next.Kind == Or {
				c.Alts = append(c.Alts, next.Alts...)
			} else {
				c.Alts = append(c.Alts, next)
			}
			c.Span[1] = next.Span[1]
			i += 2
		}
		out = append(out, c)
	}
	return out
}

// joinable is a clause an OR may join: positive text.
func joinable(c Clause) bool { return c.IsText() && !c.Neg }

// TextTerms are the words the query looks for, as typed: plain words,
// every word of a phrase, every alternative's. Excluded text is not
// among them.
func (q Query) TextTerms() []string {
	var out []string
	var add func(c Clause)
	add = func(c Clause) {
		if c.Neg {
			return
		}
		switch c.Kind {
		case Word, Phrase:
			out = append(out, strings.Fields(c.Value)...)
		case Or:
			for _, a := range c.Alts {
				add(a)
			}
		}
	}
	for _, c := range q.Clauses {
		add(c)
	}
	return out
}

// HasText is whether any clause, kept or excluded, is about words.
func (q Query) HasText() bool {
	for _, c := range q.Clauses {
		if c.IsText() && (c.Kind == Or || strings.TrimSpace(c.Value) != "") {
			return true
		}
	}
	return false
}

// ClauseAt is the clause the caret (a rune offset) is in or at the end
// of.
func (q Query) ClauseAt(caret int) (Clause, bool) {
	for _, c := range q.Clauses {
		if c.Span[0] <= caret && caret <= c.Span[1] {
			return c, true
		}
	}
	return Clause{}, false
}
