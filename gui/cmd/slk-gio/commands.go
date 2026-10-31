package main

import (
	"image"
	"strings"
	"unicode/utf8"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/internal/slackfmt"
	"github.com/gammons/slk/internal/text"
)

// popupRows is how many commands or completions the popup lists at once.
const popupRows = 8

// Things a chat window's inbox carries for commands.
type (
	commandsLoaded struct{ cmds []backend.Command }
	// composeText puts text in an empty composer (/msg's message).
	composeText struct{ text string }
)

// commandPopup is the list shown above the composer while something
// can be completed: slash commands while a command name is typed, and
// people, channels or emoji while an @, # or : token is (chat.Trigger).
type commandPopup struct {
	cmds      []backend.Command // everything the workspace offers
	matches   []backend.Command // the slash commands the text matches, at most popupRows
	choices   []chat.Choice     // the @, # or : completions, at most popupRows
	start     int               // where the token the choices replace starts (runes)
	sel       int
	dismissed string // Escape hides the popup until the text changes
	rows      [popupRows]widget.Clickable
}

// completionSources is what @, # and : complete from.
type completionSources struct {
	people  []backend.Person
	groups  []backend.UserGroup
	convs   []backend.Conversation
	customs map[string]string
}

// update recomputes what the popup lists for the composer's text with
// the caret at caret (runes). While editing a message there are no
// commands, only completions.
func (p *commandPopup) update(text string, caret int, src func() completionSources, editing bool) {
	if text != p.dismissed {
		p.dismissed = ""
	}
	prevN, prevFirst := p.n(), p.first()
	p.matches, p.choices = nil, nil
	if p.dismissed == "" {
		if !editing {
			p.matches = chat.MatchCommands(p.cmds, text)
		}
		if len(p.matches) == 0 {
			if sigil, q, start, ok := chat.Trigger(text, caret); ok {
				s := src()
				p.start = start
				switch sigil {
				case '@':
					p.choices = chat.MatchPeople(q, s.people, s.groups, popupRows)
				case '#':
					p.choices = chat.MatchChannels(q, s.convs, popupRows)
				case ':':
					p.choices = chat.EmojiChoices(q, s.customs, popupRows)
				}
			}
		}
	}
	if len(p.matches) > popupRows {
		p.matches = p.matches[:popupRows]
	}
	if p.n() != prevN || p.first() != prevFirst || p.sel >= p.n() {
		p.sel = 0
	}
}

// n is how many rows the popup lists.
func (p *commandPopup) n() int { return len(p.matches) + len(p.choices) }

// first names the first row, to notice the list changing under the
// highlight.
func (p *commandPopup) first() string {
	switch {
	case len(p.matches) > 0:
		return p.matches[0].Name
	case len(p.choices) > 0:
		return p.choices[0].Insert
	}
	return ""
}

func (p *commandPopup) open() bool { return p.n() > 0 }

// findConversation matches the start of target ("#general", "@Ada
// Lovelace hi") against conversation names, case- and accent-blind,
// preferring the longest name. dmOnly limits it to direct messages.
// rest is what follows the name.
func findConversation(convs []backend.Conversation, target string, dmOnly bool) (backend.Conversation, string, bool) {
	t := strings.TrimLeft(target, "#@")
	folded := text.Fold(t)
	var best backend.Conversation
	bestLen := -1
	for _, cv := range convs {
		if dmOnly && cv.Kind != backend.KindDM && cv.Kind != backend.KindGroupDM {
			continue
		}
		name := text.Fold(cv.Name)
		if name == "" || !strings.HasPrefix(folded, name) {
			continue
		}
		if after := folded[len(name):]; after != "" && !strings.HasPrefix(after, " ") {
			continue
		}
		if len(name) > bestLen {
			best, bestLen = cv, len(name)
		}
	}
	if bestLen < 0 {
		return backend.Conversation{}, "", false
	}
	// Folding can change byte lengths; cut by the name's rune count.
	runes := []rune(t)
	n := utf8.RuneCountInString(best.Name)
	if n > len(runes) {
		n = len(runes)
	}
	return best, string(runes[n:]), true
}

// popupList draws a completion popup's n rows (at most popupRows), row
// i drawn by row, the selected one lit. The caller handles the rows'
// clicks before drawing.
func popupList(gtx C, rows *[popupRows]widget.Clickable, n, sel int, row func(gtx C, i int) D) D {
	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
		return widget.Border{Color: colBorder, CornerRadius: unit.Dp(6), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
			children := make([]layout.FlexChild, 0, n)
			for i := 0; i < n; i++ {
				children = append(children, layout.Rigid(func(gtx C) D {
					return rows[i].Layout(gtx, func(gtx C) D {
						pointer.CursorPointer.Add(gtx.Ops)
						bg := colBg
						if i == sel {
							bg = colHover
						}
						return rounded(gtx, bg, unit.Dp(4), func(gtx C) D {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
								return row(gtx, i)
							})
						})
					})
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

// choiceRow is one completion: a person's avatar, a channel's glyph or
// an emoji, then its name and, muted, a detail.
func choiceRow(gtx C, th *material.Theme, rv *richView, c chat.Choice) D {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(20)), 0)
				switch {
				case c.ID != "" && rv != nil:
					return rv.avatar(gtx, c.ID, c.Avatar, c.Label, unit.Dp(20))
				case c.Emoji != "" && rv != nil:
					return rv.emoji(gtx, c.Emoji, unit.Dp(18))
				case c.Kind != "":
					g := bodyLabel(th, slackfmt.ChannelGlyph(string(c.Kind)))
					g.Color = colMuted
					return g.Layout(gtx)
				}
				return D{Size: gtx.Constraints.Min}
			})
		}),
		layout.Rigid(func(gtx C) D {
			lbl := bodyLabel(th, c.Label)
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx C) D {
			if c.Detail == "" {
				return D{}
			}
			return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				lbl := captionLabel(th, c.Detail)
				lbl.Color = colMuted
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			})
		}),
	)
}
