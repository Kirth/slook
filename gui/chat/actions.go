package chat

import (
	"strings"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/internal/slackurl"
)

// slackEntities are the only escapes Slack's message text uses.
var slackEntities = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&")

// EditableText is a message's text as the composer shows it for an
// edit: Slack's three entities decoded, so "a &lt; b" reads "a < b" and
// sending escapes it once again. Mentions and links stay in their wire
// form (<@U…>, <#C…|name>), which sending keeps as they are.
func EditableText(text string) string { return slackEntities.Replace(text) }

// LastOwn is the newest of msgs the user posted that Slack keeps (not
// an ephemeral): what Up in an empty composer edits.
func LastOwn(msgs []backend.Message) (backend.Message, bool) {
	for i := len(msgs) - 1; i >= 0; i-- {
		if m := msgs[i]; m.FromSelf && !m.Ephemeral {
			return m, true
		}
	}
	return backend.Message{}, false
}

// ForwardedLink is the permalink a forwarded message consists of: its
// text is exactly one Slack archive link, <url> or <url|label>, and
// nothing else (the TUI's rule for "open the original"). A message with
// a comment, or with a link among other text, is not one.
func ForwardedLink(text string) (string, bool) {
	t := strings.TrimSpace(text)
	if len(t) < 2 || t[0] != '<' || t[len(t)-1] != '>' {
		return "", false
	}
	inner := t[1 : len(t)-1]
	url, _, _ := strings.Cut(inner, "|")
	if strings.ContainsAny(inner, "<>\n") || strings.ContainsAny(url, " \t") {
		return "", false
	}
	url = slackEntities.Replace(url) // Slack's text escapes & in a link
	if _, ok := slackurl.Parse(url); !ok {
		return "", false
	}
	return url, true
}

// ButtonActions is m's legacy buttons in order; menus are not buttons
// and are left out. Only a message only the user can see has any
// (backend.Message.Actions).
func ButtonActions(m backend.Message) []backend.Action {
	var out []backend.Action
	for _, a := range m.Actions {
		if a.Type == "button" {
			out = append(out, a)
		}
	}
	return out
}

// HasButtonActions reports whether m has a legacy button to press.
func HasButtonActions(m backend.Message) bool {
	for _, a := range m.Actions {
		if a.Type == "button" {
			return true
		}
	}
	return false
}
