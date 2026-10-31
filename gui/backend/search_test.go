package backend

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/cache"
)

// TestSearchFakeMatchesCache runs the same queries over the same
// messages through the fake's matcher and the cache's SQL (via
// searchSpec), which must agree on what they find.
func TestSearchFakeMatchesCache(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	type row struct {
		ch, user, text, thread string
		replies                int
		edited, file           bool
		emoji                  string
	}
	rows := []row{
		{ch: "C1", user: "U1", text: "deploy went fine", thread: "self", replies: 1, emoji: "tada"},
		{ch: "C1", user: "U2", text: "the build failed on staging", thread: "1"},
		{ch: "C2", user: "U1", text: "build failed again <@U2>", edited: true, emoji: "eyes"},
		{ch: "C2", user: "U2", text: "see <https://example.com|the docs> <!subteam^S1|@eng>"},
		{ch: "C1", user: "U2", text: "Café screenshot attached", file: true},
	}
	db, err := cache.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.UpsertWorkspace(cache.Workspace{ID: "T1", Name: "T"}); err != nil {
		t.Fatal(err)
	}
	var msgs []Message
	for i, r := range rows {
		at := base.Add(time.Duration(i+1) * time.Hour)
		ts := strconv.FormatInt(at.Unix(), 10) + ".000000"
		threadTS := ""
		switch r.thread {
		case "self":
			threadTS = ts
		case "1":
			threadTS = msgs[0].TS
		}
		m := Message{ChannelID: r.ch, TS: ts, AuthorID: r.user, Text: r.text, Body: rich.FromMrkdwn(r.text, fakeNames),
			Time: at, Edited: r.edited, ThreadTS: threadTS, ReplyCount: r.replies, InThreadOnly: r.thread == "1"}
		cm := cache.Message{TS: ts, ChannelID: r.ch, WorkspaceID: "T1", UserID: r.user, Text: r.text, ThreadTS: threadTS, ReplyCount: r.replies}
		if r.edited {
			cm.EditedAt = ts
		}
		if r.file {
			m.Body = append(m.Body, rich.File{Name: "shot.png"})
			cm.RawJSON = `{"files":[{"id":"F1"}]}`
		}
		if r.emoji != "" {
			m.Reactions = []Reaction{{Name: r.emoji, Count: 1, Users: []string{"U9"}}}
			if err := db.UpsertReaction(ts, r.ch, r.emoji, []string{"U9"}, 1); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.UpsertMessage(cm); err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, m)
	}

	for _, c := range []struct {
		name string
		q    SearchQuery
	}{
		{"a word", Words("build")},
		{"a prefix", Words("dep*")},
		{"a whole word", Words("deploy")},
		{"a phrase ending in a prefix", SearchQuery{All: [][]string{{"build fail*"}}}},
		{"accents", Words("cafe")},
		{"a phrase", SearchQuery{All: [][]string{{"build failed again"}}}},
		{"alternatives", SearchQuery{All: [][]string{{"deploy", "screenshot"}}}},
		{"excluded", SearchQuery{All: [][]string{{"build"}}, None: []string{"staging"}}},
		{"from", SearchQuery{Users: []string{"U1"}}},
		{"not in", SearchQuery{NotChannels: []string{"C1"}}},
		{"mentions", SearchQuery{Mentions: []string{"<@U2>", "<!subteam^S1"}}},
		{"since", SearchQuery{Since: base.Add(3 * time.Hour)}},
		{"until", SearchQuery{Until: base.Add(2 * time.Hour)}},
		{"thread", SearchQuery{Thread: Yes}},
		{"reply", SearchQuery{Reply: Yes}},
		{"not a reply", SearchQuery{All: [][]string{{"build"}}, Reply: No}},
		{"edited", SearchQuery{Edited: Yes}},
		{"link", SearchQuery{Link: Yes}},
		{"file", SearchQuery{File: Yes}},
		{"reaction", SearchQuery{Reaction: Yes}},
		{"emoji", SearchQuery{Emoji: []string{"tada"}}},
		{"not emoji", SearchQuery{Reaction: Yes, NotEmoji: []string{"tada"}}},
	} {
		var fake []string
		for _, m := range msgs {
			if _, ok := fakeMatches(c.q, m); ok {
				fake = append(fake, m.TS)
			}
		}
		rows, err := db.SearchMessagesSpec("T1", searchSpec(c.q), 50, cache.SearchNewest)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var real []string
		for _, r := range rows {
			real = append(real, r.TS)
		}
		slices.Sort(fake)
		slices.Sort(real)
		if len(real) == 0 {
			t.Errorf("%s: the cache found nothing; the case tests nothing", c.name)
		}
		if !slices.Equal(fake, real) {
			t.Errorf("%s: fake %v, cache %v", c.name, fake, real)
		}
	}
}

func TestTriMatchesCache(t *testing.T) {
	if cache.Tri(Any) != cache.Any || cache.Tri(Yes) != cache.Yes || cache.Tri(No) != cache.No {
		t.Fatal("backend.Tri and cache.Tri must convert one to one")
	}
}
