package rich

import (
	"time"

	"github.com/gammons/slk/internal/blockkit"
	"github.com/gammons/slk/internal/core/blocks"
)

// Body is a message's main content: its rich_text block when it has one
// (Slack sends one for everything typed in a client), else its other
// blocks when they carry the body, else its mrkdwn text. Blocks that do
// not carry the body still follow the text.
func Body(text string, bs []blocks.Block, n Names) []Block {
	hasRich := false
	for _, b := range bs {
		if rt, ok := b.(blocks.RichTextBlock); ok && len(rt.Elements) > 0 {
			hasRich = true
		}
	}
	var out []Block
	if !hasRich && !blockkit.RendersBody(bs) {
		out = FromMrkdwn(text, n)
	}
	return append(out, FromBlocks(bs, n)...)
}

// FromBlocks converts Block Kit blocks.
func FromBlocks(bs []blocks.Block, n Names) []Block {
	var out []Block
	for _, b := range bs {
		switch v := b.(type) {
		case blocks.RichTextBlock:
			out = append(out, FromRichText(v.Elements, n)...)
		case blocks.SectionBlock:
			out = append(out, FromMrkdwn(v.Text, n)...)
			if len(v.Fields) > 0 {
				f := Fields{}
				for _, field := range v.Fields {
					f.Items = append(f.Items, Field{Spans: Spans(field, n), Short: true})
				}
				out = append(out, f)
			}
			switch acc := v.Accessory.(type) {
			case blocks.ImageAccessory:
				out = append(out, Image{URL: acc.URL, Alt: acc.AltText, Open: acc.URL, Small: true})
			case blocks.LabelAccessory:
				if acc.Label != "" {
					out = append(out, Buttons{Labels: []string{acc.Label}})
				}
			}
		case blocks.HeaderBlock:
			if v.Text != "" {
				out = append(out, Heading{Spans: Spans(v.Text, n)})
			}
		case blocks.ContextBlock:
			c := Context{}
			for _, el := range v.Elements {
				if el.ImageURL != "" {
					c.Images = append(c.Images, Image{URL: el.ImageURL, Alt: el.AltText, Open: el.ImageURL, Small: true})
				}
				if el.Text != "" {
					if len(c.Items) > 0 {
						c.Items = append(c.Items, Span{Text: "  "})
					}
					c.Items = append(c.Items, Spans(el.Text, n)...)
				}
			}
			if len(c.Items) > 0 || len(c.Images) > 0 {
				out = append(out, c)
			}
		case blocks.DividerBlock:
			out = append(out, Divider{})
		case blocks.ImageBlock:
			out = append(out, Image{URL: v.URL, Title: v.Title, Alt: v.Alt, W: v.Width, H: v.Height, Open: v.URL})
		case blocks.ActionsBlock:
			var labels []string
			for _, el := range v.Elements {
				if el.Label != "" {
					labels = append(labels, el.Label)
				}
			}
			if len(labels) > 0 {
				out = append(out, Buttons{Labels: labels})
			}
		}
	}
	return out
}

// FromAttachments converts legacy attachments: each becomes its pretext
// followed by a colored Card.
func FromAttachments(atts []blocks.LegacyAttachment, n Names) []Block {
	var out []Block
	for _, a := range atts {
		out = append(out, FromMrkdwn(a.Pretext, n)...)
		card := Card{Color: a.Color}
		if a.AuthorName != "" {
			card.Blocks = append(card.Blocks, Paragraph{Spans: []Span{{Text: a.AuthorName, Kind: SpanMuted}}})
		}
		if a.Title != "" {
			card.Blocks = append(card.Blocks, Paragraph{Spans: []Span{{Text: a.Title, Bold: true, URL: a.TitleLink}}})
		}
		card.Blocks = append(card.Blocks, FromMrkdwn(a.Text, n)...)
		if len(a.Fields) > 0 {
			f := Fields{}
			for _, field := range a.Fields {
				var spans []Span
				if field.Title != "" {
					spans = append(spans, Span{Text: field.Title + "\n", Bold: true})
				}
				spans = append(spans, Spans(field.Value, n)...)
				f.Items = append(f.Items, Field{Spans: spans, Short: field.Short})
			}
			card.Blocks = append(card.Blocks, f)
		}
		card.Blocks = append(card.Blocks, FromBlocks(a.Blocks, n)...)
		switch {
		case a.ImageURL != "":
			card.Blocks = append(card.Blocks, Image{URL: a.ImageURL, Open: a.ImageURL})
		case a.ThumbURL != "":
			card.Blocks = append(card.Blocks, Image{URL: a.ThumbURL, Open: a.ThumbURL, Small: true})
		}
		if len(a.Actions) > 0 {
			var labels []string
			for _, act := range a.Actions {
				if act.Text != "" {
					labels = append(labels, act.Text)
				}
			}
			if len(labels) > 0 {
				card.Blocks = append(card.Blocks, Buttons{Labels: labels})
			}
		}
		card.Footer = a.Footer
		if a.TS > 0 {
			when := time.Unix(a.TS, 0).Local().Format("Jan 2, 2006 15:04")
			if card.Footer != "" {
				card.Footer += " · "
			}
			card.Footer += when
		}
		if len(card.Blocks) > 0 || card.Footer != "" {
			out = append(out, card)
		}
	}
	return out
}
