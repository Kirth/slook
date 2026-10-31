package blockkit

import "github.com/gammons/slk/internal/core"

// MessageText returns the mrkdwn string that should be rendered as the
// visible message body. For most messages it's just msg.Text, but for
// messages whose body originated as a rich_text block (typical of bot
// apps like GitHub Pending Reviews, PagerDuty, etc.) Slack's text
// fallback collapses standalone "\n" elements into spaces — so we
// reconstruct a newline-faithful mrkdwn from the parsed block when one
// is available. See RichTextToMrkdwn.
//
// When no RichTextBlock is present (the overwhelmingly common case for
// user-typed messages) this is a zero-cost passthrough of msg.Text.
func MessageText(msg core.MessageItem) string {
	for _, b := range msg.Blocks {
		if rt, ok := b.(RichTextBlock); ok {
			if reconstructed := RichTextToMrkdwn(rt); reconstructed != "" {
				return reconstructed
			}
		}
	}
	return msg.Text
}
