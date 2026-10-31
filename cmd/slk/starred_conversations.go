package main

import (
	"context"
	"log"

	"github.com/gammons/slk/internal/slack/boot"
	"github.com/slack-go/slack"
)

// missingStarredConversations returns eligible starred conversations absent
// from knownIDs, in stars.list order. Boot IM metadata avoids a network request
// and admits closed 1:1 DMs; resolve is the read-only conversations.info path.
// Errors are best-effort and cancellation stops further work, retaining any
// additions already resolved. Nothing is opened remotely or fabricated.
// Missing MPIMs are deliberately excluded: stars must not undo the ordinary
// closed-group-DM filter. Regular channels still require joined membership.
func missingStarredConversations(ctx context.Context, knownIDs, starredIDs []string, ims []boot.IM, resolve func(context.Context, string) (*slack.Channel, error)) []slack.Channel {
	seen := make(map[string]bool, len(knownIDs)+len(starredIDs))
	for _, id := range knownIDs {
		seen[id] = true
	}
	bootIMs := make(map[string]boot.IM, len(ims))
	for _, im := range ims {
		bootIMs[im.ID] = im
	}
	var added []slack.Channel
	for _, id := range starredIDs {
		if ctx.Err() != nil {
			break
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true // including failures: at most one lookup per ID/pass
		var ch *slack.Channel
		if im, ok := bootIMs[id]; ok {
			if im.IsArchived {
				continue
			}
			if im.UserID != "" {
				mapped := bootIMConversation(im)
				ch = &mapped
			}
		}
		if ch == nil && resolve != nil {
			var err error
			ch, err = resolve(ctx, id)
			if err != nil {
				log.Printf("starred conversations: looking up %s: %v (skipping missing sidebar row)", id, err)
				continue
			}
		}
		if ctx.Err() != nil {
			break
		}
		if ch == nil || ch.ID != id || ch.IsArchived || ch.IsMpIM {
			continue
		}
		if ch.IsIM {
			if ch.User == "" {
				continue
			}
		} else if !ch.IsMember || ch.Name == "" || (!ch.IsChannel && !ch.IsGroup && !ch.IsPrivate) {
			continue
		}
		added = append(added, *ch)
	}
	return added
}
