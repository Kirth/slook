package main

import (
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/widget"

	"github.com/gammons/slk/gui/textedit"
)

// readline adds readline's editing keys to an editor, beyond what Gio's
// widget.Editor does itself (Ctrl+A select all, Ctrl+arrows, undo and
// redo, clipboard), plus the Ctrl+Backspace and Ctrl+Delete it misses
// on Linux. Ctrl+K is the conversation switcher, so there is no
// kill-to-end; Ctrl+B and Ctrl+I are left for bold and italic. kill is
// the last text cut, for Ctrl+Y.
type readline struct{ kill string }

// readlineKeys maps keys (Ctrl or Alt, Shift optional) to actions.
var readlineKeys = []struct {
	name   key.Name
	mod    key.Modifiers
	action textedit.Action
}{
	{"E", key.ModCtrl, textedit.LineEnd},
	{"W", key.ModCtrl, textedit.RuboutSpace},
	{"U", key.ModCtrl, textedit.KillLineStart},
	{"Y", key.ModCtrl, textedit.Yank},
	{"T", key.ModCtrl, textedit.Transpose},
	// Gio's editor means to delete a word on Ctrl+Backspace/Delete, but
	// on Linux its shortcut modifier is Ctrl too and that branch returns
	// first, so they do nothing there; they are handled here instead.
	{key.NameDeleteBackward, key.ModCtrl, textedit.KillWordBack},
	{key.NameDeleteForward, key.ModCtrl, textedit.KillWordForward},
	{key.NameDeleteBackward, key.ModAlt, textedit.KillWordBack},
	{"D", key.ModAlt, textedit.KillWordForward},
	{"B", key.ModAlt, textedit.WordBack},
	{"F", key.ModAlt, textedit.WordForward},
}

// keys applies the readline keys pressed since the last frame to ed.
// Call it before ed.Update; the editor does not claim these keys.
func (r *readline) keys(gtx C, ed *widget.Editor) {
	filters := make([]event.Filter, 0, len(readlineKeys)+2)
	for _, k := range readlineKeys {
		filters = append(filters, key.Filter{Focus: ed, Name: k.name, Required: k.mod, Optional: key.ModShift})
	}
	// Ctrl+H and Ctrl+D are Backspace and Delete; the editor's own
	// Delete removes a whole grapheme (an emoji with its modifiers).
	filters = append(filters,
		key.Filter{Focus: ed, Name: "H", Required: key.ModCtrl},
		key.Filter{Focus: ed, Name: "D", Required: key.ModCtrl},
	)
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			return
		}
		k, ok := ev.(key.Event)
		if !ok || k.State != key.Press {
			continue
		}
		if k.Modifiers.Contain(key.ModCtrl) && (k.Name == "H" || k.Name == "D") {
			n := -1
			if k.Name == "D" {
				n = 1
			}
			if s, e := ed.Selection(); s != e {
				ed.Insert("")
			} else {
				ed.Delete(n)
			}
			continue
		}
		for _, b := range readlineKeys {
			if b.name == k.Name && k.Modifiers.Contain(b.mod) {
				r.apply(ed, b.action, k.Modifiers.Contain(key.ModShift))
				break
			}
		}
	}
}

// apply runs one action on ed. extend keeps the selection's anchor for
// a motion, as Shift does with the arrows.
func (r *readline) apply(ed *widget.Editor, a textedit.Action, extend bool) {
	caret, anchor := ed.Selection()
	e, ok := textedit.Apply([]rune(ed.Text()), caret, anchor, a, r.kill)
	if !ok {
		return
	}
	if e.Move {
		if extend {
			ed.SetCaret(e.Caret, anchor)
		} else {
			ed.SetCaret(e.Caret, e.Caret)
		}
		return
	}
	// Through the editor's own insert, so undo still works.
	ed.SetCaret(e.From, e.To)
	ed.Insert(e.Insert)
	ed.SetCaret(e.Caret, e.Caret)
	if e.Killed != "" {
		r.kill = e.Killed
	}
}
