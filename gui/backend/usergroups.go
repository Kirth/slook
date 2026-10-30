package backend

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/usergroups"
)

// groupsRefreshGap is the least time between two reads of the
// workspace's usergroups prompted by the socket: a burst of subteam
// events (an admin editing several groups) costs one request.
const groupsRefreshGap = time.Minute

// groupState is the workspace's usergroups, guarded by Slack.mu.
type groupState struct {
	list    []UserGroup // by handle
	byID    map[string]UserGroup
	fetched time.Time // last read from Slack
	pending bool      // a read prompted by the socket is waiting
}

// rawSubteams is the socket saying the usergroups changed.
type rawSubteams struct{}

// setGroupsLocked makes list the workspace's usergroups, marking the
// user's own; it reports whether anything changed. s.mu is held.
func (s *Slack) setGroupsLocked(list []UserGroup) bool {
	self := s.client.UserID()
	for i := range list {
		list[i].Mine = slices.Contains(list[i].Members, self)
	}
	slices.SortFunc(list, func(a, b UserGroup) int { return strings.Compare(a.Handle, b.Handle) })
	if reflect.DeepEqual(list, s.groups.list) {
		return false
	}
	s.groups.list = list
	s.groups.byID = make(map[string]UserGroup, len(list))
	for _, g := range list {
		s.groups.byID[g.ID] = g
	}
	return true
}

// loadCachedGroups reads the usergroups the cache holds, so mentions
// read as names from the first frame after a restart.
func (s *Slack) loadCachedGroups() {
	cached, err := s.db.ListUserGroups(s.client.TeamID())
	if err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: cached usergroups:", err)
		return
	}
	list := make([]UserGroup, 0, len(cached))
	for _, g := range cached {
		list = append(list, UserGroup{ID: g.ID, Handle: g.Handle, Name: g.Name, Description: g.Description, Members: g.Members})
	}
	s.mu.Lock()
	s.setGroupsLocked(list)
	s.mu.Unlock()
}

// refreshGroups reads the usergroups with their members from Slack (one
// request), keeps them in the cache and, when they changed, tells the
// windows. Failure keeps what was known.
func (s *Slack) refreshGroups(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	groups, err := s.client.GetUserGroupsWithMembers(cctx)
	cancel()
	s.mu.Lock()
	s.groups.fetched = time.Now()
	s.mu.Unlock()
	if err != nil {
		if ctx.Err() == nil {
			fmt.Fprintln(os.Stderr, "slk-gio: usergroups:", err)
		}
		return
	}
	list := make([]UserGroup, 0, len(groups))
	rows := make([]cache.UserGroup, 0, len(groups))
	for _, g := range groups {
		handle := usergroups.Handle(g)
		if handle == "" {
			continue // nothing a mention could read
		}
		list = append(list, UserGroup{ID: g.ID, Handle: handle, Name: g.Name, Description: g.Description, Members: g.Users})
		rows = append(rows, cache.UserGroup{ID: g.ID, Handle: handle, Name: g.Name, Description: g.Description, Members: g.Users})
	}
	s.mu.Lock()
	changed := s.setGroupsLocked(list)
	mine := s.mineLocked()
	s.mu.Unlock()
	if !changed {
		return
	}
	if s.db != nil {
		if err := s.db.ReplaceUserGroups(s.client.TeamID(), rows); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: cache usergroups:", err)
		}
	}
	select {
	case s.queue <- rawEmit{UserGroupsChanged{Mine: mine}}:
	case <-ctx.Done():
	}
}

// groupsChangedInSlack reads the usergroups again after the socket said
// they changed, at most once per groupsRefreshGap; a change inside the
// gap is read when it ends. Called on the event loop.
func (s *Slack) groupsChangedInSlack(ctx context.Context) {
	s.mu.Lock()
	if s.groups.pending {
		s.mu.Unlock()
		return
	}
	s.groups.pending = true
	wait := max(0, groupsRefreshGap-time.Since(s.groups.fetched))
	s.mu.Unlock()
	go func() {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}
		s.mu.Lock()
		s.groups.pending = false
		s.mu.Unlock()
		s.refreshGroups(ctx)
	}()
}

// mineLocked is the IDs of the user's own groups. s.mu is held.
func (s *Slack) mineLocked() []string {
	var out []string
	for _, g := range s.groups.list {
		if g.Mine {
			out = append(out, g.ID)
		}
	}
	return out
}

// UserGroups is the workspace's usergroups as last loaded.
func (s *Slack) UserGroups() []UserGroup {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.groups.list)
}

// groupProfile describes a usergroup for the hover card, from memory.
func (s *Slack) groupProfile(id string) (Profile, bool) {
	s.mu.Lock()
	g, ok := s.groups.byID[id]
	s.mu.Unlock()
	if !ok {
		return Profile{}, false
	}
	return GroupProfile(g), true
}

// GroupProfile is how a usergroup shows on a hover card: its name, its
// handle, its description and how many are in it.
func GroupProfile(g UserGroup) Profile {
	name := g.Name
	if name == "" {
		name = "@" + g.Handle
	}
	size := strconv.Itoa(len(g.Members)) + " members"
	if len(g.Members) == 1 {
		size = "1 member"
	}
	if g.Mine {
		size += ", including you"
	}
	return Profile{ID: g.ID, Name: name, Handle: g.Handle, Title: g.Description, StatusText: size}
}
