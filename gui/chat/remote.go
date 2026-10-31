package chat

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gammons/slk/gui/backend"
	itext "github.com/gammons/slk/internal/text"
)

// RemoteDelay is how long typing must pause before a filter asks Slack
// which channels match (edgeapi's channels/search, whose callers must
// debounce): one request per pause, not one per key.
const RemoteDelay = 300 * time.Millisecond

// remoteMinRunes is the shortest query worth asking Slack about.
const remoteMinRunes = 2

// remoteKeep bounds how many answers a RemoteSearch remembers.
const remoteKeep = 32

// RemoteSearch debounces a filter's channel search: Type with the
// filter's text every frame, ask Slack when Due says so, hand the
// answer to Done, and show Results. A query is asked once; typing on
// before the answer comes makes it stale, and it is not shown for
// what is typed now. Wake is when the next frame should look again.
//
// The zero value is ready to use. It is not safe for concurrent use: a
// window's event loop owns it.
type RemoteSearch struct {
	query    string    // what is typed now, trimmed; "" when too short
	typedAt  time.Time // when query last changed
	asked    string    // the query asked since it was typed
	inflight bool
	answers  map[string][]backend.ChannelResult
}

// Type records the filter's text at now. Under two characters clears
// the search.
func (r *RemoteSearch) Type(q string, now time.Time) {
	q = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(q), "#"))
	if utf8.RuneCountInString(q) < remoteMinRunes {
		q = ""
	}
	if q == r.query {
		return
	}
	r.query, r.typedAt, r.asked = q, now, ""
}

// Due reports the query to ask Slack about now: typing has paused for
// RemoteDelay, it has not been answered or asked, and no request is in
// flight. It marks the request in flight.
func (r *RemoteSearch) Due(now time.Time) (string, bool) {
	if !r.pending() || r.inflight || now.Sub(r.typedAt) < RemoteDelay {
		return "", false
	}
	r.asked, r.inflight = r.query, true
	return r.query, true
}

// pending is whether the current query still needs asking.
func (r *RemoteSearch) pending() bool {
	if r.query == "" || r.asked == r.query {
		return false
	}
	_, ok := r.answers[r.query]
	return !ok
}

// Done takes Slack's answer for q; nil res (a failure) is not kept, so
// the query is asked again if it is typed again. An answer for a query
// no longer typed is kept under its own query and not shown for the
// current one.
func (r *RemoteSearch) Done(q string, res []backend.ChannelResult) {
	r.inflight = false
	if res == nil {
		return
	}
	if r.answers == nil || len(r.answers) >= remoteKeep {
		r.answers = map[string][]backend.ChannelResult{}
	}
	r.answers[q] = res
}

// Results is Slack's answer for the query typed now, q being the
// filter's text; nil until it has come.
func (r *RemoteSearch) Results(q string) []backend.ChannelResult {
	q = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(q), "#"))
	if q == "" || q != r.query {
		return nil
	}
	return r.answers[q]
}

// Searching is whether an answer for what is typed now is still to
// come: waiting for the pause, or asked.
func (r *RemoteSearch) Searching() bool {
	return r.query != "" && (r.pending() || r.inflight && r.asked == r.query)
}

// Wake is when the next frame should run for Due to fire, zero when no
// frame is needed (nothing pending, or a request is out and its answer
// will wake the window).
func (r *RemoteSearch) Wake() time.Time {
	if !r.pending() || r.inflight {
		return time.Time{}
	}
	return r.typedAt.Add(RemoteDelay)
}

// Unjoined is the search results the user is not in and the list does
// not have, in the server's order.
func Unjoined(res []backend.ChannelResult, convs []backend.Conversation) []backend.ChannelResult {
	listed := make(map[string]bool, len(convs))
	for _, c := range convs {
		listed[c.ID] = true
	}
	var out []backend.ChannelResult
	for _, r := range res {
		if !r.Member && !listed[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

// ExactChannel is the channel among res named name, case- and
// accent-blind, a leading "#" ignored: what /open and /join take. Only
// channels and private channels match, joined or not.
func ExactChannel(name string, res []backend.ChannelResult) (backend.ChannelResult, bool) {
	want := itext.Fold(strings.TrimPrefix(strings.TrimSpace(name), "#"))
	if want == "" {
		return backend.ChannelResult{}, false
	}
	for _, r := range res {
		if r.Kind != backend.KindChannel && r.Kind != backend.KindPrivate {
			continue
		}
		if itext.Fold(r.Name) == want {
			return r, true
		}
	}
	return backend.ChannelResult{}, false
}
