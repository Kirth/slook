package msgsync

import (
	"context"
	"fmt"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
)

// VersionWindow is how many of a channel's newest cached messages a
// history request vouches for through cached_latest_updates: 28, the
// page size the official client asks for, so the window is the page.
const VersionWindow = 28

// VersionStore is the slice of *cache.DB Versions reads. It is an
// interface so a test can observe the window asked for: "the request
// never vouches for an unbounded window" is the property that must not
// regress.
type VersionStore interface {
	GetMessages(channelID string, limit int, beforeTS string) ([]cache.Message, error)
	MessageVersions(channelID, oldestTS, latestTS string) (map[string]string, error)
}

// Versions returns {ts: version} for at most VersionWindow of the
// channel's newest cached messages, the map a head page sends as
// cached_latest_updates so the server answers "unchanged" for them
// instead of resending bodies.
//
// The window is derived from the rows themselves rather than from a
// timestamp arithmetic guess: GetMessages returns the newest N
// (ascending), so its first and last ts are the bounds exactly, with
// no assumption about how densely a channel is posted in. A quiet
// channel and a firehose both get 28 entries rather than "everything
// in the last hour" being 1 in one and 4000 in the other.
//
// The window is a ts range, so a thread reply inside it that
// GetMessages filters out of the main feed is still included. That is
// left alone deliberately: the request sets ignore_replies, so the
// server simply says nothing about it.
func Versions(store VersionStore, channelID string) (map[string]string, error) {
	return versionsBefore(store, channelID, "")
}

// versionsBefore is Versions for the page at or before beforeTS ("" is
// the head).
func versionsBefore(store VersionStore, channelID, beforeTS string) (map[string]string, error) {
	recent, err := store.GetMessages(channelID, VersionWindow, beforeTS)
	if err != nil {
		return nil, fmt.Errorf("reading recent messages for %s: %w", channelID, err)
	}
	// The page is asked inclusive of beforeTS, so that row (a span's
	// oldest, when paging back) is vouched for too: the window is
	// [oldest of the recent rows, beforeTS], or just beforeTS when
	// nothing older is cached yet.
	lo, hi := beforeTS, beforeTS
	if len(recent) > 0 {
		lo = recent[0].TS
		if beforeTS == "" {
			hi = recent[len(recent)-1].TS
		}
	}
	if lo == "" {
		// nil becomes "{}" on the wire, which is what the official
		// client sends when it holds nothing.
		return nil, nil
	}
	return store.MessageVersions(channelID, lo, hi)
}

// page fetches one history page, sleeping out a rate limit and asking
// again for the same page.
func (s *Syncer) page(ctx context.Context, channelID string, opts slackclient.HistoryOpts) (slackclient.HistoryResult, error) {
	for {
		res, err := s.Source.HistoryWithVersions(ctx, channelID, opts)
		if err == nil {
			return res, nil
		}
		limited, waitErr := slackclient.WaitOutRateLimit(ctx, err)
		if waitErr != nil {
			return res, waitErr
		}
		if !limited {
			return res, err
		}
	}
}

// Open fetches the channel's head page, vouching for the newest cached
// messages so unchanged ones cost no bodies, and ingests it. It is what
// a window open does; the result is the window's catch-up.
func (s *Syncer) Open(ctx context.Context, channelID string, connected bool) (Changed, error) {
	versions, err := Versions(s.DB, channelID)
	if err != nil {
		return Changed{ChannelID: channelID}, err
	}
	start := s.now()
	res, err := s.page(ctx, channelID, slackclient.HistoryOpts{CachedVersions: versions})
	if err != nil {
		return Changed{ChannelID: channelID}, err
	}
	return s.Ingest(res, channelID, Anchor{}, start, connected)
}

// Older fetches the page at or before beforeTS and ingests it. Anchored
// inclusive, so beforeTS (normally a span's oldest message) is in the
// page and the page joins that span.
func (s *Syncer) Older(ctx context.Context, channelID, beforeTS string, connected bool) (Changed, error) {
	versions, err := versionsBefore(s.DB, channelID, beforeTS)
	if err != nil {
		return Changed{ChannelID: channelID}, err
	}
	start := s.now()
	res, err := s.page(ctx, channelID, slackclient.HistoryOpts{Latest: beforeTS, CachedVersions: versions})
	if err != nil {
		return Changed{ChannelID: channelID}, err
	}
	return s.Ingest(res, channelID, Anchor{Latest: beforeTS}, start, connected)
}

// CatchUp brings the channel's newest span to the head: one page from
// its latest message forward. With no span it is Open. When more than a
// page was posted since, the page becomes a separate span at the head
// and the hole below it waits for the next Older across it.
func (s *Syncer) CatchUp(ctx context.Context, channelID string, connected bool) (Changed, error) {
	spans, err := s.DB.Spans(channelID)
	if err != nil {
		return Changed{ChannelID: channelID}, err
	}
	if len(spans) == 0 {
		return s.Open(ctx, channelID, connected)
	}
	head := spans[len(spans)-1]
	versions, err := Versions(s.DB, channelID)
	if err != nil {
		return Changed{ChannelID: channelID}, err
	}
	start := s.now()
	res, err := s.page(ctx, channelID, slackclient.HistoryOpts{Oldest: head.LatestTS, CachedVersions: versions})
	if err != nil {
		return Changed{ChannelID: channelID}, err
	}
	return s.Ingest(res, channelID, Anchor{Oldest: head.LatestTS}, start, connected)
}

// Thread fetches a whole thread and caches it, soft-deleting cached
// replies Slack no longer returns. The parent comes back too and is
// upserted with its current reply_count.
func (s *Syncer) Thread(ctx context.Context, channelID, threadTS string) (Changed, error) {
	changed := Changed{ChannelID: channelID}
	start := s.now()
	msgs, err := s.Source.GetReplies(ctx, channelID, threadTS)
	if err != nil {
		return changed, err
	}
	now := s.now()
	rows := make([]cache.Message, 0, len(msgs))
	returned := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m.Timestamp == "" {
			continue
		}
		rows = append(rows, Row(s.WorkspaceID, channelID, m, nil, "", now))
		returned = append(returned, m.Timestamp)
	}
	if err := s.DB.UpsertMessages(rows); err != nil {
		return changed, err
	}
	for _, m := range msgs {
		for _, r := range m.Reactions {
			if err := s.DB.UpsertReaction(m.Timestamp, channelID, r.Name, r.Users, r.Count); err != nil {
				return changed, fmt.Errorf("caching reaction: %w", err)
			}
		}
	}
	sortRows(rows)
	changed.Upserted = rows
	pruned, err := s.DB.PruneThreadRepliesTS(channelID, threadTS, returned, start.Unix())
	if err != nil {
		return changed, err
	}
	changed.Deleted = pruned
	return changed, nil
}
