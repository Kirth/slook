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
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/gui/sidebar"
	"github.com/gammons/slk/internal/slackfmt"
)

// switcherRows is how many conversations the switcher lists.
const switcherRows = 8

// switcher is a chat window's quick switcher (Ctrl+K or the menu
// button): a filter over every conversation, unread ones first when it
// is empty, and under them the channels the user is not in that Slack
// finds for the filter (asked once typing pauses; see
// chat.RemoteSearch). Choosing a conversation opens its window, here:
// an open window on another workspace is moved to this one. Choosing a
// channel the user is not in joins it.
type switcher struct {
	open  bool
	focus bool // move the keyboard to the filter next frame
	ed    widget.Editor
	rl    readline
	sel   int
	// rows are the lines' clickables by position: the conversations,
	// the channels to join, then "All conversations".
	rows    [2*switcherRows + 1]widget.Clickable
	results []backend.Conversation
	// remote is the channel search; joinable is its answer for the
	// filter, without the channels the user is in; remoteErr is a
	// search that failed.
	remote    chat.RemoteSearch
	joinable  []backend.ChannelResult
	remoteErr string
	// openedAt is when it last opened, for its entrance.
	openedAt time.Time
}

// switcherPick is what the switcher's update chose: a conversation to
// open, a channel to join, or the list (all); closed is whether it
// closed.
type switcherPick struct {
	conv   *backend.Conversation
	join   *backend.ChannelResult
	all    bool
	closed bool
}

// found takes Slack's answer to the switcher's channel search.
func (s *switcher) found(v channelsFound) {
	s.remote.Done(v.q, v.res)
	s.remoteErr = ""
	if v.err != nil {
		s.remoteErr = "Could not search channels: " + v.err.Error()
	}
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

// update handles the switcher's keys and clicks. search asks Slack
// which channels match a query (hub.searchChannels), its answer coming
// back through found.
func (s *switcher) update(gtx C, convs []backend.Conversation, current string, search func(q string)) switcherPick {
	if !s.open {
		return switcherPick{}
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
		n := s.n()
		switch k.Name {
		case key.NameUpArrow:
			s.sel = (s.sel + n - 1) % n
		case key.NameDownArrow:
			s.sel = (s.sel + 1) % n
		case key.NameEscape:
			s.open = false
			return switcherPick{closed: true}
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
	q := s.ed.Text()
	s.results = switcherMatches(convs, q, current)
	s.remote.Type(q, gtx.Now)
	if rq, ok := s.remote.Due(gtx.Now); ok && search != nil {
		search(rq)
	}
	if at := s.remote.Wake(); !at.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: at})
	}
	s.joinable = chat.Unjoined(s.remote.Results(q), convs)
	if len(s.joinable) > switcherRows {
		s.joinable = s.joinable[:switcherRows]
	}
	if s.sel >= s.n() {
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
	local, remote := len(s.results), len(s.joinable)
	switch {
	case pick < 0:
		return switcherPick{}
	case pick < local:
		c := s.results[pick]
		s.open = false
		return switcherPick{conv: &c, closed: true}
	case pick < local+remote:
		r := s.joinable[pick-local]
		s.open = false
		return switcherPick{join: &r, closed: true}
	default:
		s.open = false
		return switcherPick{all: true, closed: true}
	}
}

// n is how many lines the switcher lists: conversations, channels to
// join, and "All conversations".
func (s *switcher) n() int { return len(s.results) + len(s.joinable) + 1 }

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
			if len(s.joinable) > 0 || s.remoteErr != "" {
				children = append(children, layout.Rigid(func(gtx C) D {
					text := "Channels you're not in"
					if len(s.joinable) == 0 {
						text = s.remoteErr
					}
					lbl := captionLabel(th, text)
					lbl.Color = colMuted
					return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(2), Left: unit.Dp(8)}.Layout(gtx, lbl.Layout)
				}))
			}
			for i, r := range s.joinable {
				children = append(children, layout.Rigid(func(gtx C) D {
					return s.row(gtx, th, view, n+i, func(gtx C) D { return joinLine(gtx, th, r) })
				}))
			}
			n += len(s.joinable)
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

// joinLine is a channel the user is not in: its glyph, its name, and
// "Join" at the end.
func joinLine(gtx C, th *material.Theme, r backend.ChannelResult) D {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Dp(30)
			g := bodyLabel(th, slackfmt.ChannelGlyph(string(r.Kind)))
			g.Color = colMuted
			return g.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx C) D {
			lbl := bodyLabel(th, r.Name)
			lbl.MaxLines = 1
			lbl.Color = colMuted
			return lbl.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			lbl := captionLabel(th, "Join")
			lbl.Color = colAccent
			return lbl.Layout(gtx)
		}),
	)
}
