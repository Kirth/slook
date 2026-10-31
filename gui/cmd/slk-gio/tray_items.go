package main

import (
	"time"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
)

// trayPresenceLabels are the presence choices the tray's menu offers,
// a short form of the presence menu's: Active, Away, a one-hour snooze,
// and ending do-not-disturb.
var trayPresenceLabels = []string{"Active", "Away", "Snooze for 1 hour", "End do not disturb"}

// trayPresenceItems is what the tray's menu offers for st: the
// presence menu's items named in trayPresenceLabels that it has now
// (End only while do-not-disturb is on), in that order.
func trayPresenceItems(st backend.SelfStatus, now time.Time) []chat.PresenceItem {
	all := chat.PresenceItems(st, now)
	var out []chat.PresenceItem
	for _, label := range trayPresenceLabels {
		for _, it := range all {
			if it.Label == label {
				out = append(out, it)
			}
		}
	}
	return out
}

// trayTooltip is the tray icon's tooltip: the workspace, and the
// user's state when it is not simply active.
func trayTooltip(workspace string, st backend.SelfStatus, now time.Time) string {
	tip := "slk — " + workspace
	if s := chat.SelfText(st, now); s != "" {
		tip += " · " + s
	}
	return tip
}
