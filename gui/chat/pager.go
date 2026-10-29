package chat

import "time"

// olderThreshold is how close to the top of what is shown, in rows,
// scrolling must come before the page before it is asked for.
const olderThreshold = 5

// pagerRetry is how long a failed page waits before the next attempt,
// so a failure does not become a request every frame.
const pagerRetry = 10 * time.Second

// Pager decides when a timeline asks for the page before what it
// shows: one page at a time, only when the user has scrolled near the
// top, and never again once the conversation's first message is shown.
type Pager struct {
	Loading bool // a page is in flight
	AtStart bool // the conversation's first message is shown
	retryAt time.Time
}

// Want reports whether to ask for the page before what is shown, given
// the index of the first visible message among the shown ones (negative
// when a row above the messages is visible), and marks a page in flight
// when so.
func (p *Pager) Want(firstShown, shown int, now time.Time) bool {
	if p.Loading || p.AtStart || shown == 0 || firstShown >= olderThreshold || now.Before(p.retryAt) {
		return false
	}
	p.Loading = true
	return true
}

// Done records a page's result. An empty page means there is nothing
// older: the start is shown. A failure waits before the next attempt.
func (p *Pager) Done(got int, err error, now time.Time) {
	p.Loading = false
	switch {
	case err != nil:
		p.retryAt = now.Add(pagerRetry)
	case got == 0:
		p.AtStart = true
	}
}
