package backend

import (
	"context"
	"time"
)

// SelfStatus is the user's presence and do-not-disturb as last set.
func (f *Fake) SelfStatus(context.Context) (SelfStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.self, nil
}

// SetPresence, Snooze, EndSnooze and EndDND change the user's status
// and report it, as Slack's echo would.
func (f *Fake) SetPresence(ctx context.Context, away bool) error {
	return f.setSelf(ctx, func(st *SelfStatus) { st.Away = away })
}

func (f *Fake) Snooze(ctx context.Context, minutes int) error {
	until := time.Now().Add(time.Duration(minutes) * time.Minute)
	return f.setSelf(ctx, func(st *SelfStatus) { st.DND, st.DNDUntil = true, until })
}

func (f *Fake) EndSnooze(ctx context.Context) error { return f.setSelf(ctx, endDND) }

func (f *Fake) EndDND(ctx context.Context) error { return f.setSelf(ctx, endDND) }

func (f *Fake) setSelf(ctx context.Context, change func(*SelfStatus)) error {
	f.mu.Lock()
	change(&f.self)
	st := f.self
	f.mu.Unlock()
	select {
	case f.events <- SelfPresenceChanged{Status: st}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
