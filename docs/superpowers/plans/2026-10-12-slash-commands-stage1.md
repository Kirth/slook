# Slash commands, stage 1 — running commands — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enter on `/name args` in either composer runs the slash command the
way Slack's web client does — through `chat.command`, or through an existing
slk feature for the built-ins the web client handles locally — instead of
posting the literal text.

**Architecture:** A new `chat.command` call in `internal/slack`, behind a new
`core.CommandService` port wired in `cmd/slk`. In `internal/ui`, one helper
called from both Enter branches of `mode_insert.go` detects a command, and the
existing vi-style `:` command registry is generalised to a shared dispatcher with
a second table for slash built-ins; its unknown-name fallback is the remote path.
Built-ins reuse extracted copies of the presence-menu and channel-finder logic
and the new-message flow. Stage 2 (Block Kit buttons) and stage 3 (the `/`
picker) are separate plans.

**Tech Stack:** Go, Bubble Tea v2 (`charm.land/bubbletea/v2`), slack-go,
stdlib `testing` only.

**Spec:** `docs/superpowers/specs/2026-10-12-slash-commands-design.md` §1–3, §6–10.
Captures: `docs/superpowers/specs/2026-10-12-slash-commands-captures/`.
Read `AGENTS.md` before starting; it is the map of shared code and the rules
below come from it.

## Global Constraints

- Tests are plain `testing.T`, stdlib only: no testify, no gomock, no golden libraries. White-box (`package ui`, not `package ui_test`).
- `internal/ui` does no I/O: no `internal/slack`, `slackhttp`, `cache`, `config`, `net/http`, `os/exec`. Everything goes through `internal/core` ports wired in `cmd/slk`. `internal/ui/boundary_test.go` enforces it.
- `App.Update` is a reducer chain: add behaviour in a `reducer_*.go` file, never in `Update`.
- Toasts are set on the Update goroutine with `toastWithClear(a, text, d)`; a `tea.Cmd` that wants a toast returns `ToastMsg{Text: …}`. Never call `statusbar.SetToast` directly (`TestToastsGoThroughToastSetters`).
- A refactor commit must not change behaviour; extractions get their own commit, before the feature that uses them.
- Request shapes copy the web client's captures: field names and values match; business fields go out alphabetically (`url.Values.Encode()`, the accepted divergence pinned by `TestPostForm_BodyFieldOrderIsAlphabeticalThenEnvelope`); JSON-valued fields match byte for byte, encoded with HTML escaping off.
- `client_token` is `web-<unix milliseconds>`, generated only by `newClientToken()` in `internal/slack`.
- A command is text starting at position 0 with `/` followed by one or more `[A-Za-z0-9_-]`, then whitespace or end. Name lowercased. Args = the rest after the name and following whitespace, newlines kept; whitespace-only args = none.
- Every new reusable helper is added to the `AGENTS.md` shared-code tables **in the same commit** that adds it; every new test helper to its test-helper table.
- Per-package iteration: `go test ./internal/ui -run <Name>` etc. Before the final commit: `go build ./...`, `go vet ./...`, `go test ./... -race`, `gofmt -l .` (empty), `golangci-lint run`.

## Review Focus

Inputs the spec implies that are most likely to bite a user; each has a test in
the task named.

1. **Leading space** — ` /giphy cats` posts as an ordinary message (the escape hatch). Test in Task 3.
2. **Multi-line arguments** — `/shrug line one⏎line two` (Shift+Enter) keeps the newline in `Args`. Test in Task 3.
3. **Upper-case name** — `/GIPHY cats` runs `/giphy`. Test in Task 3.
4. **Name with only trailing spaces** — `/giphy   ` sends no arguments, so no `blocks` field. Tests in Task 1 (wire) and Task 3 (`Args == ""`).
5. **Failure after moving away** — the command fails after the user switched conversation: the text becomes that conversation's stored draft, not text in the new conversation's composer. Test in Task 4.

---

## File map

| File | Responsibility | Task |
|---|---|---|
| `internal/slack/command.go` (new) | `CommandRequest`, `CommandRejectedError`, `RunCommand`, `commandForm`, `newClientToken` | 1 |
| `internal/slack/command_test.go` (new) | marshal vs captures, HTTP behaviour | 1 |
| `internal/slack/testdata/chat_command_*.json` (copied) | captures used by the tests | 1 |
| `internal/slackhttp/reason.go` | `"chat.command": "executeCommand"` | 1 |
| `internal/core/ports.go`, `adapters.go`, `types.go` | `CommandService`, `CommandServiceFuncs`, `CommandError` | 2 |
| `internal/core/command_adapter_test.go` (new) | adapter tests | 2 |
| `cmd/slk/commands.go` (+ `_test.go`) (new) | `runCommand`: request mapping, error translation | 2 |
| `cmd/slk/main.go` | wire `SetCommandService` | 2 |
| `internal/ui/service_defaults.go`, `app.go`, `services_helpers_test.go` | noop default, field, setter, test fake | 2 |
| `internal/ui/slash_command.go` (new) | `parseSlashCommand`, `trySlashCommand`, remote run, `commandRanMsg` | 3, 4 |
| `internal/ui/reducer_slash_command.go` (new) | the `commandRanMsg` reducer | 3, 4 |
| `internal/ui/reducer.go` (or wherever the chain is listed) | register the reducer | 3 |
| `internal/ui/mode_insert.go` | call `trySlashCommand` from both Enter branches | 3 |
| `internal/ui/compose/model.go` | `OfferDraft` | 4 |
| `internal/ui/command.go` | `commandArgs`, `dispatchCommand`, `slashCommands` | 5 |
| `internal/ui/presence_action.go` (new) | `applyPresenceAction` (extracted) | 6 |
| `internal/slackfmt/slackfmt.go` | `MentionedChannelIDs` | 7 |
| `internal/ui/open_channel.go` (new) | `openChannel` (extracted) | 7 |
| `internal/ui/slash_builtins.go` (new) | the built-in handlers | 6, 7, 8 |
| `internal/ui/reducer_new_message.go`, `app.go` | `/msg` pre-fill on open | 8 |
| `internal/slack/events.go` | `view_opened`; `me_message` | 9, 10 |
| `cmd/slk/rtm_handler.go` | `OnViewOpened` | 9 |
| `internal/ui/messages/render_message.go` | italic `me_message` | 10 |
| `AGENTS.md` | shared-code and test-helper tables | every task that adds a helper |

Before Task 3, find the file that lists the reducer chain:
`grep -rn "reduceIO" internal/ui/*.go | grep -v _test` — the slice that contains
`reduceIO` is where Task 3 registers its reducer.

---

### Task 1: `chat.command` on the wire

**Files:**
- Create: `internal/slack/command.go`, `internal/slack/command_test.go`
- Create (copy): `internal/slack/testdata/chat_command_channel.json`, `chat_command_thread.json`, `chat_command_thread_no_args.json`, `chat_command_unknown.json`, `chat_command_shrug.json` from `docs/superpowers/specs/2026-10-12-slash-commands-captures/`
- Modify: `internal/slack/client_test.go` or wherever `pointClientAtTestServer` lives — no change, just use it
- Modify: `internal/slackhttp/reason.go` (`defaultReasons`), and its test table if one enumerates the map (`grep -n defaultReasons internal/slackhttp/*_test.go`)
- Modify: `AGENTS.md`

**Interfaces:**
- Produces:
  - `type CommandRequest struct { ChannelID, ThreadTS, Command, Args, TeamID string }`
  - `type CommandRejectedError struct { Code string }` with `Error() string`
  - `func (c *Client) RunCommand(ctx context.Context, req CommandRequest) error`
  - `func newClientToken() string` (unexported; stage 2 reuses it)
  - `func commandForm(req CommandRequest, clientToken string) (url.Values, error)` (unexported)

- [ ] **Step 1: Copy the captures into testdata**

```bash
for f in chat_command_channel chat_command_thread chat_command_thread_no_args chat_command_unknown chat_command_shrug; do
  cp docs/superpowers/specs/2026-10-12-slash-commands-captures/$f.json internal/slack/testdata/$f.json
done
```

- [ ] **Step 2: Write the failing tests**

`internal/slack/command_test.go`:

```go
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

// Review Focus 4: a name followed only by whitespace has no arguments,
// so blocks is omitted exactly as for bare /giphy.
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
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/slack -run 'TestCommandForm|TestRunCommand' -v`
Expected: build failure — `undefined: commandForm`, `CommandRequest`, `newClientToken`, `CommandRejectedError`.

- [ ] **Step 4: Implement**

`internal/slack/command.go`:

```go
package slackclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/slack/mrkdwn"
)

// CommandRequest is one run of a slash command, as Slack's web client
// sends it to chat.command (undocumented). Captures:
// docs/superpowers/specs/2026-10-12-slash-commands-captures/chat_command_*.json.
type CommandRequest struct {
	ChannelID string
	// ThreadTS runs the command in a thread; "" for the conversation.
	ThreadTS string
	// Command is the name with its slash, lowercased: "/giphy".
	Command string
	// Args is the text after the name in wire form (<@U…>, <#C…|name>);
	// "" or whitespace for none.
	Args string
	// TeamID is the workspace. "" omits team_id, which is what the web
	// client sends for a command it does not know.
	TeamID string
}

// CommandRejectedError is chat.command's {"ok":false,"error":Code}, for
// example "unknown_command". Adapters translate it to core.CommandError.
type CommandRejectedError struct {
	Code string
}

func (e *CommandRejectedError) Error() string { return "chat.command: " + e.Code }

// RunCommand runs a slash command as Slack's web client does. What the
// command produces (a message, an ephemeral, a modal) arrives over the
// WebSocket, so only the error comes back.
func (c *Client) RunCommand(ctx context.Context, req CommandRequest) error {
	form, err := commandForm(req, newClientToken())
	if err != nil {
		return err
	}
	debuglog.General("chat.command: command=%s channel=%s thread_ts=%q has_args=%v",
		req.Command, req.ChannelID, req.ThreadTS, form.Has("blocks"))
	raw, err := c.postForm(ctx, "chat.command", form)
	if err != nil {
		debuglog.General("chat.command: error=%v", err)
		return err
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parsing chat.command: %w (body=%s)", err, truncateForLog(raw))
	}
	if !resp.OK {
		debuglog.General("chat.command: rejected error=%s", resp.Error)
		return &CommandRejectedError{Code: resp.Error}
	}
	debuglog.General("chat.command: ok")
	return nil
}

// commandForm builds chat.command's business fields (postForm adds the
// token, slackhttp the _x_* envelope). blocks carries the arguments as
// rich text, the same converter chat.postMessage uses, so mentions become
// typed elements; it is omitted when there are no arguments.
func commandForm(req CommandRequest, clientToken string) (url.Values, error) {
	form := url.Values{
		"command":      {req.Command},
		"disp":         {req.Command},
		"channel":      {req.ChannelID},
		"client_token": {clientToken},
	}
	if strings.TrimSpace(req.Args) != "" {
		_, block := mrkdwn.Convert(req.Args)
		if block != nil {
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			if err := enc.Encode([]slack.Block{block}); err != nil {
				return nil, fmt.Errorf("encoding chat.command blocks: %w", err)
			}
			form.Set("blocks", strings.TrimSuffix(buf.String(), "\n"))
		}
	}
	if req.ThreadTS != "" {
		form.Set("thread_ts", req.ThreadTS)
	}
	if req.TeamID != "" {
		form.Set("team_id", req.TeamID)
	}
	return form, nil
}

// newClientToken is the web client's client_token: "web-" and the unix
// time in milliseconds. The only generator of it in slk.
func newClientToken() string {
	return "web-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
}
```

Then add to `defaultReasons` in `internal/slackhttp/reason.go`:

```go
	"chat.command":               "executeCommand",
```

If a test in `internal/slackhttp` enumerates `defaultReasons` (for example
`reason_test.go`'s table at the `"client.userBoot":            "initial-data",`
line), add the same entry there.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/slack ./internal/slackhttp -run 'TestCommandForm|TestRunCommand|Reason' -v`
Expected: PASS. If `TestCommandForm_MatchesCaptures` fails only on `blocks`,
print both strings — the converter output was verified byte-identical to the
capture for `gorilla` and `hi` when this plan was written, so a diff means the
encoder setup differs from the code above.

- [ ] **Step 6: Add to `AGENTS.md`**

In "Shared code → Text and rendering", next to the
`(*slackclient.Client).AttachmentAction` row, add:

```markdown
| Run a slash command (`chat.command`, request shape copied from the web client; arguments as rich text via `mrkdwn.Convert`) | `(*slackclient.Client).RunCommand(ctx, CommandRequest)`; a refusal (`unknown_command`, …) is `*slackclient.CommandRejectedError` |
| A web-client `client_token` (`web-<unix ms>`) for any request that sends one | `newClientToken()` (`internal/slack`); the only generator |
```

- [ ] **Step 7: Commit**

```bash
gofmt -l internal/slack internal/slackhttp
git add internal/slack/command.go internal/slack/command_test.go internal/slack/testdata/chat_command_*.json internal/slackhttp/ AGENTS.md
git commit -m "feat(slack): run slash commands through chat.command

Copies the web client's request: command/disp, the arguments as a
rich_text block (omitted when there are none), channel, thread_ts,
team_id and a web-<ms> client_token. A refusal such as unknown_command
is a typed CommandRejectedError. Tested against scrubbed captures."
```

---

### Task 2: the `CommandService` port and its wiring

**Files:**
- Modify: `internal/core/ports.go` (after `InteractionService`), `internal/core/adapters.go` (after `interactionAdapter`), `internal/core/types.go` (after `RateLimitedError`)
- Create: `internal/core/command_adapter_test.go`
- Create: `cmd/slk/commands.go`, `cmd/slk/commands_test.go`
- Modify: `cmd/slk/main.go` (after the `app.SetInteractionService(...)` call)
- Modify: `internal/ui/service_defaults.go`, `internal/ui/app.go` (field next to `interactions`, default in `NewApp`, setter next to `SetInteractionService`), `internal/ui/services_helpers_test.go`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: Task 1's `RunCommand`, `CommandRequest`, `CommandRejectedError`; `(*slackclient.Client).TeamID() string`.
- Produces:
  - `core.CommandService` interface: `Run(channelID ids.ChannelID, threadTS ids.ThreadTS, command, args string) error`
  - `core.CommandServiceFuncs{ Run func(ids.ChannelID, ids.ThreadTS, string, string) error }`, `core.NewCommandService(CommandServiceFuncs) CommandService` (nil `Run` → `errors.ErrUnsupported`)
  - `core.CommandError{ Code string }` (pointer receiver `Error()`)
  - `(*App).SetCommandService(core.CommandService)`; App field `commands core.CommandService` — **name it `commandSvc`**, because `commands` is already the `:` registry map in `command.go`
  - test helper `(*App).setCommandRunnerForTest(fn func(ids.ChannelID, ids.ThreadTS, string, string) error)`

- [ ] **Step 1: Write the failing core tests**

`internal/core/command_adapter_test.go`:

```go
package core

import (
	"errors"
	"testing"

	"github.com/gammons/slk/internal/ids"
)

func TestCommandServiceNilFuncIsUnsupported(t *testing.T) {
	err := NewCommandService(CommandServiceFuncs{}).Run("C1", "", "/giphy", "cats")
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
}

func TestCommandServicePassesArguments(t *testing.T) {
	var got []any
	svc := NewCommandService(CommandServiceFuncs{
		Run: func(ch ids.ChannelID, th ids.ThreadTS, cmd, args string) error {
			got = []any{ch, th, cmd, args}
			return nil
		},
	})
	if err := svc.Run("C1", "10.0", "/giphy", "cats"); err != nil {
		t.Fatal(err)
	}
	want := []any{ids.ChannelID("C1"), ids.ThreadTS("10.0"), "/giphy", "cats"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestCommandErrorMessageIsTheCode(t *testing.T) {
	if got := (&CommandError{Code: "unknown_command"}).Error(); got != "unknown_command" {
		t.Errorf("Error() = %q, want unknown_command", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/core -run 'TestCommand' -v`
Expected: build failure — `undefined: NewCommandService`, `CommandServiceFuncs`, `CommandError`.

- [ ] **Step 3: Implement the port**

In `internal/core/ports.go`, after the `InteractionService` interface:

```go
// CommandService runs Slack slash commands. Run is blocking (callers run
// it from a tea.Cmd). What a command produces arrives over the WebSocket,
// so only the error comes back: *CommandError when Slack refuses the
// command, any other error for a transport failure.
//
// Build one with NewCommandService.
type CommandService interface {
	// Run executes command ("/giphy", with its slash, lowercased) with
	// args in channelID, in threadTS's thread when threadTS is set.
	// args is wire-form text (<@U…>, <#C…|name>); "" for none.
	Run(channelID ids.ChannelID, threadTS ids.ThreadTS, command, args string) error
}
```

In `internal/core/adapters.go`, after `interactionAdapter`'s method:

```go
// CommandServiceFuncs bundles the closures behind a CommandService. A nil
// func makes its method return errors.ErrUnsupported.
type CommandServiceFuncs struct {
	Run func(channelID ids.ChannelID, threadTS ids.ThreadTS, command, args string) error
}

// NewCommandService builds a CommandService from fns. Used by
// cmd/slk/main.go (production wiring) and tests.
func NewCommandService(fns CommandServiceFuncs) CommandService {
	return commandAdapter{fns: fns}
}

type commandAdapter struct{ fns CommandServiceFuncs }

func (c commandAdapter) Run(channelID ids.ChannelID, threadTS ids.ThreadTS, command, args string) error {
	if c.fns.Run == nil {
		return errors.ErrUnsupported
	}
	return c.fns.Run(channelID, threadTS, command, args)
}
```

In `internal/core/types.go`, after `RateLimitedError`'s `Error` method:

```go
// CommandError is how adapters report Slack refusing a slash command
// (chat.command's {"ok":false,"error":Code}), so the UI never parses
// Slack error strings. Code is Slack's, e.g. "unknown_command".
type CommandError struct {
	Code string
}

func (e *CommandError) Error() string { return e.Code }
```

- [ ] **Step 4: Run to verify the core tests pass**

Run: `go test ./internal/core -run 'TestCommand' -v`
Expected: PASS.

- [ ] **Step 5: Write the failing `cmd/slk` adapter test**

`cmd/slk/commands_test.go`:

```go
package main

import (
	"context"
	"errors"
	"testing"

	"github.com/gammons/slk/internal/core"
	slackclient "github.com/gammons/slk/internal/slack"
)

type fakeCommandRunner struct {
	got  slackclient.CommandRequest
	err  error
	team string
}

func (f *fakeCommandRunner) RunCommand(_ context.Context, req slackclient.CommandRequest) error {
	f.got = req
	return f.err
}

func (f *fakeCommandRunner) TeamID() string { return f.team }

func TestRunCommandMapsFields(t *testing.T) {
	f := &fakeCommandRunner{team: "T0EXAMPLE01"}
	if err := runCommand(context.Background(), f, "C0EXAMPLE01", "10.0", "/giphy", "cats"); err != nil {
		t.Fatal(err)
	}
	want := slackclient.CommandRequest{ChannelID: "C0EXAMPLE01", ThreadTS: "10.0", Command: "/giphy", Args: "cats", TeamID: "T0EXAMPLE01"}
	if f.got != want {
		t.Errorf("request = %+v, want %+v", f.got, want)
	}
}

func TestRunCommandTranslatesRejection(t *testing.T) {
	f := &fakeCommandRunner{err: &slackclient.CommandRejectedError{Code: "unknown_command"}}
	err := runCommand(context.Background(), f, "C1", "", "/nope", "")
	var ce *core.CommandError
	if !errors.As(err, &ce) || ce.Code != "unknown_command" {
		t.Fatalf("err = %v, want *core.CommandError{unknown_command}", err)
	}
}

func TestRunCommandPassesOtherErrorsThrough(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeCommandRunner{err: boom}
	if err := runCommand(context.Background(), f, "C1", "", "/giphy", ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}
```

- [ ] **Step 6: Run to verify failure**

Run: `go test ./cmd/slk -run 'TestRunCommand' -v`
Expected: build failure — `undefined: runCommand`.

- [ ] **Step 7: Implement `cmd/slk/commands.go` and wire it**

`cmd/slk/commands.go`:

```go
package main

import (
	"context"
	"errors"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	slackclient "github.com/gammons/slk/internal/slack"
)

// commandRunner is the part of *slackclient.Client running a slash
// command needs.
type commandRunner interface {
	RunCommand(ctx context.Context, req slackclient.CommandRequest) error
	TeamID() string
}

// runCommand runs a slash command through client and translates Slack's
// refusal into core.CommandError, so the UI never sees slackclient types.
func runCommand(ctx context.Context, client commandRunner, channelID ids.ChannelID,
	threadTS ids.ThreadTS, command, args string) error {
	err := client.RunCommand(ctx, slackclient.CommandRequest{
		ChannelID: string(channelID),
		ThreadTS:  string(threadTS),
		Command:   command,
		Args:      args,
		TeamID:    client.TeamID(),
	})
	var rej *slackclient.CommandRejectedError
	if errors.As(err, &rej) {
		return &core.CommandError{Code: rej.Code}
	}
	return err
}
```

In `cmd/slk/main.go`, directly after the `app.SetInteractionService(...)` block:

```go
		app.SetCommandService(core.NewCommandService(core.CommandServiceFuncs{
			Run: func(channelID ids.ChannelID, threadTS ids.ThreadTS, command, args string) error {
				wctx := router.Active()
				if wctx == nil {
					return errors.New("no active workspace")
				}
				ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				return runCommand(ctx, wctx.Client, channelID, threadTS, command, args)
			},
		}))
```

In `internal/ui/service_defaults.go`, add to the `var (...)` block:

```go
	noopCommandService     = core.NewCommandService(core.CommandServiceFuncs{})
```

In `internal/ui/app.go`: add the field next to `interactions  core.InteractionService`:

```go
	commandSvc    core.CommandService
```

initialise it in `NewApp` next to `interactions:          noopInteractionService,`:

```go
		commandSvc:            noopCommandService,
```

and add the setter after `SetInteractionService`:

```go
// SetCommandService wires the service slash commands run through. A nil s
// restores the no-op, whose runs fail with errors.ErrUnsupported and so
// surface as a toast.
func (a *App) SetCommandService(s core.CommandService) {
	if s == nil {
		s = noopCommandService
	}
	a.commandSvc = s
}
```

In `internal/ui/services_helpers_test.go`, after `setInteractionPressForTest`:

```go
func (a *App) setCommandRunnerForTest(fn func(ids.ChannelID, ids.ThreadTS, string, string) error) {
	a.SetCommandService(core.NewCommandService(core.CommandServiceFuncs{Run: fn}))
}
```

- [ ] **Step 8: Run to verify everything builds and passes**

Run: `go build ./... && go test ./cmd/slk -run 'TestRunCommand' -v && go test ./internal/core ./internal/ui -run 'TestCommand|TestBoundary|Bubbles' `
Expected: PASS; `boundary_test.go` still passes (the UI only gained a `core` import).

- [ ] **Step 9: Add to `AGENTS.md`**

In "Shared code → Text and rendering", after the `RunCommand` row added in Task 1:

```markdown
| Run a slash command from the UI | `core.CommandService.Run` (App field `a.commandSvc`); wired in `cmd/slk` via `runCommand` to the active workspace's `Client.RunCommand`. Slack's refusal arrives as `*core.CommandError{Code}` |
```

In "Test helpers", after the `setDesktopForTest` sentence of the "Fake one service method" row, add `setCommandRunnerForTest(fn)` to that row's list.

- [ ] **Step 10: Commit**

```bash
gofmt -l internal cmd
git add internal/core cmd/slk/commands.go cmd/slk/commands_test.go cmd/slk/main.go internal/ui/service_defaults.go internal/ui/app.go internal/ui/services_helpers_test.go AGENTS.md
git commit -m "feat(core): CommandService port for slash commands

core.CommandService.Run, wired in cmd/slk to the active workspace's
RunCommand. Slack's refusal becomes core.CommandError so the UI never
sees slackclient types. Nothing calls it yet."
```

---

### Task 3: Enter on `/name args` runs the command (remote path)

This is the first build the owner can run against a real workspace: after this
task `/giphy cats`, `/remind …`, `/shrug …` and app commands work, and `/…` is
no longer posted as text. Built-ins come in Tasks 5–8; until then they also take
this path.

**Files:**
- Create: `internal/ui/slash_command.go`, `internal/ui/reducer_slash_command.go`, `internal/ui/slash_command_test.go`
- Modify: `internal/ui/mode_insert.go` (both Enter branches), `internal/ui/app.go` (reducer chain, after `reduceActionPrompt,` at the `Update` call site)
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: Task 2's `a.commandSvc`, `core.CommandError`, `setCommandRunnerForTest`.
- Produces:
  - `func parseSlashCommand(text string) (name, args string, ok bool)`
  - `func (a *App) trySlashCommand(panel Panel, c *compose.Model, channelID, threadTS string) (tea.Cmd, bool)`
  - `func (a *App) runRemoteCommand(channelID, threadTS, name, args, text string) tea.Cmd`
  - `type commandRanMsg struct { teamID, channelID, threadTS, name, text string; err error }`
  - `var reduceSlashCommand reducerFunc`
  - `func commandFailureText(name string, err error) string`

- [ ] **Step 1: Write the failing parser test**

`internal/ui/slash_command_test.go`:

```go
package ui

import "testing"

func TestParseSlashCommand(t *testing.T) {
	cases := []struct {
		in         string
		name, args string
		ok         bool
	}{
		{"/giphy cats", "/giphy", "cats", true},
		{"/giphy", "/giphy", "", true},
		{"/GIPHY cats", "/giphy", "cats", true},                    // Review Focus 3
		{"/giphy   ", "/giphy", "", true},                          // Review Focus 4
		{"/giphy \t\n ", "/giphy", "", true},                       // whitespace-only args
		{"/shrug line one\nline two", "/shrug", "line one\nline two", true}, // Review Focus 2
		{"/giphy\ncats", "/giphy", "cats", true},
		{"/my-cmd_2 x", "/my-cmd_2", "x", true},
		{"/remind me  to  stretch", "/remind", "me  to  stretch", true}, // inner spacing kept
		{" /giphy cats", "", "", false},                            // Review Focus 1: leading space escapes
		{"/tmp/foo is full", "", "", false},
		{"/ hello", "", "", false},
		{"/", "", "", false},
		{"/giphy: cats", "", "", false},
		{"hello /giphy", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		name, args, ok := parseSlashCommand(tc.in)
		if name != tc.name || args != tc.args || ok != tc.ok {
			t.Errorf("parseSlashCommand(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.in, name, args, ok, tc.name, tc.args, tc.ok)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui -run TestParseSlashCommand -v`
Expected: build failure — `undefined: parseSlashCommand`.

- [ ] **Step 3: Implement the parser**

`internal/ui/slash_command.go`:

```go
// internal/ui/slash_command.go
//
// Slash commands typed in a composer (docs/superpowers/specs/
// 2026-10-12-slash-commands-design.md §3). Enter on text that starts
// with "/name" runs the command instead of posting the text.
package ui

import "strings"

// parseSlashCommand splits composer text into a slash command's name and
// arguments. ok is false for anything that is not a command, which then
// sends as an ordinary message: text not starting with "/" (a leading
// space is the escape hatch), "/" alone, or a first token with anything
// but [A-Za-z0-9_-] after the slash ("/tmp/foo", "/giphy:"). name is
// lowercased; args keeps inner whitespace and newlines and is "" when
// only whitespace follows the name.
func parseSlashCommand(text string) (name, args string, ok bool) {
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	end := 1
	for end < len(text) && isCommandNameByte(text[end]) {
		end++
	}
	if end == 1 {
		return "", "", false
	}
	if end < len(text) && !isCommandSpace(text[end]) {
		return "", "", false
	}
	name = strings.ToLower(text[:end])
	args = strings.TrimLeft(text[end:], " \t\r\n")
	if strings.TrimSpace(args) == "" {
		args = ""
	}
	return name, args, true
}

func isCommandNameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}

func isCommandSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
```

- [ ] **Step 4: Run to verify the parser passes**

Run: `go test ./internal/ui -run TestParseSlashCommand -v`
Expected: PASS.

- [ ] **Step 5: Write the failing App tests**

Replace the test file's `import "testing"` with:

```go
import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)
```

and append:

```go
// runCall records one CommandService.Run.
type runCall struct {
	channelID, threadTS, name, args string
}

// slashApp is an App in insert mode on C1 in team T1, its channel
// composer bound to (T1, C1), whose command runs are recorded and
// answered with err.
func slashApp(t *testing.T, err error) (*App, *[]runCall) {
	t.Helper()
	a := newTestApp(t, append(insertOpts(), withActiveTeam("T1"), withMode(ModeInsert))...)
	a.focusedPanel = PanelMessages
	a.compose.SetDraftContext("T1", "C1", "")
	calls := &[]runCall{}
	a.setCommandRunnerForTest(func(ch ids.ChannelID, th ids.ThreadTS, name, args string) error {
		*calls = append(*calls, runCall{string(ch), string(th), name, args})
		return err
	})
	return a, calls
}

// pressEnter sends Enter through the real Update chain.
func pressEnter(a *App) tea.Cmd {
	_, cmd := a.Update(keyCode(tea.KeyEnter))
	return cmd
}

func TestSlashCommand_ChannelComposerRunsRemote(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue("/giphy cats")

	cmd := pressEnter(a)
	ran := deliveredMsgs[commandRanMsg](t, cmd)
	if len(ran) != 1 {
		t.Fatalf("commandRanMsg delivered %d times, want 1", len(ran))
	}
	want := runCall{channelID: "C1", threadTS: "", name: "/giphy", args: "cats"}
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Fatalf("Run calls = %+v, want [%+v]", *calls, want)
	}
	if got := a.compose.Value(); got != "" {
		t.Errorf("compose = %q, want cleared", got)
	}
	if a.mode != ModeNormal {
		t.Errorf("mode = %v, want ModeNormal (left insert like a send)", a.mode)
	}
}

func TestSlashCommand_ThreadComposerRunsInThread(t *testing.T) {
	a, calls := slashApp(t, nil)
	showThread(t, a)
	a.threadCompose.SetValue("/giphy cats")

	cmd := pressEnter(a)
	_ = deliveredMsgs[commandRanMsg](t, cmd)
	want := runCall{channelID: "C1", threadTS: "10.0", name: "/giphy", args: "cats"}
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Fatalf("Run calls = %+v, want [%+v]", *calls, want)
	}
	if got := a.threadCompose.Value(); got != "" {
		t.Errorf("thread compose = %q, want cleared", got)
	}
}

func TestSlashCommand_UpperCaseNameRunsLowercased(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue("/GIPHY cats")
	_ = deliveredMsgs[commandRanMsg](t, pressEnter(a))
	if len(*calls) != 1 || (*calls)[0].name != "/giphy" {
		t.Fatalf("Run calls = %+v, want one /giphy", *calls)
	}
}

func TestSlashCommand_MultiLineArgsKeepNewline(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue("/shrug line one\nline two")
	_ = deliveredMsgs[commandRanMsg](t, pressEnter(a))
	if len(*calls) != 1 || (*calls)[0].args != "line one\nline two" {
		t.Fatalf("Run calls = %+v, want args with the newline", *calls)
	}
}

func TestSlashCommand_TrailingSpacesMeanNoArgs(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue("/giphy   ")
	_ = deliveredMsgs[commandRanMsg](t, pressEnter(a))
	if len(*calls) != 1 || (*calls)[0].args != "" {
		t.Fatalf("Run calls = %+v, want args \"\"", *calls)
	}
}

func TestSlashCommand_LeadingSpaceSendsAsMessage(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue(" /giphy cats")
	sent := deliveredMsgs[SendMessageMsg](t, pressEnter(a))
	if len(sent) != 1 || sent[0].Text != " /giphy cats" {
		t.Fatalf("SendMessageMsg = %+v, want the literal text", sent)
	}
	if len(*calls) != 0 {
		t.Errorf("Run called %d times, want 0", len(*calls))
	}
}

func TestSlashCommand_PathLikeTextSendsAsMessage(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue("/tmp/foo is full")
	sent := deliveredMsgs[SendMessageMsg](t, pressEnter(a))
	if len(sent) != 1 {
		t.Fatalf("SendMessageMsg delivered %d times, want 1", len(sent))
	}
	if len(*calls) != 0 {
		t.Errorf("Run called %d times, want 0", len(*calls))
	}
}

func TestSlashCommand_EditModeSavesLiterally(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.editing.Begin("C1", "1.0", PanelMessages, "")
	a.compose.SetValue("/giphy cats")
	edits := deliveredMsgs[EditMessageMsg](t, pressEnter(a))
	if len(edits) != 1 || edits[0].NewText != "/giphy cats" {
		t.Fatalf("EditMessageMsg = %+v, want the literal edit", edits)
	}
	if len(*calls) != 0 {
		t.Errorf("Run called %d times, want 0", len(*calls))
	}
}

func TestSlashCommand_AttachmentsRefused(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.AddAttachment(core.PendingAttachment{Filename: "cat.png", Bytes: []byte{1}})
	a.compose.SetValue("/giphy cats")
	_ = pressEnter(a)
	if len(*calls) != 0 {
		t.Errorf("Run called %d times, want 0", len(*calls))
	}
	if got := a.compose.Value(); got != "/giphy cats" {
		t.Errorf("compose = %q, want the text kept", got)
	}
	if n := len(a.compose.Attachments()); n != 1 {
		t.Errorf("attachments = %d, want 1 kept", n)
	}
	if out := statusbarText(a); !strings.Contains(out, "Files can't be sent with a command") {
		t.Errorf("status bar = %q, want the refusal toast", out)
	}
}

func TestSlashCommand_FailureToasts(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&core.CommandError{Code: "unknown_command"}, "/giphy is not a valid command"},
		{&core.CommandError{Code: "restricted_action"}, "/giphy failed: restricted_action"},
		{errors.ErrUnsupported, "/giphy failed: not available"},
		{errors.New("calling chat.command: timeout"), "/giphy failed: calling chat.command: timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			a, _ := slashApp(t, tc.err)
			a.compose.SetValue("/giphy cats")
			ran := deliveredMsgs[commandRanMsg](t, pressEnter(a))
			a.Update(ran[0])
			if out := statusbarText(a); !strings.Contains(out, tc.want) {
				t.Errorf("status bar = %q, want %q", out, tc.want)
			}
		})
	}
}

// The GIF a command posts arrives with user = you and bot_id = the app
// (ws_posted_gif.json). A command never marks the channel in flight, so
// reduceNewMessage's self-send guards must not swallow it.
func TestSlashCommand_PostedResultIsNotSwallowedAsSelfSend(t *testing.T) {
	a, _ := slashApp(t, nil)
	a.currentUserID = "U0EXAMPLE01"
	a.compose.SetValue("/giphy cats")
	ran := deliveredMsgs[commandRanMsg](t, pressEnter(a))
	a.Update(ran[0])
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "99.000100", UserID: "U0EXAMPLE01", BotID: "B0EXAMPLE01", Text: "cats",
	}})
	msgs := a.messagepane.Messages()
	if len(msgs) == 0 || msgs[len(msgs)-1].TS != "99.000100" {
		t.Fatalf("last message = %+v, want the posted GIF 99.000100", msgs[len(msgs)-1:])
	}
}

func TestSlashCommand_SuccessIsSilent(t *testing.T) {
	a, _ := slashApp(t, nil)
	a.compose.SetValue("/giphy cats")
	ran := deliveredMsgs[commandRanMsg](t, pressEnter(a))
	a.Update(ran[0])
	if out := statusbarText(a); strings.Contains(out, "/giphy") {
		t.Errorf("status bar = %q, want no toast on success", out)
	}
}
```

- [ ] **Step 6: Run to verify failure**

Run: `go test ./internal/ui -run TestSlashCommand -v`
Expected: build failure — `undefined: commandRanMsg`.

- [ ] **Step 7: Implement the remote path**

Append to `internal/ui/slash_command.go` (and extend its imports to
`"strings"`, `"time"`, `tea "charm.land/bubbletea/v2"`,
`"github.com/gammons/slk/internal/ids"`, `"github.com/gammons/slk/internal/ui/compose"`):

```go
// commandRanMsg is the result of running a slash command through
// CommandService. text is the composer text as typed, kept so a failure
// can put it back (Task 4); teamID/channelID/threadTS identify the
// conversation it was typed in.
type commandRanMsg struct {
	teamID, channelID, threadTS string
	name                        string
	text                        string
	err                         error
}

// trySlashCommand runs c's text as a slash command when it is one,
// called by both Enter branches of handleInsertMode before anything else
// is sent. It reports false for ordinary text and while panel is editing
// a message (an edit is saved as written), and the caller's send path
// runs unchanged. With files attached nothing is sent: a command cannot
// carry them.
func (a *App) trySlashCommand(panel Panel, c *compose.Model, channelID, threadTS string) (tea.Cmd, bool) {
	if a.editing.IsActive() && a.editing.Panel() == panel {
		return nil, false
	}
	raw := c.Value()
	name, _, ok := parseSlashCommand(raw)
	if !ok {
		return nil, false
	}
	if len(c.Attachments()) > 0 {
		return toastWithClear(a, "Files can't be sent with a command", 3*time.Second), true
	}
	// Translate @name/#name to wire form, as a send does. The name is
	// untouched by translation, so re-parsing yields the wire-form args.
	_, args, _ := parseSlashCommand(c.TranslateMentionsForSend(raw))
	c.Reset()
	a.exitInsertAfterSend()
	return tea.Batch(a.runRemoteCommand(channelID, threadTS, name, args, raw), a.tryOpenActionPrompt()), true
}

// runRemoteCommand runs name through CommandService off the Update
// goroutine. What the command produces arrives over the WebSocket; only
// the error comes back, in commandRanMsg.
func (a *App) runRemoteCommand(channelID, threadTS, name, args, text string) tea.Cmd {
	svc, teamID := a.commandSvc, a.activeTeamID
	return func() tea.Msg {
		err := svc.Run(ids.ChannelID(channelID), ids.ThreadTS(threadTS), name, args)
		return commandRanMsg{teamID: teamID, channelID: channelID, threadTS: threadTS, name: name, text: text, err: err}
	}
}
```

`internal/ui/reducer_slash_command.go`:

```go
// internal/ui/reducer_slash_command.go
//
// Result of a slash command run through CommandService.
package ui

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
)

var reduceSlashCommand reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(commandRanMsg)
	if !ok {
		return nil, false
	}
	if m.err == nil {
		return nil, true
	}
	return toastWithClear(a, commandFailureText(m.name, m.err), 4*time.Second), true
}

// commandFailureText is the toast for a command that did not run.
func commandFailureText(name string, err error) string {
	var ce *core.CommandError
	switch {
	case errors.As(err, &ce) && ce.Code == "unknown_command":
		return name + " is not a valid command"
	case errors.As(err, &ce):
		return name + " failed: " + ce.Code
	case errors.Is(err, errors.ErrUnsupported):
		return name + " failed: not available"
	default:
		return name + " failed: " + err.Error()
	}
}
```

Register it in the reducer chain in `internal/ui/app.go`, directly after
`reduceActionPrompt,`:

```go
		reduceSlashCommand,
```

In `internal/ui/mode_insert.go`, in the **thread** branch, insert before
`if len(a.threadCompose.Attachments()) > 0 {` (inside `if isSend {`):

```go
			if cmd, ok := a.trySlashCommand(PanelThread, &a.threadCompose, a.threadPanel.ChannelID(), a.threadPanel.ThreadTS()); ok {
				return cmd
			}
```

and in the **channel** branch, before `if len(a.compose.Attachments()) > 0 {`:

```go
		if cmd, ok := a.trySlashCommand(PanelMessages, &a.compose, a.activeChannelID, ""); ok {
			return cmd
		}
```

- [ ] **Step 8: Run to verify the tests pass**

Run: `go test ./internal/ui -run 'TestSlashCommand|TestParseSlashCommand' -v`
Expected: PASS.
Then the neighbours: `go test ./internal/ui -run 'Insert|Send|Edit|ActionPrompt'`
Expected: PASS (nothing that is not a command changed).

- [ ] **Step 9: Try it**

```bash
go build -o /tmp/slk ./cmd/slk && /tmp/slk
```

In a test channel: `/giphy cats` shows the preview ephemeral (its buttons can't be
pressed until stage 2); `/shrug hi` posts "hi ¯\_(ツ)_/¯"; `/remind me test in 1
minute` shows Slackbot's ephemeral; `/notarealcommand` toasts "/notarealcommand
is not a valid command"; ` /giphy cats` (leading space) posts the text.

- [ ] **Step 10: Add to `AGENTS.md`**

"Shared code → App state and actions", new row:

```markdown
| Is composer text a slash command, and its name/args? | `parseSlashCommand(text)` (`internal/ui/slash_command.go`); `(*App).trySlashCommand(panel, c, channelID, threadTS)` runs it from either Enter branch. The result is `commandRanMsg`, reduced in `reducer_slash_command.go`; `commandFailureText` words the toast |
```

"Test helpers", new row:

```markdown
| An `App` in insert mode on C1/T1 whose slash-command runs are recorded and answered with an error you choose; press Enter through `Update` | `slashApp(t, err)`, `pressEnter(a)`, the `runCall` record (`internal/ui/slash_command_test.go`) |
```

- [ ] **Step 11: Commit**

```bash
gofmt -l internal/ui
git add internal/ui/slash_command.go internal/ui/slash_command_test.go internal/ui/reducer_slash_command.go internal/ui/mode_insert.go internal/ui/app.go AGENTS.md
git commit -m "feat(ui): Enter on /name runs the slash command

Both composers: text starting with /name runs through CommandService
instead of being posted. A leading space, /tmp/paths and edits are
unchanged; files cannot ride along. Failures toast; unknown_command
reads as \"/x is not a valid command\". Fixes the literal-text post
reported in #247."
```

---

### Task 4: `OfferDraft`, and putting a failed command back

**Files:**
- Modify: `internal/ui/compose/model.go` (after `restoreDraft`), `internal/ui/compose/draft_test.go`
- Modify: `internal/ui/reducer_slash_command.go`, `internal/ui/slash_command_test.go`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: Task 3's `commandRanMsg`, `slashApp`, `pressEnter`.
- Produces: `func (m *Model) OfferDraft(teamID, channelID, threadTS, text string) bool` in package `compose` (Task 8 uses it for `/msg`).

- [ ] **Step 1: Write the failing compose tests**

Append to `internal/ui/compose/draft_test.go`:

```go
func TestOfferDraftFillsTheEmptyVisibleComposer(t *testing.T) {
	m := New("general")
	m.SetDraftContext("T1", "C1", "")
	if !m.OfferDraft("T1", "C1", "", "/giphy cats") {
		t.Fatal("OfferDraft = false, want true for the empty visible conversation")
	}
	if got := m.Value(); got != "/giphy cats" {
		t.Errorf("value = %q, want the offered text", got)
	}
}

func TestOfferDraftKeepsNewerVisibleText(t *testing.T) {
	m := New("general")
	m.SetDraftContext("T1", "C1", "")
	m.SetValue("something newer")
	if m.OfferDraft("T1", "C1", "", "/giphy cats") {
		t.Fatal("OfferDraft = true, want false over existing text")
	}
	if got := m.Value(); got != "something newer" {
		t.Errorf("value = %q, want the newer text kept", got)
	}
}

func TestOfferDraftKeepsVisibleAttachments(t *testing.T) {
	m := New("general")
	m.SetDraftContext("T1", "C1", "")
	m.AddAttachment(core.PendingAttachment{Filename: "a.png", Bytes: []byte{1}})
	if m.OfferDraft("T1", "C1", "", "/giphy cats") {
		t.Fatal("OfferDraft = true, want false while attachments are pending")
	}
}

func TestOfferDraftStoresForAnInactiveConversation(t *testing.T) {
	m := New("general")
	m.SetDraftContext("T1", "C2", "")
	if !m.OfferDraft("T1", "C1", "", "/giphy cats") {
		t.Fatal("OfferDraft = false, want true for an inactive conversation with no draft")
	}
	if got := m.Value(); got != "" {
		t.Errorf("visible value = %q, want untouched", got)
	}
	m.SetDraftContext("T1", "C1", "")
	if got := m.Value(); got != "/giphy cats" {
		t.Errorf("after switching back: value = %q, want the stored draft", got)
	}
}

func TestOfferDraftKeepsAnInactiveDraft(t *testing.T) {
	m := New("general")
	m.SetDraftContext("T1", "C1", "")
	m.SetValue("half a thought")
	m.SetDraftContext("T1", "C2", "")
	if m.OfferDraft("T1", "C1", "", "/giphy cats") {
		t.Fatal("OfferDraft = true, want false over a stored draft")
	}
	m.SetDraftContext("T1", "C1", "")
	if got := m.Value(); got != "half a thought" {
		t.Errorf("value = %q, want the stored draft kept", got)
	}
}

func TestOfferDraftThreadKeyIsDistinct(t *testing.T) {
	m := New("general")
	m.SetDraftContext("T1", "C1", "10.0")
	if !m.OfferDraft("T1", "C1", "10.0", "/giphy cats") {
		t.Fatal("OfferDraft = false, want true for the visible thread")
	}
	if got := m.Value(); got != "/giphy cats" {
		t.Errorf("value = %q, want the offered text", got)
	}
}

func TestOfferDraftRejectsEmpty(t *testing.T) {
	m := New("general")
	m.SetDraftContext("T1", "C1", "")
	if m.OfferDraft("T1", "C1", "", "") || m.OfferDraft("T1", "", "", "x") {
		t.Fatal("OfferDraft = true for empty text or channel, want false")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/compose -run TestOfferDraft -v`
Expected: build failure — `m.OfferDraft undefined`.

- [ ] **Step 3: Implement `OfferDraft`**

In `internal/ui/compose/model.go`, after `restoreDraft`:

```go
// OfferDraft makes text the draft of the conversation (teamID,
// channelID, threadTS) — the visible composer when that is the
// conversation it is bound to, otherwise the stored inactive draft — but
// only when that conversation has no draft (no text, no attachments), so
// it never overwrites something newer. Reports whether it wrote. Puts a
// failed slash command back, and pre-fills /msg's text.
func (m *Model) OfferDraft(teamID, channelID, threadTS, text string) bool {
	if channelID == "" || text == "" {
		return false
	}
	key := draftKey{teamID: teamID, channelID: channelID, threadTS: threadTS}
	if key == m.activeKey {
		if m.input.Value() != "" || len(m.pending) > 0 {
			return false
		}
		m.restoreDraft(draft{text: text})
		return true
	}
	if d, ok := m.drafts[key]; ok && !d.isEmpty() {
		return false
	}
	m.drafts[key] = draft{text: text}
	return true
}
```

- [ ] **Step 4: Run to verify the compose tests pass**

Run: `go test ./internal/ui/compose -v -run 'TestOfferDraft|TestSetDraftContext'`
Expected: PASS.

- [ ] **Step 5: Write the failing App tests**

Append to `internal/ui/slash_command_test.go`:

```go
func TestSlashCommand_FailurePutsTextBack(t *testing.T) {
	a, _ := slashApp(t, &core.CommandError{Code: "unknown_command"})
	a.compose.SetValue("/notarealcommand hi")
	ran := deliveredMsgs[commandRanMsg](t, pressEnter(a))
	a.Update(ran[0])
	if got := a.compose.Value(); got != "/notarealcommand hi" {
		t.Errorf("compose = %q, want the command put back", got)
	}
}

func TestSlashCommand_FailurePutsTypedFormBack(t *testing.T) {
	// The composer shows what the user typed (@alice), not the wire
	// form (<@U9>) the command was run with.
	a, calls := slashApp(t, &core.CommandError{Code: "restricted_action"})
	a.compose.SetUsers([]mentionpicker.User{{ID: "U9", DisplayName: "alice", Username: "alice", InChannel: true}})
	a.compose.SetValue("/invite @alice")
	ran := deliveredMsgs[commandRanMsg](t, pressEnter(a))
	if (*calls)[0].args != "<@U9>" {
		t.Fatalf("args = %q, want the wire form <@U9>", (*calls)[0].args)
	}
	a.Update(ran[0])
	if got := a.compose.Value(); got != "/invite @alice" {
		t.Errorf("compose = %q, want the typed form", got)
	}
}

func TestSlashCommand_FailureKeepsNewerText(t *testing.T) {
	a, _ := slashApp(t, &core.CommandError{Code: "unknown_command"})
	a.compose.SetValue("/notarealcommand")
	ran := deliveredMsgs[commandRanMsg](t, pressEnter(a))
	a.compose.SetValue("typed while it ran")
	a.Update(ran[0])
	if got := a.compose.Value(); got != "typed while it ran" {
		t.Errorf("compose = %q, want the newer text kept", got)
	}
}

// Review Focus 5: the user moved to another conversation before the
// failure landed. The command becomes C1's stored draft; C2's composer
// is untouched.
func TestSlashCommand_FailureAfterMovingAwayStoresTheDraft(t *testing.T) {
	a, _ := slashApp(t, &core.CommandError{Code: "unknown_command"})
	a.compose.SetValue("/notarealcommand")
	ran := deliveredMsgs[commandRanMsg](t, pressEnter(a))
	a.compose.SetDraftContext("T1", "C2", "")
	a.Update(ran[0])
	if got := a.compose.Value(); got != "" {
		t.Errorf("C2 compose = %q, want untouched", got)
	}
	a.compose.SetDraftContext("T1", "C1", "")
	if got := a.compose.Value(); got != "/notarealcommand" {
		t.Errorf("back in C1: compose = %q, want the stored command", got)
	}
}

func TestSlashCommand_ThreadFailurePutsTextBackInThread(t *testing.T) {
	a, _ := slashApp(t, &core.CommandError{Code: "unknown_command"})
	showThread(t, a)
	a.threadCompose.SetDraftContext("T1", "C1", "10.0")
	a.threadCompose.SetValue("/notarealcommand")
	ran := deliveredMsgs[commandRanMsg](t, pressEnter(a))
	a.Update(ran[0])
	if got := a.threadCompose.Value(); got != "/notarealcommand" {
		t.Errorf("thread compose = %q, want the command put back", got)
	}
	if got := a.compose.Value(); got != "" {
		t.Errorf("channel compose = %q, want untouched", got)
	}
}
```

Add `"github.com/gammons/slk/internal/ui/mentionpicker"` to the test file's
imports.

- [ ] **Step 6: Run to verify failure**

Run: `go test ./internal/ui -run 'TestSlashCommand_Failure|TestSlashCommand_Thread' -v`
Expected: FAIL — the composer stays empty after a failure.

- [ ] **Step 7: Put the text back**

In `internal/ui/reducer_slash_command.go`, replace the error return of
`reduceSlashCommand` with:

```go
	// Put the command back where it was typed, unless that conversation
	// has a draft again by now. A thread command goes back to the
	// thread composer, keyed by its thread; anything else to the
	// channel composer.
	c := &a.compose
	if m.threadTS != "" {
		c = &a.threadCompose
	}
	c.OfferDraft(m.teamID, m.channelID, m.threadTS, m.text)
	return toastWithClear(a, commandFailureText(m.name, m.err), 4*time.Second), true
```

- [ ] **Step 8: Run to verify the tests pass**

Run: `go test ./internal/ui -run 'TestSlashCommand|TestParseSlashCommand' -v && go test ./internal/ui/compose`
Expected: PASS.

- [ ] **Step 9: Add to `AGENTS.md`**

"Shared code → UI chrome", next to the `SetDraftContext` row:

```markdown
| Put text into a conversation's draft without overwriting anything newer (visible composer or stored draft) | `(*compose.Model).OfferDraft(teamID, channelID, threadTS, text)`; false when that conversation already has text or attachments. Used for a failed slash command and `/msg`'s pre-fill |
```

- [ ] **Step 10: Commit**

```bash
gofmt -l internal/ui
git add internal/ui/compose/model.go internal/ui/compose/draft_test.go internal/ui/reducer_slash_command.go internal/ui/slash_command_test.go AGENTS.md
git commit -m "feat(ui): a failed slash command goes back into its composer

compose.OfferDraft makes text a conversation's draft only when that
conversation has none, visible or stored, so a failure never overwrites
something newer and lands in the right conversation after a switch."
```

---

### Task 5: one command dispatcher for `:` and `/`

A behaviour-preserving refactor plus an empty hook: after it, `:` commands work
exactly as before and every slash command still takes the remote path, but
built-ins (Tasks 6–8) only need a table entry.

**Files:**
- Modify: `internal/ui/command.go`, `internal/ui/command_test.go`, `internal/ui/slash_command.go`, `internal/ui/slash_command_test.go`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: Task 3's `trySlashCommand`, `runRemoteCommand`, `parseSlashCommand`.
- Produces:
  - `type commandArgs struct { Fields []string; Rest string }`
  - `type commandFunc func(a *App, args commandArgs) tea.Cmd` (was `func(a *App, args []string) tea.Cmd`)
  - `func dispatchCommand(a *App, table map[string]commandFunc, line string, unknown func(a *App, name string, args commandArgs) tea.Cmd) tea.Cmd`
  - `var slashCommands = map[string]commandFunc{}` — keys include the slash, lowercased (`"/away"`)

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/command_test.go` (add `tea "charm.land/bubbletea/v2"` to
its imports):

```go
func TestDispatchCommand_FieldsAndRest(t *testing.T) {
	var got commandArgs
	table := map[string]commandFunc{
		"echo": func(_ *App, args commandArgs) tea.Cmd { got = args; return nil },
	}
	_ = dispatchCommand(NewApp(), table, "  echo  one  two\nthree ", nil)
	if strings.Join(got.Fields, "|") != "one|two|three" {
		t.Errorf("Fields = %q, want [one two three]", got.Fields)
	}
	if got.Rest != "one  two\nthree " {
		t.Errorf("Rest = %q, want the raw remainder with inner whitespace kept", got.Rest)
	}
}

func TestDispatchCommand_UnknownFallback(t *testing.T) {
	var gotName string
	var gotArgs commandArgs
	_ = dispatchCommand(NewApp(), map[string]commandFunc{}, "nope a b", func(_ *App, name string, args commandArgs) tea.Cmd {
		gotName, gotArgs = name, args
		return nil
	})
	if gotName != "nope" || gotArgs.Rest != "a b" {
		t.Errorf("unknown(%q, %+v), want (nope, Rest \"a b\")", gotName, gotArgs)
	}
}

func TestDispatchCommand_EmptyIsNoop(t *testing.T) {
	called := false
	cmd := dispatchCommand(NewApp(), map[string]commandFunc{}, " \n ", func(*App, string, commandArgs) tea.Cmd {
		called = true
		return nil
	})
	if cmd != nil || called {
		t.Error("empty line should neither return a cmd nor reach unknown")
	}
}
```

Append to `internal/ui/slash_command_test.go`:

```go
// withSlashBuiltin registers fn under name for one test.
func withSlashBuiltin(t *testing.T, name string, fn commandFunc) {
	t.Helper()
	prev, had := slashCommands[name]
	slashCommands[name] = fn
	t.Cleanup(func() {
		if had {
			slashCommands[name] = prev
		} else {
			delete(slashCommands, name)
		}
	})
}

func TestSlashCommand_BuiltinRunsInsteadOfRemote(t *testing.T) {
	a, calls := slashApp(t, nil)
	var got commandArgs
	withSlashBuiltin(t, "/testbuiltin", func(_ *App, args commandArgs) tea.Cmd {
		got = args
		return nil
	})
	a.compose.SetValue("/TestBuiltin  hello\nworld")
	_ = pressEnter(a)
	if len(*calls) != 0 {
		t.Errorf("Run called %d times, want 0 for a built-in", len(*calls))
	}
	if got.Rest != "hello\nworld" {
		t.Errorf("built-in Rest = %q, want \"hello\\nworld\"", got.Rest)
	}
	if a.compose.Value() != "" || a.mode != ModeNormal {
		t.Errorf("compose = %q, mode = %v; want cleared and ModeNormal", a.compose.Value(), a.mode)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui -run 'TestDispatchCommand|TestSlashCommand_Builtin' -v`
Expected: build failure — `undefined: dispatchCommand`, `commandArgs`, `slashCommands`.

- [ ] **Step 3: Generalise the registry**

In `internal/ui/command.go`:

Replace the `commandFunc` declaration and its comment with:

```go
// commandArgs is a command line after its name. Fields is the remainder
// split on whitespace, which most handlers want; Rest is the remainder
// as typed, inner whitespace and newlines intact, for a handler that
// passes text on (/msg).
type commandArgs struct {
	Fields []string
	Rest   string
}

// commandFunc executes a named command. Both registries use it: the
// vi-style ":" commands below and the slash built-ins (slashCommands).
type commandFunc func(a *App, args commandArgs) tea.Cmd
```

Change each handler's parameter from `_ []string` to `_ commandArgs`
(`cmdActivity`, `cmdSplit`, `cmdVSplit`, `cmdCloseWindow`, `cmdOnlyWindow`,
`cmdWorkspaceFinder`, `cmdMarks`), and in `cmdDelMarks` change the parameter to
`args commandArgs` and its loop to `for _, arg := range args.Fields {`.

Replace `executeCommand` with:

```go
// dispatchCommand runs line's command from table: its first
// whitespace-separated token names the command, the rest are its args.
// unknown runs when table has no such name. Empty input is a no-op.
func dispatchCommand(a *App, table map[string]commandFunc, line string,
	unknown func(a *App, name string, args commandArgs) tea.Cmd) tea.Cmd {
	trimmed := strings.TrimLeft(line, " \t\r\n")
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil
	}
	name := fields[0]
	args := commandArgs{
		Fields: fields[1:],
		Rest:   strings.TrimLeft(trimmed[len(name):], " \t\r\n"),
	}
	if fn, ok := table[name]; ok {
		return fn(a, args)
	}
	return unknown(a, name, args)
}

// executeCommand parses and runs one ":" command line (without the
// leading ':'). Empty input is a no-op; unknown commands show a
// transient status-bar toast.
func executeCommand(a *App, line string) tea.Cmd {
	return dispatchCommand(a, commands, line, func(a *App, name string, _ commandArgs) tea.Cmd {
		return toastWithClear(a, "Unknown command: "+name, 2*time.Second)
	})
}
```

Update the file's header comment: it is now "The command registries: vi-style
`:` commands (window-management design §5) and the shared dispatcher slash
built-ins use (slash_command.go)."

In `internal/ui/slash_command.go`, add the table:

```go
// slashCommands are the slash built-ins slk runs itself, because Slack's
// web client handles them locally rather than through chat.command
// (spec §3.3). Keys are lowercased names with their slash. Anything not
// here runs remotely.
var slashCommands = map[string]commandFunc{}
```

and in `trySlashCommand` replace

```go
	return tea.Batch(a.runRemoteCommand(channelID, threadTS, name, args, raw), a.tryOpenActionPrompt()), true
```

with

```go
	run := dispatchCommand(a, slashCommands, name+" "+args, func(a *App, name string, args commandArgs) tea.Cmd {
		return a.runRemoteCommand(channelID, threadTS, name, args.Rest, raw)
	})
	return tea.Batch(run, a.tryOpenActionPrompt()), true
```

(`name` is already lowercased by `parseSlashCommand`; joining it to `args`
with one space makes `dispatchCommand`'s `Rest` exactly `args`.)

- [ ] **Step 4: Run to verify the tests pass and `:` is unchanged**

Run: `go test ./internal/ui -run 'TestDispatchCommand|TestExecuteCommand|Command|TestSlashCommand|Windows|Marks' -v`
Expected: PASS, including every pre-existing `TestExecuteCommand_*`,
`command_marks_test.go` and `windows_test.go` case.

- [ ] **Step 5: Add to `AGENTS.md`**

"Shared code → App state and actions", new row:

```markdown
| Add a command | `:` commands: an entry in `commands` (`internal/ui/command.go`); slash built-ins slk runs itself: an entry in `slashCommands` (`internal/ui/slash_command.go`). Both are `commandFunc(a, commandArgs{Fields, Rest})`, dispatched by `dispatchCommand`; a slash name not in the table runs through `chat.command` |
```

- [ ] **Step 6: Commit**

```bash
gofmt -l internal/ui
git add internal/ui/command.go internal/ui/command_test.go internal/ui/slash_command.go internal/ui/slash_command_test.go AGENTS.md
git commit -m "refactor(ui): one command dispatcher for : and slash built-ins

commandFunc takes commandArgs{Fields, Rest} so a handler can pass text
on with its whitespace intact. executeCommand is dispatchCommand over
the : table with its unknown-command toast, unchanged. Slash commands
dispatch over slashCommands, empty for now, falling back to the remote
run, so behaviour is unchanged too."
```

---

### Task 6: `/away`, `/active`, `/status away|active`

Two commits: the extraction (no behaviour change), then the built-ins.

**Files:**
- Create: `internal/ui/presence_action.go`, `internal/ui/slash_builtins.go`, `internal/ui/slash_builtins_test.go`
- Modify: `internal/ui/mode_presence_menu.go` (lines 46–52 today), `internal/ui/mode_presence_snooze.go` (lines 37–41 today), `internal/ui/mode_presence_snooze_test.go` (one comment), `internal/ui/slash_command.go` (table)
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: Task 5's `slashCommands`, `commandArgs`; `a.presence.Status(teamID)`; existing test helpers `setStatusSetterForTest`, `statusCall`, `statusbarText`.
- Produces:
  - `func (a *App) applyPresenceAction(action core.PresenceAction, snoozeMinutes int)`
  - `func slashAway(a *App, _ commandArgs) tea.Cmd`, `slashActive`, `slashStatus`
  - `func slashUnsupported(name string) commandFunc`

- [ ] **Step 1: Extract `applyPresenceAction`**

`internal/ui/presence_action.go`:

```go
package ui

import "github.com/gammons/slk/internal/core"

// applyPresenceAction applies a presence choice for the workspace on
// screen. Optimistic: the cached status and the status bar change at
// once, before the API call returns; the WebSocket echo reaffirms them.
// The presence menu, its custom snooze and the /away family share it.
func (a *App) applyPresenceAction(action core.PresenceAction, snoozeMinutes int) {
	st := a.presence.Apply(a.activeTeamID, action, snoozeMinutes)
	a.statusbar.SetStatus(st.Presence, st.DNDEnabled, st.DNDEndTS)
	if a.presenceSvc != nil {
		a.presenceSvc.SetStatus(action, snoozeMinutes)
	}
}
```

In `internal/ui/mode_presence_menu.go` replace

```go
		// Optimistic UI: update local state + status bar before the API
		// call returns. The WS echo will reaffirm it.
		st := a.presence.Apply(a.activeTeamID, result.Action, result.SnoozeMinutes)
		a.statusbar.SetStatus(st.Presence, st.DNDEnabled, st.DNDEndTS)
		if a.presenceSvc != nil {
			a.presenceSvc.SetStatus(result.Action, result.SnoozeMinutes)
		}
```

with

```go
		a.applyPresenceAction(result.Action, result.SnoozeMinutes)
```

In `internal/ui/mode_presence_snooze.go` replace

```go
		st := a.presence.Apply(a.activeTeamID, presencemenu.ActionSnooze, mins)
		a.statusbar.SetStatus(st.Presence, st.DNDEnabled, st.DNDEndTS)
		if a.presenceSvc != nil {
			a.presenceSvc.SetStatus(presencemenu.ActionSnooze, mins)
		}
```

with

```go
		a.applyPresenceAction(presencemenu.ActionSnooze, mins)
```

Remove any import either file no longer uses (`go build` will say). In
`mode_presence_snooze_test.go`, the comment on `snoozeSetupNoSetter` names
"the handler's `if a.presenceSvc != nil` guard (mode_presence_snooze.go:41)";
change it to "the `if a.presenceSvc != nil` guard in `applyPresenceAction`
(presence_action.go)". No test logic changes.

- [ ] **Step 2: Verify the extraction changed nothing**

Run: `go build ./... && go test ./internal/ui -run 'Presence|Snooze|Status' -v`
Expected: PASS, with no test edited beyond the comment.

- [ ] **Step 3: Add to `AGENTS.md` and commit the extraction**

"Shared code → App state and actions", new row:

```markdown
| Set the user's own presence or DND from the UI (optimistic cache + status bar, then the service) | `(*App).applyPresenceAction(action, snoozeMinutes)` (`internal/ui/presence_action.go`); the presence menu, custom snooze and `/away` use it |
```

```bash
gofmt -l internal/ui
git add internal/ui/presence_action.go internal/ui/mode_presence_menu.go internal/ui/mode_presence_snooze.go internal/ui/mode_presence_snooze_test.go AGENTS.md
git commit -m "refactor(ui): extract applyPresenceAction

The presence menu and its custom snooze each inlined the same
apply/status-bar/service triple. No behaviour change."
```

- [ ] **Step 4: Write the failing built-in tests**

`internal/ui/slash_builtins_test.go`:

```go
package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/internal/core"
)

// presenceSlashApp is slashApp with the status setter recorded and the
// workspace's cached presence set to presence ("" = no entry).
func presenceSlashApp(t *testing.T, presence string) (*App, *[]runCall, *[]statusCall) {
	t.Helper()
	a, runs := slashApp(t, nil)
	if presence != "" {
		a.presence.Set("T1", presence, false, time.Time{})
	}
	statuses := &[]statusCall{}
	a.setStatusSetterForTest(func(action core.PresenceAction, mins int) {
		*statuses = append(*statuses, statusCall{action: action, mins: mins})
	})
	return a, runs, statuses
}

func TestSlashPresence(t *testing.T) {
	cases := []struct {
		name, cached, typed string
		want                []statusCall
		toast               string
	}{
		{"/away when active goes away", "active", "/away", []statusCall{{action: core.PresenceSetAway}}, ""},
		{"/away when away comes back (toggle)", "away", "/away", []statusCall{{action: core.PresenceSetActive}}, ""},
		{"/away with nothing cached goes away", "", "/away", []statusCall{{action: core.PresenceSetAway}}, ""},
		{"/active", "away", "/active", []statusCall{{action: core.PresenceSetActive}}, ""},
		{"/status away", "active", "/status away", []statusCall{{action: core.PresenceSetAway}}, ""},
		{"/status Active is case-insensitive", "away", "/status Active", []statusCall{{action: core.PresenceSetActive}}, ""},
		{"bare /status is unsupported", "active", "/status", nil, "/status isn't supported in slk yet"},
		{"/status with a custom status is unsupported", "active", "/status :palm_tree: on holiday", nil, "/status isn't supported in slk yet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, runs, statuses := presenceSlashApp(t, tc.cached)
			a.compose.SetValue(tc.typed)
			_ = pressEnter(a)
			if len(*runs) != 0 {
				t.Errorf("chat.command ran %d times, want 0 for a built-in", len(*runs))
			}
			if len(*statuses) != len(tc.want) {
				t.Fatalf("SetStatus calls = %+v, want %+v", *statuses, tc.want)
			}
			for i := range tc.want {
				if (*statuses)[i] != tc.want[i] {
					t.Errorf("SetStatus[%d] = %+v, want %+v", i, (*statuses)[i], tc.want[i])
				}
			}
			if tc.toast != "" && !strings.Contains(statusbarText(a), tc.toast) {
				t.Errorf("status bar = %q, want %q", statusbarText(a), tc.toast)
			}
		})
	}
}

func TestSlashAwayUpdatesTheCachedPresence(t *testing.T) {
	a, _, _ := presenceSlashApp(t, "active")
	a.compose.SetValue("/away")
	_ = pressEnter(a)
	if p, _, _, _ := a.presence.Status("T1"); p != "away" {
		t.Errorf("cached presence = %q, want away", p)
	}
}
```

- [ ] **Step 5: Run to verify failure**

Run: `go test ./internal/ui -run 'TestSlashPresence|TestSlashAway' -v`
Expected: FAIL — `chat.command ran 1 times, want 0` (no built-ins yet).

- [ ] **Step 6: Implement**

`internal/ui/slash_builtins.go`:

```go
// internal/ui/slash_builtins.go
//
// Slash built-ins slk runs itself, because Slack's web client handles
// them locally instead of through chat.command. Each cites the capture
// in docs/superpowers/specs/2026-10-12-slash-commands-captures/ (or the
// spec §2 table) that shows the web client's behaviour.
package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
)

// slashAway toggles presence, as the web client's /away did twice in a
// row (presence_set_away.json, then presence.set active).
func slashAway(a *App, _ commandArgs) tea.Cmd {
	if p, _, _, _ := a.presence.Status(a.activeTeamID); p == "away" {
		a.applyPresenceAction(core.PresenceSetActive, 0)
		return nil
	}
	a.applyPresenceAction(core.PresenceSetAway, 0)
	return nil
}

// slashActive sets presence to active.
func slashActive(a *App, _ commandArgs) tea.Cmd {
	a.applyPresenceAction(core.PresenceSetActive, 0)
	return nil
}

// slashStatus handles "/status away" (captured: the web client called
// presence.set) and, by symmetry, "/status active". Bare /status and a
// custom status are not captured, so they are unsupported for now.
func slashStatus(a *App, args commandArgs) tea.Cmd {
	if len(args.Fields) == 1 {
		switch strings.ToLower(args.Fields[0]) {
		case "away":
			a.applyPresenceAction(core.PresenceSetAway, 0)
			return nil
		case "active":
			a.applyPresenceAction(core.PresenceSetActive, 0)
			return nil
		}
	}
	return slashUnsupported("/status")(a, args)
}

// slashUnsupported is a built-in the web client handles locally that slk
// does not do yet: it says so rather than sending it to chat.command.
func slashUnsupported(name string) commandFunc {
	return func(a *App, _ commandArgs) tea.Cmd {
		return toastWithClear(a, name+" isn't supported in slk yet", 3*time.Second)
	}
}
```

In `internal/ui/slash_command.go`, fill the table:

```go
var slashCommands = map[string]commandFunc{
	"/away":   slashAway,
	"/active": slashActive,
	"/status": slashStatus,
}
```

- [ ] **Step 7: Run to verify the tests pass**

Run: `go test ./internal/ui -run 'TestSlash|Presence|Snooze' -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
gofmt -l internal/ui
git add internal/ui/slash_builtins.go internal/ui/slash_builtins_test.go internal/ui/slash_command.go
git commit -m "feat(ui): /away, /active and /status away|active

Handled in slk, as the web client does (presence.set, not
chat.command). /away toggles. Other /status forms say they are not
supported yet rather than guessing."
```

---

### Task 7: `/open #channel` and `/join #channel`

Three commits: `slackfmt.MentionedChannelIDs`, the `openChannel` extraction (no
behaviour change), then the built-ins.

**Files:**
- Modify: `internal/slackfmt/slackfmt.go`, `internal/slackfmt/slackfmt_test.go`
- Create: `internal/ui/open_channel.go`
- Modify: `internal/ui/mode_channel_finder.go` (lines 37–50 today), `internal/ui/slash_builtins.go`, `internal/ui/slash_builtins_test.go`, `internal/ui/slash_command.go` (table)
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: Task 5's table and `commandArgs`; `a.channelFinder.Items() []core.ChannelFinderItem`; `channelfinder.ChannelResult{ID, Name, Type string; Joined bool}`.
- Produces:
  - `func MentionedChannelIDs(text string) []string` (package `slackfmt`)
  - `func (a *App) openChannel(r channelfinder.ChannelResult) tea.Cmd`
  - `func (a *App) findChannelForCommand(arg string) (core.ChannelFinderItem, bool)`
  - `func slashOpen(a *App, args commandArgs) tea.Cmd`

- [ ] **Step 1: Failing test for `MentionedChannelIDs`**

Append to `internal/slackfmt/slackfmt_test.go`:

```go
func TestMentionedChannelIDs(t *testing.T) {
	got := MentionedChannelIDs("see <#C0EXAMPLE01|general> and <#G0EXAMPLE02|secret>, not <#bad> or <@U1>")
	want := []string{"C0EXAMPLE01", "G0EXAMPLE02"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("MentionedChannelIDs = %v, want %v", got, want)
	}
	if got := MentionedChannelIDs("no mentions"); len(got) != 0 {
		t.Errorf("MentionedChannelIDs(no mentions) = %v, want empty", got)
	}
}
```

(add `"strings"` to the file's imports if it is not there).

Run: `go test ./internal/slackfmt -run TestMentionedChannelIDs -v`
Expected: build failure — `undefined: MentionedChannelIDs`.

- [ ] **Step 2: Implement by extending the existing regex**

In `internal/slackfmt/slackfmt.go`, change

```go
	channelMentionRe = regexp.MustCompile(`<#[A-Z0-9]+\|([^>]+)>`)
```

to

```go
	// Group 1 is the channel ID, group 2 its label.
	channelMentionRe = regexp.MustCompile(`<#([A-Z0-9]+)\|([^>]+)>`)
```

and in `StripMarkupWithUserGroups` change
`channelMentionRe.ReplaceAllString(text, "#$1")` to
`channelMentionRe.ReplaceAllString(text, "#$2")`. Add after `MentionedUserIDs`:

```go
// MentionedChannelIDs returns the channel IDs of every <#C…|name>
// mention in text, in order, duplicates included. This is the form the
// composer sends (compose.TranslateMentionsForSend).
func MentionedChannelIDs(text string) []string {
	matches := channelMentionRe.FindAllStringSubmatch(text, -1)
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, m[1])
	}
	return ids
}
```

Run: `go test ./internal/slackfmt ./internal/notify ./cmd/slk`
Expected: PASS (every existing `StripMarkup` test unchanged).

Add to `AGENTS.md` "Shared code → Text and rendering", next to the
`slackfmt.MentionedUserIDs` row:

```markdown
| Channel IDs mentioned in message text (`<#C…\|name>`) | `slackfmt.MentionedChannelIDs(text)` |
```

```bash
gofmt -l internal/slackfmt
git add internal/slackfmt AGENTS.md
git commit -m "feat(slackfmt): MentionedChannelIDs

Extends the existing channel-mention regex to capture the ID rather
than adding a fourth one."
```

- [ ] **Step 3: Extract `openChannel`**

`internal/ui/open_channel.go`:

```go
package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/channelfinder"
)

// openChannel switches to a channel the user picked. One they are in is
// selected at once; one they are not is joined first, and
// ChannelJoinedMsg (reducer_channels) folds it into the sidebar and
// selects it. Shared by the channel finder and /open.
func (a *App) openChannel(r channelfinder.ChannelResult) tea.Cmd {
	if r.Joined {
		a.sidebar.SelectByID(r.ID)
		return func() tea.Msg {
			return ChannelSelectedMsg{ID: r.ID, Name: r.Name, Type: r.Type}
		}
	}
	channels := a.channels
	id, name := ids.ChannelID(r.ID), r.Name
	return func() tea.Msg {
		return channels.Join(id, name)
	}
}
```

In `internal/ui/mode_channel_finder.go`, replace from the comment
`// Already-joined: switch immediately. Not joined: kick off` through the
closing `}` of the `channels.Join` closure with:

```go
		return a.openChannel(*result)
```

and drop the `ids` import if it is now unused.

Run: `go build ./... && go test ./internal/ui -run 'ChannelFinder|Finder|Join' -v`
Expected: PASS, no test edited.

Add to `AGENTS.md` "Shared code → App state and actions":

```markdown
| Switch to a channel, joining it first if the user is not a member | `(*App).openChannel(channelfinder.ChannelResult)` (`internal/ui/open_channel.go`); the channel finder and `/open` use it |
```

```bash
gofmt -l internal/ui
git add internal/ui/open_channel.go internal/ui/mode_channel_finder.go AGENTS.md
git commit -m "refactor(ui): extract openChannel from the channel finder

No behaviour change; /open will share it."
```

- [ ] **Step 4: Failing tests for `/open` and `/join`**

Append to `internal/ui/slash_builtins_test.go` (imports: add `"github.com/gammons/slk/internal/ids"`):

```go
// openSlashApp is slashApp with three channels in the finder (design
// not joined) and joins recorded.
func openSlashApp(t *testing.T) (*App, *[]runCall, *[]string) {
	t.Helper()
	a, runs := slashApp(t, nil)
	a.channelFinder.SetItems([]core.ChannelFinderItem{
		{ID: "C1", Name: "general", Type: "channel", Joined: true},
		{ID: "C2", Name: "random", Type: "channel", Joined: true},
		{ID: "C9", Name: "design", Type: "channel", Joined: false},
	})
	joins := &[]string{}
	a.SetChannelService(core.NewChannelService(core.ChannelServiceFuncs{
		Join: func(id ids.ChannelID, name string) core.Msg {
			*joins = append(*joins, string(id))
			return ChannelJoinedMsg{ID: string(id), Name: name}
		},
	}))
	return a, runs, joins
}

func TestSlashOpen_SelectsAJoinedChannelByName(t *testing.T) {
	a, runs, _ := openSlashApp(t)
	a.compose.SetValue("/open #random")
	sel := deliveredMsgs[ChannelSelectedMsg](t, pressEnter(a))
	if len(sel) != 1 || sel[0].ID != "C2" {
		t.Fatalf("ChannelSelectedMsg = %+v, want C2", sel)
	}
	if len(*runs) != 0 {
		t.Errorf("chat.command ran %d times, want 0", len(*runs))
	}
}

func TestSlashJoin_SelectsByMentionID(t *testing.T) {
	a, _, _ := openSlashApp(t)
	a.compose.SetValue("/join <#C2|random>")
	sel := deliveredMsgs[ChannelSelectedMsg](t, pressEnter(a))
	if len(sel) != 1 || sel[0].ID != "C2" {
		t.Fatalf("ChannelSelectedMsg = %+v, want C2", sel)
	}
}

func TestSlashOpen_JoinsAChannelTheUserIsNotIn(t *testing.T) {
	a, _, joins := openSlashApp(t)
	a.compose.SetValue("/open #Design")
	_ = deliveredMsgs[ChannelJoinedMsg](t, pressEnter(a))
	if len(*joins) != 1 || (*joins)[0] != "C9" {
		t.Fatalf("joins = %v, want [C9]", *joins)
	}
}

func TestSlashOpen_ErrorsToast(t *testing.T) {
	cases := map[string]string{
		"/open #nope": "No channel named #nope",
		"/open":       "Usage: /open #channel",
	}
	for typed, want := range cases {
		t.Run(typed, func(t *testing.T) {
			a, runs, joins := openSlashApp(t)
			a.compose.SetValue(typed)
			_ = pressEnter(a)
			if !strings.Contains(statusbarText(a), want) {
				t.Errorf("status bar = %q, want %q", statusbarText(a), want)
			}
			if len(*runs) != 0 || len(*joins) != 0 {
				t.Errorf("runs = %v, joins = %v; want neither", *runs, *joins)
			}
		})
	}
}
```

Run: `go test ./internal/ui -run 'TestSlashOpen|TestSlashJoin' -v`
Expected: FAIL — `/open` still runs remotely (`deliveredMsgs` times out waiting for `ChannelSelectedMsg`, or the toast is missing).

- [ ] **Step 5: Implement**

Append to `internal/ui/slash_builtins.go` (imports: add
`"github.com/gammons/slk/internal/slackfmt"`, `"github.com/gammons/slk/internal/text"`,
`"github.com/gammons/slk/internal/ui/channelfinder"`):

```go
// slashOpen switches to a channel, joining it first if needed, as the
// web client's /open did (navigation only, no chat.command). /join is
// its alias (client_app_commands.json: alias_of "/open").
func slashOpen(a *App, args commandArgs) tea.Cmd {
	if strings.TrimSpace(args.Rest) == "" {
		return toastWithClear(a, "Usage: /open #channel", 3*time.Second)
	}
	item, ok := a.findChannelForCommand(args.Rest)
	if !ok {
		return toastWithClear(a, "No channel named "+strings.TrimSpace(args.Fields[0]), 3*time.Second)
	}
	return a.openChannel(channelfinder.ChannelResult{ID: item.ID, Name: item.Name, Type: item.Type, Joined: item.Joined})
}

// findChannelForCommand resolves a command's channel argument against
// the finder's rows: by ID when the composer translated it to
// <#C…|name>, otherwise by name ("#design" or "design", case- and
// accent-insensitive) among channels, joined or not.
func (a *App) findChannelForCommand(arg string) (core.ChannelFinderItem, bool) {
	items := a.channelFinder.Items()
	if ids := slackfmt.MentionedChannelIDs(arg); len(ids) > 0 {
		for _, it := range items {
			if !it.Synthetic && it.ID == ids[0] {
				return it, true
			}
		}
		return core.ChannelFinderItem{}, false
	}
	fields := strings.Fields(arg)
	if len(fields) == 0 {
		return core.ChannelFinderItem{}, false
	}
	want := text.Fold(strings.TrimPrefix(fields[0], "#"))
	for _, it := range items {
		if it.Synthetic || (it.Type != "channel" && it.Type != "private") {
			continue
		}
		if text.Fold(it.Name) == want {
			return it, true
		}
	}
	return core.ChannelFinderItem{}, false
}
```

Add to the table in `internal/ui/slash_command.go`:

```go
	"/open":   slashOpen,
	"/join":   slashOpen,
```

Run: `go test ./internal/ui -run 'TestSlash|ChannelFinder' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -l internal/ui
git add internal/ui/slash_builtins.go internal/ui/slash_builtins_test.go internal/ui/slash_command.go
git commit -m "feat(ui): /open and /join switch channel

Resolved by the composer's <#C…> mention or by name against the
channel finder's rows; a channel the user is not in is joined first,
through the same openChannel the finder uses."
```

---

### Task 8: `/msg`, `/dm`, and the unsupported built-ins

Two commits: the `openConversation` extraction (no behaviour change), then the
built-ins.

**Files:**
- Modify: `internal/ui/mode_new_message.go` (lines 36–42 today), `internal/ui/reducer_new_message.go` (`NewMessageOpenedMsg` and `NewMessageFailedMsg` arms), `internal/ui/app.go` (one field next to `newMessageInFlightID`)
- Create: `internal/ui/open_conversation.go`
- Modify: `internal/ui/slash_builtins.go`, `internal/ui/slash_builtins_test.go`, `internal/ui/slash_command.go` (table)
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: Task 4's `compose.OfferDraft`; `slackfmt.MentionedUserIDs`; test helper `wireOpenConversation(a, fn)` (`seams_wiring_test.go`); `NewMessageOpenedMsg{ChannelID string; AlreadyOpen bool; UserIDs []string; RequestID uint64}`.
- Produces:
  - App field `newMessagePrefill string`
  - `func (a *App) openConversation(userIDs []string, prefill string) tea.Cmd`
  - `func slashMsg(a *App, args commandArgs) tea.Cmd`

- [ ] **Step 1: Extract `openConversation`, with an unused pre-fill**

`internal/ui/open_conversation.go`:

```go
package ui

import tea "charm.land/bubbletea/v2"

// openConversation opens (or finds) the DM or group DM with userIDs: a
// fresh in-flight request, whose NewMessageOpenedMsg switches to the
// conversation (reducer_new_message.go) and drops late results of
// cancelled ones. prefill, when not empty, is offered as the
// conversation's draft once it opens. Shared by the new-message picker
// and /msg.
func (a *App) openConversation(userIDs []string, prefill string) tea.Cmd {
	a.newMessageInFlightID++
	a.newMessageCancelled = false
	a.newMessagePrefill = prefill
	return teaCmd(a.channels.OpenConversation(userIDs, a.newMessageInFlightID))
}
```

In `internal/ui/app.go`, next to the `newMessageInFlightID` field:

```go
	// newMessagePrefill is the text /msg offers as the opened
	// conversation's draft; "" for the new-message picker. Consumed by
	// NewMessageOpenedMsg, cleared by NewMessageFailedMsg.
	newMessagePrefill string
```

In `internal/ui/mode_new_message.go`, replace

```go
		// Submit. Bump the in-flight ID and clear cancellation
		// before dispatch so a fresh result is honored.
		a.newMessageInFlightID++
		a.newMessageCancelled = false
		reqID := a.newMessageInFlightID
		userIDs := result.UserIDs
		return teaCmd(a.channels.OpenConversation(userIDs, reqID))
```

with

```go
		return a.openConversation(result.UserIDs, "")
```

In `internal/ui/reducer_new_message.go`, in the `NewMessageOpenedMsg` arm, after
`a.newMessageCancelled = false`:

```go
		prefill := a.newMessagePrefill
		a.newMessagePrefill = ""
		var keptDraft tea.Cmd
		if prefill != "" && !a.compose.OfferDraft(a.activeTeamID, m.ChannelID, "", prefill) {
			keptDraft = func() tea.Msg {
				return ToastMsg{Text: "Kept your existing draft there; the /msg text was not added"}
			}
		}
```

and change that arm's return to
`return tea.Batch(emitSelected, a.compose.Focus(), keptDraft), true`.
In the `NewMessageFailedMsg` arm, after `a.newMessageInFlightID = 0`, add
`a.newMessagePrefill = ""`.

Run: `go build ./... && go test ./internal/ui -run 'NewMessage' -v`
Expected: PASS, no test edited (the picker always passes "").

Add to `AGENTS.md` "Shared code → App state and actions":

```markdown
| Open (or find) a DM / group DM and switch to it, optionally pre-filling its draft | `(*App).openConversation(userIDs, prefill)` (`internal/ui/open_conversation.go`); the new-message picker and `/msg` use it |
```

```bash
gofmt -l internal/ui
git add internal/ui/open_conversation.go internal/ui/mode_new_message.go internal/ui/reducer_new_message.go internal/ui/app.go AGENTS.md
git commit -m "refactor(ui): extract openConversation from the new-message picker

Adds a pre-fill offered as the opened conversation's draft; the picker
passes none, so no behaviour change."
```

- [ ] **Step 2: Failing tests for `/msg`, `/dm` and the unsupported list**

Append to `internal/ui/slash_builtins_test.go` (imports: add
`tea "charm.land/bubbletea/v2"` and `"github.com/gammons/slk/internal/ui/mentionpicker"`):

```go
// msgSlashApp is slashApp whose conversations.open is recorded and
// answers with DM D9.
func msgSlashApp(t *testing.T) (*App, *[]runCall, *[][]string) {
	t.Helper()
	a, runs := slashApp(t, nil)
	opens := &[][]string{}
	wireOpenConversation(a, func(userIDs []string, reqID uint64) tea.Cmd {
		*opens = append(*opens, userIDs)
		return func() tea.Msg {
			return NewMessageOpenedMsg{ChannelID: "D9", UserIDs: userIDs, RequestID: reqID, AlreadyOpen: true}
		}
	})
	return a, runs, opens
}

// draftIn returns compose's draft for (T1, channelID) by switching to it.
func draftIn(a *App, channelID string) string {
	a.compose.SetDraftContext("T1", channelID, "")
	return a.compose.Value()
}

func TestSlashMsg_OpensTheDMAndPrefills(t *testing.T) {
	a, runs, opens := msgSlashApp(t)
	a.compose.SetValue("/msg <@U9> are you free\nat 3?")
	opened := deliveredMsgs[NewMessageOpenedMsg](t, pressEnter(a))
	if len(*opens) != 1 || strings.Join((*opens)[0], ",") != "U9" {
		t.Fatalf("OpenConversation calls = %v, want [[U9]]", *opens)
	}
	a.Update(opened[0])
	if got := draftIn(a, "D9"); got != "are you free\nat 3?" {
		t.Errorf("D9 draft = %q, want the /msg text", got)
	}
	if len(*runs) != 0 {
		t.Errorf("chat.command ran %d times, want 0", len(*runs))
	}
}

func TestSlashMsg_TypedMentionIsTranslated(t *testing.T) {
	a, _, opens := msgSlashApp(t)
	a.compose.SetUsers([]mentionpicker.User{{ID: "U9", DisplayName: "alice", Username: "alice", InChannel: true}})
	a.compose.SetValue("/msg @alice hi")
	_ = deliveredMsgs[NewMessageOpenedMsg](t, pressEnter(a))
	if len(*opens) != 1 || (*opens)[0][0] != "U9" {
		t.Fatalf("OpenConversation calls = %v, want [[U9]]", *opens)
	}
}

func TestSlashDM_WithoutTextOnlyOpens(t *testing.T) {
	a, _, opens := msgSlashApp(t)
	a.compose.SetValue("/dm <@U9>")
	opened := deliveredMsgs[NewMessageOpenedMsg](t, pressEnter(a))
	a.Update(opened[0])
	if len(*opens) != 1 {
		t.Fatalf("OpenConversation calls = %v, want one", *opens)
	}
	if got := draftIn(a, "D9"); got != "" {
		t.Errorf("D9 draft = %q, want none", got)
	}
}

func TestSlashMsg_KeepsAnExistingDraft(t *testing.T) {
	a, _, _ := msgSlashApp(t)
	a.compose.SetDraftContext("T1", "D9", "")
	a.compose.SetValue("older draft")
	a.compose.SetDraftContext("T1", "C1", "")
	a.compose.SetValue("/msg <@U9> hi")
	opened := deliveredMsgs[NewMessageOpenedMsg](t, pressEnter(a))
	_, cmd := a.Update(opened[0])
	toasts := deliveredMsgs[ToastMsg](t, cmd)
	if len(toasts) != 1 || !strings.Contains(toasts[0].Text, "Kept your existing draft") {
		t.Errorf("toasts = %+v, want the kept-draft notice", toasts)
	}
	if got := draftIn(a, "D9"); got != "older draft" {
		t.Errorf("D9 draft = %q, want the older draft kept", got)
	}
}

func TestSlashMsg_UsageErrors(t *testing.T) {
	cases := map[string]string{
		"/msg":              "Usage: /msg @someone [message]",
		"/msg hello there":  "Usage: /msg @someone [message]",
		"/msg @nobody hi":   "No one called @nobody",
		"/dm <#C2|random>":  "Usage: /dm @someone [message]",
	}
	for typed, want := range cases {
		t.Run(typed, func(t *testing.T) {
			a, runs, opens := msgSlashApp(t)
			a.compose.SetValue(typed)
			_ = pressEnter(a)
			if !strings.Contains(statusbarText(a), want) {
				t.Errorf("status bar = %q, want %q", statusbarText(a), want)
			}
			if len(*runs) != 0 || len(*opens) != 0 {
				t.Errorf("runs = %v, opens = %v; want neither", *runs, *opens)
			}
		})
	}
}

func TestSlashUnsupportedBuiltins(t *testing.T) {
	for _, name := range []string{"/leave", "/close", "/part"} {
		t.Run(name, func(t *testing.T) {
			a, runs := slashApp(t, nil)
			a.compose.SetValue(name)
			_ = pressEnter(a)
			if want := name + " isn't supported in slk yet"; !strings.Contains(statusbarText(a), want) {
				t.Errorf("status bar = %q, want %q", statusbarText(a), want)
			}
			if len(*runs) != 0 {
				t.Errorf("chat.command ran %d times, want 0", len(*runs))
			}
		})
	}
}
```

Run: `go test ./internal/ui -run 'TestSlashMsg|TestSlashDM|TestSlashUnsupported' -v`
Expected: FAIL — `/msg` runs remotely.

- [ ] **Step 3: Implement**

Append to `internal/ui/slash_builtins.go`:

```go
// slashMsg opens the DM with one user. The web client's /msg resolved
// the user and posted with chat.postMessage (spec §2); slk pre-fills the
// message in the DM's composer instead of sending it (spec §3.3). The
// text is wire form, so a mention inside it shows as <@U…> in the
// composer; it still sends correctly. /dm is its alias. The toasts are
// built only on the error paths: building one sets it.
func slashMsg(name string) commandFunc {
	return func(a *App, args commandArgs) tea.Cmd {
		usage := func() tea.Cmd {
			return toastWithClear(a, "Usage: "+name+" @someone [message]", 3*time.Second)
		}
		if len(args.Fields) == 0 {
			return usage()
		}
		first := args.Fields[0]
		users := slackfmt.MentionedUserIDs(first)
		if len(users) != 1 || first != "<@"+users[0]+">" {
			if strings.HasPrefix(first, "@") {
				return toastWithClear(a, "No one called "+first, 3*time.Second)
			}
			return usage()
		}
		text := strings.TrimLeft(strings.TrimPrefix(args.Rest, first), " \t\r\n")
		return a.openConversation([]string{users[0]}, text)
	}
}
```

Complete the table in `internal/ui/slash_command.go`:

```go
var slashCommands = map[string]commandFunc{
	"/away":   slashAway,
	"/active": slashActive,
	"/status": slashStatus,
	"/open":   slashOpen,
	"/join":   slashOpen,
	"/msg":    slashMsg("/msg"),
	"/dm":     slashMsg("/dm"),
	"/leave":  slashUnsupported("/leave"),
	"/close":  slashUnsupported("/close"),
	"/part":   slashUnsupported("/part"),
}
```

Run: `go test ./internal/ui -run 'TestSlash|NewMessage' -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
gofmt -l internal/ui
git add internal/ui/slash_builtins.go internal/ui/slash_builtins_test.go internal/ui/slash_command.go
git commit -m "feat(ui): /msg and /dm open the DM; /leave, /close, /part say not yet

/msg @someone text opens the DM through the new-message flow and
offers the text as its draft instead of sending it. The leave family
is handled locally by the web client and is not built yet, so it says
so rather than going to chat.command."
```

---

### Task 9: a command that opens a modal says so

**Files:**
- Create (copy): `internal/slack/testdata/ws_view_opened.json` from the captures directory
- Modify: `internal/slack/events.go` (`EventHandler` interface; a `case "view_opened":` in `dispatchWebSocketEvent`, next to `case "manual_presence_change":`), `internal/slack/events_test.go` (mock + test)
- Modify: `cmd/slk/rtm_handler.go` (after `OnEphemeralMessage`), and a test beside it (`cmd/slk/rtm_view_opened_test.go`, new)

**Interfaces:**
- Produces: `EventHandler.OnViewOpened(title string)`; `(*rtmEventHandler).OnViewOpened(title string)`, which sends `ui.ToastMsg`.

- [ ] **Step 1: Failing tests**

```bash
cp docs/superpowers/specs/2026-10-12-slash-commands-captures/ws_view_opened.json internal/slack/testdata/ws_view_opened.json
```

In `internal/slack/events_test.go`, add a field to `mockEventHandler`:

```go
	viewsOpened []string
```

the method:

```go
func (m *mockEventHandler) OnViewOpened(title string) {
	m.viewsOpened = append(m.viewsOpened, title)
}
```

and the test:

```go
func TestDispatchViewOpened_CapturedFrame(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "ws_view_opened.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := &mockEventHandler{}
	dispatchWebSocketEvent(data, h)
	if len(h.viewsOpened) != 1 || h.viewsOpened[0] != "Giphy" {
		t.Fatalf("viewsOpened = %v, want [Giphy]", h.viewsOpened)
	}
}

func TestDispatchViewOpened_FallsBackToTheViewTitle(t *testing.T) {
	h := &mockEventHandler{}
	dispatchWebSocketEvent([]byte(`{"type":"view_opened","view":{"title":{"type":"plain_text","text":"Polly"}}}`), h)
	if len(h.viewsOpened) != 1 || h.viewsOpened[0] != "Polly" {
		t.Fatalf("viewsOpened = %v, want [Polly]", h.viewsOpened)
	}
}
```

`cmd/slk/rtm_view_opened_test.go`:

```go
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
```

Run: `go test ./internal/slack -run TestDispatchViewOpened -v; go test ./cmd/slk -run TestOnViewOpened -v`
Expected: build failures — `mockEventHandler` does not satisfy a method that
does not exist yet / `h.OnViewOpened undefined`.

- [ ] **Step 2: Implement**

In `internal/slack/events.go`, add to the `EventHandler` interface after
`OnEphemeralMessage`:

```go
	// OnViewOpened delivers view_opened: an app opened a modal for the
	// current user, typically in answer to a slash command (bare
	// /giphy). title is the modal's title. slk cannot show modals yet;
	// see docs/superpowers/specs/2026-10-12-app-modals-research.md.
	OnViewOpened(title string)
```

and to `dispatchWebSocketEvent`'s switch, after the `manual_presence_change`
case:

```go
	case "view_opened":
		var evt struct {
			Title struct {
				Text string `json:"text"`
			} `json:"title"`
			View struct {
				Title struct {
					Text string `json:"text"`
				} `json:"title"`
			} `json:"view"`
		}
		if err := json.Unmarshal(data, &evt); err != nil {
			return
		}
		title := evt.Title.Text
		if title == "" {
			title = evt.View.Title.Text
		}
		debuglog.WS("view_opened: title=%q", title)
		handler.OnViewOpened(title)
```

In `cmd/slk/rtm_handler.go`, after `OnEphemeralMessage`:

```go
// OnViewOpened tells the user an app opened a modal slk cannot show yet
// (spec §3.5). Nothing else is done with the view.
func (h *rtmEventHandler) OnViewOpened(title string) {
	if h.isActive != nil && !h.isActive() {
		return
	}
	if h.program == nil {
		return
	}
	text := "This command opened a dialog slk can't show yet"
	if title != "" {
		text = title + " opened a dialog slk can't show yet"
	}
	h.program.Send(ui.ToastMsg{Text: text})
}
```

`go build ./...` lists any other `EventHandler` implementation that now fails
to compile (at the time of writing there are only these two); add the same
method to each.

Run: `go test ./internal/slack ./cmd/slk -run 'ViewOpened|Dispatch' -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
gofmt -l internal/slack cmd/slk
git add internal/slack/events.go internal/slack/events_test.go internal/slack/testdata/ws_view_opened.json cmd/slk/rtm_handler.go cmd/slk/rtm_view_opened_test.go
git commit -m "feat(ws): say when a command opens a dialog slk can't show

view_opened (bare /giphy, any app modal) becomes a toast naming the
app's dialog. App modals are future work; see the research note."
```

---

### Task 10: `/me` lines arrive live, in italics

**Files:**
- Create (copy): `internal/slack/testdata/ws_me_message.json`
- Modify: `internal/slack/events.go` (the `case "", "bot_message", "thread_broadcast", "file_share":` line), `internal/slack/events_test.go`
- Modify: `internal/ui/messages/render_message.go` (the `text := styles.MessageText.Render(...)` line in `RenderMessage`), and a test (`internal/ui/messages/me_message_test.go`, new)

**Interfaces:** none new.

- [ ] **Step 1: Failing tests**

```bash
cp docs/superpowers/specs/2026-10-12-slash-commands-captures/ws_me_message.json internal/slack/testdata/ws_me_message.json
```

Append to `internal/slack/events_test.go`:

```go
// A live /me line used to be dropped: the message switch passed only
// "", bot_message, thread_broadcast and file_share to OnMessage.
func TestDispatchMeMessage_CapturedFrame(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "ws_me_message.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := &mockEventHandler{}
	dispatchWebSocketEvent(data, h)
	if len(h.messages) != 1 || h.messages[0] != "waves" || h.subtypes[0] != "me_message" {
		t.Fatalf("messages = %v subtypes = %v, want [waves] [me_message]", h.messages, h.subtypes)
	}
}
```

`internal/ui/messages/me_message_test.go`:

```go
package messages

import (
	"regexp"
	"testing"
)

// italicSGR matches an SGR sequence that turns italic on (parameter 3,
// alone or among others).
var italicSGR = regexp.MustCompile(`\x1b\[(?:[0-9]+;)*3(?:;[0-9]+)*m`)

func TestRenderMessage_MeMessageIsItalic(t *testing.T) {
	items := []MessageItem{
		{TS: "1.0", UserID: "U1", UserName: "priya", Text: "waves", Timestamp: "3:40 PM", Subtype: "me_message"},
		{TS: "2.0", UserID: "U1", UserName: "priya", Text: "waves", Timestamp: "3:41 PM"},
	}
	m := New(items, "general")
	me := RenderMessage(items[0], 60, m.renderContext(), RenderOptions{})
	plain := RenderMessage(items[1], 60, m.renderContext(), RenderOptions{})
	if !italicSGR.MatchString(me.Content) {
		t.Errorf("me_message content has no italic SGR:\n%q", me.Content)
	}
	if italicSGR.MatchString(plain.Content) {
		t.Errorf("ordinary message content is italic:\n%q", plain.Content)
	}
}
```

Run: `go test ./internal/slack -run TestDispatchMeMessage -v; go test ./internal/ui/messages -run TestRenderMessage_MeMessage -v`
Expected: FAIL — no message delivered; no italic SGR.

- [ ] **Step 2: Implement**

In `internal/slack/events.go`, change

```go
		case "", "bot_message", "thread_broadcast", "file_share":
```

to

```go
		case "", "bot_message", "thread_broadcast", "file_share", "me_message":
```

and extend the comment below it with: "me_message is a /me line; the UI
renders it in italics."

In `internal/ui/messages/render_message.go`, replace

```go
	text := styles.MessageText.Render(WordWrap(rendered, contentWidth))
```

with

```go
	bodyStyle := styles.MessageText
	if msg.Subtype == "me_message" {
		// A /me line, italic as in Slack's own clients.
		bodyStyle = bodyStyle.Italic(true)
	}
	text := bodyStyle.Render(WordWrap(rendered, contentWidth))
```

Run: `go test ./internal/slack ./internal/ui/messages ./internal/ui/thread && go test ./internal/ui -run TestGolden`
Expected: PASS; every golden unchanged (no golden fixture has a `me_message`).

- [ ] **Step 3: Commit**

```bash
gofmt -l internal/slack internal/ui
git add internal/slack/events.go internal/slack/events_test.go internal/slack/testdata/ws_me_message.json internal/ui/messages/render_message.go internal/ui/messages/me_message_test.go
git commit -m "fix(ws): show /me lines live, in italics

me_message was missing from the subtypes the WebSocket passes on, so a
/me line appeared only after a history refetch. RenderMessage draws it
in italics in both panes."
```

---

### Task 11: gate, AGENTS.md check, and a real-workspace run

**Files:** possibly `AGENTS.md`; the spec if anything was learned.

- [ ] **Step 1: The full gate**

```bash
go build ./...
go vet ./...
gofmt -l .
golangci-lint run
go test ./... -race
```

Expected: build and vet clean, `gofmt -l .` prints nothing, golangci-lint
reports 0 issues, every package passes.

- [ ] **Step 2: AGENTS.md is complete**

Every helper this plan added has a row:
`RunCommand`/`CommandRejectedError`, `newClientToken`, `core.CommandService`,
`parseSlashCommand`/`trySlashCommand`/`commandRanMsg`, `compose.OfferDraft`,
`dispatchCommand`/`commands`/`slashCommands`, `applyPresenceAction`,
`slackfmt.MentionedChannelIDs`, `openChannel`, `openConversation`; and the test
helpers `setCommandRunnerForTest`, `slashApp`/`pressEnter`/`runCall`. Check:

```bash
for s in RunCommand newClientToken CommandService parseSlashCommand OfferDraft dispatchCommand slashCommands applyPresenceAction MentionedChannelIDs openChannel openConversation setCommandRunnerForTest slashApp; do
  grep -q "$s" AGENTS.md || echo "AGENTS.md is missing $s"
done
```

Expected: no output. Add any missing row, then commit
`docs(agents): list the slash-command helpers`.

- [ ] **Step 3: Run it against a real workspace**

```bash
go build -o /tmp/slk ./cmd/slk && /tmp/slk
```

In a test channel, then in a thread in it, then in a DM:

| Type | Expect |
|---|---|
| `/giphy cats` | Giphy's preview ephemeral (its buttons wait for stage 2) |
| `/shrug hi` | "hi ¯\_(ツ)_/¯" posted |
| `/me waves` | "waves" posted, in italics, without a refresh |
| `/remind me test in 1 minute` | Slackbot's ephemeral confirmation, then the reminder |
| `/notarealcommand x` | toast "/notarealcommand is not a valid command", text back in the composer |
| ` /giphy cats` (leading space) | posted as text |
| `/away`, `/away` | status bar away, then active |
| `/status away` | status bar away |
| `/open #<a channel you are not in>` | joined and switched to |
| `/msg @<someone> hello` | the DM opens with "hello" in its composer, not sent |
| `/giphy` (no args) | toast "Giphy opened a dialog slk can't show yet" |
| `/leave` | toast "/leave isn't supported in slk yet" |
| `/zoom` (if installed) | whatever Zoom answers, as an ephemeral or a message |

Record anything that differs from the table under a new "Addenda
(implementation)" heading at the end of the spec, and commit it with
`docs(spec): stage 1 addenda`.

- [ ] **Step 4: Hand off**

Stage 1 is one PR. Use superpowers:finishing-a-development-branch to open it;
the PR description links the spec, this plan, and #247, and lists the
real-workspace table from Step 3 with what was observed.
