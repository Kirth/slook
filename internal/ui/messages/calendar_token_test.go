package messages

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

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
	// in internal/slackfmt cover the layouts themselves.
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
