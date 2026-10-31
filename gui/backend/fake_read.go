package backend

import (
	"context"
	"fmt"

	"github.com/gammons/slk/internal/mention"
)

// MarkUnread makes a conversation unread after boundaryTS, counting
// its mentions as Slack would (every message in a DM), or a thread in
// C1 unread (followed, with unread replies).
func (f *Fake) MarkUnread(ctx context.Context, channelID, threadTS, boundaryTS string) error {
	var evs []Event
	f.mu.Lock()
	c := f.convs[channelID]
	switch {
	case c == nil:
		f.mu.Unlock()
		return fmt.Errorf("channel_not_found")
	case threadTS != "":
		if channelID != "C1" {
			f.mu.Unlock()
			return fmt.Errorf("thread_not_found")
		}
		f.followed[threadTS] = true
		evs = []Event{ThreadState{ChannelID: channelID, ThreadTS: threadTS, Unread: true}, ThreadsChanged{}, f.threadsUnreadLocked()}
	default:
		n := 0
		for _, m := range f.logs[channelID] {
			if m.InThreadOnly || m.FromSelf || !tsAfter(m.TS, boundaryTS) {
				continue
			}
			if c.Kind == KindDM || c.Kind == KindGroupDM || mention.InText(m.Text, fakeSelf) {
				n++
			}
		}
		c.Unread, c.Mentions = true, n
		evs = []Event{ReadChanged{ChannelID: channelID, LastRead: boundaryTS, Unread: true, Mentions: n}}
	}
	f.mu.Unlock()
	for _, ev := range evs {
		select {
		case f.events <- ev:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
