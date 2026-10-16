package main

import (
	"testing"

	"github.com/gammons/slk/internal/ui"
)

func TestOnViewOpenedToasts(t *testing.T) {
	cases := map[string]string{
		"Giphy": "Giphy opened a dialog slk can't show yet",
		"":      "This command opened a dialog slk can't show yet",
	}
	for title, want := range cases {
		s := &captureSender{}
		h := &rtmEventHandler{program: s}
		h.OnViewOpened(title)
		if len(s.sent) != 1 {
			t.Fatalf("sent %d messages, want 1", len(s.sent))
		}
		if got, ok := s.sent[0].(ui.ToastMsg); !ok || got.Text != want {
			t.Errorf("sent %#v, want ToastMsg{%q}", s.sent[0], want)
		}
	}
}

func TestOnViewOpenedInactiveWorkspaceIsSilent(t *testing.T) {
	s := &captureSender{}
	h := &rtmEventHandler{program: s, isActive: func() bool { return false }}
	h.OnViewOpened("Giphy")
	if len(s.sent) != 0 {
		t.Errorf("sent %v, want nothing for an inactive workspace", s.sent)
	}
}
