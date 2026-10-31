package backend

import (
	"context"
	"fmt"
	"os"
	"time"

	slackclient "github.com/gammons/slk/internal/slack"
)

// dndState is someone's do-not-disturb, as last heard.
type dndState struct {
	on    bool
	until time.Time
}

type (
	// rawStatus is the socket's user_change / user_status_changed.
	rawStatus struct {
		userID string
		st     slackclient.UserStatus
	}
	// rawInvalidated is the socket's user_invalidated: the whole
	// profile changed.
	rawInvalidated struct{ userID string }
	// rawDND is the socket's dnd_updated_user, or a DND answer;
	// forget drops what is known (dnd_invalidated).
	rawDND struct {
		userID string
		on     bool
		until  time.Time
		forget bool
	}
)

// OnUserStatusChange, OnUserInvalidated, OnUserDNDChange and
// OnDNDInvalidated keep shown profiles current; applied on the loop.
func (h socketHandler) OnUserStatusChange(userID string, st slackclient.UserStatus) {
	h.queue <- rawStatus{userID, st}
}
func (h socketHandler) OnUserInvalidated(userID string) { h.queue <- rawInvalidated{userID} }
func (h socketHandler) OnUserDNDChange(userID string, on bool, endUnix int64) {
	d := rawDND{userID: userID, on: on}
	if endUnix > 0 {
		d.until = time.Unix(endUnix, 0)
	}
	h.queue <- d
}
func (h socketHandler) OnDNDInvalidated(userID string) {
	h.queue <- rawDND{userID: userID, forget: true}
}

// unixTime is a Unix second count as a time; zero for none.
func unixTime(sec int64) time.Time {
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

// applyStatus writes a status change into the cache and any profile
// held, and reports whether one is shown (held) at all.
func (s *Slack) applyStatus(userID string, st slackclient.UserStatus) {
	if s.db != nil {
		if err := s.db.UpdateUserStatus(userID, st.Emoji, st.Text, st.Expiration, st.HuddleState, st.HuddleExpiration); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: cache status:", err)
		}
	}
	s.mu.Lock()
	if p, ok := s.profiles[userID]; ok {
		p.StatusEmoji, p.StatusText, p.StatusExpires = st.Emoji, st.Text, unixTime(st.Expiration)
		s.profiles[userID] = p
	}
	s.mu.Unlock()
}

// forgetProfile drops a held profile: the next Profile asks Slack.
func (s *Slack) forgetProfile(userID string) {
	s.mu.Lock()
	delete(s.profiles, userID)
	s.mu.Unlock()
}

// setDND records someone's do-not-disturb (or forgets it).
func (s *Slack) setDND(d rawDND) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dnd == nil {
		s.dnd = map[string]dndState{}
	}
	if d.forget {
		delete(s.dnd, d.userID)
	} else {
		s.dnd[d.userID] = dndState{on: d.on, until: d.until}
	}
}

// withDND is p with what is known of their do-not-disturb. s.mu held.
func (s *Slack) withDNDLocked(p Profile) Profile {
	if d, ok := s.dnd[p.ID]; ok {
		p.DND, p.DNDUntil = d.on, d.until
	}
	return p
}

// WatchDND asks Slack once for someone's do-not-disturb (dnd.info),
// unless it is known already; the answer arrives as ProfileChanged.
func (s *Slack) WatchDND(userID string) {
	s.mu.Lock()
	_, known := s.dnd[userID]
	asked := s.dndAsked[userID]
	if !known && !asked {
		if s.dndAsked == nil {
			s.dndAsked = map[string]bool{}
		}
		s.dndAsked[userID] = true
	}
	s.mu.Unlock()
	if known || asked {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		st, err := s.client.GetDNDInfo(ctx, userID)
		if err != nil || st == nil {
			if err != nil {
				fmt.Fprintln(os.Stderr, "slk-gio: dnd.info:", err)
			}
			return
		}
		on, end := slackclient.DNDStateFromStatus(*st, time.Now().Unix())
		s.queue <- rawDND{userID: userID, on: on, until: unixTime(end)}
	}()
}
