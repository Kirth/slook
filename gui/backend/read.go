package backend

import (
	"context"
	"fmt"
	"os"

	"github.com/gammons/slk/internal/mention"
)

// rawThreadUnread is a thread the user marked unread from boundary.
type rawThreadUnread struct{ channelID, threadTS, boundary string }

// MarkUnread moves a conversation's read mark (conversations.mark) or a
// thread's (subscriptions.thread.mark, read=0) back to boundaryTS, as
// the TUI's mark-unread did. Not optimistic: the new state is queued
// for Run once Slack accepts it.
func (s *Slack) MarkUnread(ctx context.Context, channelID, threadTS, boundaryTS string) error {
	if threadTS != "" {
		if err := s.client.MarkThreadUnread(ctx, channelID, threadTS, boundaryTS); err != nil {
			return err
		}
		select {
		case s.queue <- rawThreadUnread{channelID, threadTS, boundaryTS}:
		case <-ctx.Done():
		}
		return nil
	}
	if err := s.client.MarkChannelUnread(ctx, channelID, boundaryTS); err != nil {
		return err
	}
	// Slack's channel_marked echo does not carry the mentions the user
	// just made unread; they are counted here from the cache, which
	// holds them: the user picked the boundary off their own screen.
	select {
	case s.queue <- rawMarked{channelID: channelID, ts: boundaryTS, unread: 1, mentions: s.mentionsAfter(channelID, boundaryTS)}:
	case <-ctx.Done():
	}
	return nil
}

// mentionsAfter counts the cached messages after ts that make a
// conversation's mention badge: every message in a DM or group DM, an
// @-mention of the user (or @here, @channel) elsewhere; never the
// user's own. Zero without a cache.
func (s *Slack) mentionsAfter(channelID, ts string) int {
	if s.db == nil {
		return 0
	}
	rows, err := s.db.GetMessagesSince(channelID, ts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: mark unread: counting mentions:", err)
		return 0
	}
	s.mu.Lock()
	kind := s.kinds[channelID]
	s.mu.Unlock()
	self := s.client.UserID()
	n := 0
	for _, m := range rows {
		if m.TS == ts || m.UserID != "" && m.UserID == self {
			continue // the boundary itself stays read
		}
		if kind == KindDM || kind == KindGroupDM || mention.InText(m.Text, self) {
			n++
		}
	}
	return n
}

// threadUnread applies a thread marked unread, on the loop: its cached
// read mark goes back to the boundary, and the windows hear it.
func (s *Slack) threadUnread(v rawThreadUnread, emit func(Event)) {
	if s.db != nil {
		if err := s.db.UpdateThreadLastReadIfExists(s.client.TeamID(), v.channelID, v.threadTS, v.boundary); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: cache thread read:", err)
		}
	}
	emit(ThreadState{ChannelID: v.channelID, ThreadTS: v.threadTS, Unread: true})
	emit(ThreadsChanged{})
	s.recountThreads(emit)
}
