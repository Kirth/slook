package backend

import (
	"context"
	"errors"
	"slices"
	"strconv"
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

// nextPosted is the next MessagePosted, skipping the startup events
// (presence, usergroups).
func nextPosted(t *testing.T, got <-chan Event) MessagePosted {
	t.Helper()
	for ev := range got {
		switch ev := ev.(type) {
		case MessagePosted:
			return ev
		case PresenceChanged, UserGroupsChanged, Typing, ConversationOpened, ProfileChanged, ThreadsUnread, ThreadState, ThreadsChanged:
		default:
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

	hits, err := f.Search(ctx, Words("offsite"), 10, Newest)
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
	if hits, _ = f.Search(ctx, Words(""), 10, Newest); hits != nil {
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

func TestFakeSearchOrders(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	ctx := context.Background()
	f.mu.Lock()
	dense := f.appendLocked("C2", "Ada Lovelace", "queue queue queue")
	newer := f.appendLocked("C2", "Grace Hopper", "the queue is fine")
	f.mu.Unlock()
	best, _ := f.Search(ctx, Words("queue"), 50, Relevance)
	if len(best) == 0 || best[0].Message.TS != dense.TS {
		t.Fatalf("relevance: first is %+v, want the message saying it three times", best[0].Message)
	}
	recent, _ := f.Search(ctx, Words("queue"), 50, Newest)
	if recent[0].Message.TS != newer.TS {
		t.Fatalf("newest: first is %s, want %s", recent[0].Message.TS, newer.TS)
	}
}

func TestFakePeople(t *testing.T) {
	ps, err := NewFake(time.Unix(1_700_000_000, 0)).People(context.Background())
	if err != nil || len(ps) == 0 || ps[0].Name != "Ada Lovelace" {
		t.Fatalf("people %v, %v", ps, err)
	}
}

func TestFakeUserGroups(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	groups := f.UserGroups()
	if len(groups) != 2 || groups[1].Handle != "eng" || !groups[1].Mine || groups[0].Mine {
		t.Fatalf("groups %+v", groups)
	}
	p, err := f.Profile(context.Background(), "S1")
	if err != nil || p.Name != "Engineering" || p.Handle != "eng" || p.StatusText != "3 members, including you" {
		t.Fatalf("profile %+v, %v", p, err)
	}
	if name, ok := fakeNames.UserGroup("S2"); !ok || name != "design" {
		t.Fatalf("UserGroup(S2) = %q, %v", name, ok)
	}
}

func TestFakeTypesBeforeATimedPost(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Event, 16)
	go f.Run(ctx, func(ev Event) { got <- ev })
	var typing *Typing
	for ev := range got {
		switch ev := ev.(type) {
		case Typing:
			typing = &ev
		case MessagePosted:
			if typing == nil {
				t.Fatal("a timed post came without typing first")
			}
			if ev.Message.ChannelID != typing.ChannelID || ev.Message.AuthorID != typing.UserID {
				t.Fatalf("typing %+v, then a post by %s in %s", *typing, ev.Message.AuthorID, ev.Message.ChannelID)
			}
			return
		}
	}
}

func TestFakeReactKeepsSlacksName(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	feed := f.pageLocked("C1", "")
	ts := feed[0].TS
	if err := f.React(context.Background(), "C1", ts, "thumbsup", true); err != nil {
		t.Fatal(err)
	}
	if ev := (<-f.events).(ReactionChanged); ev.Name != "+1" {
		t.Fatalf("reacted as %q, want +1", ev.Name)
	}
}

func TestFakeThreads(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	ts, err := f.Threads(context.Background())
	if err != nil || len(ts) != 2 {
		t.Fatalf("threads %+v, %v", ts, err)
	}
	var unread *ThreadSummary
	for i := range ts {
		if ts[i].Unread {
			unread = &ts[i]
		}
	}
	if unread == nil || unread.Replies != 3 || unread.Author == "" {
		t.Fatalf("unread thread %+v", unread)
	}
	if err := f.MarkThreadRead(context.Background(), "C1", unread.ThreadTS, ""); err != nil {
		t.Fatal(err)
	}
	if ev := (<-f.events).(ThreadState); ev.ThreadTS != unread.ThreadTS || ev.Unread {
		t.Fatalf("state %+v", ev)
	}
	if ev := (<-f.events).(ThreadsUnread); ev.Count != 0 {
		t.Fatalf("count %+v", ev)
	}
}

func TestFakeSearchChannelsFindsUnjoined(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx := context.Background()

	res, err := f.SearchChannels(ctx, "SRE")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0] != (ChannelResult{ID: "C20", Name: "platform-sre", Kind: KindChannel}) {
		t.Fatalf("search sre = %#v, want platform-sre, not joined", res)
	}
	res, _ = f.SearchChannels(ctx, "design")
	var got []string
	for _, r := range res {
		got = append(got, r.Name+":"+strconv.FormatBool(r.Member))
	}
	if want := []string{"design-review:true", "design-crit:false"}; !slices.Equal(got, want) {
		t.Fatalf("search design = %v, want %v", got, want)
	}
	if res, err := f.SearchChannels(ctx, ""); res != nil || err != nil {
		t.Fatalf("empty query = %v, %v; want nothing", res, err)
	}
	if res, _ := f.SearchChannels(ctx, "ada"); len(res) != 0 {
		t.Fatalf("search ada = %v; DMs are not channels", res)
	}
}

func TestFakeJoinOpensTheChannel(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Event, 16)
	go f.Run(ctx, func(ev Event) { got <- ev })

	conv, err := f.Join(ctx, "C22")
	if err != nil {
		t.Fatal(err)
	}
	if conv.ID != "C22" || conv.Name != "cat-pictures" || conv.Kind != KindChannel {
		t.Fatalf("joined %#v", conv)
	}
	for ev := range got {
		if o, ok := ev.(ConversationOpened); ok {
			if o.Conv.ID != "C22" {
				t.Fatalf("opened %#v, want C22", o.Conv)
			}
			break
		}
	}
	convs, _ := f.Conversations(ctx)
	if !slices.ContainsFunc(convs, func(c Conversation) bool { return c.ID == "C22" }) {
		t.Fatal("C22 is not in the conversations after joining")
	}
	if hist, err := f.History(ctx, "C22"); err != nil || len(hist) == 0 {
		t.Fatalf("history after joining = %d messages, %v", len(hist), err)
	}
	if res, _ := f.SearchChannels(ctx, "cat"); len(res) != 1 || !res[0].Member {
		t.Fatalf("search after joining = %#v, want a member", res)
	}

	// Joining again is a no-op without an event; an unknown channel fails.
	if _, err := f.Join(ctx, "C22"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Join(ctx, "C99"); err == nil || err.Error() != "channel_not_found" {
		t.Fatalf("join C99 = %v, want channel_not_found", err)
	}
	if err := f.Send(ctx, "C1", "after"); err != nil {
		t.Fatal(err)
	}
	for ev := range got {
		if _, ok := ev.(ConversationOpened); ok {
			t.Fatalf("joining a channel the user is in reported %#v", ev)
		}
		if _, ok := ev.(MessagePosted); ok {
			break
		}
	}
}
