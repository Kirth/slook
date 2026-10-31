package backend

import (
	"context"
	"fmt"
	"os"
	"time"

	slackclient "github.com/gammons/slk/internal/slack"
)

type (
	// rawSelfPresence is the socket's manual_presence_change (or
	// presence_change for the user): "active" or "away".
	rawSelfPresence struct{ presence string }
	// rawSelfDND is the socket's dnd_updated: on until end (zero when
	// unknown).
	rawSelfDND struct {
		on  bool
		end time.Time
	}
	// rawSelfSet is a whole status set here (optimistically, or taken
	// back).
	rawSelfSet struct{ st SelfStatus }
)

// OnSelfPresenceChange and OnDNDChange are the user's own presence and
// do-not-disturb changing, here or in another client; applied on the
// loop.
func (h socketHandler) OnSelfPresenceChange(presence string) {
	h.queue <- rawSelfPresence{presence}
}
func (h socketHandler) OnDNDChange(on bool, endUnix int64) {
	h.queue <- rawSelfDND{on: on, end: unixTime(endUnix)}
}

// applySelf records a change to the user's own status and returns it
// whole, on the loop.
func (s *Slack) applySelf(v any) SelfStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch v := v.(type) {
	case rawSelfPresence:
		s.self.Away = v.presence == "away"
	case rawSelfDND:
		s.self.DND, s.self.DNDUntil = v.on, v.end
		if !v.on {
			s.self.DNDUntil = time.Time{}
		}
	case rawSelfSet:
		s.self = v.st
	}
	return s.self
}

// SelfStatus asks Slack for the user's presence (users.getPresence)
// and do-not-disturb (dnd.info), and keeps them.
func (s *Slack) SelfStatus(ctx context.Context) (SelfStatus, error) {
	self := s.client.UserID()
	p, err := s.client.GetUserPresence(ctx, self)
	if err != nil {
		return SelfStatus{}, err
	}
	st := SelfStatus{Away: p.ManualAway || p.Presence == "away"}
	dnd, err := s.client.GetDNDInfo(ctx, self)
	if err != nil {
		return SelfStatus{}, err
	}
	if dnd != nil {
		on, end := slackclient.DNDStateFromStatus(*dnd, time.Now().Unix())
		st.DND, st.DNDUntil = on, unixTime(end)
	}
	s.mu.Lock()
	s.self = st
	s.mu.Unlock()
	return st, nil
}

// SetPresence sets the user away, or active ("auto": Slack goes by
// activity).
func (s *Slack) SetPresence(ctx context.Context, away bool) error {
	presence := "auto"
	if away {
		presence = "away"
	}
	return s.setSelf(ctx, func(st *SelfStatus) { st.Away = away }, func(ctx context.Context) error {
		return s.client.SetUserPresence(ctx, presence)
	})
}

// Snooze turns do-not-disturb on for minutes (dnd.setSnooze).
func (s *Slack) Snooze(ctx context.Context, minutes int) error {
	until := time.Now().Add(time.Duration(minutes) * time.Minute)
	return s.setSelf(ctx, func(st *SelfStatus) { st.DND, st.DNDUntil = true, until }, func(ctx context.Context) error {
		_, err := s.client.SetSnooze(ctx, minutes)
		return err
	})
}

// EndSnooze ends a snooze (dnd.endSnooze); a scheduled do-not-disturb
// stays.
func (s *Slack) EndSnooze(ctx context.Context) error {
	return s.setSelf(ctx, endDND, func(ctx context.Context) error {
		_, err := s.client.EndSnooze(ctx)
		return err
	})
}

// EndDND ends a snooze and the scheduled do-not-disturb session both
// (dnd.endSnooze, then dnd.endDnd), as the TUI did: either may be all
// there is. It fails only when both do.
func (s *Slack) EndDND(ctx context.Context) error {
	return s.setSelf(ctx, endDND, func(ctx context.Context) error {
		_, snoozeErr := s.client.EndSnooze(ctx)
		if err := s.client.EndDND(ctx); err != nil && snoozeErr != nil {
			return err
		}
		return nil
	})
}

func endDND(st *SelfStatus) { st.DND, st.DNDUntil = false, time.Time{} }

// setSelf changes the user's status at once (change), makes it in
// Slack (call, bounded by reactTimeout) and, if Slack refuses or does
// not answer, puts the old status back and returns why.
func (s *Slack) setSelf(ctx context.Context, change func(*SelfStatus), call func(context.Context) error) error {
	s.mu.Lock()
	old := s.self
	s.mu.Unlock()
	next := old
	change(&next)
	select {
	case s.queue <- rawSelfSet{next}:
	case <-ctx.Done():
		return ctx.Err()
	}
	cctx, cancel := context.WithTimeout(ctx, reactTimeout)
	err := call(cctx)
	cancel()
	if err == nil {
		return nil
	}
	fmt.Fprintln(os.Stderr, "slk-gio: presence:", err)
	select {
	case s.queue <- rawSelfSet{old}:
	case <-ctx.Done():
	}
	return err
}
