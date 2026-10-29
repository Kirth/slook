package msgsync

import (
	"context"
	"fmt"
	"time"

	"github.com/gammons/slk/internal/cache"
)

// Config shapes a Backfill run.
//
// Backfill is the one thing in this package that fetches without a
// user asking for that exact page, so it is deliberately slow and off
// unless a frontend turns it on: this repo's record ties Slack signing
// users out to sweeping conversations.history over every channel
// (docs/superpowers/specs/2026-07-30-enterprise-grid-bootstrap-design.md).
// Every request it makes is in the official client's shape (28 per
// page, cached_latest_updates), it waits Interval between requests,
// it stands still while Paused says the user is busy, and it stops at
// Horizon. It never runs on reconnect.
type Config struct {
	// Horizon is how far back to fetch; 0 means to each channel's first
	// message.
	Horizon time.Duration
	// Interval is the gap between requests; 0 means DefaultInterval.
	Interval time.Duration
	// Paused, when it returns true, holds the run (polled every second).
	Paused func() bool
	// Connected is whether the WebSocket is up: a span that reaches a
	// channel's head is marked live, and the run holds while it is
	// down. nil means never up, and no holding.
	Connected func() bool
	// Channels lists the channels to fill, most important first. It is
	// asked once per run.
	Channels func() []string
	// OnChanged, when set, hears about every change a fetch made, so a
	// window on the channel can redraw.
	OnChanged func(Changed)
	// OnProgress, when set, hears where the run is: before each request
	// (so the item in flight is visible), after each step, while it
	// waits on Paused, and when it ends.
	OnProgress func(Progress)
	// Logf, when set, gets a line per step.
	Logf func(format string, args ...any)
}

// Progress is where a Backfill run is.
type Progress struct {
	// Phase is PhaseChannels, PhaseThreads or PhaseDone.
	Phase Phase
	// ChannelID and ThreadTS name the item being fetched; empty between
	// items.
	ChannelID string
	ThreadTS  string
	// Done and Total count the phase's items: channels finished of
	// channels listed, then threads fetched of threads found so far.
	Done, Total int
	// Requests is how many requests the run has made.
	Requests int
	// Waiting is true while the run holds for Paused.
	Waiting bool
	// Err is the last failure of a step, kept until the run ends; a
	// step failing moves on to the next item.
	Err error
}

// Phase is a stage of a Backfill run.
type Phase string

const (
	PhaseChannels Phase = "channels"
	PhaseThreads  Phase = "threads"
	PhaseDone     Phase = "done"
)

// DefaultInterval is the gap between backfill requests when Config
// names none: about seven a minute, a fraction of the official
// client's rate when a user scrolls.
const DefaultInterval = 8 * time.Second

// threadBatch is how many under-cached threads are listed per pass.
const threadBatch = 50

// Backfill fills the cache backwards to Horizon, one request per
// Interval: for each channel, its holes between spans and then its
// oldest span's older pages until the channel's first message or the
// horizon; then every thread within the horizon whose cached parent
// has more replies than the cache holds. It returns when done or when
// ctx ends. Spans make it resumable: a later run picks up where this
// one stopped.
func (s *Syncer) Backfill(ctx context.Context, cfg Config) error {
	logf := cfg.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	connected := func() bool { return cfg.Connected != nil && cfg.Connected() }
	horizonTS := ""
	if cfg.Horizon > 0 {
		horizonTS = fmt.Sprintf("%d.000000", s.now().Add(-cfg.Horizon).Unix())
	}
	report := func(c Changed) {
		if cfg.OnChanged != nil && !c.Empty() {
			cfg.OnChanged(c)
		}
	}
	p := Progress{Phase: PhaseChannels}
	progress := func() {
		if cfg.OnProgress != nil {
			cfg.OnProgress(p)
		}
	}

	var channels []string
	if cfg.Channels != nil {
		channels = cfg.Channels()
	}
	p.Total = len(channels)
	progress()
	for _, ch := range channels {
		p.ChannelID = ch
		for {
			if err := s.wait(ctx, cfg, &p, progress); err != nil {
				return err
			}
			before, err := s.DB.Spans(ch)
			if err != nil {
				return err
			}
			var changed Changed
			switch {
			case len(before) == 0:
				p.Requests++
				progress()
				changed, err = s.Open(ctx, ch, connected())
			default:
				head := before[len(before)-1]
				if len(before) == 1 && (head.ReachesStart || (horizonTS != "" && tsLess(head.OldestTS, horizonTS))) {
					goto nextChannel
				}
				// A hole below the newest span, or the newest span's
				// older pages: both are the page at or before its
				// oldest message.
				p.Requests++
				progress()
				changed, err = s.Older(ctx, ch, head.OldestTS, connected())
			}
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				p.Err = err
				logf("backfill %s: %v (next channel)", ch, err)
				goto nextChannel
			}
			report(changed)
			after, err := s.DB.Spans(ch)
			if err != nil {
				return err
			}
			if spansEqual(before, after) {
				// Nothing moved (an empty channel, or a page that
				// changed no coverage): there is nothing more to get.
				goto nextChannel
			}
			logf("backfill %s: spans=%d oldest=%s rows=%d", ch, len(after), after[0].OldestTS, len(changed.Upserted))
		}
	nextChannel:
		p.Done++
		p.ChannelID = ""
		progress()
	}

	p.Phase, p.Done, p.Total = PhaseThreads, 0, 0
	progress()
	tried := map[cache.ThreadRef]bool{}
	for {
		refs, err := s.DB.ThreadsNeedingReplies(s.WorkspaceID, horizonTS, threadBatch)
		if err != nil {
			return err
		}
		progressed := false
		for _, ref := range refs {
			if tried[ref] {
				continue
			}
			tried[ref] = true
			progressed = true
			p.Total++
			if err := s.wait(ctx, cfg, &p, progress); err != nil {
				return err
			}
			p.ChannelID, p.ThreadTS = ref.ChannelID, ref.TS
			p.Requests++
			progress()
			changed, err := s.Thread(ctx, ref.ChannelID, ref.TS)
			p.ChannelID, p.ThreadTS = "", ""
			p.Done++
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				p.Err = err
				logf("backfill thread %s/%s: %v", ref.ChannelID, ref.TS, err)
				progress()
				continue
			}
			report(changed)
			progress()
		}
		if !progressed {
			break
		}
	}
	p.Phase = PhaseDone
	progress()
	return nil
}

// wait pauses Interval, then holds while Paused, reporting the hold.
func (s *Syncer) wait(ctx context.Context, cfg Config, p *Progress, progress func()) error {
	interval := cfg.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	if err := sleep(ctx, interval); err != nil {
		return err
	}
	held := false
	// Hold while the user is busy, and while the socket is down: a
	// request made offline fails, and failing through every channel
	// would leave nothing for the run to do once it is back.
	hold := func() bool {
		return (cfg.Paused != nil && cfg.Paused()) || (cfg.Connected != nil && !cfg.Connected())
	}
	for hold() {
		if !held {
			held, p.Waiting = true, true
			progress()
		}
		if err := sleep(ctx, time.Second); err != nil {
			return err
		}
	}
	if held {
		p.Waiting = false
		progress()
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func spansEqual(a, b []cache.Span) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
