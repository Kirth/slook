package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/gammons/slk/internal/export"
	"github.com/gammons/slk/internal/msgsync"
)

// The cache's status for the search window: what is held, what is in
// flight, and the opt-in backfill with its controls.

// backfillRun is the running backfill; cancel is nil when none runs.
type backfillRun struct {
	cancel context.CancelFunc
	status BackfillStatus
}

// SetPaused gives the backfill the function that says the user is busy
// (a chat window has the keyboard); it holds between requests while
// that is true. Call before Run.
func (s *Slack) SetPaused(paused func() bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = paused
}

// SetBackfillIgnore gives the backfill the conversations it skips, by
// display name (a channel's name, a person's name for a DM); see
// config.CacheConfig.IgnoresBackfill. Call before Run.
func (s *Slack) SetBackfillIgnore(ignore func(name string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ignore = ignore
}

// ignoredLocked reports whether the backfill skips a conversation.
// Callers hold mu.
func (s *Slack) ignoredLocked(channelID string) bool {
	if s.ignore == nil {
		return false
	}
	name, ok := s.channels[channelID]
	return ok && s.ignore(name)
}

// EnableBackfill starts a backfill to horizon as soon as Run is up: the
// -backfill flag. Call before Run.
func (s *Slack) EnableBackfill(horizon time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoStart = &horizon
}

// StartBackfill fills the cache back to horizon in the background. See
// msgsync.Backfill for its pace and why it is opt-in.
func (s *Slack) StartBackfill(horizon time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sync == nil {
		return errors.New("no cache: nothing to fill")
	}
	if s.runCtx == nil {
		return errors.New("not connected yet")
	}
	if s.bf.cancel != nil {
		return errors.New("a backfill is already running")
	}
	ctx, cancel := context.WithCancel(s.runCtx)
	s.bf = backfillRun{cancel: cancel, status: BackfillStatus{
		State: BackfillRunning, Horizon: horizon, Phase: "conversations", StartedAt: time.Now(),
	}}
	s.notifySyncLocked()
	go s.runBackfill(ctx, horizon)
	return nil
}

// StopBackfill ends a running backfill; nothing happens when none runs.
func (s *Slack) StopBackfill() {
	s.mu.Lock()
	cancel := s.bf.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// runBackfill waits for the conversation list (it sets the order), runs
// the backfill, and records how it ended.
func (s *Slack) runBackfill(ctx context.Context, horizon time.Duration) {
	for {
		s.mu.Lock()
		ready := len(s.convOrder) > 0
		s.mu.Unlock()
		if ready {
			break
		}
		select {
		case <-ctx.Done():
			s.endBackfill(ctx.Err(), ctx)
			return
		case <-time.After(time.Second):
		}
	}
	s.mu.Lock()
	paused := s.paused
	s.mu.Unlock()
	err := s.sync.Backfill(ctx, msgsync.Config{
		Horizon:   horizon,
		Paused:    paused,
		Connected: s.connected.Load,
		Channels: func() []string {
			s.mu.Lock()
			defer s.mu.Unlock()
			var out []string
			for _, id := range s.convOrder {
				if !s.ignoredLocked(id) {
					out = append(out, id)
				}
			}
			return out
		},
		OnChanged: func(c msgsync.Changed) {
			s.mu.Lock()
			s.bf.status.Fetched += len(c.Upserted)
			s.mu.Unlock()
			select {
			case s.queue <- rawChanged{changed: c}:
			case <-ctx.Done():
			}
		},
		OnProgress: s.onProgress,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "slk-gio: "+format+"\n", args...)
		},
	})
	s.endBackfill(err, ctx)
}

// endBackfill records the run's end: done, stopped, or failed.
func (s *Slack) endBackfill(err error, ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bf.cancel = nil
	switch {
	case err == nil:
		s.bf.status.State = BackfillDone
		fmt.Fprintln(os.Stderr, "slk-gio: backfill done")
	case ctx.Err() != nil:
		s.bf.status.State = BackfillOff
	default:
		s.bf.status.State = BackfillFailed
		s.bf.status.LastError = err.Error()
		fmt.Fprintln(os.Stderr, "slk-gio: backfill:", err)
	}
	s.bf.status.Current = ""
	s.notifySyncLocked()
}

// onProgress is the backfill's progress hook.
func (s *Slack) onProgress(p msgsync.Progress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &s.bf.status
	switch p.Phase {
	case msgsync.PhaseChannels:
		st.Phase = "conversations"
	case msgsync.PhaseThreads:
		st.Phase = "threads"
	}
	st.Current = ""
	if p.ChannelID != "" {
		st.Current = s.convNameLocked(p.ChannelID)
		if p.ThreadTS != "" {
			st.Current += " · thread " + p.ThreadTS
		}
	}
	st.Done, st.Total, st.Requests = p.Done, p.Total, p.Requests
	if p.Err != nil {
		st.LastError = p.Err.Error()
	}
	if s.bf.cancel != nil { // still running; endBackfill sets the final state
		st.State = BackfillRunning
		if p.Waiting {
			st.State = BackfillPaused
		}
	}
	s.notifySyncLocked()
}

// convNameLocked is a conversation's name for people: "#general", or
// the person's name. Callers hold mu.
func (s *Slack) convNameLocked(channelID string) string {
	name := s.channels[channelID]
	if name == "" {
		return channelID
	}
	switch s.kinds[channelID] {
	case KindChannel, KindPrivate:
		return "#" + name
	}
	return name
}

// track records a fetch in flight under label until done is called,
// telling the windows both times.
func (s *Slack) track(label string) (done func()) {
	s.mu.Lock()
	s.active[label]++
	s.notifySyncLocked()
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.active[label]--
		if s.active[label] <= 0 {
			delete(s.active, label)
		}
		s.notifySyncLocked()
	}
}

// trackIn is track for a fetch in a conversation.
func (s *Slack) trackIn(verb, channelID string) func() {
	s.mu.Lock()
	label := verb + " " + s.convNameLocked(channelID)
	s.mu.Unlock()
	return s.track(label)
}

// notifySyncLocked queues one SyncChanged for Run to emit, unless one is
// already queued. A full queue drops the notice; the next change queues
// another. Callers hold mu.
func (s *Slack) notifySyncLocked() {
	if s.syncPending {
		return
	}
	select {
	case s.queue <- rawSync{}:
		s.syncPending = true
	default:
	}
}

// SyncStatus reads the cache's coverage and reports the fetches in
// flight and the backfill.
func (s *Slack) SyncStatus(context.Context) (SyncStatus, error) {
	s.mu.Lock()
	st := SyncStatus{Backfill: s.bf.status}
	for label := range s.active {
		st.Active = append(st.Active, label)
	}
	slices.Sort(st.Active)
	order := slices.Clone(s.convOrder)
	names := make(map[string]string, len(s.channels))
	for id := range s.channels {
		names[id] = s.convNameLocked(id)
	}
	kinds := make(map[string]Kind, len(s.kinds))
	for id, k := range s.kinds {
		kinds[id] = k
	}
	ignored := map[string]bool{}
	for id := range s.channels {
		if s.ignoredLocked(id) {
			ignored[id] = true
		}
	}
	s.mu.Unlock()
	if s.db == nil {
		return st, nil
	}
	spans, err := s.db.SpansForWorkspace(s.client.TeamID())
	if err != nil {
		return st, err
	}
	counts, err := s.db.MessageCounts(s.client.TeamID())
	if err != nil {
		return st, err
	}
	byChannel := map[string][]int{}
	for i, sp := range spans {
		byChannel[sp.ChannelID] = append(byChannel[sp.ChannelID], i)
	}
	// Every conversation the list knows, then any the cache holds that
	// the list does not (left or archived).
	seen := map[string]bool{}
	for _, id := range order {
		seen[id] = true
	}
	var extra []string
	for id := range counts {
		if !seen[id] {
			extra = append(extra, id)
			seen[id] = true
		}
	}
	for id := range byChannel {
		if !seen[id] {
			extra = append(extra, id)
			seen[id] = true
		}
	}
	slices.Sort(extra)
	for _, id := range append(order, extra...) {
		c := Coverage{ID: id, Name: names[id], Kind: kinds[id], Messages: counts[id], Ignored: ignored[id]}
		if c.Name == "" {
			c.Name = id
		}
		if idx := byChannel[id]; len(idx) > 0 {
			first, last := spans[idx[0]], spans[idx[len(idx)-1]]
			c.Oldest, _ = export.TimeFromTS(first.OldestTS)
			c.Complete = first.ReachesStart
			c.Live = last.Live
			c.Holes = len(idx) - 1
			for _, i := range idx {
				var ts TimeSpan
				ts.Oldest, _ = export.TimeFromTS(spans[i].OldestTS)
				ts.Latest, _ = export.TimeFromTS(spans[i].LatestTS)
				c.Spans = append(c.Spans, ts)
			}
		}
		st.Messages += c.Messages
		st.Conversations = append(st.Conversations, c)
	}
	return st, nil
}

// Search finds cached messages matching query, newest first.
func (s *Slack) Search(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	if s.db == nil {
		return nil, errors.New("no cache to search")
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	rows, err := s.db.SearchMessages(s.client.TeamID(), query, limit)
	if err != nil {
		return nil, err
	}
	msgs := s.fromCache(ctx, rows)
	hits := make([]SearchHit, 0, len(msgs))
	for _, m := range msgs {
		hits = append(hits, SearchHit{Message: m})
	}
	// fromCache sorts oldest first for timelines; a search reads newest
	// first.
	slices.Reverse(hits)
	return hits, nil
}
