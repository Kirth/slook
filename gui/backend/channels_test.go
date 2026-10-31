package backend

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/gammons/slk/internal/slack/slacktest"
)

// TestSearchChannels_MapsResultsAndHintsFromTheList carries the rules
// of the TUI's channel finder search (cmd/slk/channel_search.go): one
// channels/search per call, the conversations most recently visited or
// active as top_channels, archived channels, DMs and group DMs left
// out, membership from the list rather than member_channels.
func TestSearchChannels_MapsResultsAndHintsFromTheList(t *testing.T) {
	bodies := bootBodies()
	bodies["/cache/T1/channels/search"] = `{"ok":true,"results":[
		{"id":"C1","name":"general","is_channel":true},
		{"id":"C5","name":"secret","is_channel":true,"is_private":true},
		{"id":"C6","name":"old","is_channel":true,"is_archived":true},
		{"id":"D7","name":"dm","is_im":true},
		{"id":"G8","name":"group","is_mpim":true}],
		"member_channels":["C5"]}`
	srv := slacktest.NewServer(t, bodies)
	s, _ := bootSlack(t, srv)
	ctx := context.Background()
	if _, err := s.Conversations(ctx); err != nil {
		t.Fatal(err)
	}

	if res, err := s.SearchChannels(ctx, ""); res != nil || err != nil {
		t.Fatalf("empty query = %v, %v; want nothing", res, err)
	}
	if n := srv.Count("/cache/T1/channels/search"); n != 0 {
		t.Fatalf("empty query made %d requests", n)
	}

	res, err := s.SearchChannels(ctx, "e")
	if err != nil {
		t.Fatal(err)
	}
	want := []ChannelResult{
		{ID: "C1", Name: "general", Kind: KindChannel, Member: true},
		{ID: "C5", Name: "secret", Kind: KindPrivate},
	}
	if !slices.Equal(res, want) {
		t.Fatalf("results %#v, want %#v", res, want)
	}
	reqs := srv.Requests("/cache/T1/channels/search")
	if len(reqs) != 1 {
		t.Fatalf("%d channels/search requests, want 1", len(reqs))
	}
	var body struct {
		Query string   `json:"query"`
		Top   []string `json:"top_channels"`
	}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("body %q: %v", reqs[0].Body, err)
	}
	if body.Query != "e" || len(body.Top) == 0 || len(body.Top) > maxTopChannels || !slices.Contains(body.Top, "C1") {
		t.Fatalf("query %q, top_channels %v; want e and the listed conversations", body.Query, body.Top)
	}
}

// TestSearchChannels_WithoutEdge fails without a request.
func TestSearchChannels_WithoutEdge(t *testing.T) {
	srv := slacktest.NewServer(t, bootBodies())
	s, _ := bootSlack(t, srv)
	s.edge = nil
	if _, err := s.SearchChannels(context.Background(), "x"); err == nil {
		t.Fatal("no error without edge")
	}
}
