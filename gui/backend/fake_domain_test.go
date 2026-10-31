package backend_test

import (
	"context"
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
)

// The fake's permalinks, and what Forward posts, name its own workspace,
// so a link to one of its messages opens in-app (chat.LinkTarget).
func TestFakeDomainMatchesItsPermalinks(t *testing.T) {
	f := backend.NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan backend.Event, 8)
	go f.Run(ctx, func(ev backend.Event) { got <- ev })

	hist, err := f.History(ctx, "C1")
	if err != nil || len(hist) == 0 {
		t.Fatalf("history: %v, %d messages", err, len(hist))
	}
	orig := hist[len(hist)-1]
	link, err := f.Permalink(ctx, "C1", orig.TS)
	if err != nil {
		t.Fatal(err)
	}
	ch, ts, _, ok := chat.LinkTarget(link, f.Domain())
	if !ok || ch != "C1" || ts != orig.TS {
		t.Fatalf("LinkTarget(%q, %q) = %q %q %v", link, f.Domain(), ch, ts, ok)
	}

	if err := f.Forward(ctx, orig, "D1", ""); err != nil {
		t.Fatal(err)
	}
	for ev := range got {
		p, isPost := ev.(backend.MessagePosted)
		if !isPost {
			continue
		}
		url, ok := chat.ForwardedLink(p.Message.Text)
		if !ok {
			t.Fatalf("forwarded text %q is not a forwarded link", p.Message.Text)
		}
		if ch, ts, _, ok := chat.LinkTarget(url, f.Domain()); !ok || ch != "C1" || ts != orig.TS {
			t.Fatalf("forwarded link %q leads to %q %q %v", url, ch, ts, ok)
		}
		return
	}
}
