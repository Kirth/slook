package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
)

// searchTestWindow is a search window over the fake, never shown: its
// app.Window does nothing until run, so the window's own code runs
// without a display.
func searchTestWindow(t *testing.T) *searchWindow {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f := backend.NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	h := newHub(ctx, f, func(error) {})
	if _, err := h.conversations(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(h.people()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("people never loaded")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return newSearchWindow(h, true)
}

// settle applies what the window's searches post until none is in
// flight.
func settle(t *testing.T, s *searchWindow) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.searching {
		if time.Now().After(deadline) {
			t.Fatal("the search never answered")
		}
		time.Sleep(2 * time.Millisecond)
		s.apply(time.Now())
	}
}

func TestSearchWindowSuggestsAndFilters(t *testing.T) {
	s := searchTestWindow(t)

	s.ed.SetText("from:ad")
	s.ed.SetCaret(7, 7)
	s.suggest()
	if len(s.sug.Choices) == 0 || s.sug.Choices[0].Label != "Ada Lovelace" {
		t.Fatalf("suggestions for from:ad: %+v", s.sug.Choices)
	}
	s.accept(0)
	if got := s.ed.Text(); got != `from:"Ada Lovelace" ` {
		t.Fatalf("after taking the suggestion: %q", got)
	}

	s.fire(strings.TrimSpace(s.ed.Text()))
	settle(t, s)
	if len(s.hits) == 0 {
		t.Fatal("nothing from Ada")
	}
	for _, h := range s.hits {
		if h.Message.AuthorID != "U1" {
			t.Fatalf("a hit by %s", h.Message.AuthorID)
		}
	}
	if len(s.chips) != 1 || s.chips[0].Text != "from Ada Lovelace" || s.ranked {
		t.Fatalf("chips %+v, ranked %v (filters alone have no rank)", s.chips, s.ranked)
	}

	s.fire(`from:ada queue`)
	settle(t, s)
	if len(s.hits) == 0 || !s.ranked {
		t.Fatalf("from:ada queue: %d hits, ranked %v", len(s.hits), s.ranked)
	}

	s.fire("from:zzz")
	if s.searching || s.hits != nil || len(s.chips) != 1 || !s.chips[0].Err {
		t.Fatalf("an unknown name: searching %v, hits %v, chips %+v", s.searching, s.hits, s.chips)
	}

	s.fire("from:")
	if s.searching || s.query != "" {
		t.Fatalf("a filter still being typed searched: %+v", s.query)
	}
}

func TestSearchWindowEscapeHidesSuggestions(t *testing.T) {
	s := searchTestWindow(t)
	s.ed.SetText("is:th")
	s.ed.SetCaret(5, 5)
	s.suggest()
	if len(s.sug.Choices) != 1 {
		t.Fatalf("suggestions %+v", s.sug.Choices)
	}
	s.sugDismissed = s.ed.Text() // as Escape does
	s.sugText = ""
	s.suggest()
	if len(s.sug.Choices) != 0 {
		t.Fatal("Escape's text still suggests")
	}
	s.ed.SetText("is:t")
	s.ed.SetCaret(4, 4)
	s.suggest()
	if len(s.sug.Choices) != 1 {
		t.Fatal("a changed text suggests again")
	}
}
