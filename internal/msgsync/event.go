package msgsync

import (
	"fmt"
	"slices"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
)

// ApplyEvent writes a WebSocket message frame to the cache. connected
// is whether the socket is up (it is, since the frame arrived; the
// parameter exists for callers replaying a queue after a drop).
//
//   - A new message is upserted with the frame as its raw_json and
//     extends the channel's live span, or starts one when there is none
//     (the socket vouches for nothing before it). A reply also counts on
//     its cached parent.
//   - A message_changed is applied with cache.ApplyEdit: text, raw_json,
//     the edit's ts, and the thread_ts / reply_count the server reports
//     for a parent; nothing else. An edit of an uncached message changes
//     nothing.
//   - A message_deleted soft-deletes.
func (s *Syncer) ApplyEvent(ev slackclient.MessageEvent, connected bool) (Changed, error) {
	changed := Changed{ChannelID: ev.ChannelID}
	m := ev.Message
	switch ev.Kind {
	case slackclient.MessageNew:
		if m.Timestamp == "" {
			return changed, nil
		}
		row := Row(s.WorkspaceID, ev.ChannelID, m, ev.Raw, "", s.now())
		if err := s.DB.UpsertMessages([]cache.Message{row}); err != nil {
			return changed, err
		}
		changed.Upserted = []cache.Message{row}
		if IsReply(row) {
			if err := s.DB.IncrementReplyCount(ev.ChannelID, row.ThreadTS); err != nil {
				return changed, err
			}
		}
		if InFeed(row) {
			extended, err := s.DB.ExtendLiveSpan(ev.ChannelID, row.TS)
			if err != nil {
				return changed, err
			}
			if !extended && connected {
				if err := s.DB.AddSpan(cache.Span{
					ChannelID: ev.ChannelID, WorkspaceID: s.WorkspaceID,
					OldestTS: row.TS, LatestTS: row.TS, Live: true,
				}); err != nil {
					return changed, err
				}
			}
		}
	case slackclient.MessageChanged:
		edit := cache.Edit{
			Text:       m.Text,
			RawJSON:    string(ev.Raw),
			ThreadTS:   m.ThreadTimestamp,
			ReplyCount: m.ReplyCount,
		}
		if m.Edited != nil {
			edit.EditedAt = m.Edited.Timestamp
		}
		if edit.RawJSON == "" {
			edit.RawJSON = Row(s.WorkspaceID, ev.ChannelID, m, nil, "", s.now()).RawJSON
		}
		applied, err := s.DB.ApplyEdit(ev.ChannelID, m.Timestamp, edit)
		if err != nil {
			return changed, err
		}
		if !applied {
			return changed, nil
		}
		row, err := s.DB.GetMessage(ev.ChannelID, m.Timestamp)
		if err != nil {
			return changed, fmt.Errorf("reading edited message: %w", err)
		}
		changed.Upserted = []cache.Message{row}
	case slackclient.MessageDeleted:
		if err := s.DB.DeleteMessage(ev.ChannelID, m.Timestamp); err != nil {
			return changed, err
		}
		changed.Deleted = []string{m.Timestamp}
	}
	return changed, nil
}

// ApplyReaction writes a reaction_added (added=true) or reaction_removed
// event: the reaction's user list and count are adjusted, and a
// reaction nobody holds any more is dropped.
func (s *Syncer) ApplyReaction(channelID, ts, userID, emoji string, added bool) error {
	reactions, err := s.DB.GetReactions(ts, channelID)
	if err != nil {
		return err
	}
	var current *cache.ReactionRow
	for i := range reactions {
		if reactions[i].Emoji == emoji {
			current = &reactions[i]
			break
		}
	}
	switch {
	case added && current == nil:
		return s.DB.UpsertReaction(ts, channelID, emoji, []string{userID}, 1)
	case added:
		if slices.Contains(current.UserIDs, userID) {
			return nil
		}
		return s.DB.UpsertReaction(ts, channelID, emoji, append(current.UserIDs, userID), current.Count+1)
	case current == nil:
		return nil
	default:
		users := slices.DeleteFunc(slices.Clone(current.UserIDs), func(u string) bool { return u == userID })
		if len(users) == len(current.UserIDs) {
			return nil
		}
		if len(users) == 0 || current.Count <= 1 {
			return s.DB.DeleteReaction(ts, channelID, emoji)
		}
		return s.DB.UpsertReaction(ts, channelID, emoji, users, current.Count-1)
	}
}
