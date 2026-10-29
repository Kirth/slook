package slashcmd

import (
	"errors"
	"testing"

	"github.com/gammons/slk/internal/core"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in         string
		name, args string
		ok         bool
	}{
		{"/giphy cats", "/giphy", "cats", true},
		{"/giphy", "/giphy", "", true},
		{"/GIPHY cats", "/giphy", "cats", true},
		{"/giphy   ", "/giphy", "", true},
		{"/giphy \t\n ", "/giphy", "", true},
		{"/shrug line one\nline two", "/shrug", "line one\nline two", true},
		{"/giphy\ncats", "/giphy", "cats", true},
		{"/my-cmd_2 x", "/my-cmd_2", "x", true},
		{"/remind me  to  stretch", "/remind", "me  to  stretch", true},
		{"/giphy cats", "/giphy", "cats", true},
		{"/giphy　 cats", "/giphy", "cats", true},
		{"/giphy ", "/giphy", "", true},
		{" /giphy cats", "", "", false},
		{"/tmp/foo is full", "", "", false},
		{"/ hello", "", "", false},
		{"/", "", "", false},
		{"/giphy: cats", "", "", false},
		{"hello /giphy", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		name, args, ok := Parse(tc.in)
		if name != tc.name || args != tc.args || ok != tc.ok {
			t.Errorf("Parse(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.in, name, args, ok, tc.name, tc.args, tc.ok)
		}
	}
}

func TestIsNamePrefix(t *testing.T) {
	for in, want := range map[string]bool{
		"/": true, "/re": true, "/my-cmd_2": true,
		"": false, "re": false, "/re ": false, "/tmp/": false, " /re": false,
	} {
		if got := IsNamePrefix(in); got != want {
			t.Errorf("IsNamePrefix(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFailureText(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&core.CommandError{Code: "unknown_command"}, "/nope is not a valid command"},
		{&core.CommandError{Code: "restricted_action"}, "/nope failed: restricted_action"},
		{errors.ErrUnsupported, "/nope failed: not available"},
		{errors.New("timeout"), "/nope failed: timeout"},
	}
	for _, tc := range cases {
		if got := FailureText("/nope", tc.err); got != tc.want {
			t.Errorf("FailureText(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
