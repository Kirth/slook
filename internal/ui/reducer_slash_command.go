// internal/ui/reducer_slash_command.go
//
// Result of a slash command run through CommandService, and the
// server's answer to a /open lookup.
package ui

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
)

var reduceSlashCommand reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	if lookup, ok := msg.(channelLookupMsg); ok {
		// An answer for a workspace the user has since left is dropped;
		// the command goes back to the conversation it was typed in.
		if lookup.teamID != a.activeTeamID {
			a.offerTypedBack(lookup.origin)
			return nil, true
		}
		if !lookup.found {
			return a.refuseCommand(lookup.origin, "No channel named #"+lookup.name), true
		}
		return a.openChannel(channelResultOf(lookup.item)), true
	}
	m, ok := msg.(commandRanMsg)
	if !ok {
		return nil, false
	}
	if m.err == nil {
		return nil, true
	}
	a.offerTypedBack(commandOrigin{teamID: m.teamID, channelID: m.channelID, threadTS: m.threadTS, typed: m.text})
	return toastWithClear(a, commandFailureText(m.name, m.err), 4*time.Second), true
}

// commandFailureText is the toast for a command that did not run.
func commandFailureText(name string, err error) string {
	var ce *core.CommandError
	switch {
	case errors.As(err, &ce) && ce.Code == "unknown_command":
		return name + " is not a valid command"
	case errors.As(err, &ce):
		return name + " failed: " + ce.Code
	case errors.Is(err, errors.ErrUnsupported):
		return name + " failed: not available"
	default:
		return name + " failed: " + err.Error()
	}
}
