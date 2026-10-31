package main

import "sync"

// userNameStore is a workspace's user ID -> display name cache: the
// names `slk export` resolves while it writes a channel out, and the
// ones the kept engine helpers (user_resolver.go) record. Locked, so it
// can be read and written from any goroutine; Snapshot hands out an
// independent copy.
//
// It used to be shared with the TUI, which is why it is a store and not
// a plain map: on 2026-10-02 a map shared between the UI goroutine and
// a history fetcher crashed slk with "concurrent map read and map
// write". The rule stands: never hand out the map itself.
//
// A nil *userNameStore reads as empty and drops writes, so helpers whose
// name source is optional (tests, cache-only lookups) can pass nil.
type userNameStore struct {
	mu    sync.RWMutex
	names map[string]string
}

// newUserNameStore returns a store seeded with a copy of seed (which
// may be nil).
func newUserNameStore(seed map[string]string) *userNameStore {
	names := make(map[string]string, len(seed))
	for id, name := range seed {
		names[id] = name
	}
	return &userNameStore{names: names}
}

// Get returns the display name recorded for userID.
func (s *userNameStore) Get(userID string) (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	name, ok := s.names[userID]
	return name, ok
}

// Set records a display name for userID.
func (s *userNameStore) Set(userID, name string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.names[userID] = name
	s.mu.Unlock()
}

// Snapshot returns an independent copy of every recorded name. The
// caller owns the result; nothing else holds a reference to it.
func (s *userNameStore) Snapshot() map[string]string {
	if s == nil {
		return map[string]string{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.names))
	for id, name := range s.names {
		out[id] = name
	}
	return out
}
