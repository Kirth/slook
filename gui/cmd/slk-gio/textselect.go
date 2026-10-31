package main

import (
	"image"
	"image/color"
	"io"
	"strings"
	"time"

	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/internal/styledtext"
	"github.com/gammons/slk/gui/selection"
)

// multiClick is how quickly a second press must follow the first, and
// how near, to count as a double (then triple) click.
const (
	multiClick     = 400 * time.Millisecond
	multiClickNear = 6 // px
)

// textSel is one message's text selection: the text blocks as drawn
// (where each is, relative to the message body, and where its runes
// are), and the selection from a to b over them. Only one message at a
// time has one (msgView.selTS).
type textSel struct {
	on     bool        // the body being drawn is selectable
	origin image.Point // where the block being drawn is, in the body
	blocks []selection.Block

	a, b     selection.Pos
	active   bool // a and b are set (they may be equal: nothing selected)
	dragging bool

	// tag takes the body's pointer and key events; size is the body's
	// size last frame, the area they are taken over.
	tag  byte
	size image.Point

	// lastPress and clicks count a double or triple click.
	lastPress time.Time
	lastAt    image.Point
	clicks    int
}

// at runs fn with the origin moved by d: fn draws d further into the
// body than what encloses it.
func (s *textSel) at(d image.Point, fn func()) {
	s.origin = s.origin.Add(d)
	fn()
	s.origin = s.origin.Sub(d)
}

// without runs fn with selection off, for blocks whose places are not
// tracked.
func (s *textSel) without(fn func() D) D {
	on := s.on
	s.on = false
	defer func() { s.on = on }()
	return fn()
}

// drawn records a text block just laid out (segs, at the current
// origin) and paints its part of the selection, behind the text that
// is added after.
func (s *textSel) drawn(gtx C, segs []styledtext.Segment) {
	blk := selection.Block{Origin: s.origin}
	for _, sg := range segs {
		blk.Segs = append(blk.Segs, selection.Seg{Text: sg.Text, Rect: sg.Rect, X: sg.X})
	}
	i := len(s.blocks)
	s.blocks = append(s.blocks, blk)
	if !s.active {
		return
	}
	from, to, ok := selection.Range(s.blocks, i, s.a, s.b)
	if !ok {
		return
	}
	for _, r := range selection.Rects(blk, from, to) {
		paint.FillShape(gtx.Ops, colSelection(), clip.Rect(r).Op())
	}
}

// has reports whether something is selected.
func (s *textSel) has() bool { return s.active && s.a != s.b }

// text is what is selected.
func (s *textSel) text() string {
	if !s.has() {
		return ""
	}
	return selection.Text(s.blocks, s.a, s.b)
}

// clear drops the selection.
func (s *textSel) clear() { s.active, s.dragging = false, false }

// colSelection is the selection's tint: the accent, see-through.
func colSelection() color.NRGBA {
	c := colAccent
	c.A = 0x55
	return c
}

// selectable draws a message's body (body) so its text can be selected
// by dragging, a word by double-clicking, a paragraph by triple-
// clicking; Ctrl+C (Cmd+C) copies it, Escape drops it. The pointer area
// encloses the body (sized from the last frame), so links and mentions
// in it still get their clicks.
func (v *msgView) selectable(gtx C, w *msgWidgets, m backend.Message, body layout.Widget) D {
	s := &w.tsel
	v.selectEvents(gtx, w, m)
	area := clip.Rect{Max: s.size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &s.tag)
	pointer.CursorText.Add(gtx.Ops)
	s.blocks, s.origin, s.on = s.blocks[:0:0], image.Point{}, true
	dims := body(gtx)
	s.on = false
	area.Pop()
	s.size = dims.Size
	return dims
}

// selectEvents handles the body's pointer and keys since the last frame.
func (v *msgView) selectEvents(gtx C, w *msgWidgets, m backend.Message) {
	s := &w.tsel
	for {
		if _, ok := gtx.Event(key.FocusFilter{Target: &s.tag}); !ok {
			break
		}
	}
	for {
		ev, ok := gtx.Event(
			pointer.Filter{Target: &s.tag, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel},
			key.Filter{Focus: &s.tag, Name: "C", Required: key.ModShortcut},
			key.Filter{Focus: &s.tag, Name: key.NameEscape},
		)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case pointer.Event:
			v.selectPointer(gtx, w, m, e)
		case key.Event:
			if e.State != key.Press {
				break
			}
			if e.Name == key.NameEscape {
				s.clear()
			} else {
				v.copySelection(gtx, w)
			}
		}
	}
}

func (v *msgView) selectPointer(gtx C, w *msgWidgets, m backend.Message, e pointer.Event) {
	s := &w.tsel
	at := image.Pt(int(e.Position.X), int(e.Position.Y))
	switch e.Kind {
	case pointer.Press:
		if !e.Buttons.Contain(pointer.ButtonPrimary) {
			return
		}
		p, ok := selection.Hit(s.blocks, at)
		if !ok {
			return
		}
		near := abs(at.X-s.lastAt.X) <= multiClickNear && abs(at.Y-s.lastAt.Y) <= multiClickNear
		if gtx.Now.Sub(s.lastPress) <= multiClick && near {
			s.clicks++
		} else {
			s.clicks = 1
		}
		s.lastPress, s.lastAt = gtx.Now, at
		v.claimSelection(m.TS)
		switch {
		case s.clicks == 2:
			s.a, s.b = selection.Word(s.blocks, p)
		case s.clicks >= 3:
			s.a, s.b = selection.Whole(s.blocks, p)
		default:
			s.a, s.b = p, p
		}
		s.active, s.dragging = true, s.clicks == 1
		// The keyboard, for Ctrl+C and Escape.
		gtx.Execute(key.FocusCmd{Tag: &s.tag})
	case pointer.Drag:
		if !s.dragging {
			return
		}
		if p, ok := selection.Hit(s.blocks, at); ok {
			s.b = p
		}
	case pointer.Release, pointer.Cancel:
		s.dragging = false
	}
}

// claimSelection makes ts the message with the selection, dropping any
// other message's.
func (v *msgView) claimSelection(ts string) {
	if v.selTS != "" && v.selTS != ts {
		if w := v.widgets[v.selTS]; w != nil {
			w.tsel.clear()
		}
	}
	v.selTS = ts
}

// copySelection puts the selected text on the clipboard.
func (v *msgView) copySelection(gtx C, w *msgWidgets) {
	text := w.tsel.text()
	if strings.TrimSpace(text) == "" {
		return
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
	if v.notify != nil {
		v.notify("Copied", false)
	}
}

// selectionOf is the text selected in message ts, "" for none.
func (v *msgView) selectionOf(ts string) string {
	if w := v.widgets[ts]; w != nil {
		return w.tsel.text()
	}
	return ""
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
