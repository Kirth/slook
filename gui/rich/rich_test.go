package rich

import (
	"reflect"
	"strings"
	"testing"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/core/blocks"
)

var names = MapNames{
	Users:      map[string]string{"U1": "ada"},
	Channels:   map[string]string{"C1": "general"},
	UserGroups: map[string]string{"S1": "backend"},
}

func style(b, i, s, c bool) *slack.RichTextSectionTextStyle {
	return &slack.RichTextSectionTextStyle{Bold: b, Italic: i, Strike: s, Code: c}
}

func TestFromRichTextStylesAndEntities(t *testing.T) {
	els := []slack.RichTextElement{
		&slack.RichTextSection{Elements: []slack.RichTextSectionElement{
			slack.NewRichTextSectionTextElement("hi ", nil),
			slack.NewRichTextSectionTextElement("bold", style(true, false, false, false)),
			slack.NewRichTextSectionTextElement(" ", nil),
			slack.NewRichTextSectionUserElement("U1", nil),
			slack.NewRichTextSectionTextElement(" ", nil),
			slack.NewRichTextSectionUserElement("U9", nil),
			slack.NewRichTextSectionTextElement(" ", nil),
			slack.NewRichTextSectionChannelElement("C1", nil),
			slack.NewRichTextSectionTextElement(" ", nil),
			slack.NewRichTextSectionUserGroupElement("S1"),
			slack.NewRichTextSectionTextElement(" ", nil),
			slack.NewRichTextSectionBroadcastElement("here"),
			slack.NewRichTextSectionTextElement(" ", nil),
			slack.NewRichTextSectionLinkElement("https://x.com", "X", style(false, true, false, false)),
			slack.NewRichTextSectionTextElement(" ", nil),
			slack.NewRichTextSectionLinkElement("https://y.com", "", nil),
			slack.NewRichTextSectionTextElement(" <&>\n", nil),
		}},
	}
	got := FromRichText(els, names)
	want := []Block{Paragraph{Spans: []Span{
		{Text: "hi "},
		{Text: "bold", Bold: true},
		{Text: " "},
		{Text: "@ada", ID: "U1", Kind: SpanMention},
		{Text: " "},
		{Text: "@U9", ID: "U9", Kind: SpanMention},
		{Text: " "},
		{Text: "#general", ID: "C1", Kind: SpanChannel},
		{Text: " "},
		{Text: "@backend", ID: "S1", Kind: SpanBroadcast},
		{Text: " "},
		{Text: "@here", Kind: SpanBroadcast},
		{Text: " "},
		{Text: "X", Italic: true, URL: "https://x.com"},
		{Text: " "},
		{Text: "https://y.com", URL: "https://y.com"},
		{Text: " <&>"}, // rich_text is not entity-escaped; trailing newline dropped
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%#v\nwant\n%#v", got, want)
	}
}

func TestFromRichTextBlocks(t *testing.T) {
	fallback := "yesterday"
	els := []slack.RichTextElement{
		&slack.RichTextList{Style: slack.RTEListOrdered, Offset: 2, Indent: 1, Elements: []slack.RichTextElement{
			&slack.RichTextSection{Elements: []slack.RichTextSectionElement{slack.NewRichTextSectionTextElement("three", nil)}},
			&slack.RichTextSection{Elements: []slack.RichTextSectionElement{slack.NewRichTextSectionTextElement("four", nil)}},
		}},
		&slack.RichTextPreformatted{Elements: []slack.RichTextSectionElement{
			slack.NewRichTextSectionTextElement("x := 1\n", nil),
			slack.NewRichTextSectionLinkElement("https://go.dev", "", nil),
		}},
		&slack.RichTextQuote{Elements: []slack.RichTextSectionElement{
			slack.NewRichTextSectionTextElement("said", style(false, false, true, false)),
		}},
		&slack.RichTextSection{Elements: []slack.RichTextSectionElement{
			&slack.RichTextSectionEmojiElement{Name: "thumbsup", Unicode: "1f44d-1f3fd"},
			&slack.RichTextSectionEmojiElement{Name: "rocket"},
			&slack.RichTextSectionEmojiElement{Name: "partyparrot"},
			&slack.RichTextSectionDateElement{Timestamp: 1, Fallback: &fallback},
		}},
	}
	got := FromRichText(els, names)
	want := []Block{
		List{Ordered: true, Start: 2, Indent: 1, Items: [][]Span{{{Text: "three"}}, {{Text: "four"}}}},
		CodeBlock{Text: "x := 1\nhttps://go.dev"},
		Quote{Blocks: []Block{Paragraph{Spans: []Span{{Text: "said", Strike: true}}}}},
		Paragraph{Spans: []Span{
			{Text: "👍🏽"},
			{Text: "🚀"},
			{Text: ":partyparrot:", Kind: SpanEmoji},
			{Text: "yesterday"},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%#v\nwant\n%#v", got, want)
	}
}

func TestUserIDs(t *testing.T) {
	els := []slack.RichTextElement{
		&slack.RichTextSection{Elements: []slack.RichTextSectionElement{slack.NewRichTextSectionUserElement("U1", nil)}},
		&slack.RichTextQuote{Elements: []slack.RichTextSectionElement{slack.NewRichTextSectionUserElement("U2", nil)}},
		&slack.RichTextList{Elements: []slack.RichTextElement{
			&slack.RichTextSection{Elements: []slack.RichTextSectionElement{slack.NewRichTextSectionUserElement("U3", nil)}},
		}},
	}
	if got := UserIDs(els); !reflect.DeepEqual(got, []string{"U1", "U2", "U3"}) {
		t.Fatalf("got %v", got)
	}
}

func TestFromMrkdwn(t *testing.T) {
	got := FromMrkdwn("a *b* <@U1> &lt;@U1&gt; &amp;\n&gt; quoted\n&gt; _two_\nafter ~gone~", names)
	want := []Block{
		Paragraph{Spans: []Span{{Text: "a "}, {Text: "b", Bold: true}, {Text: " "}, {Text: "@ada", ID: "U1", Kind: SpanMention}, {Text: " <@U1> &"}}},
		Quote{Blocks: []Block{Paragraph{Spans: []Span{{Text: "quoted\n"}, {Text: "two", Italic: true}}}}},
		Paragraph{Spans: []Span{{Text: "after "}, {Text: "gone", Strike: true}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%#v\nwant\n%#v", got, want)
	}
}

func TestFromMrkdwnQuoteRestAndCode(t *testing.T) {
	got := FromMrkdwn("```\n&gt; not a quote\n```\n&gt;&gt;&gt; all\nof it", names)
	want := []Block{
		CodeBlock{Text: "> not a quote"},
		Quote{Blocks: []Block{Paragraph{Spans: []Span{{Text: "all\nof it"}}}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%#v\nwant\n%#v", got, want)
	}
}

func TestBodyPrecedence(t *testing.T) {
	rt := blocks.RichTextBlock{Elements: []slack.RichTextElement{
		&slack.RichTextSection{Elements: []slack.RichTextSectionElement{slack.NewRichTextSectionTextElement("rich", nil)}},
	}}
	// rich_text wins over text.
	got := Body("fallback", []blocks.Block{rt}, names)
	if want := []Block{Paragraph{Spans: []Span{{Text: "rich"}}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rich: got %#v", got)
	}
	// A section carries the body, so the text is hidden.
	got = Body("fallback", []blocks.Block{blocks.SectionBlock{Text: "sec"}}, names)
	if want := []Block{Paragraph{Spans: []Span{{Text: "sec"}}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("section: got %#v", got)
	}
	// Nothing carries the body: text, then the blocks.
	got = Body("text", []blocks.Block{blocks.DividerBlock{}}, names)
	if want := []Block{Paragraph{Spans: []Span{{Text: "text"}}}, Divider{}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("plain: got %#v", got)
	}
}

func TestFromBlocks(t *testing.T) {
	bs := []blocks.Block{
		blocks.HeaderBlock{Text: "Head"},
		blocks.SectionBlock{Text: "*s*", Fields: []string{"a", "b"}, Accessory: blocks.ImageAccessory{URL: "https://i/x.png", AltText: "x"}},
		blocks.ContextBlock{Elements: []blocks.ContextElement{{ImageURL: "https://i/c.png", AltText: "c"}, {Text: "ctx"}}},
		blocks.DividerBlock{},
		blocks.ImageBlock{URL: "https://i/big.png", Title: "Big", Alt: "big", Width: 800, Height: 600},
		blocks.ActionsBlock{Elements: []blocks.ActionElement{{Kind: "button", Label: "OK"}, {Kind: "button", Label: "No"}}},
	}
	got := FromBlocks(bs, names)
	want := []Block{
		Heading{Spans: []Span{{Text: "Head"}}},
		Paragraph{Spans: []Span{{Text: "s", Bold: true}}},
		Fields{Items: []Field{{Spans: []Span{{Text: "a"}}, Short: true}, {Spans: []Span{{Text: "b"}}, Short: true}}},
		Image{URL: "https://i/x.png", Alt: "x", Open: "https://i/x.png", Small: true},
		Context{Items: []Span{{Text: "ctx"}}, Images: []Image{{URL: "https://i/c.png", Alt: "c", Open: "https://i/c.png", Small: true}}},
		Divider{},
		Image{URL: "https://i/big.png", Title: "Big", Alt: "big", W: 800, H: 600, Open: "https://i/big.png"},
		Buttons{Labels: []string{"OK", "No"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%#v\nwant\n%#v", got, want)
	}
}

func TestFromAttachments(t *testing.T) {
	atts := []blocks.LegacyAttachment{{
		Color:      "good",
		Pretext:    "pre",
		AuthorName: "CI",
		Title:      "Build 12",
		TitleLink:  "https://ci/12",
		Text:       "passed",
		Fields:     []blocks.LegacyField{{Title: "Branch", Value: "main", Short: true}},
		ImageURL:   "https://ci/12.png",
		Footer:     "ci.example",
		Actions:    []blocks.LegacyAction{{Text: "Rerun"}},
	}}
	got := FromAttachments(atts, names)
	if len(got) != 2 {
		t.Fatalf("want pretext and card, got %#v", got)
	}
	if want := (Paragraph{Spans: []Span{{Text: "pre"}}}); !reflect.DeepEqual(got[0], want) {
		t.Fatalf("pretext: got %#v", got[0])
	}
	card, ok := got[1].(Card)
	if !ok || card.Color != "good" || card.Footer != "ci.example" {
		t.Fatalf("card: got %#v", got[1])
	}
	wantBlocks := []Block{
		Paragraph{Spans: []Span{{Text: "CI", Kind: SpanMuted}}},
		Paragraph{Spans: []Span{{Text: "Build 12", Bold: true, URL: "https://ci/12"}}},
		Paragraph{Spans: []Span{{Text: "passed"}}},
		Fields{Items: []Field{{Spans: []Span{{Text: "Branch\n", Bold: true}, {Text: "main"}}, Short: true}}},
		Image{URL: "https://ci/12.png", Open: "https://ci/12.png"},
		Buttons{Labels: []string{"Rerun"}},
	}
	if !reflect.DeepEqual(card.Blocks, wantBlocks) {
		t.Fatalf("card blocks\n%#v\nwant\n%#v", card.Blocks, wantBlocks)
	}
}

func TestFromFiles(t *testing.T) {
	files := []slack.File{
		{ID: "F1", Name: "cat.png", Title: "Cat", Mimetype: "image/png", URLPrivate: "https://files/cat.png", Permalink: "https://slack/F1",
			OriginalW: 1200, OriginalH: 900, Thumb360: "https://files/cat_360.png", Thumb360W: 360, Thumb360H: 270},
		{ID: "F2", Name: "notes.pdf", Mimetype: "application/pdf", Size: 2048, Permalink: "https://slack/F2",
			URLPrivate: "https://files/notes.pdf", URLPrivateDownload: "https://files/download/notes.pdf"},
	}
	got := FromFiles(files)
	want := []Block{
		Image{URL: "https://files/cat.png", FileName: "cat.png", Title: "Cat", Alt: "Cat", W: 1200, H: 900, FileID: "F1",
			Thumbs: []Thumb{{URL: "https://files/cat_360.png", W: 360, H: 270}}, Open: "https://slack/F1"},
		File{Name: "notes.pdf", FileName: "notes.pdf", Mime: "application/pdf", Size: 2048, FileID: "F2", URL: "https://files/download/notes.pdf", Open: "https://slack/F2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%#v\nwant\n%#v", got, want)
	}
}

func TestSpansFlattens(t *testing.T) {
	got := Spans("one\n\ntwo *b*", names)
	want := []Span{{Text: "one"}, {Text: "\n"}, {Text: "two "}, {Text: "b", Bold: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestFromMrkdwnKeepsBackslashes(t *testing.T) {
	got := FromMrkdwn("¯\\_(ツ)_/¯ in C:\\tmp\n```\na\\nb\n```", names)
	want := []Block{
		Paragraph{Spans: []Span{{Text: "¯\\_(ツ)_/¯ in C:\\tmp"}}},
		CodeBlock{Text: "a\\nb"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%#v\nwant\n%#v", got, want)
	}
}

func TestFromMrkdwnDecodesEntitiesOnce(t *testing.T) {
	// "&amp;lt;" on the wire is a typed "&lt;", which must not become "<".
	got := FromMrkdwn("&gt; quoted &amp;lt;b&amp;gt; \\o/", names)
	want := []Block{Quote{Blocks: []Block{Paragraph{Spans: []Span{{Text: "quoted &lt;b&gt; \\o/"}}}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%#v\nwant\n%#v", got, want)
	}
}

func TestItalicize(t *testing.T) {
	bs := FromMrkdwn("waves at *everyone*\n&gt; quoted", names)
	Italicize(bs)
	want := []Block{
		Paragraph{Spans: []Span{{Text: "waves at ", Italic: true}, {Text: "everyone", Bold: true, Italic: true}}},
		Quote{Blocks: []Block{Paragraph{Spans: []Span{{Text: "quoted", Italic: true}}}}},
	}
	if !reflect.DeepEqual(bs, want) {
		t.Fatalf("got\n%#v\nwant\n%#v", bs, want)
	}
}

func TestEmojiGlyph(t *testing.T) {
	for in, want := range map[string]string{
		"leaves":                "🍃",
		":robot_face:":          "🤖",
		"wave::skin-tone-3":     "👋🏼",
		":+1::skin-tone-6:":     "👍🏿",
		"thumbsup::skin-tone-9": "👍", // not a tone: the base glyph
	} {
		if got, ok := EmojiGlyph(in); !ok || got != want {
			t.Errorf("EmojiGlyph(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"salesforce_logo_custom", "", "::"} {
		if got, ok := EmojiGlyph(in); ok {
			t.Errorf("EmojiGlyph(%q) = %q, want none", in, got)
		}
	}
}

func TestPlainBlocks(t *testing.T) {
	blocks := []Block{
		Paragraph{Spans: []Span{{Text: "hello "}, {Text: "world", Bold: true}}},
		Divider{},
		Quote{Blocks: []Block{Paragraph{Spans: []Span{{Text: "quoted"}}}}},
		CodeBlock{Text: "x := 1"},
		List{Items: [][]Span{{{Text: "one"}}, {{Text: "two"}}}},
		Image{FileName: "sunset.png", Title: "Sunset"},
		File{Name: "Notes", FileName: "notes.txt"},
		Buttons{Labels: []string{"Go"}},
		Card{Blocks: []Block{Paragraph{Spans: []Span{{Text: "card"}}}}},
	}
	want := "hello world\nquoted\nx := 1\none\ntwo\nSunset\nNotes\ncard"
	if got := PlainBlocks(blocks); got != want {
		t.Fatalf("PlainBlocks = %q, want %q", got, want)
	}
	long := []Block{Paragraph{Spans: []Span{{Text: strings.Repeat("a", 400)}}}, Paragraph{Spans: []Span{{Text: "tail"}}}}
	if got := PlainBlocks(long); strings.HasSuffix(got, "tail") {
		t.Fatal("PlainBlocks did not stop after the limit")
	}
	if PlainBlocks(nil) != "" {
		t.Fatal("PlainBlocks(nil)")
	}
}
