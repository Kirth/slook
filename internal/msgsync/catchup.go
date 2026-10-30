package msgsync

import (
	"slices"
	"strings"
)

// Stale lists the conversations a socket connect should catch up, most
// important first: the ones the cache already follows (heads holds
// their newest covered ts) whose newest message per Slack (latest, from
// client.counts) is past it. Conversations ignored reports true for are
// left out, and so are ones the cache has never followed: a connect
// catches up what the socket missed, it does not start following
// anything, so a first run or a never-opened conversation costs
// nothing.
//
// order is the preferred order (most recently visited, then most
// recently active); stale conversations missing from it come after,
// newest first. At most limit are returned (limit <= 0 means none).
//
// This is the one autonomous fetch besides the opt-in backfill (see the
// package doc and Config): bounded by the cache's own coverage, by what
// actually changed, and by limit, so it can never become a sweep.
func Stale(order []string, heads, latest map[string]string, ignored func(channelID string) bool, limit int) []string {
	if limit <= 0 {
		return nil
	}
	stale := func(id string) bool {
		head, followed := heads[id]
		l := latest[id]
		if !followed || l == "" || head == "" || !tsAfter(l, head) {
			return false
		}
		return ignored == nil || !ignored(id)
	}
	var out []string
	listed := make(map[string]bool, len(order))
	for _, id := range order {
		if listed[id] {
			continue
		}
		listed[id] = true
		if stale(id) {
			out = append(out, id)
		}
	}
	var rest []string
	for id := range heads {
		if !listed[id] && stale(id) {
			rest = append(rest, id)
		}
	}
	slices.SortFunc(rest, func(a, b string) int {
		switch {
		case tsAfter(latest[a], latest[b]):
			return -1
		case tsAfter(latest[b], latest[a]):
			return 1
		}
		return strings.Compare(a, b)
	})
	out = append(out, rest...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// tsAfter reports whether Slack ts a is after b, comparing the seconds
// as numbers (a shorter seconds part is earlier), then the fraction.
func tsAfter(a, b string) bool {
	as, af, _ := strings.Cut(a, ".")
	bs, bf, _ := strings.Cut(b, ".")
	if len(as) != len(bs) {
		return len(as) > len(bs)
	}
	if as != bs {
		return as > bs
	}
	return af > bf
}
