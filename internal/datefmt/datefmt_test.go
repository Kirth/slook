package datefmt

import (
	"testing"
	"time"
)

// DateSeparator must agree with a date string derived from the local
// day (what the frontends' DateFromTS helpers produce). Running the
// pair under a zone east of UTC used to label yesterday's messages
// "Today", because the parsed date (UTC midnight) was subtracted from a
// local midnight.
func TestDateSeparatorAcrossTimezones(t *testing.T) {
	zones := []string{"UTC", "Europe/Kyiv", "Europe/Berlin", "Asia/Tokyo", "America/New_York", "Pacific/Kiritimati"}
	for _, name := range zones {
		t.Run(name, func(t *testing.T) {
			// Loaded per subtest so a zone missing from the host's tzdata
			// skips only itself instead of the whole table.
			loc, err := time.LoadLocation(name)
			if err != nil {
				t.Skipf("tzdata for %s unavailable: %v", name, err)
			}

			orig := time.Local
			time.Local = loc
			t.Cleanup(func() { time.Local = orig })

			now := time.Now()
			cases := []struct {
				offsetDays int
				want       string
			}{
				{0, "Today"},
				{-1, "Yesterday"},
				{-2, now.AddDate(0, 0, -2).Format("Monday")},
				{-6, now.AddDate(0, 0, -6).Format("Monday")},
				{-7, now.AddDate(0, 0, -7).Format("Monday, January 2, 2006")},
			}
			for _, tc := range cases {
				// Mid-day so the timestamp is unambiguously on its day.
				ts := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, loc).
					AddDate(0, 0, tc.offsetDays)
				got := DateSeparator(localDay(ts))
				if got != tc.want {
					t.Errorf("offset %d days: got %q, want %q", tc.offsetDays, got, tc.want)
				}
			}
		})
	}
}

// localDay is the local-day date string of t's instant, as a Slack ts
// read back through time.Unix yields it.
func localDay(t time.Time) string {
	return time.Unix(t.Unix(), 0).Format("2006-01-02")
}
