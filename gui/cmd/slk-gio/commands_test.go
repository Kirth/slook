package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/rich"
)

func TestFindConversation(t *testing.T) {
	convs := []backend.Conversation{
		{ID: "C1", Name: "general", Kind: backend.KindChannel},
		{ID: "C2", Name: "general-chat", Kind: backend.KindChannel},
		{ID: "D1", Name: "Ada Lovelace", Kind: backend.KindDM},
		{ID: "D2", Name: "Zoë", Kind: backend.KindDM},
	}
	cases := []struct {
		target string
		dmOnly bool
		id     string
		rest   string
		ok     bool
	}{
		{"#general", false, "C1", "", true},
		{"general-chat", false, "C2", "", true}, // longest name wins
		{"GENERAL", false, "C1", "", true},
		{"@Ada Lovelace hi there", true, "D1", " hi there", true},
		{"ada lovelace", true, "D1", "", true},
		{"zoe hello", true, "D2", " hello", true}, // accent-blind
		{"general", true, "", "", false},          // not a DM
		{"gen", false, "", "", false},             // a prefix of a name is not a match
		{"nobody", false, "", "", false},
	}
	for _, tc := range cases {
		conv, rest, ok := findConversation(convs, tc.target, tc.dmOnly)
		if ok != tc.ok || conv.ID != tc.id || rest != tc.rest {
			t.Errorf("findConversation(%q, %v) = (%s, %q, %v), want (%s, %q, %v)",
				tc.target, tc.dmOnly, conv.ID, rest, ok, tc.id, tc.rest, tc.ok)
		}
	}
}

func TestSwitcherMatches(t *testing.T) {
	now := time.Now()
	convs := []backend.Conversation{
		{ID: "C1", Name: "general", Latest: now.Add(-time.Minute)},
		{ID: "C2", Name: "random", Latest: now.Add(-time.Hour), Unread: true},
		{ID: "C3", Name: "eng", Latest: now.Add(-2 * time.Hour), Mentions: 1},
		{ID: "C4", Name: "alerts", Latest: now, Unread: true, Muted: true},
	}
	ids := func(cs []backend.Conversation) (out []string) {
		for _, c := range cs {
			out = append(out, c.ID)
		}
		return out
	}
	// No filter: what needs reading first (muted unread does not), then
	// by activity; the current conversation is left out.
	if got, want := ids(switcherMatches(convs, "", "C1")), []string{"C2", "C3", "C4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("empty filter: got %v, want %v", got, want)
	}
	if got, want := ids(switcherMatches(convs, "r", "")), []string{"C2", "C4", "C1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filter r: got %v, want %v", got, want)
	}
}

func TestAttachmentsInDrawingOrder(t *testing.T) {
	blocks := []rich.Block{
		rich.Paragraph{Spans: []rich.Span{{Text: "look"}}},
		rich.Image{FileID: "F1", URL: "https://files/a.png", FileName: "a.png", Title: "A"},
		rich.Card{Blocks: []rich.Block{
			rich.Context{Images: []rich.Image{{URL: "https://cdn/icon.png"}}},
			rich.Image{URL: "https://cdn/unfurl.jpg"},
		}},
		rich.File{FileID: "F2", URL: "https://files/b.pdf", Name: "Report", FileName: "b.pdf"},
		rich.Image{FileID: "F3", URL: "https://files/c", Title: "Screenshot"}, // no file name: the title
	}
	got := attachments(blocks)
	want := []backend.Attachment{
		{Key: "F1", URL: "https://files/a.png", Name: "a.png"},
		{URL: "https://cdn/icon.png"},
		{URL: "https://cdn/unfurl.jpg"},
		{Key: "F2", URL: "https://files/b.pdf", Name: "b.pdf"},
		{Key: "F3", URL: "https://files/c", Name: "Screenshot"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%+v\nwant\n%+v", got, want)
	}
}

func TestIsShortcode(t *testing.T) {
	for in, want := range map[string]bool{
		"rocket": true, ":robot_face:": true, "wave::skin-tone-2": true, "+1": true, "salesforce-logo": true,
		"★": false, "🚀": false, "": false,
	} {
		if got := isShortcode(in); got != want {
			t.Errorf("isShortcode(%q) = %v, want %v", in, got, want)
		}
	}
}
