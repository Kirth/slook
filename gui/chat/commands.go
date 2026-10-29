package chat

import (
	"strings"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/internal/slashcmd"
)

// MatchCommands is what the composer's command popup lists for text:
// nothing unless the text is still a command name being typed ("/" and
// name characters, no space yet), else the commands whose names start
// with it, an exact match first and the rest in the given order.
func MatchCommands(cmds []backend.Command, text string) []backend.Command {
	if !slashcmd.IsNamePrefix(text) {
		return nil
	}
	prefix := strings.ToLower(text)
	var exact, rest []backend.Command
	for _, c := range cmds {
		switch {
		case c.Name == prefix:
			exact = append(exact, c)
		case strings.HasPrefix(c.Name, prefix):
			rest = append(rest, c)
		}
	}
	return append(exact, rest...)
}

// IsCommand reports whether name is one of cmds.
func IsCommand(cmds []backend.Command, name string) bool {
	for _, c := range cmds {
		if c.Name == name {
			return true
		}
	}
	return false
}
