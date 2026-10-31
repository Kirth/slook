// Package datefmt labels calendar days for display, the same way in
// every frontend: the day-divider label ("Today", "Yesterday", a weekday
// within the last week, or a full date).
package datefmt

import "time"

// now is the clock DateSeparator reads. Production leaves it as
// time.Now; tests override it via SetNow so that day-divider labels are
// deterministic.
//
// Not guarded by a mutex: no production code path writes it, so every
// write comes from test setup on the test goroutine before any render,
// and no test in the repo calls t.Parallel. A parallel test or a
// production caller would need this to become an atomic.Value.
var now = time.Now

// SetNow injects a clock for tests. Pass nil to revert to time.Now.
func SetNow(fn func() time.Time) {
	if fn == nil {
		now = time.Now
		return
	}
	now = fn
}

// DateSeparator turns a "2006-01-02" date string into the
// human-readable label used in day-divider rows ("Today", "Yesterday",
// a weekday name within the last week, or a fully-qualified date). An
// unparseable string is returned unchanged.
func DateSeparator(dateStr string) string {
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	// Compare calendar days, not elapsed time: both endpoints are anchored
	// to UTC midnight so the delta is always a whole number of days.
	n := now()
	// time.Parse already yields UTC midnight; restating it is defensive, not
	// load-bearing.
	dDay := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
	days := int(today.Sub(dDay).Hours() / 24)

	switch {
	case days <= 0:
		return "Today"
	case days == 1:
		return "Yesterday"
	case days < 7:
		return d.Format("Monday")
	default:
		return d.Format("Monday, January 2, 2006")
	}
}
