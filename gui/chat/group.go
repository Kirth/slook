package chat

import (
	"time"

	"github.com/gammons/slk/gui/backend"
)

// GroupGap is how close together two messages by the same author must
// be to share one author line.
const GroupGap = 5 * time.Minute

// NewDay reports whether m is the first message of its day after prev
// (nil for the first message shown), where a date separator goes.
func NewDay(prev *backend.Message, m backend.Message) bool {
	return prev == nil || !SameDay(prev.Time, m.Time)
}

// StartsGroup reports whether m begins a new group after prev (nil for
// the first message shown), and so draws its author line and avatar: a
// new day, another author (by ID and by name, since an app can post
// under several names), a pause longer than GroupGap, or an ephemeral
// on either side, which always stands alone.
func StartsGroup(prev *backend.Message, m backend.Message) bool {
	return NewDay(prev, m) || prev.AuthorID != m.AuthorID || prev.Author != m.Author ||
		m.Time.Sub(prev.Time) > GroupGap || m.Ephemeral || prev.Ephemeral
}

// SameDay reports whether a and b fall on the same local calendar day.
func SameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
