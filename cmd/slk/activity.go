package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui"
)

// activityFetchFunc builds the ActivityService.Fetch closure: one page
// of the workspace's activity.feed.
//
// Both Activity closures resolve the workspace by the requested teamID,
// not router.Active(): a workspace switch can land between dispatch and
// execution, and the result is labelled with teamID.
func activityFetchFunc(router *workspaceRouter) core.ActivityFetchFunc {
	return func(teamID ids.TeamID, limit int, unreadOnly bool) core.Msg {
		teamIDStr := string(teamID)
		wctx := router.ByID(teamIDStr)
		if wctx == nil {
			return nil
		}
		fetchCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		result, err := wctx.Client.GetActivityFeed(fetchCtx, limit, "", unreadOnly)
		if err != nil {
			log.Printf("Warning: GetActivityFeed(%s): %v", teamIDStr, err)
			// Return nil (no message) on failure so the existing
			// feed + badge are kept rather than blanked. Mirrors
			// the "nil = keep cache" pattern used for message
			// loads; a transient network error must not wipe the
			// Activity view.
			return nil
		}
		return ui.ActivityListLoadedMsg{
			TeamID: teamIDStr,
			Items:  result.Items,
		}
	}
}

// activityHydrateFunc builds the ActivityService.Hydrate closure: the
// messages.list call that fills in bodies for a page of activity refs.
func activityHydrateFunc(router *workspaceRouter) core.ActivityHydrateFunc {
	return func(teamID ids.TeamID, refs map[string][]string) core.Msg {
		teamIDStr := string(teamID)
		wctx := router.ByID(teamIDStr)
		if wctx == nil {
			return nil
		}
		fetchCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		bodies, err := wctx.Client.GetActivityMessages(fetchCtx, refs)
		if err != nil {
			log.Printf("Warning: GetActivityMessages(%s): %v", teamIDStr, err)
			// nil = keep whatever bodies we already have.
			return nil
		}
		return ui.ActivityBodiesLoadedMsg{TeamID: teamIDStr, Bodies: bodies}
	}
}

// activitySetReadFunc builds the ActivityService.SetRead closure:
// activity.markRead / activity.markUnread for one item. It always
// returns an ActivityReadSetMsg, so the UI can undo its optimistic
// change when the call fails.
func activitySetReadFunc(router *workspaceRouter) core.ActivitySetReadFunc {
	return func(teamID ids.TeamID, item core.ActivityItem, read bool) core.Msg {
		teamIDStr := string(teamID)
		msg := ui.ActivityReadSetMsg{TeamID: teamIDStr, Key: item.Key, Read: read}
		wctx := router.ByID(teamIDStr)
		if wctx == nil {
			msg.Err = fmt.Errorf("workspace %s is not connected", teamIDStr)
			return msg
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := wctx.Client.SetActivityRead(ctx, item.Type, item.Key, item.FeedTS, read); err != nil {
			log.Printf("Warning: SetActivityRead(%s, %s, read=%v): %v", teamIDStr, item.Key, read, err)
			msg.Err = err
		}
		return msg
	}
}
