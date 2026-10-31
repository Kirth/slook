// Package threadsubs keeps the cache's list of the threads the user
// follows (thread_subscriptions) in step with Slack's
// subscriptions.thread.getView, for any frontend's Threads view: the
// sweep itself (Sync) and the throttle that keeps it rare (Gate). The
// sweep is up to ~60 paginated requests, so it runs at most once per
// Interval; between sweeps the socket's thread events keep the table
// current. Moved from cmd/slk so the Gio frontend shares it.
package threadsubs

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/debuglog"
	slackclient "github.com/gammons/slk/internal/slack"
)

// Lister is the one call this file makes:
// subscriptions.thread.getView, paginated to a 1000-item hard cap.
type Lister interface {
	ListThreadSubscriptions(ctx context.Context) ([]slackclient.ThreadSubscriptionView, error)
}

// Sync reconciles the local thread_subscriptions
// table against the server's view, and caches each subscription's root
// message so the Threads view can render parents for threads slk has
// never seen a message from.
//
// It used to be a phase of the reconnect backfill, and that was the
// single most expensive thing slk did: measured 2026-08-01 on a
// 105-channel workspace, the subscription phase took 132 seconds
// against the channel phase's 2.7, hit its 1000-item hard cap every
// time, and ran on every reconnect — four passes in one ~3-minute
// session, roughly six minutes of work for a 90-second outage that
// produced no new messages. It is now driven from the places that
// actually need the data.
type Sync struct {
	Client      Lister
	DB          *cache.DB
	WorkspaceID string

	// Available, if non-nil, is called with the outcome: true on
	// success, false on error. Wired to wctx.SubscriptionsAvailable so
	// the Threads view's banner reflects the most recent attempt.
	Available func(bool)
}

// Run fetches the subscription list and writes it through.
//
// Side effects:
//  1. thread_subscriptions reflects the server's authoritative state.
//  2. Available is called with the outcome.
//  3. Each ThreadSubscriptionView.RootMessage is upserted into the
//     messages cache (idempotent by (ts, channel_id)).
//
// Errors from the API call are returned; per-thread message-upsert
// failures are logged and skipped, since one bad message should not
// cost the caller the whole list.
func (s *Sync) Run(ctx context.Context) error {
	start := time.Now()
	views, err := s.Client.ListThreadSubscriptions(ctx)
	if err != nil {
		debuglog.Backfill("team=%s subscription-sync err=%v", s.WorkspaceID, err)
		if s.Available != nil {
			s.Available(false)
		}
		return err
	}
	if s.Available != nil {
		s.Available(true)
	}

	// Adapt slack-client view rows into cache.ThreadSubscription. The
	// API method already filters out subscribed=false items, so the
	// list is conservative: every item here is currently active.
	fresh := make([]cache.ThreadSubscription, 0, len(views))
	for _, v := range views {
		if !v.Subscription.Active {
			continue
		}
		fresh = append(fresh, cache.ThreadSubscription{
			WorkspaceID: s.WorkspaceID,
			ChannelID:   v.Subscription.ChannelID,
			ThreadTS:    v.Subscription.ThreadTS,
			LastRead:    v.Subscription.LastRead,
			// Authoritative newest-reply watermark from the getView
			// root_msg. Lets the threads view compute unread state
			// without the thread's replies being cached locally.
			LatestReply: v.RootMessage.LatestReply,
			Active:      true,
		})
	}
	if err := s.DB.ReconcileThreadSubscriptions(s.WorkspaceID, fresh); err != nil {
		debuglog.Backfill("team=%s subscription-sync reconcile err=%v", s.WorkspaceID, err)
		return err
	}

	// Upsert the root_msg from every view into the messages cache.
	// Skip entries where RootMessage is empty (Subscription kept but
	// RootMessage couldn't be decoded; see the ListThreadSubscriptions
	// docstring).
	upserted := 0
	for _, v := range views {
		if v.RootMessage.Timestamp == "" {
			continue
		}
		raw, _ := json.Marshal(v.RootMessage)
		if err := s.DB.UpsertMessage(cache.Message{
			TS:          v.RootMessage.Timestamp,
			ChannelID:   v.Subscription.ChannelID,
			WorkspaceID: s.WorkspaceID,
			UserID:      v.RootMessage.User,
			Text:        v.RootMessage.Text,
			ThreadTS:    v.RootMessage.ThreadTimestamp,
			ReplyCount:  v.RootMessage.ReplyCount,
			Subtype:     v.RootMessage.SubType,
			RawJSON:     string(raw),
			CreatedAt:   time.Now().Unix(),
		}); err != nil {
			debuglog.Backfill("team=%s subscription-sync upsert root_msg %s/%s err=%v",
				s.WorkspaceID, v.Subscription.ChannelID, v.Subscription.ThreadTS, err)
			continue
		}
		upserted++
	}

	debuglog.Backfill("team=%s subscription-sync subs=%d root_msgs_upserted=%d dur_ms=%d",
		s.WorkspaceID, len(fresh), upserted, time.Since(start).Milliseconds())
	return nil
}

// Interval is the minimum gap between a workspace's
// subscriptions.thread.getView sweeps. The call paginates to a
// 1000-item hard cap, measured at ~62 requests per workspace on a real
// account, so it cannot run on every trigger: boot, Threads-view
// activation and every WS reconnect would otherwise multiply it. 30
// minutes bounds the request volume while keeping the list fresh
// across the gaps the socket cannot cover (app closed, laptop asleep
// — the socket replays nothing).
const Interval = 30 * time.Minute

// Gate admits at most one subscription sync per window per
// workspace, and never two concurrently. It replaces the sync.Once
// that gated the fetch when only the first Threads-view open triggered
// it: a Once cannot express "again after a long offline gap", which is
// exactly when the table goes stale.
type Gate struct {
	mu      sync.Mutex
	last    time.Time
	running bool
	// Window is the least time between two admitted sweeps.
	Window time.Duration
}

// TryStart reports whether a sync may begin at `now`. first is true
// only for the workspace's first-ever admission, which is what the
// startup stagger keys off. last is recorded at admission, not
// completion, so a failed sync cannot be retried into a hammering
// loop against a rate-limited endpoint.
func (g *Gate) TryStart(now time.Time) (first, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running {
		return false, false
	}
	if !g.last.IsZero() && now.Sub(g.last) < g.Window {
		return false, false
	}
	first = g.last.IsZero()
	g.last = now
	g.running = true
	return first, true
}

func (g *Gate) Done() {
	g.mu.Lock()
	g.running = false
	g.mu.Unlock()
}

// Running reports whether an admitted sweep has not yet called Done.
func (g *Gate) Running() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.running
}
