package main

import (
	"testing"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
)

func TestAnchorSurvivesAPrepend(t *testing.T) {
	m := func(ts string) backend.Message { return backend.Message{ChannelID: "C1", TS: ts} }
	var tl chat.Timeline
	tl.Merge([]backend.Message{m("1700000300.000100"), m("1700000400.000100"), m("1700000500.000100")}, nil)

	// The second shown message is at the top, 7px scrolled into.
	anchor, off := anchorOf(tl.Messages(), 1, 7)
	tl.Merge([]backend.Message{m("1700000100.000100"), m("1700000200.000100")}, nil)
	if i, ok := tl.Index(anchor); !ok || i != 3 || off != 7 || anchor != "1700000400.000100" {
		t.Fatalf("anchor %s at %d (offset %d), want 1700000400.000100 at 3 (offset 7)", anchor, i, off)
	}

	// With the history edge visible, the oldest message stays on top.
	if anchor, off := anchorOf(tl.Messages(), -1, 30); anchor != "1700000100.000100" || off != 0 {
		t.Fatalf("got %s offset %d, want the oldest at offset 0", anchor, off)
	}
	if anchor, _ := anchorOf(nil, 0, 0); anchor != "" {
		t.Fatal("anchored in an empty list")
	}
}
