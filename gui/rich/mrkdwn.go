package rich

import (
	"html"
	"strings"

	"github.com/gammons/slk/internal/slack/mrkdwn"
)

// FromMrkdwn parses Slack mrkdwn text (a message's `text`, a section's
// text, an attachment's text) into blocks. mrkdwn.Convert does the
// parsing; this adds what it lacks for received text: quote lines and
// the &amp; &lt; &gt; entities Slack escapes on the wire. Entities are
// decoded after parsing, so an escaped "&lt;@U1&gt;" stays literal text
// rather than becoming a mention.
func FromMrkdwn(text string, n Names) []Block {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	// Slack's mrkdwn has no backslash escapes, but the CommonMark parser
	// behind mrkdwn.Convert would eat them ("¯\_(ツ)_/¯", "C:\_tmp"), so
	// they cross it as a stand-in. A stand-in already in the text is
	// replaced first, so it cannot turn into a backslash.
	text = strings.ReplaceAll(text, backslashStandIn, "\uFFFD")
	text = strings.ReplaceAll(text, "\\", backslashStandIn)
	var out []Block
	for _, seg := range splitQuotes(text) {
		if strings.TrimSpace(seg.text) == "" {
			continue
		}
		var blocks []Block
		if _, rt := mrkdwn.Convert(seg.text); rt != nil {
			blocks = FromRichText(rt.Elements, n)
		}
		if seg.quote {
			if len(blocks) > 0 {
				out = append(out, Quote{Blocks: blocks})
			}
			continue
		}
		out = append(out, blocks...)
	}
	unescape(out)
	return out
}

// Spans is FromMrkdwn flattened to one run of spans, for places that
// hold a line of text rather than blocks (fields, context elements).
// Block boundaries become line breaks.
func Spans(text string, n Names) []Span {
	var spans []Span
	for _, b := range FromMrkdwn(text, n) {
		if len(spans) > 0 {
			spans = append(spans, Span{Text: "\n"})
		}
		switch v := b.(type) {
		case Paragraph:
			spans = append(spans, v.Spans...)
		case Heading:
			spans = append(spans, v.Spans...)
		case CodeBlock:
			spans = append(spans, Span{Text: v.Text, Code: true})
		case Quote:
			for _, q := range v.Blocks {
				if p, ok := q.(Paragraph); ok {
					spans = append(spans, p.Spans...)
				}
			}
		case List:
			for i, item := range v.Items {
				if i > 0 {
					spans = append(spans, Span{Text: "\n"})
				}
				spans = append(spans, Span{Text: "• "})
				spans = append(spans, item...)
			}
		}
	}
	return spans
}

type segment struct {
	text  string
	quote bool
}

// splitQuotes cuts text into runs of quoted and unquoted lines. A line
// quoted with ">" or "&gt;" (Slack escapes it) opens a quote; ">>>"
// quotes everything that follows.
func splitQuotes(text string) []segment {
	var segs []segment
	add := func(line string, quote bool) {
		if len(segs) > 0 && segs[len(segs)-1].quote == quote {
			segs[len(segs)-1].text += "\n" + line
			return
		}
		segs = append(segs, segment{text: line, quote: quote})
	}
	rest := false
	inCode := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
		}
		if inCode && !rest {
			add(line, false)
			continue
		}
		if rest {
			add(line, true)
			continue
		}
		if inner, ok := quoteRest(line); ok {
			rest = true
			add(inner, true)
			continue
		}
		if inner, ok := quoteLine(line); ok {
			add(inner, true)
			continue
		}
		add(line, false)
	}
	return segs
}

func quoteRest(line string) (string, bool) {
	for _, p := range []string{"&gt;&gt;&gt;", ">>>"} {
		if strings.HasPrefix(line, p) {
			return strings.TrimPrefix(line[len(p):], " "), true
		}
	}
	return "", false
}

func quoteLine(line string) (string, bool) {
	for _, p := range []string{"&gt;", ">"} {
		if strings.HasPrefix(line, p) {
			return strings.TrimPrefix(line[len(p):], " "), true
		}
	}
	return "", false
}

// backslashStandIn is a private-use character mrkdwn.Convert does not
// reserve (it uses U+E000 and U+E001).
const backslashStandIn = "\uE002"

// unescape decodes the entities Slack escapes and puts backslashes back.
func unescape(blocks []Block) {
	fix := func(s string) string {
		return strings.ReplaceAll(html.UnescapeString(s), backslashStandIn, "\\")
	}
	mapSpans(blocks, func(s *Span) { s.Text = fix(s.Text) })
	fixCode(blocks, fix)
}

// fixCode applies fix to code blocks, which hold text but no spans, at
// any depth. Spans are mapSpans' job, so nothing is decoded twice.
func fixCode(blocks []Block, fix func(string) string) {
	for i, b := range blocks {
		switch v := b.(type) {
		case CodeBlock:
			v.Text = fix(v.Text)
			blocks[i] = v
		case Quote:
			fixCode(v.Blocks, fix)
		case Card:
			fixCode(v.Blocks, fix)
		}
	}
}
