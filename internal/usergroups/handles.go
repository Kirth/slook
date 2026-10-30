package usergroups

import (
	"strings"

	"github.com/slack-go/slack"
)

// Handles maps each usergroup's ID to the handle a mention of it uses
// (the "eng" of @eng). Groups with nothing mentionable are left out.
func Handles(groups []slack.UserGroup) map[string]string {
	byID := make(map[string]string, len(groups))
	for _, g := range groups {
		if h := Handle(g); h != "" {
			byID[g.ID] = h
		}
	}
	return byID
}

// Handle is a usergroup's mention handle: its own, or its display name
// slugified ("Platform Team" -> "platform-team"). A name with a space
// in it would be a handle neither a composer's @-completion nor a
// send-time translation can round-trip. "" when nothing usable is left.
func Handle(g slack.UserGroup) string {
	if g.Handle != "" {
		return g.Handle
	}
	return slugifyHandle(g.Name)
}

// slugifyHandle turns a usergroup display name into a mention-safe
// handle: lowercased, with every run of characters Slack handles don't
// use collapsed to a single "-" ("Platform Team" -> "platform-team").
// Returns "" when nothing usable survives, so the caller can skip the
// group entirely rather than register an unmentionable handle.
func slugifyHandle(name string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.':
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingDash = false
			b.WriteRune(r)
		default:
			pendingDash = true
		}
	}
	return b.String()
}
