package chat

import (
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
)

func TestRemoteSearchFiresOncePerPause(t *testing.T) {
	var r RemoteSearch
	t0 := time.Unix(1_700_000_000, 0)
	r.Type("s", t0)
	if _, ok := r.Due(t0.Add(time.Second)); ok {
		t.Fatal("one letter is asked about")
	}
	// Typing on: each key restarts the pause.
	r.Type("sr", t0)
	r.Type("sre", t0.Add(200*time.Millisecond))
	if _, ok := r.Due(t0.Add(400 * time.Millisecond)); ok {
		t.Fatal("due before the pause after the last key")
	}
	if w := r.Wake(); !w.Equal(t0.Add(200*time.Millisecond + RemoteDelay)) {
		t.Fatalf("wake %v, want the end of the pause", w)
	}
	at := t0.Add(200*time.Millisecond + RemoteDelay)
	q, ok := r.Due(at)
	if !ok || q != "sre" {
		t.Fatalf("Due = %q, %v; want sre", q, ok)
	}
	if _, ok := r.Due(at.Add(time.Second)); ok {
		t.Fatal("asked twice while in flight")
	}
	if !r.Wake().IsZero() || !r.Searching() {
		t.Fatal("in flight: no wake needed, still searching")
	}
	res := []backend.ChannelResult{{ID: "C20", Name: "platform-sre", Kind: backend.KindChannel}}
	r.Done("sre", res)
	if _, ok := r.Due(at.Add(2 * time.Second)); ok {
		t.Fatal("asked again after the answer")
	}
	if got := r.Results(" #sre "); len(got) != 1 || got[0].ID != "C20" {
		t.Fatalf("results %v", got)
	}
	// Back to a query already answered: no request.
	r.Type("sr", at.Add(3*time.Second))
	r.Type("sre", at.Add(3*time.Second))
	if _, ok := r.Due(at.Add(5 * time.Second)); ok {
		t.Fatal("an answered query was asked again")
	}
	// Cleared: nothing to ask, nothing shown.
	r.Type("", at.Add(6*time.Second))
	if _, ok := r.Due(at.Add(7 * time.Second)); ok || r.Results("") != nil || r.Searching() {
		t.Fatal("an empty filter searches")
	}
	// A failure is not asked again until the query is typed again.
	r.Type("cat", at.Add(8*time.Second))
	if q, ok := r.Due(at.Add(9 * time.Second)); !ok || q != "cat" {
		t.Fatalf("Due = %q, %v", q, ok)
	}
	r.Done("cat", nil)
	if _, ok := r.Due(at.Add(10 * time.Second)); ok {
		t.Fatal("a failed query was asked again at once")
	}
	r.Type("ca", at.Add(11*time.Second))
	r.Type("cat", at.Add(11*time.Second))
	if _, ok := r.Due(at.Add(12 * time.Second)); !ok {
		t.Fatal("a failed query typed again was not asked")
	}
}

func TestRemoteSearchIgnoresStaleAnswers(t *testing.T) {
	var r RemoteSearch
	t0 := time.Unix(1_700_000_000, 0)
	r.Type("des", t0)
	if _, ok := r.Due(t0.Add(RemoteDelay)); !ok {
		t.Fatal("not due")
	}
	r.Type("desi", t0.Add(time.Second))
	if _, ok := r.Due(t0.Add(2 * time.Second)); ok {
		t.Fatal("a second request while the first is in flight")
	}
	r.Done("des", []backend.ChannelResult{{ID: "C21", Name: "design-crit", Kind: backend.KindChannel}})
	if got := r.Results("desi"); got != nil {
		t.Fatalf("the answer for des shown for desi: %v", got)
	}
	if q, ok := r.Due(t0.Add(2 * time.Second)); !ok || q != "desi" {
		t.Fatalf("after the stale answer Due = %q, %v; want desi", q, ok)
	}
}

func TestUnjoinedDropsMembersAndListed(t *testing.T) {
	res := []backend.ChannelResult{
		{ID: "C1", Name: "general", Member: true},
		{ID: "C2", Name: "random"}, // listed, though the search says otherwise
		{ID: "C20", Name: "platform-sre"},
		{ID: "C21", Name: "design-crit"},
	}
	convs := []backend.Conversation{{ID: "C2", Name: "random"}}
	got := Unjoined(res, convs)
	if len(got) != 2 || got[0].ID != "C20" || got[1].ID != "C21" {
		t.Fatalf("Unjoined = %v", got)
	}
}

func TestExactChannel(t *testing.T) {
	res := []backend.ChannelResult{
		{ID: "C20", Name: "platform-sre", Kind: backend.KindChannel},
		{ID: "C23", Name: "Café", Kind: backend.KindPrivate},
		{ID: "D1", Name: "ada", Kind: backend.KindDM},
	}
	for _, tc := range []struct {
		name, want string
	}{
		{"#platform-sre", "C20"},
		{"PLATFORM-SRE", "C20"},
		{"cafe", "C23"},
		{"platform", ""}, // a prefix is not the name
		{"ada", ""},      // not a channel
		{"", ""},
	} {
		r, ok := ExactChannel(tc.name, res)
		if (tc.want == "") == ok || ok && r.ID != tc.want {
			t.Errorf("ExactChannel(%q) = %v, %v; want %q", tc.name, r, ok, tc.want)
		}
	}
}
