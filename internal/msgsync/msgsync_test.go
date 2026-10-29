package msgsync

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
)

// fakeSource is a channel's history as Slack serves it: pages of 28,
// newest first, anchored by latest/oldest inclusive, with versions
// (here "v:"+text) answering cached_latest_updates.
type fakeSource struct {
	mu      sync.Mutex
	feed    map[string][]slack.Message // channel feed, ascending
	threads map[string][]slack.Message // "ch/ts" -> parent then replies
	calls   []slackclient.HistoryOpts
	replies []string
	// limitOnce makes the next history call fail with a rate limit.
	limitOnce bool
}

func newFake() *fakeSource {
	return &fakeSource{feed: map[string][]slack.Message{}, threads: map[string][]slack.Message{}}
}

func ts(n int) string { return fmt.Sprintf("%d.000000", 1700000000+n) }

func fm(n int, text string) slack.Message {
	return slack.Message{Msg: slack.Msg{Type: "message", Timestamp: ts(n), User: "U1", Text: text}}
}

func version(m slack.Message) string { return "v:" + m.Text }

func (f *fakeSource) post(ch string, ms ...slack.Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.feed[ch] = append(f.feed[ch], ms...)
	slices.SortFunc(f.feed[ch], func(a, b slack.Message) int { return strings.Compare(a.Timestamp, b.Timestamp) })
}

func (f *fakeSource) remove(ch, ts string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.feed[ch] = slices.DeleteFunc(f.feed[ch], func(m slack.Message) bool { return m.Timestamp == ts })
}

func (f *fakeSource) HistoryWithVersions(_ context.Context, ch string, opts slackclient.HistoryOpts) (slackclient.HistoryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, opts)
	if f.limitOnce {
		f.limitOnce = false
		return slackclient.HistoryResult{}, &slack.RateLimitedError{RetryAfter: time.Millisecond}
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 28
	}
	var window []slack.Message
	for _, m := range f.feed[ch] {
		if opts.Latest != "" && strings.Compare(m.Timestamp, opts.Latest) > 0 {
			continue
		}
		if opts.Oldest != "" && strings.Compare(m.Timestamp, opts.Oldest) < 0 {
			continue
		}
		window = append(window, m)
	}
	slices.Reverse(window)
	res := slackclient.HistoryResult{LatestUpdates: map[string]string{}}
	if len(window) > limit {
		window = window[:limit]
		res.HasMore = true
	}
	for _, m := range window {
		v := version(m)
		res.LatestUpdates[m.Timestamp] = v
		if opts.CachedVersions[m.Timestamp] == v {
			res.UnchangedTS = append(res.UnchangedTS, m.Timestamp)
			continue
		}
		res.Messages = append(res.Messages, m)
	}
	return res, nil
}

func (f *fakeSource) GetReplies(_ context.Context, ch, threadTS string) ([]slack.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies = append(f.replies, ch+"/"+threadTS)
	msgs, ok := f.threads[ch+"/"+threadTS]
	if !ok {
		return nil, fmt.Errorf("no thread %s", threadTS)
	}
	return slices.Clone(msgs), nil
}

// clock is a settable clock; tests move it so rows cached "earlier" are
// prune candidates for a fetch "now".
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newSyncer(t *testing.T) (*Syncer, *fakeSource, *clock) {
	t.Helper()
	db, err := cache.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertWorkspace(cache.Workspace{ID: "T1", Name: "Test"}); err != nil {
		t.Fatal(err)
	}
	src := newFake()
	clk := &clock{t: time.Unix(1700001000, 0)}
	return &Syncer{DB: db, Source: src, WorkspaceID: "T1", Now: clk.now}, src, clk
}

func spans(t *testing.T, s *Syncer, ch string) []cache.Span {
	t.Helper()
	got, err := s.DB.Spans(ch)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func feedTexts(t *testing.T, s *Syncer, ch string) []string {
	t.Helper()
	rows, err := s.DB.GetMessagesBetween(ch, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.Text)
	}
	return out
}

func TestOpen_ColdChannelCachesThePageAndCoversIt(t *testing.T) {
	s, src, _ := newSyncer(t)
	for i := 0; i < 10; i++ {
		src.post("C1", fm(i, fmt.Sprintf("m%d", i)))
	}
	changed, err := s.Open(context.Background(), "C1", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Upserted) != 10 || changed.Upserted[0].TS != ts(0) || len(changed.Deleted) != 0 {
		t.Fatalf("changed = %d rows (first %s), %d deleted", len(changed.Upserted), changed.Upserted[0].TS, len(changed.Deleted))
	}
	if got := feedTexts(t, s, "C1"); len(got) != 10 {
		t.Fatalf("cached %v", got)
	}
	got := spans(t, s, "C1")
	if len(got) != 1 || got[0].OldestTS != ts(0) || got[0].LatestTS != ts(9) || !got[0].ReachesStart || !got[0].Live {
		t.Fatalf("spans = %+v, want one live span over everything reaching the start", got)
	}
	row, _ := s.DB.GetMessage("C1", ts(3))
	if row.Version != "v:m3" || row.RawJSON == "" {
		t.Fatalf("row = %+v, want version and raw_json", row)
	}
}

func TestOpen_RevalidationCostsNoBodies(t *testing.T) {
	s, src, _ := newSyncer(t)
	for i := 0; i < 5; i++ {
		src.post("C1", fm(i, fmt.Sprintf("m%d", i)))
	}
	if _, err := s.Open(context.Background(), "C1", true); err != nil {
		t.Fatal(err)
	}
	changed, err := s.Open(context.Background(), "C1", true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed.Empty() {
		t.Fatalf("second open changed %+v, want nothing", changed)
	}
	if got := src.calls[1].CachedVersions; len(got) != 5 || got[ts(2)] != "v:m2" {
		t.Fatalf("second open vouched for %v, want all five", got)
	}
	if got := spans(t, s, "C1"); len(got) != 1 {
		t.Fatalf("spans = %+v", got)
	}
}

func TestOlder_PagesBackAndMergesUntilTheStart(t *testing.T) {
	s, src, _ := newSyncer(t)
	for i := 0; i < 40; i++ {
		src.post("C1", fm(i, fmt.Sprintf("m%d", i)))
	}
	if _, err := s.Open(context.Background(), "C1", false); err != nil {
		t.Fatal(err)
	}
	got := spans(t, s, "C1")
	if len(got) != 1 || got[0].OldestTS != ts(12) || got[0].ReachesStart || got[0].Live {
		t.Fatalf("after open: %+v, want [12, 39] not reaching the start, not live (offline)", got)
	}
	changed, err := s.Older(context.Background(), "C1", got[0].OldestTS, false)
	if err != nil {
		t.Fatal(err)
	}
	// The anchor message is cached and vouched for, so it comes back
	// unchanged; the 12 older ones are new.
	if len(changed.Upserted) != 12 {
		t.Fatalf("older page wrote %d rows, want 12", len(changed.Upserted))
	}
	if v := src.calls[1].CachedVersions; v[ts(12)] != "v:m12" {
		t.Fatalf("older page did not vouch for the anchor: %v", v)
	}
	got = spans(t, s, "C1")
	if len(got) != 1 || got[0].OldestTS != ts(0) || got[0].LatestTS != ts(39) || !got[0].ReachesStart {
		t.Fatalf("after older: %+v, want one span [0, 39] reaching the start", got)
	}
	if n := len(feedTexts(t, s, "C1")); n != 40 {
		t.Fatalf("cached %d rows, want 40", n)
	}
}

func TestCatchUp_HoleLeavesTwoSpansUntilBridged(t *testing.T) {
	s, src, _ := newSyncer(t)
	for i := 0; i < 5; i++ {
		src.post("C1", fm(i, fmt.Sprintf("m%d", i)))
	}
	if _, err := s.Open(context.Background(), "C1", true); err != nil {
		t.Fatal(err)
	}
	// Offline, 40 messages arrive: more than a page.
	if err := s.DB.ClearLiveSpans("T1"); err != nil {
		t.Fatal(err)
	}
	for i := 5; i < 45; i++ {
		src.post("C1", fm(i, fmt.Sprintf("m%d", i)))
	}
	changed, err := s.CatchUp(context.Background(), "C1", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Upserted) != 28 {
		t.Fatalf("catch-up wrote %d rows, want the head page of 28", len(changed.Upserted))
	}
	if src.calls[1].Oldest != ts(4) {
		t.Fatalf("catch-up anchored at %q, want the head span's latest %s", src.calls[1].Oldest, ts(4))
	}
	got := spans(t, s, "C1")
	if len(got) != 2 || got[0].LatestTS != ts(4) || got[1].OldestTS != ts(17) || got[1].LatestTS != ts(44) || !got[1].Live || got[0].Live {
		t.Fatalf("spans = %+v, want [0,4] and a live [17,44]", got)
	}
	covered, _ := s.DB.Covers("C1", ts(4), ts(17))
	if covered {
		t.Fatal("the hole counts as covered")
	}
	// The page across the hole bridges it.
	if _, err := s.Older(context.Background(), "C1", ts(17), true); err != nil {
		t.Fatal(err)
	}
	got = spans(t, s, "C1")
	if len(got) != 1 || got[0].OldestTS != ts(0) || got[0].LatestTS != ts(44) || !got[0].Live || !got[0].ReachesStart {
		t.Fatalf("after bridging: %+v, want one live span [0, 44] reaching the start", got)
	}
}

func TestCatchUp_NothingNewMakesTheSpanLive(t *testing.T) {
	s, src, _ := newSyncer(t)
	src.post("C1", fm(0, "only"))
	if _, err := s.Open(context.Background(), "C1", false); err != nil {
		t.Fatal(err)
	}
	if got := spans(t, s, "C1"); got[0].Live {
		t.Fatal("offline open made a live span")
	}
	changed, err := s.CatchUp(context.Background(), "C1", true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed.Empty() {
		t.Fatalf("changed = %+v, want nothing", changed)
	}
	got := spans(t, s, "C1")
	if len(got) != 1 || !got[0].Live || got[0].OldestTS != ts(0) || got[0].LatestTS != ts(0) {
		t.Fatalf("spans = %+v, want the one span, now live", got)
	}
}

func TestOpen_PrunesWhatSlackDroppedButNotWhatArrivedMeanwhile(t *testing.T) {
	s, src, clk := newSyncer(t)
	for i := 0; i < 5; i++ {
		src.post("C1", fm(i, fmt.Sprintf("m%d", i)))
	}
	if _, err := s.Open(context.Background(), "C1", true); err != nil {
		t.Fatal(err)
	}
	// A live message lands (the socket has it, the fake's history does
	// not yet), then m2 is deleted elsewhere while we are offline.
	live := fm(9, "live")
	ev := slackclient.MessageEvent{ChannelID: "C1", Kind: slackclient.MessageNew, Message: live}
	if _, err := s.ApplyEvent(ev, true); err != nil {
		t.Fatal(err)
	}
	src.remove("C1", ts(2))
	clk.advance(time.Minute)

	changed, err := s.Open(context.Background(), "C1", true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed.Deleted, []string{ts(2)}) {
		t.Fatalf("deleted = %v, want [%s]", changed.Deleted, ts(2))
	}
	got := feedTexts(t, s, "C1")
	if !slices.Equal(got, []string{"m0", "m1", "m3", "m4", "live"}) {
		t.Fatalf("feed = %v", got)
	}
	// The head page's range stops at m4; the live row above it is not
	// pruned, and the span still covers it.
	if sp := spans(t, s, "C1"); len(sp) != 1 || sp[0].LatestTS != ts(9) {
		t.Fatalf("spans = %+v", sp)
	}
}

func TestApplyEvent_ReplyThenParentChangeCountsOnce(t *testing.T) {
	s, src, _ := newSyncer(t)
	src.post("C1", fm(0, "parent"))
	if _, err := s.Open(context.Background(), "C1", true); err != nil {
		t.Fatal(err)
	}
	reply := slack.Message{Msg: slack.Msg{Timestamp: ts(5), User: "U2", Text: "reply", ThreadTimestamp: ts(0)}}
	if _, err := s.ApplyEvent(slackclient.MessageEvent{ChannelID: "C1", Kind: slackclient.MessageNew, Message: reply}, true); err != nil {
		t.Fatal(err)
	}
	parent, _ := s.DB.GetMessage("C1", ts(0))
	if parent.ReplyCount != 1 {
		t.Fatalf("after the reply: reply_count = %d, want 1", parent.ReplyCount)
	}
	// Slack then reports the parent's new shape.
	changedParent := slack.Message{Msg: slack.Msg{Timestamp: ts(0), User: "U1", Text: "parent", ThreadTimestamp: ts(0), ReplyCount: 1, LatestReply: ts(5)}}
	changed, err := s.ApplyEvent(slackclient.MessageEvent{ChannelID: "C1", Kind: slackclient.MessageChanged, Message: changedParent, Raw: json.RawMessage(`{"text":"parent","reply_count":1}`)}, true)
	if err != nil {
		t.Fatal(err)
	}
	parent, _ = s.DB.GetMessage("C1", ts(0))
	if parent.ReplyCount != 1 || parent.ThreadTS != ts(0) || parent.EditedAt != "" {
		t.Fatalf("after the change: %+v, want reply_count 1, thread_ts set, no edit", parent)
	}
	if len(changed.Upserted) != 1 || changed.Upserted[0].ReplyCount != 1 {
		t.Fatalf("changed = %+v", changed)
	}
	// The reply is a thread row, not a feed row, and did not touch the span.
	if sp := spans(t, s, "C1"); len(sp) != 1 || sp[0].LatestTS != ts(0) {
		t.Fatalf("spans = %+v, want unchanged [0, 0]", sp)
	}
	replies, _ := s.DB.GetThreadReplies("C1", ts(0))
	if len(replies) != 2 { // parent (thread_ts = ts) and the reply
		t.Fatalf("thread rows = %d, want parent and reply", len(replies))
	}
}

func TestApplyEvent_EditKeepsSubtypeAndVersion(t *testing.T) {
	s, src, _ := newSyncer(t)
	bc := fm(3, "broadcast")
	bc.SubType, bc.ThreadTimestamp = "thread_broadcast", ts(0)
	src.post("C1", fm(0, "parent"), bc)
	if _, err := s.Open(context.Background(), "C1", true); err != nil {
		t.Fatal(err)
	}
	edit := slack.Message{Msg: slack.Msg{Timestamp: ts(3), User: "U1", Text: "broadcast, edited", ThreadTimestamp: ts(0), Edited: &slack.Edited{Timestamp: ts(8)}}}
	changed, err := s.ApplyEvent(slackclient.MessageEvent{ChannelID: "C1", Kind: slackclient.MessageChanged, Message: edit, Raw: json.RawMessage(`{"text":"broadcast, edited"}`)}, true)
	if err != nil {
		t.Fatal(err)
	}
	row, _ := s.DB.GetMessage("C1", ts(3))
	if row.Text != "broadcast, edited" || row.Subtype != "thread_broadcast" || row.Version != "v:broadcast" || row.EditedAt != ts(8) || row.RawJSON != `{"text":"broadcast, edited"}` {
		t.Fatalf("row = %+v", row)
	}
	if len(changed.Upserted) != 1 || changed.Upserted[0].Subtype != "thread_broadcast" {
		t.Fatalf("changed = %+v", changed)
	}
	// An edit of something not cached changes nothing and makes no row.
	ghost := slack.Message{Msg: slack.Msg{Timestamp: ts(77), Text: "ghost"}}
	changed, err = s.ApplyEvent(slackclient.MessageEvent{ChannelID: "C1", Kind: slackclient.MessageChanged, Message: ghost}, true)
	if err != nil || !changed.Empty() {
		t.Fatalf("ghost edit: %+v, %v", changed, err)
	}
	if _, err := s.DB.GetMessage("C1", ts(77)); err == nil {
		t.Fatal("ghost edit made a row")
	}
}

func TestApplyEvent_NewMessageExtendsOrStartsTheLiveSpan(t *testing.T) {
	s, src, _ := newSyncer(t)
	src.post("C1", fm(0, "a"))
	if _, err := s.Open(context.Background(), "C1", true); err != nil {
		t.Fatal(err)
	}
	post := func(ch string, n int) {
		t.Helper()
		m := fm(n, fmt.Sprintf("live%d", n))
		raw, _ := json.Marshal(m)
		if _, err := s.ApplyEvent(slackclient.MessageEvent{ChannelID: ch, Kind: slackclient.MessageNew, Message: m, Raw: raw}, true); err != nil {
			t.Fatal(err)
		}
	}
	post("C1", 5)
	if sp := spans(t, s, "C1"); len(sp) != 1 || sp[0].LatestTS != ts(5) || !sp[0].Live {
		t.Fatalf("C1 spans = %+v, want the live span extended to 5", sp)
	}
	// A channel never opened: the message starts a live span of itself.
	post("C2", 7)
	if sp := spans(t, s, "C2"); len(sp) != 1 || sp[0].OldestTS != ts(7) || sp[0].LatestTS != ts(7) || !sp[0].Live {
		t.Fatalf("C2 spans = %+v, want a live span [7, 7]", sp)
	}
	row, _ := s.DB.GetMessage("C2", ts(7))
	if row.RawJSON == "" || row.Text != "live7" {
		t.Fatalf("row = %+v", row)
	}
	// Deleting it.
	changed, err := s.ApplyEvent(slackclient.MessageEvent{ChannelID: "C2", Kind: slackclient.MessageDeleted, Message: slack.Message{Msg: slack.Msg{Timestamp: ts(7)}}}, true)
	if err != nil || !slices.Equal(changed.Deleted, []string{ts(7)}) {
		t.Fatalf("delete: %+v, %v", changed, err)
	}
	if row, _ = s.DB.GetMessage("C2", ts(7)); !row.IsDeleted {
		t.Fatal("row not soft-deleted")
	}
}

func TestApplyReaction(t *testing.T) {
	s, src, _ := newSyncer(t)
	src.post("C1", fm(0, "a"))
	if _, err := s.Open(context.Background(), "C1", true); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"U1", "U2", "U1"} { // U1 twice is idempotent
		if err := s.ApplyReaction("C1", ts(0), u, "thumbsup", true); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := s.DB.GetReactions(ts(0), "C1")
	if len(got) != 1 || got[0].Count != 2 || len(got[0].UserIDs) != 2 {
		t.Fatalf("reactions = %+v, want thumbsup x2", got)
	}
	if err := s.ApplyReaction("C1", ts(0), "U1", "thumbsup", false); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.DB.GetReactions(ts(0), "C1"); got[0].Count != 1 || got[0].UserIDs[0] != "U2" {
		t.Fatalf("after removal = %+v", got)
	}
	if err := s.ApplyReaction("C1", ts(0), "U2", "thumbsup", false); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.DB.GetReactions(ts(0), "C1"); len(got) != 0 {
		t.Fatalf("last removal left %+v", got)
	}
}

func TestThread_CachesRepliesAndPrunesStaleOnes(t *testing.T) {
	s, src, clk := newSyncer(t)
	parent := fm(0, "parent")
	parent.ThreadTimestamp, parent.ReplyCount = ts(0), 2
	r1 := slack.Message{Msg: slack.Msg{Timestamp: ts(1), User: "U2", Text: "r1", ThreadTimestamp: ts(0)}}
	r2 := slack.Message{Msg: slack.Msg{Timestamp: ts(2), User: "U3", Text: "r2", ThreadTimestamp: ts(0)}}
	src.post("C1", parent)
	src.threads["C1/"+ts(0)] = []slack.Message{parent, r1, r2}
	if _, err := s.Open(context.Background(), "C1", true); err != nil {
		t.Fatal(err)
	}
	changed, err := s.Thread(context.Background(), "C1", ts(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Upserted) != 3 || len(changed.Deleted) != 0 {
		t.Fatalf("changed = %+v", changed)
	}
	// r1 is deleted elsewhere; the next fetch drops it.
	src.threads["C1/"+ts(0)] = []slack.Message{parent, r2}
	clk.advance(time.Minute)
	changed, err = s.Thread(context.Background(), "C1", ts(0))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed.Deleted, []string{ts(1)}) {
		t.Fatalf("deleted = %v, want [%s]", changed.Deleted, ts(1))
	}
	replies, _ := s.DB.GetThreadReplies("C1", ts(0))
	if len(replies) != 2 || replies[1].TS != ts(2) {
		t.Fatalf("thread rows = %+v, want parent and r2", replies)
	}
}

func TestPage_WaitsOutARateLimit(t *testing.T) {
	s, src, _ := newSyncer(t)
	src.post("C1", fm(0, "a"))
	src.limitOnce = true
	changed, err := s.Open(context.Background(), "C1", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Upserted) != 1 || len(src.calls) != 2 {
		t.Fatalf("changed = %d rows after %d calls, want 1 row after 2 calls", len(changed.Upserted), len(src.calls))
	}
}

func TestBackfill_FillsToTheHorizonThenThreads(t *testing.T) {
	s, src, clk := newSyncer(t)
	// 60 messages one hour apart, newest "now"; horizon 40 hours.
	now := clk.now()
	for i := 0; i < 60; i++ {
		at := now.Add(-time.Duration(59-i) * time.Hour)
		m := slack.Message{Msg: slack.Msg{Timestamp: fmt.Sprintf("%d.000000", at.Unix()), User: "U1", Text: fmt.Sprintf("h%d", i)}}
		if i == 50 { // a thread within the horizon
			m.ThreadTimestamp, m.ReplyCount = m.Timestamp, 1
			src.threads["C1/"+m.Timestamp] = []slack.Message{m, {Msg: slack.Msg{Timestamp: fmt.Sprintf("%d.000001", at.Unix()), Text: "its reply", ThreadTimestamp: m.Timestamp}}}
		}
		if i == 10 { // a thread beyond it
			m.ThreadTimestamp, m.ReplyCount = m.Timestamp, 1
		}
		src.post("C1", m)
	}
	src.post("C2", fm(0, "other channel"))

	var reported []Changed
	var paused atomic.Bool
	paused.Store(true)
	go func() { time.Sleep(20 * time.Millisecond); paused.Store(false) }()
	err := s.Backfill(context.Background(), Config{
		Horizon:   40 * time.Hour,
		Interval:  time.Millisecond,
		Paused:    paused.Load,
		Connected: func() bool { return true },
		Channels:  func() []string { return []string{"C1", "C2"} },
		OnChanged: func(c Changed) { reported = append(reported, c) },
	})
	if err != nil {
		t.Fatal(err)
	}
	sp := spans(t, s, "C1")
	if len(sp) != 1 || sp[0].ReachesStart {
		t.Fatalf("C1 spans = %+v, want one span stopping short of the start", sp)
	}
	oldest, _ := s.DB.OldestCachedTS("C1")
	horizon := fmt.Sprintf("%d.000000", now.Add(-40*time.Hour).Unix())
	if strings.Compare(horizon, oldest) < 0 {
		t.Fatalf("oldest cached %s is newer than the horizon %s", oldest, horizon)
	}
	// Two pages of 28 (overlapping by the anchor row) reach past 40
	// hours; a third was not fetched.
	if n := len(feedTexts(t, s, "C1")); n != 55 {
		t.Fatalf("cached %d of C1, want 55 (two pages sharing one row)", n)
	}
	if sp := spans(t, s, "C2"); len(sp) != 1 || !sp[0].ReachesStart || !sp[0].Live {
		t.Fatalf("C2 spans = %+v", sp)
	}
	// Only the thread within the horizon was fetched.
	if len(src.replies) != 1 || !strings.HasSuffix(src.replies[0], fmt.Sprintf("%d.000000", now.Add(-9*time.Hour).Unix())) {
		t.Fatalf("threads fetched = %v, want the one 9 hours old", src.replies)
	}
	if len(reported) == 0 {
		t.Fatal("OnChanged never heard anything")
	}
}

func TestBackfill_StopsWhenCancelled(t *testing.T) {
	s, src, _ := newSyncer(t)
	for i := 0; i < 100; i++ {
		src.post("C1", fm(i, "m"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(15 * time.Millisecond); cancel() }()
	err := s.Backfill(ctx, Config{Interval: 5 * time.Millisecond, Channels: func() []string { return []string{"C1"} }})
	if err == nil {
		t.Fatal("Backfill returned nil after cancel")
	}
	// Whatever was fetched is consistent: spans cover cached rows only.
	for _, sp := range spans(t, s, "C1") {
		rows, _ := s.DB.GetMessagesBetween("C1", sp.OldestTS, sp.LatestTS)
		if len(rows) == 0 {
			t.Fatalf("span %+v covers no rows", sp)
		}
	}
}

func TestVersions_IsBoundedToTheWindow(t *testing.T) {
	s, src, _ := newSyncer(t)
	for i := 0; i < 40; i++ {
		src.post("C1", fm(i, "m"))
	}
	if _, err := s.Open(context.Background(), "C1", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Older(context.Background(), "C1", ts(12), true); err != nil {
		t.Fatal(err)
	}
	got, err := Versions(s.DB, "C1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != VersionWindow {
		t.Fatalf("Versions vouched for %d rows, want %d", len(got), VersionWindow)
	}
	if _, ok := got[ts(11)]; ok {
		t.Fatal("Versions reached below the newest 28")
	}
	empty, err := Versions(s.DB, "C9")
	if err != nil || empty != nil {
		t.Fatalf("Versions of an unknown channel = %v, %v; want nil", empty, err)
	}
}

func TestBackfill_ReportsProgress(t *testing.T) {
	s, src, _ := newSyncer(t)
	for i := 0; i < 3; i++ {
		src.post("C1", fm(i, "m"))
	}
	parent := fm(0, "parent")
	parent.ThreadTimestamp, parent.ReplyCount = ts(0), 1
	src.post("C2", parent)
	src.threads["C2/"+ts(0)] = []slack.Message{parent, {Msg: slack.Msg{Timestamp: ts(1), Text: "r", ThreadTimestamp: ts(0)}}}

	var seen []Progress
	var paused atomic.Bool
	paused.Store(true)
	go func() { time.Sleep(20 * time.Millisecond); paused.Store(false) }()
	err := s.Backfill(context.Background(), Config{
		Interval:   time.Millisecond,
		Paused:     paused.Load,
		Channels:   func() []string { return []string{"C1", "C2"} },
		OnProgress: func(p Progress) { seen = append(seen, p) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 || seen[0].Phase != PhaseChannels || seen[0].Total != 2 {
		t.Fatalf("first report = %+v, want channels phase with 2 listed", seen[0])
	}
	var waited, inFlight, threads bool
	for _, p := range seen {
		waited = waited || p.Waiting
		inFlight = inFlight || (p.Phase == PhaseChannels && p.ChannelID == "C1" && p.Requests == 1)
		threads = threads || (p.Phase == PhaseThreads && p.ThreadTS == ts(0) && p.ChannelID == "C2")
	}
	if !waited || !inFlight || !threads {
		t.Fatalf("waited=%v inFlight=%v threads=%v; reports: %+v", waited, inFlight, threads, seen)
	}
	last := seen[len(seen)-1]
	if last.Phase != PhaseDone || last.Requests != 3 || last.Err != nil || last.Done != 1 || last.Total != 1 {
		t.Fatalf("last report = %+v, want done after 3 requests (two channel pages, one thread) with the thread counted", last)
	}
}

func TestBackfill_HoldsWhileDisconnected(t *testing.T) {
	s, src, _ := newSyncer(t)
	src.post("C1", fm(0, "a"))
	var connected atomic.Bool
	go func() { time.Sleep(30 * time.Millisecond); connected.Store(true) }()
	var seen []Progress
	err := s.Backfill(context.Background(), Config{
		Interval:   time.Millisecond,
		Connected:  connected.Load,
		Channels:   func() []string { return []string{"C1"} },
		OnProgress: func(p Progress) { seen = append(seen, p) },
	})
	if err != nil {
		t.Fatal(err)
	}
	waited := false
	for _, p := range seen {
		waited = waited || p.Waiting
	}
	if !waited {
		t.Fatalf("the run did not hold while disconnected: %+v", seen)
	}
	if len(src.calls) != 1 {
		t.Fatalf("made %d requests, want 1 after the socket came up", len(src.calls))
	}
	if sp := spans(t, s, "C1"); len(sp) != 1 || !sp[0].Live {
		t.Fatalf("spans = %+v, want one live span (fetched once connected)", sp)
	}
}
