package chat

import (
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
)

func TestStartsGroup(t *testing.T) {
	at := time.Date(2026, 10, 29, 14, 0, 0, 0, time.Local)
	ada := backend.Message{AuthorID: "U1", Author: "Ada", Time: at}
	next := func(change func(*backend.Message)) backend.Message {
		m := ada
		m.Time = at.Add(time.Minute)
		change(&m)
		return m
	}
	cases := []struct {
		name  string
		prev  *backend.Message
		m     backend.Message
		group bool // starts a group
		day   bool // starts a day
	}{
		{"first message", nil, ada, true, true},
		{"same author a minute later", &ada, next(func(*backend.Message) {}), false, false},
		{"same author just inside the gap", &ada, next(func(m *backend.Message) { m.Time = at.Add(GroupGap) }), false, false},
		{"same author past the gap", &ada, next(func(m *backend.Message) { m.Time = at.Add(GroupGap + time.Second) }), true, false},
		{"another author", &ada, next(func(m *backend.Message) { m.AuthorID, m.Author = "U2", "Grace" }), true, false},
		{"another author, same name", &ada, next(func(m *backend.Message) { m.AuthorID = "U2" }), true, false},
		{"same app, another name", &ada, next(func(m *backend.Message) { m.Author = "Ada's bot" }), true, false},
		{"an ephemeral", &ada, next(func(m *backend.Message) { m.Ephemeral = true }), true, false},
		{"after midnight", &ada, next(func(m *backend.Message) { m.Time = time.Date(2026, 10, 30, 0, 1, 0, 0, time.Local) }), true, true},
	}
	for _, c := range cases {
		if got := StartsGroup(c.prev, c.m); got != c.group {
			t.Errorf("%s: StartsGroup = %v, want %v", c.name, got, c.group)
		}
		if got := NewDay(c.prev, c.m); got != c.day {
			t.Errorf("%s: NewDay = %v, want %v", c.name, got, c.day)
		}
	}
	eph := ada
	eph.Ephemeral = true
	if !StartsGroup(&eph, next(func(*backend.Message) {})) {
		t.Error("a message after an ephemeral joined its group")
	}
}
