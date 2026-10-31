package main

import (
	"image"
	"slices"
	"strconv"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/sidebar"
	"github.com/gammons/slk/internal/slackfmt"
)

// switcherRows is how many conversations the switcher lists.
const switcherRows = 8

// switcher is a chat window's quick switcher (Ctrl+K or the menu
// button): a filter over every conversation, unread ones first when it
// is empty. Choosing one opens its window, here: an open window on
// another workspace is moved to this one.
type switcher struct {
	open    bool
	focus   bool // move the keyboard to the filter next frame
	ed      widget.Editor
	rl      readline
	sel     int
	rows    [switcherRows + 1]widget.Clickable // the last is "All conversations"
	results []backend.Conversation
	// openedAt is when it last opened, for its entrance.
	openedAt time.Time
}

func (s *switcher) toggle() {
	s.open = !s.open
	if s.open {
		s.openedAt = time.Now()
		s.ed.SingleLine, s.ed.Submit = true, true
		s.ed.SetText("")
		s.sel, s.focus = 0, true
	}
}

// matches is what the switcher lists for its filter: with none, the
// conversations needing reading, then the most recently active.
func switcherMatches(convs []backend.Conversation, query, current string) []backend.Conversation {
	all := sidebar.Search(convs, query)
	if query == "" {
		slices.SortStableFunc(all, func(a, b backend.Conversation) int {
			ra, rb := sidebar.NeedsReading(a), sidebar.NeedsReading(b)
			switch {
			case ra && !rb:
				return -1
			case rb && !ra:
				return 1
			}
			return 0
		})
	}
	out := make([]backend.Conversation, 0, switcherRows)
	for _, c := range all {
		if c.ID != current {
			out = append(out, c)
		}
		if len(out) == switcherRows {
			break
		}
	}
	return out
}

// update handles the switcher's keys and clicks. It returns the
// conversation chosen, or showAll for "All conversations".
func (s *switcher) update(gtx C, convs []backend.Conversation, current string) (chosen *backend.Conversation, showAll, closed bool) {
	if !s.open {
		return nil, false, false
	}
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &s.ed, Name: key.NameUpArrow},
			key.Filter{Focus: &s.ed, Name: key.NameDownArrow},
			key.Filter{Focus: &s.ed, Name: key.NameEscape},
		)
		if !ok {
			break
		}
		k, ok := ev.(key.Event)
		if !ok || k.State != key.Press {
			continue
		}
		n := len(s.results) + 1
		switch k.Name {
		case key.NameUpArrow:
			s.sel = (s.sel + n - 1) % n
		case key.NameDownArrow:
			s.sel = (s.sel + 1) % n
		case key.NameEscape:
			s.open = false
			return nil, false, true
		}
	}
	s.rl.keys(gtx, &s.ed)
	submitted := false
	for {
		ev, ok := s.ed.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			s.sel = 0
		case widget.SubmitEvent:
			submitted = true
		}
	}
	s.results = switcherMatches(convs, s.ed.Text(), current)
	if s.sel > len(s.results) {
		s.sel = 0
	}
	pick := -1
	if submitted {
		pick = s.sel
	}
	for i := range s.rows {
		if s.rows[i].Clicked(gtx) {
			pick = i
		}
	}
	switch {
	case pick < 0:
		return nil, false, false
	case pick < len(s.results):
		c := s.results[pick]
		s.open = false
		return &c, false, true
	default:
		s.open = false
		return nil, true, true
	}
}

// layout draws the switcher as a panel over the window's top, under the
// header.
func (s *switcher) layout(gtx C, th *material.Theme, view *richView) {
	if !s.open {
		return
	}
	if s.focus {
		gtx.Execute(key.FocusCmd{Tag: &s.ed})
		s.focus = false
	}
	width := min(gtx.Dp(440), gtx.Constraints.Max.X-gtx.Dp(24))
	x := (gtx.Constraints.Max.X - width) / 2
	defer op.Offset(image.Pt(x, gtx.Dp(headerHeight)+gtx.Dp(6))).Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, gtx.Constraints.Max.Y)}
	enter(gtx, s.openedAt, func(gtx C) D { return s.panel(gtx, th, view) })
}

// panel is the switcher's box: the filter and the conversations.
func (s *switcher) panel(gtx C, th *material.Theme, view *richView) D {
	return overlayFrame(gtx, func(gtx C) D {
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
			children := []layout.FlexChild{
				layout.Rigid(framedEditor(th, &s.ed, "Jump to a conversation")),
				layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			}
			for i, c := range s.results {
				children = append(children, layout.Rigid(func(gtx C) D {
					return s.row(gtx, th, view, i, func(gtx C) D { return switcherConv(gtx, th, view, c) })
				}))
			}
			n := len(s.results)
			children = append(children, layout.Rigid(func(gtx C) D {
				return s.row(gtx, th, view, n, func(gtx C) D {
					lbl := secondaryLabel(th, "All conversations…")
					lbl.Color = colMuted
					return layout.Inset{Left: unit.Dp(30)}.Layout(gtx, lbl.Layout)
				})
			}))
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

// row is one clickable line of the switcher, highlighted when selected.
func (s *switcher) row(gtx C, th *material.Theme, view *richView, i int, w layout.Widget) D {
	cl := &s.rows[i]
	return cl.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		bg := colOverlay
		if i == s.sel || cl.Hovered() {
			bg = colControl
		}
		return rounded(gtx, bg, unit.Dp(6), func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, w)
		})
	})
}

// switcherConv is a conversation's line: glyph or avatar, name, and its
// unread mentions or a dot.
func switcherConv(gtx C, th *material.Theme, view *richView, c backend.Conversation) D {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Dp(30)
			if c.Kind == backend.KindDM {
				return view.avatar(gtx, c.ID, c.AvatarURL, c.Name, unit.Dp(20))
			}
			g := bodyLabel(th, slackfmt.ChannelGlyph(string(c.Kind)))
			g.Color = colMuted
			return g.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx C) D {
			lbl := bodyLabel(th, c.Name)
			lbl.MaxLines = 1
			lbl.Color = convNameColor(c)
			return lbl.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			switch {
			case c.Mentions > 0:
				lbl := captionLabel(th, strconv.Itoa(c.Mentions))
				lbl.Color = colBadge
				lbl.Font.Weight = font.SemiBold
				return lbl.Layout(gtx)
			case sidebar.NeedsReading(c):
				lbl := captionLabel(th, "●")
				lbl.Color = colAccent
				return lbl.Layout(gtx)
			}
			return D{}
		}),
	)
}
