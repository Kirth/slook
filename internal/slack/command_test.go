package slackclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// capturedForm loads a scrubbed web-client capture and returns its form
// fields minus token and the _x_* envelope, which postForm and slackhttp
// own.
func capturedForm(t *testing.T, name string) url.Values {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Form [][2]string `json:"form"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	out := url.Values{}
	for _, kv := range c.Form {
		if kv[0] == "token" || strings.HasPrefix(kv[0], "_x_") {
			continue
		}
		out.Set(kv[0], kv[1])
	}
	return out
}

var clientTokenRe = regexp.MustCompile(`^web-[0-9]{13}$`)

// assertFormMatchesCapture compares field sets: same names, same values
// (JSON byte for byte), nothing extra. client_token is checked for shape
// only: it is a timestamp.
func assertFormMatchesCapture(t *testing.T, got url.Values, captureFile string) {
	t.Helper()
	want := capturedForm(t, captureFile)
	if tok := got.Get("client_token"); !clientTokenRe.MatchString(tok) {
		t.Errorf("client_token = %q, want web-<13 digits>", tok)
	}
	for k := range want {
		if k == "client_token" {
			continue
		}
		if got.Get(k) != want.Get(k) {
			t.Errorf("%s =\n  %q\nwant\n  %q", k, got.Get(k), want.Get(k))
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected field %s=%q (not in %s)", k, got.Get(k), captureFile)
		}
	}
}

func TestCommandForm_MatchesCaptures(t *testing.T) {
	cases := []struct {
		capture string
		req     CommandRequest
	}{
		{"chat_command_channel.json", CommandRequest{ChannelID: "C0EXAMPLE01", Command: "/giphy", Args: "gorilla", TeamID: "T0EXAMPLE01"}},
		{"chat_command_thread.json", CommandRequest{ChannelID: "C0EXAMPLE02", ThreadTS: "1791758997.192499", Command: "/giphy", Args: "gorilla", TeamID: "T0EXAMPLE01"}},
		{"chat_command_thread_no_args.json", CommandRequest{ChannelID: "C0EXAMPLE02", ThreadTS: "1791758997.192499", Command: "/giphy", TeamID: "T0EXAMPLE01"}},
		{"chat_command_unknown.json", CommandRequest{ChannelID: "C0EXAMPLE02", ThreadTS: "1791758997.192499", Command: "/notarealcommand"}},
		{"chat_command_shrug.json", CommandRequest{ChannelID: "D0EXAMPLE01", Command: "/shrug", Args: "hi", TeamID: "T0EXAMPLE01"}},
	}
	for _, tc := range cases {
		t.Run(tc.capture, func(t *testing.T) {
			got, err := commandForm(tc.req, newClientToken())
			if err != nil {
				t.Fatal(err)
			}
			assertFormMatchesCapture(t, got, tc.capture)
		})
	}
}

// A name followed only by whitespace has no arguments, so blocks is
// omitted exactly as for bare /giphy.
func TestCommandForm_WhitespaceOnlyArgsOmitBlocks(t *testing.T) {
	got, err := commandForm(CommandRequest{ChannelID: "C1", Command: "/giphy", Args: "   \n "}, "web-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["blocks"]; ok {
		t.Errorf("blocks = %q, want no blocks field", got.Get("blocks"))
	}
}

// Mentions arrive in wire form and become typed rich-text elements, and
// text is not HTML-escaped (the web client sends it verbatim).
func TestCommandForm_ArgsMentionsAndNoHTMLEscape(t *testing.T) {
	got, err := commandForm(CommandRequest{ChannelID: "C1", Command: "/invite", Args: "<@U0EXAMPLE01> a<b"}, "web-1")
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"type":"rich_text","elements":[{"type":"rich_text_section","elements":[{"type":"user","user_id":"U0EXAMPLE01"},{"type":"text","text":" a<b"}]}]}]`
	if got.Get("blocks") != want {
		t.Errorf("blocks =\n  %s\nwant\n  %s", got.Get("blocks"), want)
	}
}

func TestRunCommand_PostsToChatCommand(t *testing.T) {
	var gotPath string
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"keep_input":false}`))
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)

	err := c.RunCommand(context.Background(), CommandRequest{ChannelID: "C0EXAMPLE01", Command: "/giphy", Args: "gorilla", TeamID: "T0EXAMPLE01"})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if gotPath != "/api/chat.command" {
		t.Errorf("path = %q, want /api/chat.command", gotPath)
	}
	if gotForm.Get("command") != "/giphy" || gotForm.Get("token") != "xoxc-test" {
		t.Errorf("form = %v, want command=/giphy and the xoxc token", gotForm)
	}
}

// TestRunCommand_SendsTheCapturedReason pins _x_reason to the value the web
// client sent on chat.command, read straight from the capture (no filter).
func TestRunCommand_SendsTheCapturedReason(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "chat_command_channel.json"))
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Form [][2]string `json:"form"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	want := ""
	for _, kv := range capture.Form {
		if kv[0] == "_x_reason" {
			want = kv[1]
		}
	}
	if want == "" {
		t.Fatal("chat_command_channel.json has no _x_reason")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if got := r.FormValue("_x_reason"); got != want {
			t.Errorf("_x_reason = %q; want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"keep_input":false}`))
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)
	c.teamID = "T0EXAMPLE01"
	c.Envelope().SetTeamID("T0EXAMPLE01")

	if err := c.RunCommand(context.Background(), CommandRequest{ChannelID: "C0EXAMPLE01", Command: "/giphy", Args: "gorilla", TeamID: "T0EXAMPLE01"}); err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
}

func TestRunCommand_RejectionIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error":"unknown_command"}`))
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)

	err := c.RunCommand(context.Background(), CommandRequest{ChannelID: "C1", Command: "/notarealcommand"})
	var rej *CommandRejectedError
	if !errors.As(err, &rej) || rej.Code != "unknown_command" {
		t.Fatalf("err = %v, want *CommandRejectedError{Code: unknown_command}", err)
	}
}

func TestRunCommand_HTTPFailureIsNotARejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)

	err := c.RunCommand(context.Background(), CommandRequest{ChannelID: "C1", Command: "/giphy"})
	var rej *CommandRejectedError
	if err == nil || errors.As(err, &rej) {
		t.Fatalf("err = %v, want a non-rejection transport error", err)
	}
}
