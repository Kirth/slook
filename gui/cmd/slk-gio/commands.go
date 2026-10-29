package main

import (
	"strings"
	"unicode/utf8"

	"gioui.org/widget"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/internal/text"
)

// popupRows is how many commands the popup lists at once.
const popupRows = 8

// Things a chat window's inbox carries for commands.
type (
	commandsLoaded struct{ cmds []backend.Command }
	// composeText puts text in an empty composer (/msg's message).
	composeText struct{ text string }
)

// commandPopup is the list of slash commands shown above the composer
// while a command name is being typed.
type commandPopup struct {
	cmds      []backend.Command // everything the workspace offers
	matches   []backend.Command // what the current text matches, at most popupRows
	sel       int
	dismissed string // Escape hides the popup until the text changes
	rows      [popupRows]widget.Clickable
}

// update recomputes the matches for the composer's text.
func (p *commandPopup) update(text string) {
	if text != p.dismissed {
		p.dismissed = ""
	}
	prev := p.matches
	p.matches = nil
	if p.dismissed == "" {
		p.matches = chat.MatchCommands(p.cmds, text)
	}
	if len(p.matches) > popupRows {
		p.matches = p.matches[:popupRows]
	}
	if len(prev) != len(p.matches) || len(p.matches) > 0 && prev[0].Name != p.matches[0].Name {
		p.sel = 0
	}
	if p.sel >= len(p.matches) {
		p.sel = 0
	}
}

func (p *commandPopup) open() bool { return len(p.matches) > 0 }

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
