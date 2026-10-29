package textedit

import "testing"

// run applies an action to text written with "|" for the caret and,
// optionally, "^" for the selection's anchor, and returns the result in
// the same notation (motions keep the anchor collapsed), plus the kill.
func run(t *testing.T, in string, a Action, kill string) (string, string, bool) {
	t.Helper()
	caret, anchor := -1, -1
	var text []rune
	for _, r := range in {
		switch r {
		case '|':
			caret = len(text)
		case '^':
			anchor = len(text)
		default:
			text = append(text, r)
		}
	}
	if caret < 0 {
		t.Fatalf("no caret in %q", in)
	}
	if anchor < 0 {
		anchor = caret
	}
	e, ok := Apply(text, caret, anchor, a, kill)
	if !ok {
		return in, "", false
	}
	if e.Move {
		return string(text[:e.Caret]) + "|" + string(text[e.Caret:]), "", true
	}
	out := string(text[:e.From]) + e.Insert + string(text[e.To:])
	r := []rune(out)
	return string(r[:e.Caret]) + "|" + string(r[e.Caret:]), e.Killed, true
}

func TestApply(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		a      Action
		kill   string
		want   string
		killed string
		ok     bool
	}{
		{"ctrl+e to line end", "he|llo\nworld", LineEnd, "", "hello|\nworld", "", true},
		{"ctrl+e at text end", "hello|", LineEnd, "", "hello|", "", true},
		{"ctrl+w rubs out to space", "git push --force|", RuboutSpace, "", "git push |", "--force", true},
		{"ctrl+w skips trailing space", "one two  |", RuboutSpace, "", "one |", "two  ", true},
		{"ctrl+w at start", "|abc", RuboutSpace, "", "|abc", "", false},
		{"ctrl+u to line start", "first\nsecond li|ne", KillLineStart, "", "first\n|ne", "second li", true},
		{"ctrl+u at line start", "first\n|second", KillLineStart, "", "first\n|second", "", false},
		{"alt+backspace stops at punctuation", "git push --force|", KillWordBack, "", "git push --|", "force", true},
		{"alt+backspace skips punctuation first", "foo.bar..|", KillWordBack, "", "foo.|", "bar..", true},
		{"alt+d", "say |hello, world", KillWordForward, "", "say |, world", "hello", true},
		{"alt+d skips punctuation first", "a|, world", KillWordForward, "", "a|", ", world", true},
		{"alt+b", "hello brave| world", WordBack, "", "hello |brave world", "", true},
		{"alt+f", "hello| brave world", WordForward, "", "hello brave| world", "", true},
		{"alt+f unicode word", "|café crème", WordForward, "", "café| crème", "", true},
		{"ctrl+y", "say |", Yank, "hi 👋", "say hi 👋|", "", true},
		{"ctrl+y with nothing killed", "say |", Yank, "", "say |", "", false},
		{"ctrl+y replaces a selection", "say ^old|!", Yank, "new", "say new|!", "", true},
		{"ctrl+t in the middle", "ab|cd", Transpose, "", "acb|d", "", true},
		{"ctrl+t at line end", "teh|\nnext", Transpose, "", "the|\nnext", "", true},
		{"ctrl+t at start", "|ab", Transpose, "", "|ab", "", false},
		{"ctrl+t on a one-letter line", "a|\nb", Transpose, "", "a|\nb", "", false},
		{"a kill on a selection cuts it", "keep ^this| text", KillWordBack, "", "keep | text", "this", true},
		{"selection backwards", "keep |this^ text", RuboutSpace, "", "keep | text", "this", true},
	}
	for _, tc := range cases {
		got, killed, ok := run(t, tc.in, tc.a, tc.kill)
		if got != tc.want || killed != tc.killed || ok != tc.ok {
			t.Errorf("%s: %q → (%q, killed %q, %v), want (%q, %q, %v)",
				tc.name, tc.in, got, killed, ok, tc.want, tc.killed, tc.ok)
		}
	}
}

func TestApplyClampsOutOfRange(t *testing.T) {
	text := []rune("abc")
	if e, ok := Apply(text, 99, -5, KillLineStart, ""); !ok || e.From != 0 || e.To != 3 {
		t.Fatalf("got %+v %v", e, ok)
	}
}
