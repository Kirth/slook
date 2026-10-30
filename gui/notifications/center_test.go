package notifications

import (
	"strings"
	"testing"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/config"
)

type recorder struct {
	shown   []Note
	cleared []string
}

func (r *recorder) Show(n Note) error      { r.shown = append(r.shown, n); return nil }
func (r *recorder) Clear(channelID string) { r.cleared = append(r.cleared, channelID) }

var on = config.Notifications{Enabled: true, OnMention: true, OnDM: true, OnKeyword: []string{"outage"}}

func msg(ch, ts, author, text string) backend.Message {
	return backend.Message{ChannelID: ch, TS: ts, Author: author, AuthorID: "U-" + author, Text: text,
		Body: rich.FromMrkdwn(text, rich.MapNames{})}
}

var (
	dm      = backend.Conversation{ID: "D1", Name: "Ada", Kind: backend.KindDM}
	general = backend.Conversation{ID: "C1", Name: "general", Kind: backend.KindChannel}
	group   = backend.Conversation{ID: "G1", Name: "Ada, Grace", Kind: backend.KindGroupDM}
)

func TestWhatNotifies(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.Notifications
		m       func() backend.Message
		conv    backend.Conversation
		focused bool
		want    bool
	}{
		{"a DM", on, func() backend.Message { return msg("D1", "1.0", "Ada", "hi") }, dm, false, true},
		{"a DM while a window has the keyboard", on, func() backend.Message { return msg("D1", "1.0", "Ada", "hi") }, dm, true, false},
		{"a channel message", on, func() backend.Message { return msg("C1", "1.0", "Ada", "lunch?") }, general, false, false},
		{"a mention", on, func() backend.Message { return msg("C1", "1.0", "Ada", "<@ME> look") }, general, false, true},
		{"a keyword", on, func() backend.Message { return msg("C1", "1.0", "Ada", "Big OUTAGE now") }, general, false, true},
		{"your own", on, func() backend.Message { m := msg("D1", "1.0", "You", "hi"); m.FromSelf = true; return m }, dm, false, false},
		{"your own, by ID", on, func() backend.Message { m := msg("D1", "1.0", "You", "hi"); m.AuthorID = "ME"; return m }, dm, false, false},
		{"an edit", on, func() backend.Message { m := msg("D1", "1.0", "Ada", "hi"); m.Edited = true; return m }, dm, false, false},
		{"an ephemeral", on, func() backend.Message { m := msg("D1", "1.0", "Slackbot", "hi"); m.Ephemeral = true; return m }, dm, false, false},
		{"a muted DM", on, func() backend.Message { return msg("D1", "1.0", "Ada", "hi") }, backend.Conversation{ID: "D1", Kind: backend.KindDM, Muted: true}, false, false},
		{"disabled", config.Notifications{OnDM: true}, func() backend.Message { return msg("D1", "1.0", "Ada", "hi") }, dm, false, false},
		{"every message in a channel", on, func() backend.Message { return msg("C1", "1.0", "Ada", "lunch?") }, backend.Conversation{ID: "C1", Kind: backend.KindChannel, Notify: backend.NotifyAll}, false, true},
		{"every message, though muted in Slack", on, func() backend.Message { return msg("C1", "1.0", "Ada", "lunch?") }, backend.Conversation{ID: "C1", Kind: backend.KindChannel, Muted: true, Notify: backend.NotifyAll}, false, true},
		{"every message, but your own", on, func() backend.Message { m := msg("C1", "1.0", "You", "hi"); m.AuthorID = "ME"; return m }, backend.Conversation{ID: "C1", Kind: backend.KindChannel, Notify: backend.NotifyAll}, false, false},
		{"every message, while a window has the keyboard", on, func() backend.Message { return msg("C1", "1.0", "Ada", "lunch?") }, backend.Conversation{ID: "C1", Kind: backend.KindChannel, Notify: backend.NotifyAll}, true, false},
		{"nothing, though a mention", on, func() backend.Message { return msg("C1", "1.0", "Ada", "<@ME> look") }, backend.Conversation{ID: "C1", Kind: backend.KindChannel, Notify: backend.NotifyNone}, false, false},
		{"nothing, though a DM", on, func() backend.Message { return msg("D1", "1.0", "Ada", "hi") }, backend.Conversation{ID: "D1", Kind: backend.KindDM, Notify: backend.NotifyNone}, false, false},
		{"a DM the list has not loaded", on, func() backend.Message { return msg("D9", "1.0", "Ada", "hi") }, backend.Conversation{}, false, true},
	}
	for _, c := range cases {
		rec := &recorder{}
		got := New(c.cfg, "ME", rec).Message(c.m(), c.conv, c.focused)
		if got != c.want || (len(rec.shown) == 1) != c.want {
			t.Errorf("%s: notified %v (%d shown), want %v", c.name, got, len(rec.shown), c.want)
		}
	}
}

func TestNotifiesOnceAndClears(t *testing.T) {
	rec := &recorder{}
	c := New(on, "ME", rec)
	m := msg("D1", "1.0", "Ada", "hi")
	c.Message(m, dm, false)
	c.Message(m, dm, false)
	if len(rec.shown) != 1 {
		t.Fatalf("shown %d times", len(rec.shown))
	}
	c.Read("D1")
	if len(rec.cleared) != 1 || rec.cleared[0] != "D1" {
		t.Fatalf("cleared %v", rec.cleared)
	}
}

func TestNoteText(t *testing.T) {
	reply := msg("C1", "2.0", "Ada", "<@ME> in here")
	reply.ThreadTS = "1.0"
	cases := []struct {
		m    backend.Message
		conv backend.Conversation
		want string
	}{
		{msg("D1", "1.0", "Ada", "x"), dm, "Ada"},
		{msg("C1", "1.0", "Ada", "x"), general, "Ada in #general"},
		{msg("G1", "1.0", "Ada", "x"), group, "Ada in Ada, Grace"},
		{reply, general, "Ada in #general (thread)"},
	}
	for _, c := range cases {
		if got := Title(c.m, c.conv); got != c.want {
			t.Errorf("Title = %q, want %q", got, c.want)
		}
	}
	rec := &recorder{}
	New(on, "ME", rec).Message(reply, general, false)
	if len(rec.shown) != 1 || rec.shown[0].ThreadTS != "1.0" || rec.shown[0].TS != "2.0" {
		t.Fatalf("thread reply note %+v", rec.shown)
	}

	long := msg("D1", "1.0", "Ada", "line one\nline *two* "+strings.Repeat("word ", 60))
	b := Body(long)
	if strings.Contains(b, "\n") || strings.Contains(b, "*") || len([]rune(b)) != bodyRunes || !strings.HasSuffix(b, "…") {
		t.Fatalf("body %q", b)
	}
}

func TestGroupMentionsNotifyWhenYours(t *testing.T) {
	for _, c := range []struct {
		name string
		cfg  config.Notifications
		text string
		conv backend.Conversation
		want bool
	}{
		{"your group", on, "<!subteam^S1|@eng> look", general, true},
		{"your group, bare", on, "<!subteam^S1> look", general, true},
		{"another group", on, "<!subteam^S2|@design> look", general, false},
		{"your group, muted", on, "<!subteam^S1> look", backend.Conversation{ID: "C1", Kind: backend.KindChannel, Muted: true}, false},
		{"your group, nothing here", on, "<!subteam^S1> look", backend.Conversation{ID: "C1", Kind: backend.KindChannel, Notify: backend.NotifyNone}, false},
		{"your group, mentions off", config.Notifications{Enabled: true, OnDM: true}, "<!subteam^S1> look", general, false},
	} {
		rec := &recorder{}
		center := New(c.cfg, "ME", rec)
		center.SetGroups([]string{"S1"})
		if got := center.Message(msg("C1", "1.0", "Ada", c.text), c.conv, false); got != c.want {
			t.Errorf("%s: notified %v, want %v", c.name, got, c.want)
		}
	}
}
