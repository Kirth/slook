// internal/ui/open_conversation.go
//
// Opening a DM or group DM by its user IDs, shared by the new-message
// picker and /msg.
package ui

import tea "charm.land/bubbletea/v2"

// msgPrefill is what /msg asks of an open: text is offered as the
// conversation's draft once it opens, and origin is where /msg was typed,
// so a failed open can give the command back. The zero value, which the
// new-message picker passes, asks for neither.
type msgPrefill struct {
	text   string
	origin commandOrigin
}

// openConversation opens (or finds) the DM or group DM with userIDs: a
// fresh in-flight request, whose NewMessageOpenedMsg switches to the
// conversation (reducer_new_message.go) and drops late results of
// cancelled ones. Shared by the new-message picker and /msg.
func (a *App) openConversation(userIDs []string, prefill msgPrefill) tea.Cmd {
	a.abandonMsgPrefill() // a /msg this request supersedes
	a.newMessageSeq++
	a.newMessageInFlightID = a.newMessageSeq
	a.newMessageCancelled = false
	a.newMessagePrefill = prefill
	return teaCmd(a.channels.OpenConversation(userIDs, a.newMessageInFlightID))
}

// openingConversation reports whether an open is in flight and not
// cancelled.
func (a *App) openingConversation() bool {
	return a.newMessageInFlightID != 0 && !a.newMessageCancelled
}

// abandonMsgPrefill ends the in-flight request's /msg pre-fill without
// using it — the open failed, was cancelled or was superseded — and gives
// the /msg command back to the composer it was typed in. A no-op when the
// request was the picker's.
func (a *App) abandonMsgPrefill() {
	a.offerTypedBack(a.newMessagePrefill.origin)
	a.newMessagePrefill = msgPrefill{}
}
