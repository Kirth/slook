package backend

import (
	"context"
	"fmt"
	"os"
	"time"
)

// membersFresh is how long a cached member list answers without asking
// Slack again.
const membersFresh = 6 * time.Hour

// membersMaxPages caps one member listing at 5,000 people (1,000 a
// page): the panel asks only when opened, and a huge channel should not
// become a sweep of requests.
const membersMaxPages = 5

// rawEmit is an event the backend makes itself, delivered through Run
// in order with the socket's.
type rawEmit struct{ ev Event }

// Members lists a conversation's members, from the cache when it was
// listed within membersFresh, else from Slack (at most membersMaxPages
// pages). Names come from what slk already knows; the rest carry just
// their IDs, for the caller to resolve as it shows them.
func (s *Slack) Members(ctx context.Context, channelID string) (Members, error) {
	team := s.client.TeamID()
	var ids []string
	complete := true
	if s.db != nil {
		if at, ok, err := s.db.GetChannelMembershipMeta(team, channelID); err == nil && ok && time.Since(time.Unix(at, 0)) < membersFresh {
			ids, _ = s.db.ListChannelMembers(team, channelID)
		}
	}
	if ids == nil {
		var err error
		ids, complete, err = s.client.GetUsersInConversationMax(ctx, channelID, membersMaxPages)
		if err != nil {
			return Members{}, err
		}
		if complete && s.db != nil {
			if err := s.db.ReplaceChannelMembers(team, channelID, ids, time.Now().Unix()); err != nil {
				fmt.Fprintln(os.Stderr, "slk-gio: cache members:", err)
			}
		}
	}
	known, err := s.People(ctx)
	if err != nil {
		return Members{}, err
	}
	byID := make(map[string]Person, len(known))
	for _, p := range known {
		byID[p.ID] = p
	}
	m := Members{Total: len(ids), Partial: !complete}
	for _, id := range ids {
		p, ok := byID[id]
		if !ok {
			p = Person{ID: id}
		}
		m.People = append(m.People, p)
	}
	return m, nil
}

// notifyLevels is every conversation's notify level other than the
// default, from the cache.
func (s *Slack) notifyLevels() map[string]NotifyLevel {
	out := map[string]NotifyLevel{}
	if s.db == nil {
		return out
	}
	levels, err := s.db.NotifyLevels(s.client.TeamID())
	if err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: notify levels:", err)
		return out
	}
	for id, l := range levels {
		out[id] = NotifyLevel(l)
	}
	return out
}

// SetNotify keeps a conversation's notify level in the cache and
// reports it through Run.
func (s *Slack) SetNotify(ctx context.Context, channelID string, level NotifyLevel) error {
	if s.db != nil {
		if err := s.db.SetNotifyLevel(s.client.TeamID(), channelID, int(level)); err != nil {
			return err
		}
	}
	select {
	case s.queue <- rawEmit{NotifyChanged{ChannelID: channelID, Level: level}}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
