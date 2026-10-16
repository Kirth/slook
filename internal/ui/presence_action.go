package ui

import "github.com/gammons/slk/internal/core"

// applyPresenceAction applies a presence choice for the workspace on
// screen. Optimistic: the cached status and the status bar change at
// once, before the API call returns; the WebSocket echo reaffirms them.
// The presence menu, its custom snooze and the /away family share it.
func (a *App) applyPresenceAction(action core.PresenceAction, snoozeMinutes int) {
	st := a.presence.Apply(a.activeTeamID, action, snoozeMinutes)
	a.statusbar.SetStatus(st.Presence, st.DNDEnabled, st.DNDEndTS)
	if a.presenceSvc != nil {
		a.presenceSvc.SetStatus(action, snoozeMinutes)
	}
}
