package adapt

import (
	"context"
	"slices"

	"github.com/gammons/slk/internal/bootstrap"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/slack/boot"
	"github.com/gammons/slk/internal/slack/edge"
	"github.com/slack-go/slack"
)

// Conversations converts what client.userBoot returned into the
// slack.Channel values a conversation list is built from.
//
// It exists because users.conversations is not reliably available. On
// the Enterprise Grid org in gammons/slk#5 that call is rejected, and
// because connectWorkspace treated the failure as fatal the whole
// workspace was dropped: no channels, no threads, and no active
// workspace at all. userBoot had already returned all 217 of that
// user's conversations moments earlier, so slk was discarding a
// session over data it already held.
//
// The mapping only has to satisfy the list builders (the TUI's
// buildChannelItem read ID, Name, Topic, IsIM, IsMpIM, IsPrivate,
// IsMember and User; the Gio frontend's conversationOfLocked reads a
// subset of those). Everything
// else on slack.Channel is left zero deliberately: a field invented
// here would read as measured downstream.
//
// IsMember is true for every channels[] entry because userBoot returns
// the user's OWN conversation list -- there is no is_member on the
// entries and none is needed. Ordinary IMs are filtered to the open ones;
// MissingStarredConversations separately recovers accessible closed starred
// IMs. Both the per-IM is_open flag and userBoot's top-level is_open list
// are consulted, because either alone would silently empty the DM list if
// that response shape varies.
//
// MPIMs get the same treatment via the top-level is_open list only:
// userBoot is asked for return_all_relevant_mpdms=true, so channels[]
// carries every group DM the user was ever in, and the official client
// shows just the open ones. See DropClosedMPIMs.
func Conversations(res *bootstrap.Result) []slack.Channel {
	if res == nil {
		return nil
	}
	open := make(map[string]bool, len(res.IsOpen))
	for _, id := range res.IsOpen {
		open[id] = true
	}
	out := make([]slack.Channel, 0, len(res.Channels)+len(res.IMs))
	for _, ch := range res.Channels {
		if ch.IsArchived {
			continue
		}
		if ch.IsMPIM && len(open) > 0 && !open[ch.ID] {
			continue
		}
		out = append(out, slack.Channel{
			GroupConversation: slack.GroupConversation{
				Conversation: slack.Conversation{
					ID:          ch.ID,
					IsGroup:     ch.IsGroup,
					IsPrivate:   ch.IsPrivate,
					IsMpIM:      ch.IsMPIM,
					IsShared:    ch.IsShared,
					IsOrgShared: ch.IsOrgShared,
					IsExtShared: ch.IsExtShared,
				},
				Name:    ch.Name,
				Creator: ch.Creator,
				Topic:   slack.Topic{Value: ch.Topic.Value},
				Purpose: slack.Purpose{Value: ch.Purpose.Value},
			},
			IsChannel: ch.IsChannel,
			IsGeneral: ch.IsGeneral,
			IsMember:  true,
		})
	}
	for _, im := range res.IMs {
		if im.IsArchived {
			continue
		}
		if !im.IsOpen && !open[im.ID] {
			continue
		}
		out = append(out, IMConversation(im))
	}
	return out
}

// IMConversation maps userBoot IM metadata into the conversation shape
// a list is built from. Eligibility (open, starred, archived) belongs
// to the caller; sharing this mapping keeps ordinary and starred DMs identical.
func IMConversation(im boot.IM) slack.Channel {
	return slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{
				ID:          im.ID,
				IsIM:        true,
				User:        im.UserID,
				IsOrgShared: im.IsOrgShared,
			},
			IsArchived: im.IsArchived,
		},
		IsMember: true,
	}
}

// DropClosedMPIMs removes group DMs (MPIMs) that are not in userBoot's
// top-level is_open list.
//
// users.conversations returns every mpim the user is a member of, and
// Slack never removes a user from a group DM -- closing one in the
// official client only takes it out of is_open. So a workspace of any
// age accumulates hundreds of closed group DMs that the official
// client never shows, and slk was listing all of them. The sidebar's
// staleness filter catches most, but not those a custom Slack section
// claims (sections exempt their members from hiding), which is exactly
// where a group DM with a departed colleague tends to sit.
//
// Only MPIMs are filtered. 1:1 DMs are left to the existing staleness
// path: the finder relies on closed DMs being present to offer "message
// this person", and the sidebar already hides the stale ones.
//
// An empty isOpen disables the filter. It cannot be told apart from
// "userBoot did not return the field", and hiding every group DM on
// that evidence would be a silent regression. A closed MPIM that
// receives a message is re-added by the mpim_open / message-discovery
// paths, the same way the official client reopens it.
//
// Conversations already applies the same rule to userBoot's own list,
// so this is for a list that came from elsewhere (users.conversations).
// logf (nil discards) hears how many were hidden.
func DropClosedMPIMs(channels []slack.Channel, isOpen []string, logf func(string, ...any)) []slack.Channel {
	if channels == nil || len(isOpen) == 0 {
		return channels
	}
	open := make(map[string]bool, len(isOpen))
	for _, id := range isOpen {
		open[id] = true
	}
	out := make([]slack.Channel, 0, len(channels))
	dropped := 0
	for _, ch := range channels {
		if ch.IsMpIM && !open[ch.ID] {
			dropped++
			continue
		}
		out = append(out, ch)
	}
	if dropped > 0 && logf != nil {
		logf("DropClosedMPIMs: hid %d closed group DMs (is_open has %d ids)", dropped, len(isOpen))
	}
	return out
}

// MostRecentlyVisited returns the channel ID with the latest
// last-visited timestamp, or "" when there are no recorded visits (e.g.
// the first run). It is the conversation a boot opens
// (conversations.view), as Slack's own client opens the last one viewed.
func MostRecentlyVisited(visits map[string]int64) string {
	var bestID string
	var bestTS int64
	for id, ts := range visits {
		if ts > bestTS {
			bestTS = ts
			bestID = id
		}
	}
	return bestID
}

// FillFromCounts returns the conversations client.counts names that
// client.userBoot did not list, so a boot-built list is not missing
// them.
//
// userBoot's channels[] is a SUBSET of the user's conversations on real
// workspaces: 67 of 218 on one, 60 of 71 on another (measured when the
// TUI tried booting from it, which is why it kept users.conversations
// as its primary list). client.counts, which every boot makes anyway,
// names every conversation with read state, so the ids it carries that
// userBoot left out are the gap. They are filled without enumerating:
//
//  1. from the cache, rows an earlier session wrote (a frontend's own
//     list, or an earlier fill) — no request;
//  2. the rest from one edge channels/info batch (updated_ids = 0, at
//     most 80 ids a request, inside edge.ChannelsInfo), scoped to
//     teamID, each answer upserted into the cache so the next boot
//     finds it in step 1.
//
// What neither fills is left out and counted in the log line. Two kinds
// are expected: DMs (edge fails every IM, 22 of 22 in the captures, and
// a cached DM row does not name its peer, without whom a DM cannot be
// listed), and on Enterprise Grid conversations owned by another team
// (channels/info scoped to the workspace's team fails them). Archived
// conversations are left out, as Conversations leaves them out. e may
// be nil (no edge client): step 2 is skipped. db may be nil: step 1 is
// skipped and nothing is cached.
//
// logf (nil discards) gets one line with the gap's size and how it was
// filled; a user checking a real workspace reads it there.
func FillFromCounts(ctx context.Context, db *cache.DB, e *edge.Client, teamID string, res *bootstrap.Result, logf func(string, ...any)) []slack.Channel {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if res == nil {
		return nil
	}
	known := make(map[string]bool, len(res.Channels)+len(res.IMs))
	for _, ch := range res.Channels {
		known[ch.ID] = true
	}
	for _, im := range res.IMs {
		known[im.ID] = true
	}
	var gap []string
	for _, u := range res.Counts.Unreads {
		if u.ChannelID == "" || known[u.ChannelID] {
			continue
		}
		known[u.ChannelID] = true // once, if counts repeats an id
		gap = append(gap, u.ChannelID)
	}
	if len(gap) == 0 {
		logf("boot: gap size 0 (client.counts names no conversation userBoot did not list)")
		return nil
	}

	var out []slack.Channel
	var missing []string
	fromCache := 0
	for _, id := range gap {
		if db != nil {
			if row, err := db.GetChannel(id); err == nil {
				if ch, ok := cachedConversation(row); ok {
					out = append(out, ch)
					fromCache++
				}
				// A cached DM (no peer) is not asked of edge either:
				// edge fails every IM.
				continue
			}
		}
		missing = append(missing, id)
	}

	fromEdge := 0
	if e != nil && len(missing) > 0 {
		updated := make(map[string]int64, len(missing))
		for _, id := range missing {
			updated[id] = 0 // never seen: send the full record
		}
		got, err := e.ChannelsInfo(ctx, teamID, updated)
		if err != nil {
			logf("boot: channels/info for %d conversations client.counts named: %v", len(missing), err)
		}
		for _, ch := range got.Channels {
			if !slices.Contains(missing, ch.ID) || ch.IsArchived || ch.IsIM {
				continue
			}
			out = append(out, edgeConversation(ch))
			fromEdge++
			if db != nil {
				if err := db.UpsertChannel(cache.Channel{
					ID: ch.ID, WorkspaceID: teamID, Name: ch.Name, Type: edgeChannelType(ch),
					Topic: ch.Topic.Value,
					// client.counts lists only the user's own
					// conversations.
					IsMember: true,
				}); err != nil {
					logf("boot: caching %s: %v", ch.ID, err)
				}
			}
		}
	}
	logf("boot: gap size %d (conversations client.counts names that userBoot did not list): %d from the cache, %d from channels/info, %d left out",
		len(gap), fromCache, fromEdge, len(gap)-fromCache-fromEdge)
	return out
}

// cachedConversation is a cached conversation row as a list entry, and
// false for a DM, whose row does not name the peer.
func cachedConversation(row cache.Channel) (slack.Channel, bool) {
	ch := slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{ID: row.ID},
			Name:         row.Name,
			Topic:        slack.Topic{Value: row.Topic},
		},
		IsMember: true,
	}
	switch row.Type {
	case "channel":
		ch.IsChannel = true
	case "private":
		ch.IsPrivate = true
	case "group_dm":
		ch.IsMpIM, ch.IsPrivate = true, true
	default: // "dm", "app"
		return slack.Channel{}, false
	}
	return ch, true
}

// edgeConversation is a channels/info record as a list entry.
func edgeConversation(ch edge.Channel) slack.Channel {
	return slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{
				ID:        ch.ID,
				IsGroup:   ch.IsGroup,
				IsPrivate: ch.IsPrivate,
				IsMpIM:    ch.IsMPIM,
				IsIM:      ch.IsIM,
			},
			Name:       ch.Name,
			Topic:      slack.Topic{Value: ch.Topic.Value},
			IsArchived: ch.IsArchived,
		},
		IsChannel: ch.IsChannel,
		IsMember:  true,
	}
}

// edgeChannelType is the cache's type column for a channels/info
// record, MPIM before private (an MPIM is private too), as
// HydrateFirstSight files userBoot's.
func edgeChannelType(ch edge.Channel) string {
	switch {
	case ch.IsMPIM:
		return "group_dm"
	case ch.IsPrivate:
		return "private"
	}
	return "channel"
}
