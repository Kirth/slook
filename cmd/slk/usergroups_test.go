package main

import (
	"sync"
	"testing"
)

func TestWorkspaceContextUserGroupsDefaultsEmpty(t *testing.T) {
	var wctx WorkspaceContext
	if got := wctx.UserGroups(); got == nil || len(got) != 0 {
		t.Errorf("UserGroups() before load = %v, want empty non-nil map", got)
	}
}

// The usergroups.list fetch publishes the map from a background
// goroutine while the RTM event loop and bubbletea cmds read it. Run
// under -race to catch a regression back to a plain map field.
func TestWorkspaceContextUserGroupsConcurrentAccess(t *testing.T) {
	wctx := &WorkspaceContext{}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			wctx.SetUserGroups(map[string]string{"S1": "platform-team"})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = wctx.UserGroups()["S1"]
		}
	}()
	wg.Wait()
	if got := wctx.UserGroups()["S1"]; got != "platform-team" {
		t.Errorf("UserGroups()[S1] = %q, want platform-team", got)
	}
}
