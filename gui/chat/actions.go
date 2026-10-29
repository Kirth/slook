package chat

import (
	"strings"

	"github.com/gammons/slk/gui/backend"
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
