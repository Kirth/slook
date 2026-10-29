package slackclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppCommands_RequestAndParse replays the captured client.appCommands
// exchange: the request carries the web client's _x_reason, and the
// captured response parses into the commands it lists.
func TestAppCommands_RequestAndParse(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "client_app_commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Form     [][2]string     `json:"form"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	wantReason := ""
	for _, kv := range capture.Form {
		if kv[0] == "_x_reason" {
			wantReason = kv[1]
		}
	}
	if wantReason == "" {
		t.Fatal("client_app_commands.json has no _x_reason")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/client.appCommands") {
			t.Errorf("path = %s, want …/client.appCommands", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if got := r.FormValue("_x_reason"); got != wantReason {
			t.Errorf("_x_reason = %q, want %q", got, wantReason)
		}
		// Only the token and the _x_* envelope, as captured.
		for k := range r.PostForm {
			if k != "token" && !strings.HasPrefix(k, "_x_") {
				t.Errorf("unexpected field %s", k)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(capture.Response)
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)

	cmds, err := c.AppCommands(context.Background())
	if err != nil {
		t.Fatalf("AppCommands: %v", err)
	}
	byName := map[string]SlashCommand{}
	for _, cmd := range cmds {
		byName[cmd.Name] = cmd
	}
	if len(cmds) != 32 {
		t.Errorf("got %d commands, want the capture's 32", len(cmds))
	}
	if got := byName["/remind"]; got.Desc == "" || got.Usage == "" || got.AppName != "" {
		t.Errorf("/remind = %+v, want a core command with usage and description", got)
	}
	if got := byName["/join"].AliasOf; got != "/open" {
		t.Errorf("/join alias_of = %q, want /open", got)
	}
	if got := byName["/linear"].AppName; got != "Linear" {
		t.Errorf("/linear app = %q, want Linear", got)
	}
}

func TestAppCommands_NotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error":"not_allowed"}`))
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)
	if _, err := c.AppCommands(context.Background()); err == nil || !strings.Contains(err.Error(), "not_allowed") {
		t.Fatalf("err = %v, want not_allowed", err)
	}
}
