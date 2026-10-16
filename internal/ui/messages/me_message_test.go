package messages

import (
	"regexp"
	"strings"
	"testing"
)

// italicSGR matches an SGR sequence that turns italic on (parameter 3,
// alone or among others).
var italicSGR = regexp.MustCompile(`\x1b\[(?:[0-9]+;)*3(?:;[0-9]+)*m`)

// bodyLine returns the line of content that carries the body text. The
// header is not checked: its timestamp is always italic, so it says
// nothing about the body.
func bodyLine(t *testing.T, content, timestamp string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "waves") && !strings.Contains(line, timestamp) {
			return line
		}
	}
	t.Fatalf("no body line containing \"waves\" in content:\n%q", content)
	return ""
}

func TestRenderMessage_MeMessageIsItalic(t *testing.T) {
	items := []MessageItem{
		{TS: "1.0", UserID: "U1", UserName: "priya", Text: "waves", Timestamp: "3:40 PM", Subtype: "me_message"},
		{TS: "2.0", UserID: "U1", UserName: "priya", Text: "waves", Timestamp: "3:41 PM"},
	}
	m := New(items, "general")
	me := RenderMessage(items[0], 60, m.renderContext(), RenderOptions{})
	plain := RenderMessage(items[1], 60, m.renderContext(), RenderOptions{})
	if got := bodyLine(t, me.Content, "3:40 PM"); !italicSGR.MatchString(got) {
		t.Errorf("me_message body line has no italic SGR:\n%q", got)
	}
	if got := bodyLine(t, plain.Content, "3:41 PM"); italicSGR.MatchString(got) {
		t.Errorf("ordinary message body line is italic:\n%q", got)
	}
}
