package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/slack/slacktest"
)

// bootBodies is a fake Slack for a boot: signed in to T1 (Acme) as U1;
// userBoot lists C1 (general, muted) and the DM D1 with U2; client.counts
// names both, C1 unread; edge revalidation knows U2 as Pat; the sidebar
// has a stars section and C1 is starred. Tests change entries.
func bootBodies() map[string]string {
	return map[string]string{
		"/api/auth.test":           `{"ok":true,"url":"https://acme.slack.com/","team":"Acme","user":"grant","team_id":"T1","user_id":"U1"}`,
		"/api/client.shouldReload": `{"ok":true,"should_reload":false,"recommended_build_version":1785403654}`,
		"/api/client.userBoot":     slacktest.UserBootBody,
		"/api/client.counts": `{"ok":true,
			"channels":[{"id":"C1","has_unreads":true,"mention_count":2,"last_read":"1.0","latest":"1700000000.000100"}],
			"ims":[{"id":"D1","has_unreads":false,"last_read":"2.0","latest":"1700000000.000200"}],
			"threads":{"has_unreads":false}}`,
		"/cache/T1/channels/info": `{"ok":true,"results":[],"failed_ids":["D1"]}`,
		"/cache/T1/users/info":    `{"ok":true,"results":[{"id":"U2","name":"pat","team_id":"T1","updated":1783337599010,"profile":{"display_name":"Pat"}}]}`,
		"/api/users.channelSections.list": `{"ok":true,"channel_sections":[
			{"channel_section_id":"S1","name":"Starred","type":"stars","channel_ids_page":{"channel_ids":[],"count":0}}]}`,
		"/api/stars.list": `{"ok":true,"items":[{"type":"channel","channel":"C1"}],"paging":{"count":1,"total":1}}`,
	}
}

// bootSlack is a Slack backend over srv with an empty in-memory cache,
// signed in (auth.test), not booted; warnings go to the returned buffer.
func bootSlack(t *testing.T, srv *slacktest.Server) (*Slack, *bytes.Buffer) {
	t.Helper()
	client := slacktest.NewClient(t, srv.Server)
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	db, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	tok := slackclient.Token{AccessToken: "xoxc-test", Cookie: "d-cookie", TeamID: "T1", TeamName: "Acme"}
	s := newSlack(client, tok, db, t.TempDir(), ConnectOptions{})
	var warn bytes.Buffer
	s.errOut = &warn
	return s, &warn
}

// bootPaths is the requests made, without the background build-timestamp
// refresh (client.shouldReload), which lands whenever it lands.
func bootPaths(srv *slacktest.Server) []string {
	return slices.DeleteFunc(srv.Paths(), func(p string) bool { return p == "/api/client.shouldReload" })
}

func byID(convs []Conversation) map[string]Conversation {
	out := map[string]Conversation{}
	for _, c := range convs {
		out[c.ID] = c
	}
	return out
}

// TestConversations_BootsWithoutEnumerating is the regression guard for
// the GUI's boot: Slack's own boot sequence (client.userBoot,
// client.counts, edge revalidation), and none of the enumeration that
// got Enterprise Grid users signed out (users.conversations), nor the
// per-user users.info or users.prefs.get the boot makes unnecessary.
func TestConversations_BootsWithoutEnumerating(t *testing.T) {
	srv := slacktest.NewServer(t, bootBodies())
	s, warn := bootSlack(t, srv)

	convs, err := s.Conversations(context.Background())
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	t.Logf("boot requests: %v", bootPaths(srv))
	got := byID(convs)
	c1, ok := got["C1"]
	if !ok || c1.Name != "general" || c1.Kind != KindChannel {
		t.Fatalf("C1 = %+v (present %v); want #general", c1, ok)
	}
	if !c1.Muted || !c1.Starred || !c1.Unread || c1.Mentions != 2 {
		t.Errorf("C1 = %+v; want muted (boot prefs), starred (stars.list), unread with 2 mentions (client.counts)", c1)
	}
	d1, ok := got["D1"]
	if !ok || d1.Kind != KindDM || d1.Name != "Pat" || d1.PeerID != "U2" {
		t.Errorf("D1 = %+v (present %v); want a DM with U2 named Pat (edge users/info)", d1, ok)
	}

	paths := bootPaths(srv)
	for _, forbidden := range []string{"/api/users.conversations", "/api/conversations.list", "/api/users.list", "/api/users.prefs.get", "/api/users.info"} {
		if slices.Contains(paths, forbidden) {
			t.Errorf("boot asked %s; requests: %v", forbidden, paths)
		}
	}
	if n := srv.Count("/api/client.counts"); n > 1 {
		t.Errorf("client.counts asked %d times; want at most 1", n)
	}
	if n := srv.Count("/api/client.userBoot"); n != 1 {
		t.Errorf("client.userBoot asked %d times; want 1", n)
	}
	if strings.Contains(warn.String(), "FALLING BACK") {
		t.Errorf("the boot fell back: %s", warn)
	}
	// The boot's read state is what the first connect's catch-up uses.
	s.mu.Lock()
	countsOK, latest := s.countsOK, s.counts["C1"].Latest
	s.mu.Unlock()
	if !countsOK || latest != "1700000000.000100" {
		t.Errorf("boot read state: ok %v, C1 latest %q; want the client.counts answer", countsOK, latest)
	}
}

// TestConversations_FallsBackToEnumerationOnlyWhenBootFails: when
// client.userBoot fails the list still loads, from users.conversations,
// once, and says so loudly.
func TestConversations_FallsBackToEnumerationOnlyWhenBootFails(t *testing.T) {
	bodies := bootBodies()
	bodies["/api/client.userBoot"] = `{"ok":false,"error":"unknown_method"}`
	bodies["/api/users.conversations"] = `{"ok":true,"channels":[
		{"id":"C1","name":"general","is_channel":true,"is_member":true},
		{"id":"D1","is_im":true,"user":"U2"}],"response_metadata":{"next_cursor":""}}`
	bodies["/api/users.prefs.get"] = `{"ok":true,"prefs":{"all_notifications_prefs":"{\"channels\":{\"C1\":{\"muted\":true}}}"}}`
	srv := slacktest.NewServer(t, bodies)
	s, warn := bootSlack(t, srv)

	convs, err := s.Conversations(context.Background())
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	got := byID(convs)
	if got["C1"].Name != "general" || !got["C1"].Muted || got["D1"].Name != "Pat" {
		t.Errorf("list = %+v; want #general (muted) and a DM named Pat", convs)
	}
	if n := srv.Count("/api/users.conversations"); n != 1 {
		t.Errorf("users.conversations asked %d times; want 1", n)
	}
	if !strings.Contains(warn.String(), "FALLING BACK") {
		t.Errorf("warnings = %q; want the loud FALLING BACK line", warn)
	}
}

// TestConversations_SecondCallMakesNoRequest: the list window loads the
// conversations every time it opens (the tray reopens it); only the
// first call boots.
func TestConversations_SecondCallMakesNoRequest(t *testing.T) {
	srv := slacktest.NewServer(t, bootBodies())
	s, _ := bootSlack(t, srv)
	first, err := s.Conversations(context.Background())
	if err != nil {
		t.Fatalf("first Conversations: %v", err)
	}
	before := len(bootPaths(srv))
	second, err := s.Conversations(context.Background())
	if err != nil {
		t.Fatalf("second Conversations: %v", err)
	}
	if after := bootPaths(srv); len(after) != before {
		t.Errorf("the second call asked %v", after[before:])
	}
	if !maps.EqualFunc(byID(first), byID(second), func(a, b Conversation) bool { return a.Name == b.Name && a.Unread == b.Unread }) {
		t.Errorf("second list %+v differs from the first %+v", second, first)
	}

	// Read state the socket reports reaches the next list.
	s.noteRead(slackclient.UnreadInfo{ChannelID: "C1", LastRead: "1700000000.000100"})
	third, _ := s.Conversations(context.Background())
	if byID(third)["C1"].Unread {
		t.Error("C1 still unread after channel_marked")
	}
}

// TestConversations_FillsCountsGapFromEdge: userBoot's channels[] is a
// subset on real workspaces; a conversation client.counts names that it
// left out is listed, filled by one edge channels/info.
func TestConversations_FillsCountsGapFromEdge(t *testing.T) {
	bodies := bootBodies()
	bodies["/api/client.counts"] = `{"ok":true,
		"channels":[{"id":"C1","has_unreads":false,"last_read":"1.0"},{"id":"C9","has_unreads":true,"last_read":"1.0"}],
		"ims":[{"id":"D1","has_unreads":false,"last_read":"2.0"}],
		"threads":{"has_unreads":false}}`
	bodies["/cache/T1/channels/info"] = `{"ok":true,"results":[{"id":"C9","name":"design","is_channel":true,"updated":1783337533019}],"failed_ids":["D1"]}`
	srv := slacktest.NewServer(t, bodies)
	s, _ := bootSlack(t, srv)

	convs, err := s.Conversations(context.Background())
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	c9, ok := byID(convs)["C9"]
	if !ok || c9.Name != "design" || !c9.Unread {
		t.Fatalf("C9 = %+v (present %v); want #design, unread", c9, ok)
	}
	// Revalidation asks channels/info about userBoot's ids; the gap is a
	// request of its own, for C9 alone.
	var gapAsks int
	for _, r := range srv.Requests("/cache/T1/channels/info") {
		var body struct {
			Updated map[string]int64 `json:"updated_ids"`
		}
		if err := json.Unmarshal(r.Body, &body); err != nil {
			t.Fatalf("channels/info body %q: %v", r.Body, err)
		}
		if _, ok := body.Updated["C9"]; ok {
			gapAsks++
			if len(body.Updated) != 1 || body.Updated["C9"] != 0 {
				t.Errorf("gap request asked %v; want {C9: 0}", body.Updated)
			}
		}
	}
	if gapAsks != 1 {
		t.Errorf("channels/info asked about C9 %d times; want 1", gapAsks)
	}
	if slices.Contains(bootPaths(srv), "/api/users.conversations") {
		t.Error("the gap was filled by enumerating")
	}
}
