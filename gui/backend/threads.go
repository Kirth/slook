package backend

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gammons/slk/internal/export"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/threadsubs"
)

// threadsState is what the backend knows of the followed threads'
// unread count, guarded by Slack.mu. Until a sync this session the
// count is client.counts' aggregate; after it, the cache's own count,
// which the socket's thread events keep current.
type threadsState struct {
	gate     threadsubs.Gate
	agg      slackclient.ThreadsAggregate
	synced   bool
	lastSent ThreadsUnread
	sent     bool
}

type (
	// rawThreadMarked is a thread read up to lastRead (thread_marked,
	// or the user's own mark).
	rawThreadMarked struct{ channelID, threadTS, lastRead string }
	// rawThreadSub is a thread followed or left.
	rawThreadSub struct {
		channelID, threadTS, lastRead string
		active                        bool
	}
	// rawThreadsRecount asks for the unread count again (after a sync).
	rawThreadsRecount struct{}
)

// OnThreadMarked and OnThreadSubscriptionChanged keep the followed
// threads' read state current; applied on the loop.
func (h socketHandler) OnThreadMarked(channelID, threadTS, lastRead string, _ slackclient.Subscribed) {
	h.queue <- rawThreadMarked{channelID, threadTS, lastRead}
}
func (h socketHandler) OnThreadSubscriptionChanged(channelID, threadTS, lastRead string, active bool) {
	h.queue <- rawThreadSub{channelID, threadTS, lastRead, active}
}

// noteThreadsAggregate records client.counts' threads summary and, until
// the cache's own count takes over, reports it.
func (s *Slack) noteThreadsAggregate(ctx context.Context, agg slackclient.ThreadsAggregate) {
	s.mu.Lock()
	s.threads.agg = agg
	s.mu.Unlock()
	select {
	case s.queue <- rawThreadsRecount{}:
	case <-ctx.Done():
	}
}

// Threads is the followed threads from the cache, newest first.
func (s *Slack) Threads(ctx context.Context) ([]ThreadSummary, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.ListSubscribedThreads(s.client.TeamID(), s.client.UserID())
	if err != nil {
		return nil, err
	}
	out := make([]ThreadSummary, 0, len(rows))
	for _, r := range rows {
		t := ThreadSummary{
			ChannelID: r.ChannelID, ThreadTS: r.ThreadTS, AuthorID: r.ParentUserID,
			Replies: r.ReplyCount, Unread: r.Unread,
			Preview: plainMrkdwn(r.ParentText, s),
		}
		t.LastReply, _ = export.TimeFromTS(r.LastReplyTS)
		s.mu.Lock()
		t.Channel, t.Kind = s.channels[r.ChannelID], s.kinds[r.ChannelID]
		s.mu.Unlock()
		if t.Channel == "" {
			t.Channel = r.ChannelName
		}
		if r.ParentUserID != "" {
			t.Author = s.userName(ctx, r.ParentUserID)
		}
		out = append(out, t)
	}
	return out, nil
}

// RefreshThreads syncs the followed threads with Slack in the
// background, at most once per threadsubs.Interval: the sweep pages
// through subscriptions.thread.getView, up to ~60 requests.
func (s *Slack) RefreshThreads() {
	if s.db == nil {
		return
	}
	s.mu.Lock()
	ctx := s.runCtx
	if s.threads.gate.Window == 0 {
		s.threads.gate.Window = threadsubs.Interval
	}
	s.mu.Unlock()
	if ctx == nil {
		return
	}
	if _, ok := s.threads.gate.TryStart(time.Now()); !ok {
		return
	}
	go func() {
		defer s.threads.gate.Done()
		sync := &threadsubs.Sync{Client: s.client, DB: s.db, WorkspaceID: s.client.TeamID()}
		if err := sync.Run(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: followed threads:", err)
			return
		}
		s.mu.Lock()
		s.threads.synced = true
		s.mu.Unlock()
		select {
		case s.queue <- rawEmit{ThreadsChanged{}}:
		case <-ctx.Done():
			return
		}
		select {
		case s.queue <- rawThreadsRecount{}:
		case <-ctx.Done():
		}
	}()
}

// threadMarked records a thread read up to lastRead, on the loop.
func (s *Slack) threadMarked(v rawThreadMarked, emit func(Event)) {
	if s.db == nil {
		return
	}
	if err := s.db.UpdateThreadLastReadIfExists(s.client.TeamID(), v.channelID, v.threadTS, v.lastRead); err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: cache thread read:", err)
	}
	emit(ThreadState{ChannelID: v.channelID, ThreadTS: v.threadTS, Unread: false})
	s.recountThreads(emit)
}

// threadFollowed records a thread followed or left, on the loop.
func (s *Slack) threadFollowed(v rawThreadSub, emit func(Event)) {
	if s.db == nil {
		return
	}
	if err := s.db.UpsertThreadSubscription(s.client.TeamID(), v.channelID, v.threadTS, v.lastRead, v.active); err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: cache thread subscription:", err)
	}
	emit(ThreadsChanged{})
	s.recountThreads(emit)
}

// replyArrived marks a followed thread unread when someone else
// replies in it, on the loop.
func (s *Slack) replyArrived(channelID, threadTS, author string, emit func(Event)) {
	if s.db == nil || author == s.client.UserID() {
		return
	}
	if _, err := s.db.GetThreadLastRead(s.client.TeamID(), channelID, threadTS); err != nil {
		return // not a thread the user follows
	}
	emit(ThreadState{ChannelID: channelID, ThreadTS: threadTS, Unread: true})
	emit(ThreadsChanged{})
	s.recountThreads(emit)
}

// recountThreads reports the unread followed threads when the count
// changed: the cache's count once synced this session, else
// client.counts' aggregate.
func (s *Slack) recountThreads(emit func(Event)) {
	s.mu.Lock()
	n := ThreadsUnread{Count: s.threads.agg.UnreadCount, Mentions: s.threads.agg.MentionCount}
	synced := s.threads.synced
	s.mu.Unlock()
	if synced && s.db != nil {
		if rows, err := s.db.ListSubscribedThreads(s.client.TeamID(), s.client.UserID()); err == nil {
			n.Count = 0
			for _, r := range rows {
				if r.Unread {
					n.Count++
				}
			}
			n.Mentions = min(n.Mentions, n.Count)
		}
	}
	s.mu.Lock()
	same := s.threads.sent && s.threads.lastSent == n
	s.threads.sent, s.threads.lastSent = true, n
	s.mu.Unlock()
	if !same {
		emit(n)
	}
}

// markUnreadThreads sets ThreadUnread on the parents of followed
// threads with replies newer than the user read: latest is each
// parent's newest reply as its cached row says, by ts.
func (s *Slack) markUnreadThreads(msgs []Message, latest map[string]string) {
	if s.db == nil {
		return
	}
	var subs map[string]string // channel/thread -> last read
	for i := range msgs {
		m := &msgs[i]
		if m.ReplyCount == 0 || m.ThreadTS != "" && m.ThreadTS != m.TS {
			continue
		}
		if subs == nil {
			list, err := s.db.ListActiveThreadSubscriptions(s.client.TeamID())
			if err != nil {
				return
			}
			subs = make(map[string]string, len(list))
			for _, sub := range list {
				if tsAfter(sub.LatestReply, latest[sub.ThreadTS]) {
					latest[sub.ThreadTS] = sub.LatestReply
				}
				subs[sub.ChannelID+"/"+sub.ThreadTS] = sub.LastRead
			}
		}
		if lr, ok := subs[m.ChannelID+"/"+m.TS]; ok && tsAfter(latest[m.TS], lr) {
			m.ThreadUnread = true
		}
	}
}

// tsAfter reports whether Slack ts a is later than b ("" is earliest).
func tsAfter(a, b string) bool {
	ta, okA := export.TimeFromTS(a)
	tb, okB := export.TimeFromTS(b)
	switch {
	case !okA:
		return false
	case !okB:
		return true
	case ta.Equal(tb):
		return a > b
	}
	return ta.After(tb)
}
