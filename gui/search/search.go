// Package search holds the search window's state logic apart from
// drawing it, so it can be tested without a display: the words for the
// cache's coverage and the backfill, the order of the coverage rows,
// and where a result leads.
package search

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/internal/text"
)

// Summary is the window's subtitle: how much the cache holds.
func Summary(st backend.SyncStatus) string {
	complete := 0
	for _, c := range st.Conversations {
		if c.Complete {
			complete++
		}
	}
	msgs := "1 message"
	if st.Messages != 1 {
		msgs = Commas(st.Messages) + " messages"
	}
	if len(st.Conversations) == 0 {
		return msgs
	}
	return fmt.Sprintf("%s · %d of %d conversations complete", msgs, complete, len(st.Conversations))
}

// BackfillLine is the cache panel's first line: the backfill's state.
func BackfillLine(b backend.BackfillStatus, now time.Time) string {
	switch b.State {
	case backend.BackfillRunning, backend.BackfillPaused:
		var parts []string
		switch {
		case b.Phase == "threads":
			parts = append(parts, "Threads")
		case b.Current != "":
			parts = append(parts, "Backfilling "+b.Current)
		default:
			parts = append(parts, "Backfilling")
		}
		switch {
		case b.Phase == "threads":
			parts = append(parts, fmt.Sprintf("%d fetched", b.Done))
		case b.Total > 0:
			parts = append(parts, fmt.Sprintf("%d of %d", b.Done, b.Total))
		}
		if b.Requests > 0 {
			parts = append(parts, Commas(b.Requests)+" requests")
		}
		line := strings.Join(parts, " · ")
		if b.State == backend.BackfillPaused {
			line += " · paused while you read"
		}
		return line
	case backend.BackfillDone:
		return "Done " + doneAt(b, now) + " · " + Commas(b.Requests) + " requests"
	case backend.BackfillFailed:
		return "Failed: " + b.LastError
	default:
		return "Off"
	}
}

// doneAt words when a run ended: its start time today, or the date.
func doneAt(b backend.BackfillStatus, now time.Time) string {
	if b.StartedAt.IsZero() {
		return ""
	}
	t := b.StartedAt.Local()
	if y, m, d := t.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return "today"
	}
	return t.Format("Jan 2")
}

// Horizon words a horizon for a label: "90 days", "1 year", "everything".
func Horizon(d time.Duration) string {
	switch {
	case d == 0:
		return "everything"
	case d%(365*24*time.Hour) == 0:
		y := int(d / (365 * 24 * time.Hour))
		if y == 1 {
			return "1 year"
		}
		return strconv.Itoa(y) + " years"
	default:
		return strconv.Itoa(int(d/(24*time.Hour))) + " days"
	}
}

// HorizonFor is the duration behind a horizon choice: "90d", "1y", or
// "all" (and anything unknown) for everything.
func HorizonFor(key string) time.Duration {
	switch key {
	case "90d":
		return 90 * 24 * time.Hour
	case "1y":
		return 365 * 24 * time.Hour
	}
	return 0
}

// OrderCoverage sorts coverage rows for reading: the conversations
// still being filled first (those with no coverage, then the shallowest),
// complete ones after, each group by name.
func OrderCoverage(cs []backend.Coverage) []backend.Coverage {
	out := slices.Clone(cs)
	slices.SortStableFunc(out, func(a, b backend.Coverage) int {
		if a.Complete != b.Complete {
			if a.Complete {
				return 1
			}
			return -1
		}
		return strings.Compare(text.Fold(a.Name), text.Fold(b.Name))
	})
	return out
}

// CoverageLine words one conversation's coverage: "complete", "since
// Jan 2025", or "nothing yet", with gaps when any.
func CoverageLine(c backend.Coverage) string {
	var s string
	switch {
	case c.Complete:
		s = "complete"
	case c.Oldest.IsZero():
		s = "nothing yet"
	default:
		s = "since " + c.Oldest.Local().Format("Jan 2006")
	}
	if c.Holes == 1 {
		s += " · 1 gap"
	} else if c.Holes > 1 {
		s += fmt.Sprintf(" · %d gaps", c.Holes)
	}
	return s
}

// Target is where a result leads: its conversation and the message to
// show there. A reply that lives only in its thread leads to the
// thread's parent, which the conversation window can show, and names
// the thread (threadTS, its parent's ts) to open on the reply itself;
// threadTS is "" for anything else.
func Target(h backend.SearchHit) (channelID, ts, threadTS string) {
	m := h.Message
	if m.InThreadOnly && m.ThreadTS != "" {
		return m.ChannelID, m.ThreadTS, m.ThreadTS
	}
	return m.ChannelID, m.TS, ""
}

// Commas writes n with thousands separators.
func Commas(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Fraction is how much of the window [now-horizon, now] the spans
// cover, 0 to 1. A complete conversation is 1 whatever the horizon
// (there is nothing older to get); so is one whose spans reach further
// back than the horizon. Horizon 0 (everything) has no window to
// measure against: it is 1 when complete, else 0.
func Fraction(c backend.Coverage, horizon time.Duration, now time.Time) float64 {
	if c.Complete {
		return 1
	}
	if horizon <= 0 {
		return 0
	}
	start := now.Add(-horizon)
	var covered time.Duration
	for _, sp := range c.Spans {
		lo, hi := sp.Oldest, sp.Latest
		if lo.Before(start) {
			lo = start
		}
		if hi.After(now) {
			hi = now
		}
		if hi.After(lo) {
			covered += hi.Sub(lo)
		}
	}
	// A span that reaches the head covers up to now, not just to its
	// newest message: nothing is missing above it.
	if n := len(c.Spans); n > 0 && c.Live {
		last := c.Spans[n-1]
		lo := last.Latest
		if lo.Before(start) {
			lo = start
		}
		if now.After(lo) {
			covered += now.Sub(lo)
		}
	}
	f := float64(covered) / float64(horizon)
	return max(0, min(1, f))
}

// Counted is the conversations the backfill works on: those not
// ignored by configuration. Progress is measured over these.
func Counted(cs []backend.Coverage) []backend.Coverage {
	var out []backend.Coverage
	for _, c := range cs {
		if !c.Ignored {
			out = append(out, c)
		}
	}
	return out
}

// Overall is the mean Fraction over the counted conversations: how far
// the whole cache is toward the horizon. 0 with none.
func Overall(cs []backend.Coverage, horizon time.Duration, now time.Time) float64 {
	cs = Counted(cs)
	if len(cs) == 0 {
		return 0
	}
	var sum float64
	for _, c := range cs {
		sum += Fraction(c, horizon, now)
	}
	return sum / float64(len(cs))
}

// Percent words a fraction: "42%".
func Percent(f float64) string {
	return strconv.Itoa(int(f*100+0.5)) + "%"
}

// Rate words how fast a run brings messages in: "1,230 msgs/min".
func Rate(fetched int, elapsed time.Duration) string {
	if elapsed < time.Second || fetched == 0 {
		return ""
	}
	perMin := float64(fetched) / elapsed.Minutes()
	switch {
	case perMin >= 10:
		return Commas(int(perMin+0.5)) + " msgs/min"
	case perMin >= 1:
		return fmt.Sprintf("%.1f msgs/min", perMin)
	default:
		return fmt.Sprintf("%.1f msgs/hour", perMin*60)
	}
}

// Estimator guesses how long a run has left from how the overall
// fraction has moved lately.
type Estimator struct {
	samples []sample
}

type sample struct {
	t time.Time
	f float64
}

// estimatorWindow is how far back the estimate looks: long enough to
// smooth the per-page jumps, short enough to follow a change of pace.
const estimatorWindow = 5 * time.Minute

// Add records the overall fraction f at t.
func (e *Estimator) Add(t time.Time, f float64) {
	e.samples = append(e.samples, sample{t, f})
	cut := t.Add(-estimatorWindow)
	for len(e.samples) > 1 && e.samples[0].t.Before(cut) {
		e.samples = e.samples[1:]
	}
}

// Reset forgets the samples (a run started).
func (e *Estimator) Reset() { e.samples = nil }

// Remaining is the time to reach 1 at the pace of the recorded samples;
// false when there is no pace yet (fewer than two samples, or no
// movement between them).
func (e *Estimator) Remaining() (time.Duration, bool) {
	if len(e.samples) < 2 {
		return 0, false
	}
	first, last := e.samples[0], e.samples[len(e.samples)-1]
	if last.f <= first.f || last.f >= 1 || !last.t.After(first.t) {
		return 0, false
	}
	perSecond := (last.f - first.f) / last.t.Sub(first.t).Seconds()
	return time.Duration((1 - last.f) / perSecond * float64(time.Second)), true
}

// Left words a remaining time: "~18 min left", "~2 h left", "under a
// minute left".
func Left(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute left"
	case d < 90*time.Minute:
		return fmt.Sprintf("~%d min left", int(d.Minutes()+0.5))
	case d < 48*time.Hour:
		return fmt.Sprintf("~%.1f h left", d.Hours())
	default:
		return fmt.Sprintf("~%d days left", int(d.Hours()/24+0.5))
	}
}
