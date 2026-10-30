package main

import (
	"strings"
	"unicode/utf8"

	"gioui.org/widget"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
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
					p.choices = chat.MatchPeople(q, s.people, popupRows)
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
