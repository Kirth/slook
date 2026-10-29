package msgsync

import (
	"fmt"
	"time"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
)

// Ingest applies one conversations.history page to the cache: its
// bodies become rows (with their versions and reactions), the rows in
// its range that it neither returned nor confirmed unchanged are
// soft-deleted, and the range it covers becomes a coverage span.
//
// anchor is how the page was asked for and requestStart when; connected
// says whether the WebSocket was up, which makes a span that reaches
// the head live. The range a page covers follows from its anchor:
//
//   - head (no anchor): [oldest returned, newest returned]; has_more=false
//     means the oldest returned is the channel's first message.
//   - Latest=X: [oldest returned, X], since the page was asked for
//     everything at or before X; X itself may be gone (deleted), which
//     is why the bound is X and not the newest returned.
//   - Oldest=X with has_more=false: [X, newest returned], the page held
//     everything after X.
//   - Oldest=X with has_more=true: [oldest returned, newest returned]
//     only. Slack anchors such a page at the head, not at X
//     (slackclient.GetHistoryAround documents it), so what lies between
//     X and the page is unknown and the span stays separate from the
//     one ending at X.
//
// A page with no messages and no unchanged ts covers only its anchor,
// or nothing when it had none (an empty channel).
func (s *Syncer) Ingest(res slackclient.HistoryResult, channelID string, anchor Anchor, requestStart time.Time, connected bool) (Changed, error) {
	changed := Changed{ChannelID: channelID}
	now := s.now()

	rows := make([]cache.Message, 0, len(res.Messages))
	keep := make([]string, 0, len(res.Messages)+len(res.UnchangedTS))
	lo, hi := "", ""
	track := func(ts string) {
		if lo == "" || tsLess(ts, lo) {
			lo = ts
		}
		if hi == "" || tsLess(hi, ts) {
			hi = ts
		}
	}
	for _, m := range res.Messages {
		if m.Timestamp == "" {
			continue
		}
		rows = append(rows, Row(s.WorkspaceID, channelID, m, nil, res.LatestUpdates[m.Timestamp], now))
		keep = append(keep, m.Timestamp)
		track(m.Timestamp)
	}
	for _, ts := range res.UnchangedTS {
		keep = append(keep, ts)
		track(ts)
	}

	if err := s.DB.UpsertMessages(rows); err != nil {
		return changed, err
	}
	for _, m := range res.Messages {
		for _, r := range m.Reactions {
			if err := s.DB.UpsertReaction(m.Timestamp, channelID, r.Name, r.Users, r.Count); err != nil {
				return changed, fmt.Errorf("caching reaction: %w", err)
			}
		}
	}
	sortRows(rows)
	changed.Upserted = rows

	reachesStart := !res.HasMore && anchor.Oldest == ""
	live := connected && anchor.Latest == ""
	switch {
	case anchor.Latest != "":
		if hi == "" || tsLess(hi, anchor.Latest) {
			hi = anchor.Latest
		}
		if lo == "" {
			lo = anchor.Latest
		}
	case anchor.Oldest != "" && !res.HasMore:
		if lo == "" || tsLess(anchor.Oldest, lo) {
			lo = anchor.Oldest
		}
		if hi == "" {
			hi = anchor.Oldest
		}
	}
	if lo == "" {
		// Nothing to cover: an empty channel, or a head-anchored page
		// that returned nothing while claiming more (not a shape Slack
		// sends).
		return changed, nil
	}

	pruned, err := s.DB.PruneChannelRange(channelID, lo, hi, keep, requestStart.Unix())
	if err != nil {
		return changed, err
	}
	changed.Deleted = pruned

	if err := s.DB.AddSpan(cache.Span{
		ChannelID:    channelID,
		WorkspaceID:  s.WorkspaceID,
		OldestTS:     lo,
		LatestTS:     hi,
		ReachesStart: reachesStart,
		Live:         live,
	}); err != nil {
		return changed, err
	}
	return changed, nil
}
