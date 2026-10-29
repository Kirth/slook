// Package sidebar arranges the conversation list: what needs reading
// first, then starred conversations, then the user's Slack sections,
// each with its recently active conversations shown and the rest
// folded away as quiet. A conversation that wakes up (a seasonal
// channel getting messages) rises by itself, and sinks again after.
//
// It is pure: the list window draws what Arrange returns.
package sidebar

import (
	"slices"
	"strings"
	"time"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/internal/text"
)

// QuietAfter is how long a conversation can go without messages or a
// visit before it is folded away.
const QuietAfter = 14 * 24 * time.Hour

// Group is one heading of the list.
type Group struct {
	Key   string // stable across rearrangements: "unread", "starred", a section ID
	Title string
	Emoji string
	Rows  []backend.Conversation // shown
	Quiet []backend.Conversation // folded behind "+N quiet"
}

// Activity is when a conversation was last alive for the user: its
// newest message or their last visit, whichever is later.
func Activity(c backend.Conversation) time.Time {
	if c.Visited.After(c.Latest) {
		return c.Visited
	}
	return c.Latest
}

// NeedsReading reports whether a conversation belongs in the Unread
// group: it mentions the user, or it has unread messages and is not
// muted. Mentions show through a mute, as in Slack.
func NeedsReading(c backend.Conversation) bool {
	return c.Mentions > 0 || c.Unread && !c.Muted
}

// Arrange groups and orders convs. sections is Slack's sidebar in
// order; with none, conversations fall into Channels and Direct
// messages.
func Arrange(convs []backend.Conversation, sections []backend.Section, now time.Time, quietAfter time.Duration) []Group {
	unread := Group{Key: "unread", Title: "Unread"}
	starred := Group{Key: "starred", Title: "Starred", Emoji: "★"}

	// Where conversations without a section of their own go.
	groups := map[string]*Group{}
	var order []*Group
	channelsKey, dmsKey := "", ""
	for _, s := range sections {
		switch s.Type {
		case "stars":
			continue // shown as Starred
		case "channels":
			channelsKey = s.ID
		case "direct_messages":
			dmsKey = s.ID
		}
		g := &Group{Key: s.ID, Title: s.Name, Emoji: s.Emoji}
		groups[s.ID] = g
		order = append(order, g)
	}
	if channelsKey == "" {
		channelsKey = "channels"
		g := &Group{Key: channelsKey, Title: "Channels"}
		groups[channelsKey] = g
		order = append(order, g)
	}
	if dmsKey == "" {
		dmsKey = "dms"
		g := &Group{Key: dmsKey, Title: "Direct messages"}
		groups[dmsKey] = g
		order = append(order, g)
	}

	for _, c := range convs {
		switch {
		case NeedsReading(c):
			unread.Rows = append(unread.Rows, c)
			continue
		case c.Starred:
			starred.Rows = append(starred.Rows, c)
			continue
		}
		g := groups[c.Section]
		if g == nil {
			g = groups[channelsKey]
			if c.Kind == backend.KindDM || c.Kind == backend.KindGroupDM {
				g = groups[dmsKey]
			}
		}
		if now.Sub(Activity(c)) <= quietAfter {
			g.Rows = append(g.Rows, c)
		} else {
			g.Quiet = append(g.Quiet, c)
		}
	}

	slices.SortStableFunc(unread.Rows, func(a, b backend.Conversation) int {
		if (a.Mentions > 0) != (b.Mentions > 0) {
			if a.Mentions > 0 {
				return -1
			}
			return 1
		}
		return b.Latest.Compare(a.Latest)
	})
	byActivity(starred.Rows)

	var out []Group
	for _, g := range []*Group{&unread, &starred} {
		if len(g.Rows) > 0 {
			out = append(out, *g)
		}
	}
	for _, g := range order {
		if len(g.Rows) == 0 && len(g.Quiet) == 0 {
			continue
		}
		byActivity(g.Rows)
		slices.SortStableFunc(g.Quiet, func(a, b backend.Conversation) int {
			if a.Muted != b.Muted {
				if b.Muted {
					return -1
				}
				return 1
			}
			return strings.Compare(text.Fold(a.Name), text.Fold(b.Name))
		})
		out = append(out, *g)
	}
	return out
}

// byActivity sorts the most recently alive first, ties by name.
func byActivity(cs []backend.Conversation) {
	slices.SortStableFunc(cs, func(a, b backend.Conversation) int {
		if c := Activity(b).Compare(Activity(a)); c != 0 {
			return c
		}
		return strings.Compare(text.Fold(a.Name), text.Fold(b.Name))
	})
}

// Search is every conversation whose name contains query (case- and
// accent-blind), quiet or not: names starting with it first, then the
// most recently alive.
func Search(convs []backend.Conversation, query string) []backend.Conversation {
	q := text.Fold(strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(query), "#@")))
	var prefix, inner []backend.Conversation
	for _, c := range convs {
		name := text.Fold(c.Name)
		switch {
		case strings.HasPrefix(name, q):
			prefix = append(prefix, c)
		case strings.Contains(name, q):
			inner = append(inner, c)
		}
	}
	byActivity(prefix)
	byActivity(inner)
	return append(prefix, inner...)
}
