package messages

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// utcZone pins the timezone for deterministic expectations.
func utcZone() *time.Location {
	return time.FixedZone("UTC", 0)
}

// 2026-05-28 19:36:14 UTC, a Thursday.
const testTS = "1779996974"

func TestFormatSlackDateTokenIn_Tokens(t *testing.T) {
	cases := []struct {
		format string
		want   string
	}{
		{"{date_num}", "2026-05-28"},
		{"{date}", "May 28, 2026"},
		{"{date_short}", "May 28, 2026"},
		{"{date_long}", "Thursday, May 28, 2026"},
		{"{time}", "7:36 PM"},
		{"{time_secs}", "7:36:14 PM"},
		{"{date_short_pretty}", "May 28th, 2026"},
		{"{date_pretty}", "May 28th, 2026"},
		{"{date_long_pretty}", "Thursday, May 28th, 2026"},
		{"{date_num} {time}", "2026-05-28 7:36 PM"},
	}
	for _, tc := range cases {
		got := formatSlackDateTokenIn(testTS, tc.format, "FALLBACK", "", utcZone())
		if got != tc.want {
			t.Errorf("%s = %q; want %q", tc.format, got, tc.want)
		}
	}
}

func TestFormatSlackDateTokenIn_Ordinals(t *testing.T) {
	// Independent expected suffixes (not computed via ordinalSuffix, so
	// a bug in that function cannot pass its own test). Slack's pretty
	// tokens use English ordinal day suffixes.
	suffixes := map[int]string{
		1: "st", 2: "nd", 3: "rd", 4: "th", 5: "th", 6: "th", 7: "th",
		8: "th", 9: "th", 10: "th", 11: "th", 12: "th", 13: "th",
		14: "th", 15: "th", 16: "th", 17: "th", 18: "th", 19: "th",
		20: "th", 21: "st", 22: "nd", 23: "rd", 24: "th", 25: "th",
		26: "th", 27: "th", 28: "th", 29: "th", 30: "th", 31: "st",
	}
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC).Unix()
	for d := 1; d <= 31; d++ {
		ts := base + int64(d-1)*86400
		got := formatSlackDateTokenIn(
			strconv.FormatInt(ts, 10), "{date_short_pretty}", "FB", "", utcZone())
		want := time.Unix(ts, 0).In(utcZone()).Format("Jan") +
			" " + strconv.Itoa(d) + suffixes[d] + ", 2026"
		if got != want {
			t.Errorf("day %d: got %q, want %q", d, got, want)
		}
	}
}

func TestFormatSlackDateTokenIn_TimezoneShifts(t *testing.T) {
	// 2026-05-28 19:36:14 UTC rendered in UTC+10 crosses to May 29.
	tokyo := time.FixedZone("UTC+10", 10*3600)
	got := formatSlackDateTokenIn(testTS, "{date_num}", "FB", "", tokyo)
	if got != "2026-05-29" {
		t.Errorf("UTC+10 {date_num} = %q, want 2026-05-29", got)
	}
}

func TestDateTokenTimeLayoutFollowsConfig(t *testing.T) {
	// {time} and {time_secs} render with the timestamp_format-seeded
	// layout; restore the default so other tests are unaffected.
	defer SetDateTokenTimeLayout("3:04 PM")

	SetDateTokenTimeLayout("15:04")
	if got := formatSlackDateTokenIn(testTS, "{time}", "FB", "", utcZone()); got != "19:36" {
		t.Errorf("24h {time} = %q, want 19:36", got)
	}
	if got := formatSlackDateTokenIn(testTS, "{time_secs}", "FB", "", utcZone()); got != "19:36:14" {
		t.Errorf("24h {time_secs} = %q, want 19:36:14", got)
	}

	SetDateTokenTimeLayout("3:04 PM")
	if got := formatSlackDateTokenIn(testTS, "{time_secs}", "FB", "", utcZone()); got != "7:36:14 PM" {
		t.Errorf("12h {time_secs} = %q, want 7:36:14 PM", got)
	}

	SetDateTokenTimeLayout("")
	if got := formatSlackDateTokenIn(testTS, "{time}", "FB", "", utcZone()); got != "7:36 PM" {
		t.Errorf("empty layout must keep default: got %q, want 7:36 PM", got)
	}
}

func TestTimeLayoutWithSeconds(t *testing.T) {
	cases := []struct{ in, want string }{
		{"3:04 PM", "3:04:05 PM"},
		{"15:04", "15:04:05"},
		{"15:04:05", "15:04:05"},
		{"15.04.05", "15.04.05"}, // seconds present, dot separator
		{"15:4", "15:4:05"},      // zero-suppressed minutes
		{"3:4", "3:4:05"},
		{"3:04PM", "3:04:05PM"},
	}
	for _, tc := range cases {
		if got := timeLayoutWithSeconds(tc.in); got != tc.want {
			t.Errorf("timeLayoutWithSeconds(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRenderDateToken_MalformedFallsBack(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bad ts uses fallback", "<!date^notanumber^{date}|Nov 14>", "Nov 14"},
		{"bad ts without fallback keeps raw", "<!date^notanumber^{date}>", "<!date^notanumber^{date}>"},
		{"format without tokens uses fallback", "<!date^1779996974^plain text|Nov 14>", "Nov 14"},
		{"empty ts uses fallback", "<!date^^{date}|Nov 14>", "Nov 14"},
		{"missing comps uses fallback", "<!date^only|Nov 14>", "Nov 14"},
		{"pipe link form drops link", "<!date^1779996974^{date_num}|May 28, 2026|https://x.example>", time.Unix(1779996974, 0).Format("2006-01-02")},
		{"caret link form drops link", "<!date^1779996974^{date_num}^https://x.example|May 28, 2026>", time.Unix(1779996974, 0).Format("2006-01-02")},
		{"negative ts renders", "<!date^-100^{date_num}|FB>", time.Unix(-100, 0).Format("2006-01-02")},
	}
	for _, tc := range cases {
		if got := renderDateToken(tc.in); got != tc.want {
			t.Errorf("%s: renderDateToken = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRenderSlackMarkdown_CalendarStyleRange(t *testing.T) {
	// Outlook Calendar posts a start/end pair of tokens; the times live
	// only in the FORMAT strings. Run through the styled pipeline and
	// assert both local-formatted times appear.
	start := time.Date(2026, 10, 13, 11, 30, 0, 0, time.UTC).Unix()
	end := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC).Unix()
	in := "<!date^" + strconv.FormatInt(start, 10) + "^{time}|10/13/26> - " +
		"<!date^" + strconv.FormatInt(end, 10) + "^{time}|10/13/26> " +
		"<https://teams.example|Team standup>"
	out := ansi.Strip(RenderSlackMarkdown(in, nil, nil))
	// Tokens render in the local zone, so the expected clock strings
	// are computed the same way; the deterministic token-level tests
	// above cover the layouts themselves.
	wantStart := time.Unix(start, 0).Format("3:04 PM")
	wantEnd := time.Unix(end, 0).Format("3:04 PM")
	if !strings.Contains(out, wantStart) {
		t.Errorf("start time missing: %q", out)
	}
	if !strings.Contains(out, wantEnd) {
		t.Errorf("end time missing: %q", out)
	}
	if !strings.Contains(out, "Team standup") {
		t.Errorf("link label missing: %q", out)
	}
	if strings.Contains(out, "10/13/26") {
		t.Errorf("fallback dates leaked: %q", out)
	}
}

func TestFlattenMrkdwn_DateTokenFormatted(t *testing.T) {
	// The flatten path (search snippets, notifications) must format the
	// same way; the expectation is computed in the local zone so the
	// test holds in every CI timezone.
	in := "due <!date^1700000000^{date_num}|Nov 14>"
	want := "due " + time.Unix(1700000000, 0).Format("2006-01-02")
	if got := FlattenMrkdwn(in, nil, nil); got != want {
		t.Errorf("FlattenMrkdwn = %q, want %q", got, want)
	}
}
