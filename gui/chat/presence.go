package chat

import (
	"time"

	"github.com/gammons/slk/gui/backend"
)

// PresenceItem is one choice of the presence menu: be active, be away,
// snooze notifications for Minutes, or end do-not-disturb. Current
// marks the state the user is in.
type PresenceItem struct {
	Label   string
	Away    bool
	Snooze  bool
	End     bool
	Minutes int
	Current bool
}

// snoozes are the menu's fixed snooze lengths, as the TUI's menu had
// them.
var snoozes = []struct {
	label   string
	minutes int
}{
	{"Snooze for 20 minutes", 20},
	{"Snooze for 1 hour", 60},
	{"Snooze for 2 hours", 120},
	{"Snooze for 4 hours", 240},
	{"Snooze for 8 hours", 480},
	{"Snooze for 24 hours", 1440},
}

// PresenceItems is the presence menu for st at now: Active and Away
// (the one the user is in marked, unless do-not-disturb is on: then
// none is, the header says it), the snoozes, until tomorrow morning,
// and while do-not-disturb is on, a way to end it.
func PresenceItems(st backend.SelfStatus, now time.Time) []PresenceItem {
	items := []PresenceItem{
		{Label: "Active", Current: !st.Away && !st.DND},
		{Label: "Away", Away: true, Current: st.Away && !st.DND},
	}
	for _, s := range snoozes {
		items = append(items, PresenceItem{Label: s.label, Snooze: true, Minutes: s.minutes})
	}
	items = append(items, PresenceItem{Label: "Snooze until tomorrow morning", Snooze: true, Minutes: MinutesUntilTomorrowMorning(now)})
	if st.DND {
		items = append(items, PresenceItem{Label: "End do not disturb", End: true})
	}
	return items
}

// MinutesUntilTomorrowMorning is the minutes from now until 09:00 local
// time on the next weekday (Monday after a Friday, Saturday or
// Sunday); at least 1.
func MinutesUntilTomorrowMorning(now time.Time) int {
	loc := now.Location()
	target := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, loc).AddDate(0, 0, 1)
	for target.Weekday() == time.Saturday || target.Weekday() == time.Sunday {
		target = target.AddDate(0, 0, 1)
	}
	return max(int(target.Sub(now).Minutes()), 1)
}

// SelfText words the user's own state for the list's header: "Away",
// "Do not disturb until 17:00", both, or "" while simply active.
func SelfText(st backend.SelfStatus, now time.Time) string {
	dnd := ""
	if st.DND {
		dnd = DNDText(st.DNDUntil, now)
	}
	switch {
	case st.Away && dnd != "":
		return "Away · " + dnd
	case st.Away:
		return "Away"
	case dnd != "":
		return "D" + dnd[1:]
	}
	return ""
}
