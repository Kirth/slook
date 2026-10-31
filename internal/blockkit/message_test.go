package blockkit

import (
	"strings"
	"testing"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/core"
)

// TestMessageText_NoBlocksReturnsRawText: the common case for
// user-typed messages. With no rich_text block on hand, the helper
// just hands msg.Text through unchanged.
func TestMessageText_NoBlocksReturnsRawText(t *testing.T) {
	msg := core.MessageItem{Text: "hello world"}
	if got := MessageText(msg); got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

// TestMessageText_NonRichTextBlocksReturnRawText: messages that have
// block-kit content (header/section/etc.) but no rich_text body
// continue to return msg.Text, which is what copying the message
// yields.
func TestMessageText_NonRichTextBlocksReturnRawText(t *testing.T) {
	msg := core.MessageItem{
		Text: "PR opened",
		Blocks: []Block{
			HeaderBlock{Text: "Pull Request opened"},
			SectionBlock{Text: "details"},
		},
	}
	if got := MessageText(msg); got != "PR opened" {
		t.Errorf("got %q, want %q", got, "PR opened")
	}
}

// TestMessageText_RichTextOverridesLossyText: the bug-fix contract.
// When a message has a rich_text block, the helper reconstructs the
// body from it instead of using Slack's newline-stripped text fallback.
func TestMessageText_RichTextOverridesLossyText(t *testing.T) {
	rt := RichTextBlock{Elements: []slack.RichTextElement{
		&slack.RichTextSection{
			Type: slack.RTESection,
			Elements: []slack.RichTextSectionElement{
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "line1"},
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "\n"},
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "line2"},
			},
		},
	}}
	msg := core.MessageItem{
		Text:   "line1 line2", // Slack's lossy fallback (newline → space)
		Blocks: []Block{rt},
	}
	got := MessageText(msg)
	if !strings.Contains(got, "line1\nline2") {
		t.Errorf("got %q, want it to contain %q (newline preserved from rich_text)", got, "line1\nline2")
	}
}
