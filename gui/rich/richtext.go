package rich

import (
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/emoji"
)

// FromRichText turns a rich_text block's elements into blocks. It is the
// display-side twin of blockkit.RichTextToMrkdwn, which serialises the
// same elements back to mrkdwn for the terminal.
func FromRichText(elements []slack.RichTextElement, n Names) []Block {
	var out []Block
	for _, e := range elements {
		switch v := e.(type) {
		case *slack.RichTextSection:
			if spans := inlineSpans(v.Elements, n); len(spans) > 0 {
				out = append(out, Paragraph{Spans: spans})
			}
		case *slack.RichTextList:
			out = append(out, listBlock(v, n))
		case *slack.RichTextPreformatted:
			out = append(out, CodeBlock{Text: preformattedText(v.Elements)})
		case *slack.RichTextQuote:
			if spans := inlineSpans(v.Elements, n); len(spans) > 0 {
				out = append(out, Quote{Blocks: []Block{Paragraph{Spans: spans}}})
			}
		}
	}
	return out
}

// UserIDs lists the users mentioned in rich_text elements, so a backend
// can resolve their names before building the message.
func UserIDs(elements []slack.RichTextElement) []string {
	var ids []string
	var walk func([]slack.RichTextSectionElement)
	walk = func(els []slack.RichTextSectionElement) {
		for _, e := range els {
			if u, ok := e.(*slack.RichTextSectionUserElement); ok {
				ids = append(ids, u.UserID)
			}
		}
	}
	for _, e := range elements {
		switch v := e.(type) {
		case *slack.RichTextSection:
			walk(v.Elements)
		case *slack.RichTextQuote:
			walk(v.Elements)
		case *slack.RichTextList:
			for _, item := range v.Elements {
				if sec, ok := item.(*slack.RichTextSection); ok {
					walk(sec.Elements)
				}
			}
		}
	}
	return ids
}

func listBlock(l *slack.RichTextList, n Names) List {
	out := List{Ordered: l.Style == slack.RTEListOrdered, Start: l.Offset, Indent: l.Indent}
	for _, child := range l.Elements {
		var spans []Span
		if sec, ok := child.(*slack.RichTextSection); ok {
			spans = inlineSpans(sec.Elements, n)
		}
		out.Items = append(out.Items, spans)
	}
	return out
}

func preformattedText(els []slack.RichTextSectionElement) string {
	var b strings.Builder
	for _, e := range els {
		switch v := e.(type) {
		case *slack.RichTextSectionTextElement:
			b.WriteString(v.Text)
		case *slack.RichTextSectionLinkElement:
			b.WriteString(v.URL)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// inlineSpans converts a section's inline elements. A trailing newline
// is dropped: the block boundary already breaks the line.
func inlineSpans(els []slack.RichTextSectionElement, n Names) []Span {
	var spans []Span
	for _, e := range els {
		if s, ok := inlineSpan(e, n); ok && s.Text != "" {
			spans = append(spans, s)
		}
	}
	if len(spans) > 0 {
		last := &spans[len(spans)-1]
		last.Text = strings.TrimRight(last.Text, "\n")
		if last.Text == "" {
			spans = spans[:len(spans)-1]
		}
	}
	return spans
}

func inlineSpan(e slack.RichTextSectionElement, n Names) (Span, bool) {
	switch v := e.(type) {
	case *slack.RichTextSectionTextElement:
		return styled(Span{Text: v.Text}, v.Style), true
	case *slack.RichTextSectionLinkElement:
		label := v.Text
		if label == "" {
			label = v.URL
		}
		return styled(Span{Text: label, URL: v.URL}, v.Style), true
	case *slack.RichTextSectionUserElement:
		name, ok := n.User(v.UserID)
		if !ok {
			name = v.UserID
		}
		return styled(Span{Text: "@" + name, ID: v.UserID, Kind: SpanMention}, v.Style), true
	case *slack.RichTextSectionChannelElement:
		name, ok := n.Channel(v.ChannelID)
		if !ok {
			name = v.ChannelID
		}
		return styled(Span{Text: "#" + name, ID: v.ChannelID, Kind: SpanChannel}, v.Style), true
	case *slack.RichTextSectionUserGroupElement:
		name, ok := n.UserGroup(v.UsergroupID)
		if !ok {
			name = v.UsergroupID
		}
		return styled(Span{Text: "@" + name, Kind: SpanBroadcast}, v.Style), true
	case *slack.RichTextSectionBroadcastElement:
		return styled(Span{Text: "@" + v.Range, Kind: SpanBroadcast}, v.Style), true
	case *slack.RichTextSectionEmojiElement:
		return styled(emojiSpan(v), v.Style), true
	case *slack.RichTextSectionDateElement:
		if v.Fallback != nil && *v.Fallback != "" {
			return styled(Span{Text: *v.Fallback}, v.Style), true
		}
		t := time.Unix(int64(v.Timestamp), 0)
		return styled(Span{Text: t.Local().Format("Jan 2, 2006 15:04")}, v.Style), true
	case *slack.RichTextSectionColorElement:
		return Span{Text: v.Value}, true
	case *slack.RichTextSectionTeamElement:
		return Span{Text: "@" + v.TeamID, Kind: SpanBroadcast}, true
	}
	return Span{}, false
}

func styled(s Span, st *slack.RichTextSectionTextStyle) Span {
	if st != nil {
		s.Bold, s.Italic, s.Strike, s.Code = st.Bold, st.Italic, st.Strike, st.Code
	}
	return s
}

// emojiSpan is the glyph for an emoji element: from the codepoints Slack
// sends, else the standard table, else the shortcode (a custom emoji).
func emojiSpan(v *slack.RichTextSectionEmojiElement) Span {
	if g := glyphFromCodepoints(v.Unicode); g != "" {
		return Span{Text: g}
	}
	name := v.Name
	if v.SkinTone >= 2 && v.SkinTone <= 6 {
		name += "::skin-tone-" + strconv.Itoa(v.SkinTone)
	}
	if g, ok := EmojiGlyph(name); ok {
		return Span{Text: g}
	}
	return Span{Text: ":" + v.Name + ":", Kind: SpanEmoji}
}

// EmojiGlyph is the character for a standard emoji named as Slack names
// them: "leaves", ":leaves:", or "wave::skin-tone-3". It is false for a
// workspace's custom emoji, which have pictures, not characters.
func EmojiGlyph(name string) (string, bool) {
	name = strings.Trim(strings.TrimSpace(name), ":")
	base, tone := name, 0
	if i := strings.Index(name, "::skin-tone-"); i >= 0 {
		base = name[:i]
		tone, _ = strconv.Atoi(name[i+len("::skin-tone-"):])
	}
	g, ok := emoji.CodeMap()[":"+base+":"]
	if !ok || base == "" {
		return "", false
	}
	if tone >= 2 && tone <= 6 {
		g += string(rune(0x1F3FB + tone - 2))
	}
	return g, true
}

// glyphFromCodepoints decodes Slack's "1f44d-1f3fb" form.
func glyphFromCodepoints(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, part := range strings.Split(s, "-") {
		cp, err := strconv.ParseUint(part, 16, 32)
		if err != nil {
			return ""
		}
		b.WriteRune(rune(cp))
	}
	return b.String()
}
