// Package rich is the display form of a Slack message: a list of blocks
// (paragraphs, quotes, lists, code, images, attachment cards) made of
// styled spans. It knows nothing about drawing, so the backend can build
// it and tests can check it without a display; the windows turn it into
// Gio widgets.
//
// Builders here take the typed data slack-go and internal/core/blocks
// already provide (rich_text elements, Block Kit blocks, legacy
// attachments, files) and fall back to internal/slack/mrkdwn for plain
// mrkdwn text.
package rich

import (
	"github.com/gammons/slk/internal/core"
	"strings"
)

// SpanKind says what a span stands for, beyond its text style.
type SpanKind int

const (
	SpanText      SpanKind = iota
	SpanMention            // @someone
	SpanChannel            // #channel
	SpanBroadcast          // @here, @channel, @everyone, @usergroup
	SpanEmoji              // a custom emoji with no glyph: ":name:"
	SpanMuted              // secondary text: attachment author, footer
)

// Span is a run of text with one style. URL is set on links. ID is the
// user or channel a mention stands for, so a frontend can show more
// about it.
type Span struct {
	Text                       string
	Bold, Italic, Strike, Code bool
	URL                        string
	ID                         string
	Kind                       SpanKind
}

// Block is one vertical piece of a message.
type Block interface{ isBlock() }

// Paragraph is running text. Spans may contain "\n" for line breaks.
type Paragraph struct{ Spans []Span }

// Heading is a Block Kit header: one line, larger and bold.
type Heading struct{ Spans []Span }

// Quote is indented text with a bar on the left.
type Quote struct{ Blocks []Block }

// CodeBlock is preformatted text.
type CodeBlock struct{ Text string }

// List is a bulleted or numbered list. Start is the first item's number
// (0-based, as Slack's offset); Indent is the nesting level.
type List struct {
	Ordered bool
	Start   int
	Indent  int
	Items   [][]Span
}

// Divider is a horizontal rule.
type Divider struct{}

// Thumb is one size of an uploaded image.
type Thumb = core.ThumbSpec

// Image is a picture: a Slack upload (FileID set, Thumbs to choose from)
// or any image URL (Block Kit image, unfurl). W and H are the original
// pixel size when known, so a placeholder can hold its shape. Open is
// what a click launches. Small marks thumbnails and context icons.
type Image struct {
	URL      string
	FileName string // an upload's file name, extension included
	Title    string
	Alt      string
	W, H     int
	FileID   string
	Thumbs   []Thumb
	Open     string
	Small    bool
}

// File is a non-image upload, shown as a chip. URL downloads it (with
// the workspace's credentials); Open is its page in Slack.
type File struct {
	Name     string // the title shown
	FileName string // the file name, extension included
	Mime     string
	Size     int64
	FileID   string
	URL      string
	Open     string
}

// Field is one cell of a Fields grid. Short fields share a row in pairs;
// others take the whole width.
type Field struct {
	Spans []Span
	Short bool
}

// Fields is a Block Kit section's fields or an attachment's fields.
type Fields struct{ Items []Field }

// Context is a Block Kit context block: small muted text and icons.
type Context struct {
	Items  []Span
	Images []Image
}

// Buttons are the labels of an actions block or attachment actions.
// They are drawn, not pressable.
type Buttons struct{ Labels []string }

// Card is a legacy attachment: a colored bar with content beside it.
// Color is Slack's "good", "warning", "danger" or a hex color, or empty.
type Card struct {
	Color  string
	Blocks []Block
	Footer string
}

func (Paragraph) isBlock() {}
func (Heading) isBlock()   {}
func (Quote) isBlock()     {}
func (CodeBlock) isBlock() {}
func (List) isBlock()      {}
func (Divider) isBlock()   {}
func (Image) isBlock()     {}
func (File) isBlock()      {}
func (Fields) isBlock()    {}
func (Context) isBlock()   {}
func (Buttons) isBlock()   {}
func (Card) isBlock()      {}

// Names resolves IDs to display names. A false second result means the
// ID is unknown and will be shown as is.
type Names interface {
	User(id string) (string, bool)
	Channel(id string) (string, bool)
	UserGroup(id string) (string, bool)
}

// MapNames is a Names over plain maps, for tests and the fake backend.
type MapNames struct {
	Users, Channels, UserGroups map[string]string
}

func (m MapNames) User(id string) (string, bool)      { v, ok := m.Users[id]; return v, ok }
func (m MapNames) Channel(id string) (string, bool)   { v, ok := m.Channels[id]; return v, ok }
func (m MapNames) UserGroup(id string) (string, bool) { v, ok := m.UserGroups[id]; return v, ok }

// Plain is the text of spans joined, for titles and tests.
func Plain(spans []Span) string {
	n := 0
	for _, s := range spans {
		n += len(s.Text)
	}
	b := make([]byte, 0, n)
	for _, s := range spans {
		b = append(b, s.Text...)
	}
	return string(b)
}

// mapSpans applies fn to every span in blocks, recursively.
func mapSpans(blocks []Block, fn func(*Span)) {
	for i, b := range blocks {
		switch v := b.(type) {
		case Paragraph:
			for j := range v.Spans {
				fn(&v.Spans[j])
			}
			blocks[i] = v
		case Heading:
			for j := range v.Spans {
				fn(&v.Spans[j])
			}
			blocks[i] = v
		case Quote:
			mapSpans(v.Blocks, fn)
		case List:
			for _, item := range v.Items {
				for j := range item {
					fn(&item[j])
				}
			}
		case Fields:
			for _, f := range v.Items {
				for j := range f.Spans {
					fn(&f.Spans[j])
				}
			}
		case Context:
			for j := range v.Items {
				fn(&v.Items[j])
			}
		case Card:
			mapSpans(v.Blocks, fn)
		}
	}
}

// Italicize sets every span in blocks italic, as Slack's clients draw a
// /me line (subtype me_message). It changes blocks in place.
func Italicize(blocks []Block) { mapSpans(blocks, func(s *Span) { s.Italic = true }) }

// plainBlocksLimit is where PlainBlocks stops: enough for a few lines of
// a preview, however long the message.
const plainBlocksLimit = 300

// PlainBlocks is a message body as plain text, blocks separated by
// newlines: for a search result or any one-line preview. Pictures and
// files appear as their names, dividers and buttons not at all. It
// stops once about plainBlocksLimit bytes are gathered.
func PlainBlocks(blocks []Block) string {
	var b strings.Builder
	var walk func([]Block)
	add := func(s string) {
		if s == "" || b.Len() > plainBlocksLimit {
			return
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s)
	}
	walk = func(blocks []Block) {
		for _, bl := range blocks {
			if b.Len() > plainBlocksLimit {
				return
			}
			switch v := bl.(type) {
			case Paragraph:
				add(Plain(v.Spans))
			case Heading:
				add(Plain(v.Spans))
			case Quote:
				walk(v.Blocks)
			case CodeBlock:
				add(v.Text)
			case List:
				for _, it := range v.Items {
					add(Plain(it))
				}
			case Image:
				add(firstOf(v.Title, v.FileName, v.Alt))
			case File:
				add(firstOf(v.Name, v.FileName))
			case Fields:
				for _, f := range v.Items {
					add(Plain(f.Spans))
				}
			case Context:
				add(Plain(v.Items))
			case Card:
				walk(v.Blocks)
			}
		}
	}
	walk(blocks)
	return b.String()
}

func firstOf(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
