// Package typing is who is typing where, and which apps are working
// (Slack's ai_assistant_status), for any frontend: the state with its
// expiries, and the words for it. It does no I/O and keeps no clock of
// its own beyond Now; a frontend sweeps it with Expire on its own
// schedule. A Tracker is not safe for concurrent use: keep it on one
// goroutine (a window's, the TUI's Update loop).
package typing

import (
	"sort"
	"strings"
	"time"
)

// Expiry is how long a person shows as typing after their latest
// user_typing event.
const Expiry = 5 * time.Second

// BotExpiry is how long an app's "working" status lasts without a
// clear: Slack's own timeout for assistant.threads.setStatus.
const BotExpiry = 2 * time.Minute

// Throttle is the least time between two of the user's own typing
// pings: Slack's clients send one every few seconds while typing.
const Throttle = 3 * time.Second

// botKey names one app's status on one thread.
type botKey struct {
	threadTS  string
	botUserID string
}

// botEntry is an app's status text and when it lapses.
type botEntry struct {
	text      string
	expiresAt time.Time
}

// BotStatus is one app's live status on a thread.
type BotStatus struct {
	BotUserID string
	Text      string
}

// Tracker is the typing state: people typing, by conversation, and
// apps working, by conversation and thread.
type Tracker struct {
	users map[string]map[string]time.Time // channel -> user -> expires
	bots  map[string]map[botKey]botEntry  // channel -> (thread, app) -> status
	// Now is the tracker's clock: time.Now, except in tests.
	Now func() time.Time
}

// New is an empty Tracker on the wall clock.
func New() *Tracker {
	return &Tracker{
		users: map[string]map[string]time.Time{},
		bots:  map[string]map[botKey]botEntry{},
		Now:   time.Now,
	}
}

// Add records that userID is typing in channelID, for Expiry from now.
func (t *Tracker) Add(channelID, userID string) {
	if t.users[channelID] == nil {
		t.users[channelID] = map[string]time.Time{}
	}
	t.users[channelID][userID] = t.Now().Add(Expiry)
}

// Done ends userID's typing in channelID and any app status they hold
// there: their message has arrived.
func (t *Tracker) Done(channelID, userID string) {
	if users := t.users[channelID]; users != nil {
		delete(users, userID)
		if len(users) == 0 {
			delete(t.users, channelID)
		}
	}
	t.ClearBot(channelID, userID)
}

// Expire drops whatever has lapsed and reports whether anything is
// left, so a caller knows whether to sweep again.
func (t *Tracker) Expire() bool {
	now := t.Now()
	for ch, users := range t.users {
		for uid, expires := range users {
			if now.After(expires) {
				delete(users, uid)
			}
		}
		if len(users) == 0 {
			delete(t.users, ch)
		}
	}
	for ch, statuses := range t.bots {
		for k, s := range statuses {
			if now.After(s.expiresAt) {
				delete(statuses, k)
			}
		}
		if len(statuses) == 0 {
			delete(t.bots, ch)
		}
	}
	return len(t.users) > 0 || len(t.bots) > 0
}

// Users is who is typing in channelID now, sorted. Lapsed entries are
// left out even before an Expire sweeps them.
func (t *Tracker) Users(channelID string) []string {
	now := t.Now()
	var out []string
	for uid, expires := range t.users[channelID] {
		if now.Before(expires) {
			out = append(out, uid)
		}
	}
	sort.Strings(out)
	return out
}

// UsersExcluding is Users without exclude (the user's own echo).
func (t *Tracker) UsersExcluding(channelID, exclude string) []string {
	all := t.Users(channelID)
	out := all[:0:0]
	for _, uid := range all {
		if uid != exclude {
			out = append(out, uid)
		}
	}
	return out
}

// SetBotStatus records an app's status on a thread, for BotExpiry; an
// empty status clears it.
func (t *Tracker) SetBotStatus(channelID, threadTS, botUserID, status string) {
	k := botKey{threadTS: threadTS, botUserID: botUserID}
	if status == "" {
		t.dropBot(channelID, func(key botKey) bool { return key == k })
		return
	}
	if t.bots[channelID] == nil {
		t.bots[channelID] = map[botKey]botEntry{}
	}
	t.bots[channelID][k] = botEntry{text: status, expiresAt: t.Now().Add(BotExpiry)}
}

// ClearBot drops every status botUserID holds in channelID: its reply
// there ends its work, whichever thread the status named.
func (t *Tracker) ClearBot(channelID, botUserID string) {
	t.dropBot(channelID, func(key botKey) bool { return key.botUserID == botUserID })
}

func (t *Tracker) dropBot(channelID string, match func(botKey) bool) {
	statuses := t.bots[channelID]
	for k := range statuses {
		if match(k) {
			delete(statuses, k)
		}
	}
	if len(statuses) == 0 {
		delete(t.bots, channelID)
	}
}

// WorkingBots is the apps with a live status anywhere in channelID,
// each once, sorted by ID.
func (t *Tracker) WorkingBots(channelID string) []string {
	now := t.Now()
	seen := map[string]bool{}
	var out []string
	for k, s := range t.bots[channelID] {
		if now.Before(s.expiresAt) && !seen[k.botUserID] {
			seen[k.botUserID] = true
			out = append(out, k.botUserID)
		}
	}
	sort.Strings(out)
	return out
}

// ThreadBotStatuses is the live statuses on one thread, sorted by app.
func (t *Tracker) ThreadBotStatuses(channelID, threadTS string) []BotStatus {
	now := t.Now()
	var out []BotStatus
	for k, s := range t.bots[channelID] {
		if k.threadTS == threadTS && now.Before(s.expiresAt) {
			out = append(out, BotStatus{BotUserID: k.botUserID, Text: s.text})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BotUserID < out[j].BotUserID })
	return out
}

// Text says who is typing: "Ada is typing", "Ada and Grace are
// typing", "Several people are typing", "" for no one. No ending: the
// caller adds "..." or draws its own.
func Text(names []string) string {
	return say(names, "is typing", "are typing", "Several people are typing")
}

// Working says which apps are working, as Slack's client does in a
// conversation: their own status text belongs to their thread.
func Working(names []string) string {
	return say(names, "is working", "are working", "Several apps are working")
}

// Row is a conversation's typing row: who is typing, then which apps
// are working, each part ending in end ("..." or ""), joined by " · ".
func Row(typers, apps []string, end string) string {
	var parts []string
	if s := Text(typers); s != "" {
		parts = append(parts, s+end)
	}
	if s := Working(apps); s != "" {
		parts = append(parts, s+end)
	}
	return strings.Join(parts, " · ")
}

func say(names []string, one, two, many string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0] + " " + one
	case 2:
		return names[0] + " and " + names[1] + " " + two
	}
	return many
}
