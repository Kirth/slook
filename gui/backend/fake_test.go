package backend

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/core"
)

func TestFakeSendComesBackThroughRun(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan Event, 4)
	go f.Run(ctx, func(ev Event) { got <- ev })
	if ev := <-got; ev != (ConnectionChanged{Connected: true}) {
		t.Fatalf("first event %#v, want connected", ev)
	}
	if err := f.Send(ctx, "C1", "hi :wave:"); err != nil {
		t.Fatal(err)
	}
	ev := nextPosted(t, got)
	if ev.Message.ChannelID != "C1" || ev.Message.Author != "You" || len(ev.Message.Body) != 1 {
		t.Fatalf("got %#v", ev)
	}
	if p, ok := ev.Message.Body[0].(rich.Paragraph); !ok || rich.Plain(p.Spans) != "hi 👋" {
		t.Fatalf("body %#v, want the text with the emoji resolved", ev.Message.Body[0])
	}

	hist, err := f.History(ctx, "C1")
	if err != nil {
		t.Fatal(err)
	}
	if last := hist[len(hist)-1]; last.TS != ev.Message.TS {
		t.Fatalf("history ends at %s, want the sent message %s", last.TS, ev.Message.TS)
	}
}

func TestSortConversationsChannelsFirstFolded(t *testing.T) {
	convs := []Conversation{
		{Name: "Zoë", Kind: KindDM},
		{Name: "random", Kind: KindChannel},
		{Name: "Ádmin", Kind: KindPrivate},
		{Name: "Bob", Kind: KindGroupDM},
	}
	sortConversations(convs)
	var got []string
	for _, c := range convs {
		got = append(got, c.Name)
	}
	want := []string{"Ádmin", "random", "Bob", "Zoë"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestFakeCommands(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Event, 4)
	go f.Run(ctx, func(ev Event) { got <- ev })
	<-got // connected

	if err := f.RunCommand(ctx, "C1", "", "/shrug", "fine"); err != nil {
		t.Fatal(err)
	}
	ev := nextPosted(t, got)
	if p, ok := ev.Message.Body[0].(rich.Paragraph); !ok || rich.Plain(p.Spans) != `fine ¯\_(ツ)_/¯` || ev.Message.Ephemeral {
		t.Fatalf("/shrug posted %#v", ev.Message)
	}

	if err := f.RunCommand(ctx, "C1", "", "/remind", "me later"); err != nil {
		t.Fatal(err)
	}
	ev = nextPosted(t, got)
	if !ev.Message.Ephemeral || ev.Message.Author != "Slackbot" {
		t.Fatalf("/remind replied %#v, want an ephemeral from Slackbot", ev.Message)
	}
	hist, _ := f.History(ctx, "C1")
	if hist[len(hist)-1].TS == ev.Message.TS {
		t.Fatal("the ephemeral reply was kept in history")
	}

	var ce *core.CommandError
	if err := f.RunCommand(ctx, "C1", "", "/nope", ""); !errors.As(err, &ce) || ce.Code != "unknown_command" {
		t.Fatalf("/nope err = %v, want unknown_command", err)
	}
}

func TestFakeMeIsItalic(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Event, 4)
	go f.Run(ctx, func(ev Event) { got <- ev })
	<-got // connected
	if err := f.RunCommand(ctx, "C1", "", "/me", "waves *hello*"); err != nil {
		t.Fatal(err)
	}
	m := nextPosted(t, got).Message
	p, ok := m.Body[0].(rich.Paragraph)
	if !ok || rich.Plain(p.Spans) != "waves hello" {
		t.Fatalf("body %#v", m.Body)
	}
	for _, s := range p.Spans {
		if !s.Italic {
			t.Fatalf("span %q is not italic", s.Text)
		}
	}
}

// nextPosted is the next MessagePosted, skipping presence updates.
func nextPosted(t *testing.T, got <-chan Event) MessagePosted {
	t.Helper()
	for ev := range got {
		if m, ok := ev.(MessagePosted); ok {
			return m
		}
		if _, ok := ev.(PresenceChanged); !ok {
			t.Fatalf("got %#v, want a message", ev)
		}
	}
	t.Fatal("events ended")
	return MessagePosted{}
}

func TestFakeSearchAndBackfillProgress(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Event, 64)
	go f.Run(ctx, func(ev Event) { got <- ev })
	<-got // connected

	hits, err := f.Search(ctx, "offsite", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Message.ChannelID != "C1" {
		t.Fatalf("search for the attachments message: %+v", hits)
	}
	for i := 1; i < len(hits); i++ {
		if hits[i].Message.Time.After(hits[i-1].Message.Time) {
			t.Fatal("hits are not newest first")
		}
	}
	if hits, _ = f.Search(ctx, "", 10); hits != nil {
		t.Fatal("blank query returned hits")
	}

	st, err := f.SyncStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Messages == 0 || len(st.Conversations) == 0 || !st.Conversations[0].Complete || st.Backfill.State != BackfillOff {
		t.Fatalf("status = %+v", st)
	}

	if err := f.StartBackfill(90 * 24 * time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := f.StartBackfill(0); err == nil {
		t.Fatal("a second start was accepted")
	}
	deadline := time.After(5 * time.Second)
	progressed := false
	for !progressed {
		select {
		case ev := <-got:
			if _, ok := ev.(SyncChanged); !ok {
				continue
			}
			st, _ := f.SyncStatus(ctx)
			if st.Backfill.State == BackfillRunning && st.Backfill.Requests > 0 && len(st.Active) == 1 {
				progressed = true
			}
		case <-deadline:
			t.Fatal("no progress reported")
		}
	}
	f.StopBackfill()
	deadline = time.After(5 * time.Second)
	for {
		select {
		case ev := <-got:
			if _, ok := ev.(SyncChanged); !ok {
				continue
			}
			if st, _ := f.SyncStatus(ctx); st.Backfill.State == BackfillOff && len(st.Active) == 0 {
				return
			}
		case <-deadline:
			t.Fatal("stop was not reported")
		}
	}
}

func TestFakeOlderPagesBackToTheStart(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	ctx := context.Background()
	page, err := f.History(ctx, "C1")
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != fakePage {
		t.Fatalf("history is %d messages, want a page of %d", len(page), fakePage)
	}
	seen := map[string]bool{}
	total := 0
	for pages := 0; len(page) > 0; pages++ {
		if pages > 10 {
			t.Fatal("paging back did not end")
		}
		for i, m := range page {
			if m.InThreadOnly {
				t.Fatalf("a page holds a reply only in its thread: %+v", m)
			}
			if seen[m.TS] {
				t.Fatalf("%s is in two pages", m.TS)
			}
			if i > 0 && page[i-1].TS >= m.TS {
				t.Fatal("a page is not oldest first")
			}
			seen[m.TS] = true
		}
		total += len(page)
		if page, err = f.Older(ctx, "C1", page[0].TS); err != nil {
			t.Fatal(err)
		}
	}
	f.mu.Lock()
	want := 0
	for _, m := range f.logs["C1"] {
		if !m.InThreadOnly {
			want++
		}
	}
	f.mu.Unlock()
	if total != want {
		t.Fatalf("paged through %d messages, want all %d", total, want)
	}
}

func TestFakeReactionsSeededAndToggled(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Event, 8)
	go f.Run(ctx, func(ev Event) { got <- ev })
	<-got // connected

	hist, _ := f.History(ctx, "C1")
	var mine *Message
	custom := false
	for i, m := range hist {
		for _, r := range m.Reactions {
			if r.Name == "+1" && r.Mine && r.Count == 3 {
				mine = &hist[i]
			}
			if r.Name == "partyparrot" {
				custom = true
			}
		}
	}
	if mine == nil || !custom {
		t.Fatalf("seeded reactions missing (mine %v, custom %v)", mine != nil, custom)
	}

	if err := f.React(ctx, "C1", mine.TS, "+1", false); err != nil {
		t.Fatal(err)
	}
	var ev ReactionChanged
	for e := range got {
		if r, ok := e.(ReactionChanged); ok {
			ev = r
			break
		}
	}
	if ev.TS != mine.TS || ev.Name != "+1" || ev.Added || !ev.Self {
		t.Fatalf("got %+v", ev)
	}
	if mine.Reactions[0].Count != 3 {
		t.Fatal("the window's copy changed under it")
	}
	hist, _ = f.History(ctx, "C1")
	for _, m := range hist {
		if m.TS == mine.TS && (m.Reactions[0].Count != 2 || m.Reactions[0].Mine) {
			t.Fatalf("after removing: %+v", m.Reactions)
		}
	}
}
