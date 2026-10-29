// Package textedit is readline's editing commands as pure functions on
// text and a caret, so a frontend's text fields can offer them and they
// can be tested without one. Positions are rune offsets.
//
// "Line" is the caret's line in multi-line text, as readline's single
// line. Words, for the Alt keys, are runs of letters and digits; for
// Ctrl+W (unix-word-rubout), runs of anything but whitespace.
package textedit

import "unicode"

// Action is one readline command.
type Action int

const (
	LineEnd         Action = iota // Ctrl+E
	RuboutSpace                   // Ctrl+W: delete back to whitespace
	KillLineStart                 // Ctrl+U: delete back to the line's start
	KillWordBack                  // Alt+Backspace
	KillWordForward               // Alt+D
	WordBack                      // Alt+B
	WordForward                   // Alt+F
	Yank                          // Ctrl+Y: insert the last killed text
	Transpose                     // Ctrl+T: swap the characters around the caret
)

// Edit is an action's effect. Move is a caret motion, with only Caret
// set. Otherwise Text[From:To] is replaced with Insert and the caret goes
// to Caret (an offset in the text after the edit). Killed is what the
// edit cut, for the kill buffer; empty when it cut nothing.
type Edit struct {
	Move     bool
	From, To int
	Insert   string
	Caret    int
	Killed   string
}

// Apply works out an action on text with the caret at caret and the
// selection's other end at anchor (equal when nothing is selected).
// kill is the kill buffer, for Yank. ok is false when the action does
// nothing here: Ctrl+U at a line's start, Yank with nothing killed.
//
// With text selected, the deleting actions delete the selection (and
// kill it), Yank replaces it, and motions start from the caret.
func Apply(text []rune, caret, anchor int, a Action, kill string) (Edit, bool) {
	caret, anchor = clamp(caret, len(text)), clamp(anchor, len(text))
	lo, hi := min(caret, anchor), max(caret, anchor)

	switch a {
	case LineEnd, WordBack, WordForward:
		to := caret
		switch a {
		case LineEnd:
			to = lineEnd(text, caret)
		case WordBack:
			to = wordBack(text, caret)
		case WordForward:
			to = wordForward(text, caret)
		}
		return Edit{Move: true, Caret: to}, true
	case Yank:
		if kill == "" {
			return Edit{}, false
		}
		return Edit{From: lo, To: hi, Insert: kill, Caret: lo + len([]rune(kill))}, true
	case Transpose:
		return transpose(text, caret, lo != hi)
	}

	if lo != hi { // a deleting action on a selection
		return cut(text, lo, hi), true
	}
	from, to := caret, caret
	switch a {
	case RuboutSpace:
		from = runBack(text, caret, unicode.IsSpace)
	case KillLineStart:
		from = lineStart(text, caret)
	case KillWordBack:
		from = wordBack(text, caret)
	case KillWordForward:
		to = wordForward(text, caret)
	}
	if from == to {
		return Edit{}, false
	}
	return cut(text, from, to), true
}

func cut(text []rune, from, to int) Edit {
	return Edit{From: from, To: to, Caret: from, Killed: string(text[from:to])}
}

// transpose swaps the two characters before the caret at the end of a
// line, else the ones either side of it, moving the caret past them.
func transpose(text []rune, caret int, selected bool) (Edit, bool) {
	if selected || caret == 0 || caret == lineStart(text, caret) {
		return Edit{}, false
	}
	at := caret
	if caret == lineEnd(text, caret) {
		at = caret - 1 // at line end: the last two
	}
	if at == lineStart(text, at) || at >= len(text) {
		return Edit{}, false // a line of one character
	}
	swapped := string([]rune{text[at], text[at-1]})
	return Edit{From: at - 1, To: at + 1, Insert: swapped, Caret: at + 1}, true
}

func lineStart(text []rune, i int) int {
	for i > 0 && text[i-1] != '\n' {
		i--
	}
	return i
}

func lineEnd(text []rune, i int) int {
	for i < len(text) && text[i] != '\n' {
		i++
	}
	return i
}

func isWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// wordBack is the start of the word before i: readline skips what is
// not a word, then the word.
func wordBack(text []rune, i int) int {
	for i > 0 && !isWord(text[i-1]) {
		i--
	}
	for i > 0 && isWord(text[i-1]) {
		i--
	}
	return i
}

// wordForward is the end of the word after i.
func wordForward(text []rune, i int) int {
	for i < len(text) && !isWord(text[i]) {
		i++
	}
	for i < len(text) && isWord(text[i]) {
		i++
	}
	return i
}

// runBack is unix-word-rubout's start: skip space back, then non-space.
func runBack(text []rune, i int, space func(rune) bool) int {
	for i > 0 && space(text[i-1]) {
		i--
	}
	for i > 0 && !space(text[i-1]) {
		i--
	}
	return i
}

func clamp(i, n int) int {
	return max(0, min(i, n))
}
