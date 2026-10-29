package sidebar

import (
	"reflect"
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
)

var now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) time.Time { return now.Add(-d) }

const day = 24 * time.Hour

var sections = []backend.Section{
	{ID: "S0", Name: "Starred", Type: "stars"},
	{ID: "S1", Name: "Projects", Emoji: "🚀", Type: "standard"},
	{ID: "S2", Name: "Channels", Type: "channels"},
	{ID: "S3", Name: "Direct messages", Type: "direct_messages"},
}

// layout is a group as names: title, shown rows, quiet rows.
type layout struct {
	Title string
	Rows  []string
	Quiet []string
}

func summarize(gs []Group) []layout {
	var out []layout
	for _, g := range gs {
		l := layout{Title: g.Title}
		for _, c := range g.Rows {
			l.Rows = append(l.Rows, c.Name)
		}
		for _, c := range g.Quiet {
			l.Quiet = append(l.Quiet, c.Name)
		}
		out = append(out, l)
	}
	return out
}

func TestArrange(t *testing.T) {
	convs := []backend.Conversation{
		{ID: "C1", Name: "general", Kind: backend.KindChannel, Latest: ago(time.Hour)},
		{ID: "C2", Name: "book-club", Kind: backend.KindChannel, Latest: ago(90 * day)},
		{ID: "C3", Name: "offsite", Kind: backend.KindChannel, Latest: ago(2 * day), Section: "S1"},                  // seasonal, awake now
		{ID: "C4", Name: "hackathon", Kind: backend.KindChannel, Latest: ago(300 * day), Section: "S1"},              // seasonal, asleep
		{ID: "C5", Name: "incidents", Kind: backend.KindChannel, Latest: ago(time.Minute), Unread: true},             // unread
		{ID: "C6", Name: "alerts", Kind: backend.KindChannel, Latest: ago(3 * time.Hour), Unread: true, Muted: true}, // muted: stays put
		{ID: "C7", Name: "oncall", Kind: backend.KindChannel, Latest: ago(2 * time.Hour), Unread: true, Muted: true, Mentions: 1},
		{ID: "C8", Name: "eng", Kind: backend.KindPrivate, Latest: ago(200 * day), Starred: true},                // starred: never folded
		{ID: "C9", Name: "archive-ish", Kind: backend.KindChannel, Latest: ago(60 * day), Visited: ago(3 * day)}, // a recent visit keeps it awake
		{ID: "C10", Name: "zebra", Kind: backend.KindChannel, Latest: ago(100 * day), Muted: true},
		{ID: "D1", Name: "Ada", Kind: backend.KindDM, Latest: ago(30 * time.Minute)},
		{ID: "D2", Name: "Grace", Kind: backend.KindDM, Latest: ago(50 * day)},
	}
	got := summarize(Arrange(convs, sections, now, QuietAfter))
	want := []layout{
		{Title: "Unread", Rows: []string{"oncall", "incidents"}}, // mentions first
		{Title: "Starred", Rows: []string{"eng"}},
		{Title: "Projects", Rows: []string{"offsite"}, Quiet: []string{"hackathon"}},
		{Title: "Channels", Rows: []string{"general", "alerts", "archive-ish"}, Quiet: []string{"book-club", "zebra"}}, // muted quiet ones last
		{Title: "Direct messages", Rows: []string{"Ada"}, Quiet: []string{"Grace"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%+v\nwant\n%+v", got, want)
	}
}

func TestArrangeWithoutSections(t *testing.T) {
	convs := []backend.Conversation{
		{ID: "D1", Name: "Ada", Kind: backend.KindDM, Latest: ago(time.Hour)},
		{ID: "G1", Name: "Ada, Grace", Kind: backend.KindGroupDM, Latest: ago(40 * day)},
		{ID: "C1", Name: "general", Kind: backend.KindChannel, Latest: ago(time.Hour), Section: "gone"},
	}
	got := summarize(Arrange(convs, nil, now, QuietAfter))
	want := []layout{
		{Title: "Channels", Rows: []string{"general"}}, // an unknown section falls back by kind
		{Title: "Direct messages", Rows: []string{"Ada"}, Quiet: []string{"Ada, Grace"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%+v\nwant\n%+v", got, want)
	}
}

func TestArrangeOmitsEmptyGroups(t *testing.T) {
	convs := []backend.Conversation{{ID: "C1", Name: "general", Kind: backend.KindChannel, Latest: ago(time.Hour)}}
	got := summarize(Arrange(convs, sections, now, QuietAfter))
	want := []layout{{Title: "Channels", Rows: []string{"general"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestSearch(t *testing.T) {
	convs := []backend.Conversation{
		{ID: "C1", Name: "design-review", Latest: ago(40 * day)},
		{ID: "C2", Name: "web-design", Latest: ago(time.Hour)},
		{ID: "C3", Name: "Design", Latest: ago(2 * day)},
		{ID: "C4", Name: "general", Latest: ago(time.Minute)},
	}
	var got []string
	for _, c := range Search(convs, "#désign") {
		got = append(got, c.Name)
	}
	// Prefix matches before inner ones; within each, most recent first.
	want := []string{"Design", "design-review", "web-design"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
