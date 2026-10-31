package main

import (
	"github.com/gammons/slk/internal/cache"
	"github.com/slack-go/slack"
)

// lookupUserCached returns the display name for userID using only
// local sources: the in-memory name store and the cached users
// table. Never hits the network and never records the result —
// resolveUserCached is the memoizing variant. Returns ("", false)
// when the user is unknown.
func lookupUserCached(userID string, userNames *userNameStore, db *cache.DB) (string, bool) {
	if userID == "" {
		return "", false
	}
	if name, ok := userNames.Get(userID); ok && name != "" {
		return name, true
	}
	if db != nil {
		if u, err := db.GetUser(userID); err == nil {
			name := u.DisplayName
			if name == "" {
				name = u.Name
			}
			if name != "" {
				return name, true
			}
		}
	}
	return "", false
}

// resolveUserCached is lookupUserCached plus memoization: a DB hit is
// recorded in the store so subsequent lookups skip SQLite. Safe from
// any goroutine (the store is locked). Returns ("", false) when the
// user is unknown — the caller falls back to userID-as-name and
// resolves it some other way.
func resolveUserCached(userID string, userNames *userNameStore, db *cache.DB) (string, bool) {
	name, ok := lookupUserCached(userID, userNames, db)
	if ok {
		userNames.Set(userID, name)
	}
	return name, ok
}

// messageAuthor resolves the display identity for a fetched message
// from local sources only (the name store and the cached users table).
// Human messages carry a `user` ID; bot messages (bot_message) have an
// empty `user` and only a `bot_id` + `username`, so those are keyed on
// the bot_id and use the message's username for the name. An unknown
// author is named by its ID; the caller resolves those afterwards
// (resolveExportNames).
func messageAuthor(m slack.Message, userNames *userNameStore, db *cache.DB) (userID, userName string) {
	if m.User != "" {
		name, ok := resolveUserCached(m.User, userNames, db)
		if !ok {
			name = m.User
		}
		return m.User, name
	}
	if m.BotID != "" {
		name := m.Username
		if name == "" {
			if cached, ok := resolveUserCached(m.BotID, userNames, db); ok {
				name = cached
			} else {
				name = m.BotID
			}
		}
		return m.BotID, name
	}
	return m.User, m.User
}
