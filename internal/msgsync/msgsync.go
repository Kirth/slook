// Package msgsync keeps the SQLite message cache in step with Slack.
//
// It is the one place that turns what Slack sends (history pages,
// thread replies, WebSocket frames) into cache rows and coverage spans,
// so every frontend writes the cache the same way. It knows nothing
// about drawing: callers get back a Changed, the rows a view may have
// to redraw, and do with it what they like.
//
// Fetching is demand-driven and one request per user action: Open is
// one page, Older is one page, Thread is one replies call. The only
// thing that fetches on its own is Backfill, which is opt-in, slow,
// and explained on its Config. This repo's record
// (docs/superpowers/specs/2026-07-30-enterprise-grid-bootstrap-design.md)
// ties Slack signing users out to sweeping conversations.history over
// every channel; nothing here does that unasked.
package msgsync

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
)

// Source is the slice of the Slack client this package reads: the
// official client's conversations.history shape, and conversations.replies.
// *slackclient.Client satisfies it; tests use a fake.
type Source interface {
	HistoryWithVersions(ctx context.Context, channelID string, opts slackclient.HistoryOpts) (slackclient.HistoryResult, error)
	GetReplies(ctx context.Context, channelID, threadTS string) ([]slack.Message, error)
}

var _ Source = (*slackclient.Client)(nil)

// Syncer binds one workspace's cache to its Slack client.
type Syncer struct {
	DB          *cache.DB
	Source      Source
	WorkspaceID string
	// Now is the clock; nil means time.Now. Rows record when they were
	// cached, and pruning spares rows cached after a fetch began.
	Now func() time.Time
}

func (s *Syncer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Changed is what a sync step did to one channel's rows, for a view to
// apply: the rows written (as now stored, oldest first) and the ts of
// rows soft-deleted because Slack no longer has them.
type Changed struct {
	ChannelID string
	Upserted  []cache.Message
	Deleted   []string
}

// Empty reports whether nothing changed.
func (c Changed) Empty() bool { return len(c.Upserted) == 0 && len(c.Deleted) == 0 }

// Anchor is where a history page was asked to start. At most one of
// Latest and Oldest is set; neither means the channel's head.
type Anchor struct {
	Latest string
	Oldest string
}

// Row converts a message Slack sent into its cache row. raw is the
// message's JSON as received, kept as raw_json; nil re-marshals m.
// version is the latest_updates stamp when the page carried one. The
// author is the user, or for an app's bot_message its bot ID.
func Row(workspaceID, channelID string, m slack.Message, raw []byte, version string, now time.Time) cache.Message {
	if raw == nil {
		raw, _ = json.Marshal(m)
	}
	author := m.User
	if author == "" {
		author = m.BotID
	}
	edited := ""
	if m.Edited != nil {
		edited = m.Edited.Timestamp
	}
	return cache.Message{
		TS:          m.Timestamp,
		ChannelID:   channelID,
		WorkspaceID: workspaceID,
		UserID:      author,
		Text:        m.Text,
		ThreadTS:    m.ThreadTimestamp,
		ReplyCount:  m.ReplyCount,
		EditedAt:    edited,
		RawJSON:     string(raw),
		CreatedAt:   now.Unix(),
		Subtype:     m.SubType,
		Version:     version,
	}
}

// IsReply reports whether m is a thread reply (a message in a thread
// other than its parent). A thread_broadcast is a reply too.
func IsReply(m cache.Message) bool {
	return m.ThreadTS != "" && m.ThreadTS != m.TS
}

// InFeed reports whether m is a row conversations.history returns:
// a top-level message, a thread parent, or a broadcast reply.
func InFeed(m cache.Message) bool {
	return !IsReply(m) || m.Subtype == "thread_broadcast"
}

func sortRows(rows []cache.Message) {
	sort.Slice(rows, func(i, j int) bool { return strings.Compare(rows[i].TS, rows[j].TS) < 0 })
}

// tsLess compares Slack timestamps as the cache does: as strings, which
// Slack's fixed width makes numeric order.
func tsLess(a, b string) bool { return strings.Compare(a, b) < 0 }
