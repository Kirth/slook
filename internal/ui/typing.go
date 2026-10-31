// internal/ui/typing.go
//
// Typing indicator state — both inbound (other users typing in a
// channel) and outbound (our own typing send throttle).
//
// Phase 2d of the SOLID refactor of internal/ui/app.go: extracts the
// five typing-related fields (typingUsers, typingTickerOn,
// typingEnabled, typingSendFn, lastTypingSent) and eight methods
// (addTypingUser, expireTypingUsers, getTypingUsers,
// getTypingUsersFiltered, typingIndicatorText, shouldSendTyping,
// maybeSendTyping, plus the rendering helper renderTypingLine which
// stays on App because it pulls in messagepane name-resolution and
// styles).
//
// Two cohesive types live here:
//
//	typingTracker      Inbound state: per-channel "who is typing now"
//	                   map, the global enabled flag, and the ticker-
//	                   alive bookkeeping (one tea.Tick chain at a time
//	                   regardless of how many UserTypingMsg's arrive).
//
//	typingBroadcaster  Outbound throttle: holds the TypingSendFunc and
//	                   the wall clock of the last self-emit. Refuses
//	                   to send more than once per 3 seconds.
//
// The broadcaster holds a *typingTracker reference so it can consult
// the shared Enabled() flag without forcing the App to re-check it at
// every call site.
package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/typing"
)

// typingThrottle is the minimum interval between successive
// chat.typing emissions for the local user (internal/typing.Throttle).
const typingThrottle = typing.Throttle

// typingTracker owns inbound typing-indicator state: people typing and
// apps working (ai_assistant_status), which share the typing row, the
// feature switch and the expiry tick. The state itself is
// internal/typing's Tracker, shared with the Gio frontend; what is the
// TUI's own is the feature switch and the tea.Tick bookkeeping.
type typingTracker struct {
	*typing.Tracker
	// tickerOn is true while a TypingExpiredMsg tea.Tick chain is
	// known to be alive. Guards against UserTypingMsg-burst-driven
	// duplicate chains.
	tickerOn bool
	// enabled is the global feature switch (cfg.Animations.TypingIndicators).
	// When false, Add is a no-op and renderTypingLine returns "".
	enabled bool
}

func newTypingTracker() *typingTracker {
	return &typingTracker{Tracker: typing.New()}
}

// Enabled returns the global typing-indicator on/off flag.
func (t *typingTracker) Enabled() bool { return t.enabled }

// SetEnabled toggles the global typing-indicator flag.
func (t *typingTracker) SetEnabled(b bool) { t.enabled = b }

// TickerOn reports whether a TypingExpiredMsg tick chain is alive.
func (t *typingTracker) TickerOn() bool { return t.tickerOn }

// MarkTickerOn records that the caller has just scheduled a
// TypingExpiredMsg tick chain. Subsequent Adds for any channel skip
// scheduling another until Expire() reports the chain should die.
func (t *typingTracker) MarkTickerOn() { t.tickerOn = true }

// Expire sweeps lapsed typers and statuses. It returns true if any
// remain (the caller should reschedule another TypingExpiredMsg tick);
// false otherwise, and tickerOn is flipped to false so a future Add
// can start a new chain.
func (t *typingTracker) Expire() bool {
	t.tickerOn = t.Tracker.Expire()
	return t.tickerOn
}

// typingBroadcaster owns the outbound typing-emit throttle.
type typingBroadcaster struct {
	tracker  *typingTracker // for Enabled() check
	sendFn   TypingSendFunc
	lastSent time.Time
}

func newTypingBroadcaster(tracker *typingTracker) *typingBroadcaster {
	return &typingBroadcaster{tracker: tracker}
}

// SetSender wires the function used to emit a typing event for a
// channel. nil disables outbound sends.
func (b *typingBroadcaster) SetSender(fn TypingSendFunc) { b.sendFn = fn }

// ResetThrottle clears the last-sent wall clock so the next MaybeSend
// for any channel can fire immediately. Called when the user switches
// channels: throttle is per-app, not per-channel, but the user-visible
// "I just started typing here" deserves an immediate ping in the new
// channel rather than waiting for the residual throttle window.
func (b *typingBroadcaster) ResetThrottle() { b.lastSent = time.Time{} }

// CanSend reports whether the broadcaster would dispatch right now.
// Used by the test suite; production code calls MaybeSend directly.
func (b *typingBroadcaster) CanSend() bool {
	if !b.tracker.Enabled() {
		return false
	}
	return time.Since(b.lastSent) >= typingThrottle
}

// MaybeSend dispatches a typing event for channelID via the configured
// TypingSendFunc, but only if enabled, throttled-clear, and the
// function has been wired. Returns true if it actually dispatched.
// The dispatch itself happens on a goroutine so the Update loop is
// never blocked by the HTTP call.
func (b *typingBroadcaster) MaybeSend(channelID string) bool {
	if b.sendFn == nil || !b.CanSend() {
		return false
	}
	b.lastSent = time.Now()
	fn := b.sendFn
	go fn(channelID)
	return true
}

// typingExpiredTickCmd schedules the next TypingExpiredMsg one
// second out. Used by both the UserTypingMsg arm (kick the chain
// alive) and TypingExpiredMsg arm (reschedule while typers remain).
func typingExpiredTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return TypingExpiredMsg{}
	})
}

// Handle is the typing-family reducer for App.Update (Phase 4d).
// Owns UserTypingMsg (record a remote typer + ensure the expiry
// tick chain is alive) and TypingExpiredMsg (sweep expired entries
// + maybe reschedule). Returns (nil, false) for any other message
// type.
//
// The outbound throttle (typingBroadcaster.MaybeSend) is invoked
// from compose/insert-mode handlers, not from Update, so it's not
// part of this reducer.
func (t *typingTracker) Handle(a *App, msg tea.Msg) (tea.Cmd, bool) {
	_ = a
	switch m := msg.(type) {
	case UserTypingMsg:
		if !t.Enabled() {
			return nil, true
		}
		t.Add(m.ChannelID, m.UserID)
		return t.ensureTicker(), true

	case BotStatusMsg:
		if !t.Enabled() {
			return nil, true
		}
		t.SetBotStatus(m.ChannelID, m.ThreadTS, m.BotUserID, m.Status)
		if m.Status == "" {
			return nil, true
		}
		return t.ensureTicker(), true

	case TypingExpiredMsg:
		_ = m
		// Continue ticking if there are still active typers.
		if !t.Expire() {
			return nil, true
		}
		return typingExpiredTickCmd(), true
	}
	return nil, false
}

// ensureTicker starts the TypingExpiredMsg chain unless one is alive.
func (t *typingTracker) ensureTicker() tea.Cmd {
	if t.TickerOn() {
		return nil
	}
	t.MarkTickerOn()
	return typingExpiredTickCmd()
}

// typingRowText is the channel pane's typing row: who is typing, then
// which apps are working. Pure, like typingIndicatorText.
func typingRowText(typers, bots []string) string {
	return typing.Row(typers, bots, "...")
}

// typingIndicatorText formats the user-facing line for the supplied
// display names. Pure: no state, no styling, no name resolution.
func typingIndicatorText(names []string) string {
	if s := typing.Text(names); s != "" {
		return s + "..."
	}
	return ""
}
