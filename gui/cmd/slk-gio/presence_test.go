package main

import (
	"context"
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
)

func TestComposerPresenceCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := backend.NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	h := newHub(ctx, f, func(error) {})
	go f.Run(ctx, h.dispatch)
	b := newComposeBox(h, nil, nil, "C1", "")
	away := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for h.selfStatus().Away != want {
			if time.Now().After(deadline) {
				t.Fatalf("away %v, want %v", h.selfStatus().Away, want)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	for _, c := range []struct {
		typed string
		away  bool
	}{
		{"/away", true},
		{"/away", false}, // a toggle
		{"/status away", true},
		{"/active", false},
		{"/status Away", true},
		{"/status active", false},
	} {
		b.setText(c.typed)
		b.submit(c.typed)
		if b.composer.Text() != "" || b.sendErr != "" {
			t.Fatalf("%s: composer %q, note %q", c.typed, b.composer.Text(), b.sendErr)
		}
		away(c.away)
	}
	b.setText("/status :palm_tree: on holiday")
	b.submit(b.composer.Text())
	if b.sendErr != "/status isn't supported in slk-gio yet" || b.composer.Text() == "" {
		t.Fatalf("custom status: composer %q, note %q", b.composer.Text(), b.sendErr)
	}
}
