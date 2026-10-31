package main

import (
	"context"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/threadsubs"
	"github.com/gammons/slk/internal/ui"
)

// threadSubsStagger hands out startup slots across workspaces. On
// Enterprise Grid every workspace becomes ready in the same second
// (connectWorkspace runs per-workspace goroutines), and an unstaggered
// first sync would fire N paginated getView sweeps at once. Slot i
// waits i*threadSubsStaggerStep before its first sync. Re-syncs never
// wait — the interval above already thins them out.
var threadSubsStagger atomic.Int64

// threadSubsStaggerStep is a var so tests can shrink it.
var threadSubsStaggerStep = 15 * time.Second

// ensureWorkspaceThreadSubs is the single construction site for the
// throttled subscription sync, shared by every trigger: the
// ThreadService closure (workspace-ready boot + Threads-view
// activation) and the RTM handler's reconnect/wake catch-up.
func ensureWorkspaceThreadSubs(ctx context.Context, wctx *WorkspaceContext, db *cache.DB, send func(tea.Msg)) {
	ensureThreadSubscriptions(ctx, &wctx.ThreadSubsGate,
		&threadsubs.Sync{
			Client:      wctx.Client,
			DB:          db,
			WorkspaceID: wctx.TeamID,
			Available:   func(available bool) { wctx.SubscriptionsAvailable = available },
		},
		func() { send(ui.ThreadsListDirtyMsg{TeamID: wctx.TeamID}) })
}

// ensureThreadSubscriptions starts a subscription sync in the
// background when the gate admits one, and returns immediately
// either way.
//
// Triggers: workspace-ready (boot), Threads-view activation, and the
// reconnect/wake catch-up. The gate collapses those to at most one
// sync per threadSubsSyncInterval per workspace: within a session the
// thread_subscription_changed WS events keep the table current, so
// re-fetching more often buys nothing.
//
// The workspace's first sync waits out its stagger slot first, so a
// Grid boot doesn't fire every workspace's sweep in the same second.
// A sync admitted by boot that is still in its stagger sleep when the
// user opens the Threads view is fine — the view renders from cache
// and refreshes via onDone's ThreadsListDirtyMsg.
//
// onDone fires only on success — telling the view to re-read a cache
// that a failed fetch did not change would be a wasted round trip.
func ensureThreadSubscriptions(ctx context.Context, gate *threadsubs.Gate, s *threadsubs.Sync, onDone func()) {
	first, ok := gate.TryStart(time.Now())
	if !ok {
		return
	}
	go func() {
		defer gate.Done()
		if first {
			slot := threadSubsStagger.Add(1) - 1
			if delay := time.Duration(slot) * threadSubsStaggerStep; delay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
				}
			}
		}
		if err := s.Run(ctx); err != nil {
			return
		}
		if onDone != nil {
			onDone()
		}
	}()
}

// OnThreadSubscriptionChanged persists a subscribe/unsubscribe event
// in the thread_subscriptions table. The threads-view UI refresh is
// handled by a ThreadsListDirtyMsg dispatch so a new subscription
// shows up (active=true) or an unsubscribe removes the row
// (active=false) without per-event UI logic here.
func (h *rtmEventHandler) OnThreadSubscriptionChanged(channelID, threadTS, lastRead string, active bool) {
	// Persist subscribe/unsubscribe regardless of active-workspace
	// state. Mirrors OnChannelMarked / OnMessage: dropping the DB
	// write on inactive workspaces means a thread the user just got
	// @-mentioned in (auto-subscribed) would never enter the local
	// thread_subscriptions table, and the threads view would silently
	// omit it on next workspace switch until the next reconnect's
	// ReconcileThreadSubscriptions catches up.
	if h.db != nil {
		if err := h.db.UpsertThreadSubscription(h.workspaceID, channelID, threadTS, lastRead, active); err != nil {
			debuglog.Cache("OnThreadSubscriptionChanged: UpsertThreadSubscription %s/%s: %v",
				channelID, threadTS, err)
		}
	}
	// The threads-list refresh is filtered by team in App.Update, so
	// dispatching it for an inactive workspace would only wake the UI
	// loop for a no-op; the DB write above is what the eventual switch
	// reads. The rail is different: its thread half reads the row just
	// upserted (railThreadsUnread), and an auto-subscription from an
	// @-mention may be a new unread thread to light, an unsubscribe
	// one to stop lighting. Same send as OnThreadMarked's inactive
	// branch.
	if h.isActive != nil && !h.isActive() {
		if h.program != nil {
			h.program.Send(ui.ReadStateChangedMsg{WorkspaceID: h.workspaceID})
		}
		return
	}
	if h.program != nil {
		h.program.Send(ui.ThreadsListDirtyMsg{TeamID: h.workspaceID})
	}
}
