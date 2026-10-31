package backend

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/gui/rich"
)

func TestFakeEditDeleteAndPermalink(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Event, 8)
	go f.Run(ctx, func(ev Event) { got <- ev })
	<-got // connected

	if err := f.Send(ctx, "C1", "teh plan"); err != nil {
		t.Fatal(err)
	}
	sent := nextPosted(t, got).Message
	if sent.Text != "teh plan" {
		t.Fatalf("sent message text %q", sent.Text)
	}

	if err := f.Edit(ctx, sent, "the plan"); err != nil {
		t.Fatal(err)
	}
	ed := nextPosted(t, got).Message
	if ed.TS != sent.TS || !ed.Edited || ed.Text != "the plan" || rich.PlainBlocks(ed.Body) != "the plan" {
		t.Fatalf("edited %+v", ed)
	}
	hist, _ := f.History(ctx, "C1")
	if last := hist[len(hist)-1]; last.Text != "the plan" || !last.Edited {
		t.Fatalf("history has %q", last.Text)
	}

	if err := f.Delete(ctx, "C1", sent.TS); err != nil {
		t.Fatal(err)
	}
	for ev := range got {
		if d, ok := ev.(MessageDeleted); ok {
			if d.ChannelID != "C1" || d.TS != sent.TS {
				t.Fatalf("deleted %+v", d)
			}
			break
		}
	}
	hist, _ = f.History(ctx, "C1")
	if hist[len(hist)-1].TS == sent.TS {
		t.Fatal("history still has the deleted message")
	}
	if err := f.Delete(ctx, "C1", sent.TS); err == nil {
		t.Fatal("deleting twice succeeded")
	}

	link, err := f.Permalink(ctx, "C1", "1700000123.000100")
	if err != nil || link != "https://demo.slack.com/archives/C1/p1700000123000100" || strings.Contains(link, ".0001") {
		t.Fatalf("permalink %q, %v", link, err)
	}
}

func TestFakeForwardPostsALinkCard(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Event, 8)
	go f.Run(ctx, func(ev Event) { got <- ev })
	<-got // connected

	hist, _ := f.History(ctx, "C1")
	orig := hist[len(hist)-1]
	if err := f.Forward(ctx, orig, "D1", "fyi"); err != nil {
		t.Fatal(err)
	}
	m := nextPosted(t, got).Message
	link, _ := f.Permalink(ctx, "C1", orig.TS)
	if m.ChannelID != "D1" || !m.FromSelf || m.Text != "fyi\n<"+link+">" {
		t.Fatalf("forwarded %+v", m)
	}
	if len(m.Body) != 2 {
		t.Fatalf("body %#v, want the comment and a card", m.Body)
	}
	if p := rich.PlainBlocks(m.Body[:1]); p != "fyi" {
		t.Fatalf("comment %q", p)
	}
	card, ok := m.Body[1].(rich.Card)
	if !ok || card.Footer != "Shared from #general" || !strings.Contains(rich.PlainBlocks(card.Blocks), rich.PlainBlocks(orig.Body)) {
		t.Fatalf("card %#v", m.Body[1])
	}
	hist, _ = f.History(ctx, "D1")
	if last := hist[len(hist)-1]; last.TS != m.TS || len(last.Body) != 2 {
		t.Fatalf("D1's history ends with %+v", last)
	}

	if err := f.Forward(ctx, Message{ChannelID: "C1", TS: "1.000000"}, "D1", ""); err == nil || err.Error() != "message_not_found" {
		t.Fatalf("unknown message: %v", err)
	}
}

func TestFakePressActionDeletesTheEphemeral(t *testing.T) {
	f := NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Event, 8)
	go f.Run(ctx, func(ev Event) { got <- ev })
	<-got // connected

	preview := func() Message {
		t.Helper()
		if err := f.RunCommand(ctx, "C1", "", "/standup", "shipped the cache"); err != nil {
			t.Fatal(err)
		}
		m := nextPosted(t, got).Message
		if !m.Ephemeral || len(m.Actions) != 3 {
			t.Fatalf("preview %+v", m)
		}
		return m
	}
	deleted := func(ts string) {
		t.Helper()
		for ev := range got {
			if d, ok := ev.(MessageDeleted); ok {
				if d.ChannelID != "C1" || d.TS != ts {
					t.Fatalf("deleted %+v, want C1 %s", d, ts)
				}
				return
			}
		}
	}

	m := preview()
	send, shuffle, cancelBtn := m.Actions[0], m.Actions[1], m.Actions[2]
	if send.Label != "Send" || send.Style != "primary" || cancelBtn.Confirm == nil || cancelBtn.Confirm.Title != "Cancel?" {
		t.Fatalf("actions %+v", m.Actions)
	}
	// The body's buttons name the actions they press.
	var pressed []int
	for _, b := range m.Body {
		if c, ok := b.(rich.Card); ok {
			for _, inner := range c.Blocks {
				if bs, ok := inner.(rich.Buttons); ok {
					for _, it := range bs.Items {
						pressed = append(pressed, it.Action)
					}
				}
			}
		}
	}
	if len(pressed) != 3 || pressed[0] != 0 || pressed[2] != 2 {
		t.Fatalf("buttons press %v", pressed)
	}

	if err := f.PressAction(ctx, m, shuffle); err != nil {
		t.Fatal(err)
	}
	sh := nextPosted(t, got).Message
	if sh.TS != m.TS || !sh.Edited || !sh.Ephemeral || !strings.Contains(rich.PlainBlocks(sh.Body), "the cache shipped") {
		t.Fatalf("shuffled %+v", sh)
	}

	if err := f.PressAction(ctx, sh, sh.Actions[0]); err != nil {
		t.Fatal(err)
	}
	posted := nextPosted(t, got).Message
	if posted.Author != "Standup Bot" || posted.Ephemeral || posted.Text != "the cache shipped" {
		t.Fatalf("sent %+v", posted)
	}
	deleted(m.TS)
	if err := f.PressAction(ctx, sh, sh.Actions[0]); err == nil || err.Error() != "invalid_action" {
		t.Fatalf("pressing a gone preview: %v", err)
	}

	m = preview()
	if err := f.PressAction(ctx, m, m.Actions[2]); err != nil {
		t.Fatal(err)
	}
	deleted(m.TS)

	m = preview()
	notEphemeral := m
	notEphemeral.Ephemeral = false
	if err := f.PressAction(ctx, notEphemeral, m.Actions[0]); err == nil {
		t.Fatal("pressed a button of a message everyone sees")
	}
}
