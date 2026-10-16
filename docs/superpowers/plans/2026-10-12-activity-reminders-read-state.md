# Activity reminders and read state Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every Activity item readable and clearable: render Slack `saved_reminder` items, label unknown types, and let `U` (and Enter on unopenable items) set an item's read state through Slack's `activity.markRead` / `activity.markUnread`.

**Architecture:** The Slack client learns the `saved_reminder` shape and a `SetActivityRead` call. `core.ActivityService` gains `SetRead`, wired in `cmd/slk` to return `ui.ActivityReadSetMsg`. The App changes read state optimistically (`activityview.SetItemRead` + sidebar badge) and rolls back on an error result.

**Tech Stack:** Go, bubbletea v2, lipgloss v2, stdlib `testing` only (white-box tests, no testify).

**Spec:** `docs/superpowers/specs/2026-10-12-activity-reminders-read-state-design.md`

## Global Constraints

- Tests are plain `testing.T`, stdlib only, white-box (same package as the code).
- `internal/ui` does no I/O; Slack calls go through `core.ActivityService` (wired in `cmd/slk`).
- Reminder glyph: `◷` (one column, not an emoji). Context label: `Reminder`. Line-2 hint: `Open Slack to see this reminder`.
- Enter toasts: `Reminder: open Slack to see it` (saved_reminder), `slk can't open this activity item yet` (any other item with no channel).
- Failure toast: `Couldn't mark activity item read: <err>` / `Couldn't mark activity item unread: <err>`.
- Slack request: `activity.markRead` (read) / `activity.markUnread` (unread), form field `entries` = `[{"type":…,"key":…,"feed_ts":…}]`.
- Toasts go through `toastWithClear(a, text, 3*time.Second)`, set on the Update goroutine.
- Before every commit: `gofmt -l .` is empty and `go build ./...` passes.
- Never print, log or commit Slack tokens or cookies.

## Review Focus

1. **Two quick `U` presses, second request fails.** The item must end in the state Slack holds (the first request's), not flip-flop. Tested in Task 5.
2. **Feed refetched between a toggle and its failure result.** The rollback must not panic or touch a different item, and the badge must match the new list. Tested in Task 5.
3. **`U` on an empty Activity list.** No-op, no Slack call. Tested in Task 5.
4. **`U` with the thread pane focused inside the Activity view.** Keeps its message meaning; no activity call. Tested in Task 5.
5. **Marking read with the unread-only filter on.** The item stays listed (so `U` can undo) until the next fetch. Tested in Task 4 (`SetItemRead` never removes).

## File map

| File | Change |
|---|---|
| `internal/slack/activity.go` | `saved_reminder` case, `Subtype`, raw-log exemption, `SetActivityRead` |
| `internal/slack/activity_test.go` | parse + request tests |
| `internal/core/types.go` | `ActivityItem.Subtype` |
| `internal/core/ports.go` | `ActivityService.SetRead`, doc |
| `internal/core/adapters.go` | `ActivityServiceFuncs`, `ActivitySetReadFunc`, adapter |
| `internal/ui/service_defaults.go` | new constructor call |
| `internal/ui/msgs.go` | `ActivityReadSetMsg`; `ActivitySelectedMsg{Item}` |
| `cmd/slk/activity.go`, `cmd/slk/activity_test.go` | `activitySetReadFunc` |
| `cmd/slk/main.go` | wiring |
| `internal/ui/activityview/model.go`, `model_test.go` | reminder card, `SetItemRead` |
| `internal/ui/reducer_activity.go` | helpers, `ActivityReadSetMsg` arm, unopenable Enter |
| `internal/ui/mode_normal.go` | `U` arm in the Activity view |
| `internal/ui/app.go`, `internal/ui/reducer_mouse.go` | build `ActivitySelectedMsg{Item: it}` |
| `internal/ui/activity_read_test.go` (new) | App-level tests |
| `wiki/Keybindings.md` | `U` row |

---

### Task 1: Bring in PR #296

**Files:**
- Modify (via cherry-pick): `internal/slack/activity.go`, `internal/ui/activityview/model.go`, `internal/ui/activityview/model_test.go`

**Interfaces:**
- Produces: `fallbackVerb(itemType string) string` in `activityview` (`"saved_reminder"` → `"Saved reminder"`, `""` → `"Unknown activity"`); `contextLabel` uses it when `contextVerb` returns `""`. `parseActivityFeed` debug-logs `activity.feed: item with no channel ref …` for items with an empty `ChannelID`.

- [ ] **Step 1: Cherry-pick both commits, keeping authorship**

```bash
git cherry-pick 6f8d5ed1 b57958ba
```

Expected: two new commits, no conflicts (verified against this base).

- [ ] **Step 2: Run the affected packages**

Run: `go test ./internal/slack ./internal/ui/activityview -count=1`
Expected: PASS

No extra commit; the cherry-picks are the commits.

---

### Task 2: Slack client — parse `saved_reminder`, set read state

**Files:**
- Modify: `internal/core/types.go` (`ActivityItem`)
- Modify: `internal/slack/activity.go`
- Test: `internal/slack/activity_test.go`

**Interfaces:**
- Produces: `core.ActivityItem.Subtype string`; `(*slackclient.Client).SetActivityRead(ctx context.Context, itemType, key, feedTS string, read bool) error`.

- [ ] **Step 1: Write the failing tests** (append to `internal/slack/activity_test.go`; add `"context"`, `"net/http"`, `"net/http/httptest"` to its imports)

```go
// The saved_reminder payload as captured live from activity.feed
// (2026-10-12). It names a saved item and its due time, and carries no
// message ref, so ChannelID and TS stay empty.
func TestParseActivityFeed_SavedReminder(t *testing.T) {
	body := []byte(`{"ok":true,"items":[{"feed_ts":"1791803337.000000","is_archived":false,"is_unread":true,"item":{"saved_reminder_payload":{"date_due":1791803337,"item_id":"Sa0C8D6B3E77","item_id_2":"0000000000.000000","item_id_3":"","item_type":"reminder"},"type":"saved_reminder"},"key":"saved_reminder.12285215116245.1.0","version":"a4a9800d"}]}`)

	res, err := parseActivityFeed(body)
	if err != nil {
		t.Fatalf("parseActivityFeed: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	want := core.ActivityItem{
		Key:      "saved_reminder.12285215116245.1.0",
		Type:     "saved_reminder",
		Subtype:  "reminder",
		IsUnread: true,
		FeedTS:   "1791803337.000000",
	}
	if got := res.Items[0]; got != want {
		t.Fatalf("reminder item =\n  %+v\nwant\n  %+v", got, want)
	}
}

func TestSetActivityRead_PostsEntries(t *testing.T) {
	for _, read := range []bool{true, false} {
		var gotPath, gotEntries string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			_ = r.ParseForm()
			gotEntries = r.PostForm.Get("entries")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		c := NewClient("xoxc-test", "d-cookie")
		pointClientAtTestServer(t, c, srv)

		err := c.SetActivityRead(context.Background(), "saved_reminder", "saved_reminder.1.1.0", "1791803337.000000", read)
		srv.Close()
		if err != nil {
			t.Fatalf("read=%v: SetActivityRead: %v", read, err)
		}
		wantPath := "/api/activity.markUnread"
		if read {
			wantPath = "/api/activity.markRead"
		}
		if gotPath != wantPath {
			t.Errorf("read=%v: path = %q, want %q", read, gotPath, wantPath)
		}
		wantEntries := `[{"type":"saved_reminder","key":"saved_reminder.1.1.0","feed_ts":"1791803337.000000"}]`
		if gotEntries != wantEntries {
			t.Errorf("read=%v: entries = %s, want %s", read, gotEntries, wantEntries)
		}
	}
}

func TestSetActivityRead_NotOKIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_arguments"}`))
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)

	err := c.SetActivityRead(context.Background(), "at_user", "k", "1.1", true)
	if err == nil || !strings.Contains(err.Error(), "invalid_arguments") {
		t.Fatalf("err = %v, want one carrying Slack's invalid_arguments", err)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/slack -run 'SavedReminder|SetActivityRead' -count=1`
Expected: FAIL to compile (`unknown field Subtype`, `c.SetActivityRead undefined`).

- [ ] **Step 3: Add `Subtype` to `core.ActivityItem`** (`internal/core/types.go`, after `Reaction`)

```go
	Reaction  string // emoji short name, for message_reaction only
	Subtype   string // saved_reminder's item_type ("reminder"); "" for other types
```

- [ ] **Step 4: Parse the reminder** (`internal/slack/activity.go`)

Add to `rawActivityInner`:

```go
	SavedReminder *rawSavedReminder `json:"saved_reminder_payload"`
```

Add after `rawActivityInner`:

```go
// rawSavedReminder is a saved_reminder item's payload: the saved item
// and its due time. No observed reminder points at a message.
type rawSavedReminder struct {
	ItemType string `json:"item_type"`
}
```

Add a case to `flattenActivityItem`, before `default:`:

```go
	case "saved_reminder":
		// A reminder from Slack's Later list. There is no conversation
		// to open, so ChannelID and TS stay empty.
		if r := raw.Item.SavedReminder; r != nil {
			out.Subtype = r.ItemType
		}
```

In `parseActivityFeed`, reminders are now understood, so stop logging them as unknown shapes. Change the condition Task 1 added:

```go
		if item.ChannelID == "" && item.Type != "saved_reminder" {
```

- [ ] **Step 5: Add `SetActivityRead`** (end of `internal/slack/activity.go`)

```go
// activityReadEntry is one item in an activity.markRead / markUnread
// request. Slack requires all three fields.
type activityReadEntry struct {
	Type   string `json:"type"`
	Key    string `json:"key"`
	FeedTS string `json:"feed_ts"`
}

// SetActivityRead marks one Activity item read or unread via Slack's
// internal activity.markRead / activity.markUnread, the calls behind the
// web client's "Mark as read/unread". Every feed item carries the type,
// key and feed_ts they need, so this works for items slk cannot open.
// ok:false is an error carrying Slack's error string.
func (c *Client) SetActivityRead(ctx context.Context, itemType, key, feedTS string, read bool) error {
	method := "activity.markUnread"
	if read {
		method = "activity.markRead"
	}
	entries, err := json.Marshal([]activityReadEntry{{Type: itemType, Key: key, FeedTS: feedTS}})
	if err != nil {
		return fmt.Errorf("marshaling %s entries: %w", method, err)
	}
	raw, err := c.postForm(ctx, method, url.Values{"entries": {string(entries)}})
	if err != nil {
		return err
	}
	return parseOKResponse(method, raw)
}
```

Delete the `TODO(activity): activity.markRead …` paragraph (4 lines) from the `GetActivityFeed` doc comment.

- [ ] **Step 6: Run the package**

Run: `go test ./internal/slack -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
gofmt -l . && go build ./...
git add internal/core/types.go internal/slack/activity.go internal/slack/activity_test.go
git commit -m "feat(activity): parse saved reminders and set an item's read state

saved_reminder items carry a saved_reminder_payload and no message ref.
SetActivityRead calls activity.markRead / activity.markUnread, whose
entries shape was probed live.

Refs #285"
```

---

### Task 3: Core port `SetRead` and its `cmd/slk` wiring

**Files:**
- Modify: `internal/core/adapters.go` (`NewActivityService`, adapter, types near `ActivityFetchFunc`)
- Modify: `internal/core/ports.go` (`ActivityService`)
- Modify: `internal/ui/service_defaults.go`, `internal/ui/msgs.go`
- Modify: `cmd/slk/activity.go`, `cmd/slk/main.go`
- Test: `cmd/slk/activity_test.go`

**Interfaces:**
- Consumes: `(*slackclient.Client).SetActivityRead` (Task 2).
- Produces:
  - `core.ActivitySetReadFunc func(teamID ids.TeamID, item ActivityItem, read bool) Msg`
  - `core.ActivityServiceFuncs{Fetch ActivityFetchFunc; Hydrate ActivityHydrateFunc; SetRead ActivitySetReadFunc}`
  - `core.NewActivityService(fns ActivityServiceFuncs) ActivityService`
  - `ActivityService.SetRead(teamID ids.TeamID, item ActivityItem, read bool) Msg`
  - `ui.ActivityReadSetMsg{TeamID string; Key string; Read bool; Err error}`
  - `activitySetReadFunc(router *workspaceRouter) core.ActivitySetReadFunc`

- [ ] **Step 1: Write the failing test** (append to `cmd/slk/activity_test.go`; add `"github.com/gammons/slk/internal/ui"` to imports)

```go
// SetRead must always answer, even for a workspace that is not
// connected, so the UI can undo its optimistic change. Like the other
// closures it resolves the requested team: T1 is active with a nil
// Client, so going through it would panic.
func TestActivitySetRead_UnconnectedTeamReportsError(t *testing.T) {
	router := newWorkspaceRouter()
	active := &WorkspaceContext{TeamID: "T1"}
	router.Add(active)
	router.Set(active)

	msg := activitySetReadFunc(router)("T2", core.ActivityItem{Key: "k1", Type: "at_user", FeedTS: "1.1"}, true)
	got, ok := msg.(ui.ActivityReadSetMsg)
	if !ok {
		t.Fatalf("SetRead returned %#v, want ui.ActivityReadSetMsg", msg)
	}
	if got.TeamID != "T2" || got.Key != "k1" || !got.Read || got.Err == nil {
		t.Fatalf("SetRead for unconnected T2 = %+v, want TeamID T2, Key k1, Read true and an error", got)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./cmd/slk -run TestActivitySetRead -count=1`
Expected: FAIL to compile (`undefined: activitySetReadFunc`, `undefined: ui.ActivityReadSetMsg`).

- [ ] **Step 3: Core types and adapter** (`internal/core/adapters.go`)

Replace the `NewActivityService` doc, function, `activityAdapter` type and its two methods with:

```go
// ActivityServiceFuncs is the closure bundle accepted by
// NewActivityService. Any field may be nil; the resulting service
// no-ops that operation (returns a nil Msg).
type ActivityServiceFuncs struct {
	Fetch   ActivityFetchFunc
	Hydrate ActivityHydrateFunc
	SetRead ActivitySetReadFunc
}

// NewActivityService builds an ActivityService from an
// ActivityServiceFuncs bundle.
func NewActivityService(fns ActivityServiceFuncs) ActivityService {
	return activityAdapter{fns: fns}
}

type activityAdapter struct {
	fns ActivityServiceFuncs
}

func (a activityAdapter) Fetch(teamID ids.TeamID, limit int, unreadOnly bool) Msg {
	if a.fns.Fetch == nil {
		return nil
	}
	return a.fns.Fetch(teamID, limit, unreadOnly)
}

func (a activityAdapter) Hydrate(teamID ids.TeamID, refs map[string][]string) Msg {
	if a.fns.Hydrate == nil {
		return nil
	}
	return a.fns.Hydrate(teamID, refs)
}

func (a activityAdapter) SetRead(teamID ids.TeamID, item ActivityItem, read bool) Msg {
	if a.fns.SetRead == nil {
		return nil
	}
	return a.fns.SetRead(teamID, item, read)
}
```

After `ActivityHydrateFunc` (bottom of the same file):

```go
// ActivitySetReadFunc marks one Activity item read (read=true) or
// unread in teamID's feed.
type ActivitySetReadFunc func(teamID ids.TeamID, item ActivityItem, read bool) Msg
```

- [ ] **Step 4: Port** (`internal/core/ports.go`)

In the `ActivityService` doc comment, replace

```go
//
// Marking items read is out of scope: opening an item marks the
// underlying conversation read via existing paths.
```

with

```go
//
// SetRead changes one item's read state in the feed itself; opening an
// item also marks its conversation read through the existing paths.
```

and add the method after `Hydrate`:

```go

	// SetRead marks item read (read=true) or unread in teamID's
	// Activity feed. Returns a Msg (typically ActivityReadSetMsg,
	// carrying any error so the caller can undo an optimistic change).
	SetRead(teamID ids.TeamID, item ActivityItem, read bool) Msg
```

- [ ] **Step 5: UI message and default** 

`internal/ui/msgs.go`, after `ActivityToggleUnreadMsg struct{}`:

```go
	// ActivityReadSetMsg reports the result of asking Slack to mark one
	// Activity item read (Read=true) or unread. The App changed the
	// item before asking; a non-nil Err makes it undo that change.
	// Ignored if TeamID is not the active team.
	ActivityReadSetMsg struct {
		TeamID string
		Key    string
		Read   bool
		Err    error
	}
```

(`gofmt` will re-align the following `ConnectionStateMsg      struct {` line; that is expected.)

`internal/ui/service_defaults.go`:

```go
	noopActivityService    = core.NewActivityService(core.ActivityServiceFuncs{})
```

- [ ] **Step 6: Wiring** 

`cmd/slk/activity.go`, add `"fmt"` to imports and append:

```go
// activitySetReadFunc builds the ActivityService.SetRead closure:
// activity.markRead / activity.markUnread for one item. It always
// returns an ActivityReadSetMsg, so the UI can undo its optimistic
// change when the call fails.
func activitySetReadFunc(router *workspaceRouter) core.ActivitySetReadFunc {
	return func(teamID ids.TeamID, item core.ActivityItem, read bool) core.Msg {
		teamIDStr := string(teamID)
		msg := ui.ActivityReadSetMsg{TeamID: teamIDStr, Key: item.Key, Read: read}
		wctx := router.ByID(teamIDStr)
		if wctx == nil {
			msg.Err = fmt.Errorf("workspace %s is not connected", teamIDStr)
			return msg
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := wctx.Client.SetActivityRead(ctx, item.Type, item.Key, item.FeedTS, read); err != nil {
			log.Printf("Warning: SetActivityRead(%s, %s, read=%v): %v", teamIDStr, item.Key, read, err)
			msg.Err = err
		}
		return msg
	}
}
```

`cmd/slk/main.go`, replace the `app.SetActivityService(...)` call:

```go
		app.SetActivityService(core.NewActivityService(core.ActivityServiceFuncs{
			Fetch:   activityFetchFunc(router),
			Hydrate: activityHydrateFunc(router),
			SetRead: activitySetReadFunc(router),
		}))
```

- [ ] **Step 7: Find any other caller and run**

Run: `grep -rn "NewActivityService(" --include=*.go . | grep -v ActivityServiceFuncs`
Expected: no output (only `adapters.go`'s definition line, which contains `ActivityServiceFuncs`).

Run: `go build ./... && go test ./cmd/slk -run TestActivity -count=1 && go test ./internal/core -count=1`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
gofmt -l . && go vet ./internal/core ./cmd/slk ./internal/ui
git add internal/core cmd/slk/activity.go cmd/slk/activity_test.go cmd/slk/main.go internal/ui/msgs.go internal/ui/service_defaults.go
git commit -m "feat(activity): add ActivityService.SetRead

NewActivityService takes an ActivityServiceFuncs bundle, like the other
services. The cmd/slk closure always answers with ActivityReadSetMsg so
the UI can undo an optimistic change.

Refs #285"
```

---

### Task 4: Activity view — reminder card and `SetItemRead`

**Files:**
- Modify: `internal/ui/activityview/model.go`
- Test: `internal/ui/activityview/model_test.go`

**Interfaces:**
- Consumes: `core.ActivityItem.Subtype` (Task 2), `fallbackVerb` (Task 1).
- Produces: `(*activityview.Model).SetItemRead(key string, read bool) bool`.

- [ ] **Step 1: Update #296's tests, which used `saved_reminder` as the unknown type**

In `TestContextLabel_UnknownTypeIsNeverBlank`, change the first case to

```go
		{"unrecognised type, no channel", "brand_new_type", "", "Brand new type"},
```

In `TestRenderCard_UnknownTypeWithNoRefIsLabelled`, change `Type: "saved_reminder"` to `Type: "brand_new_type"` and the expected substring `"Saved reminder"` to `"Brand new type"`.

- [ ] **Step 2: Write the failing tests**

Add to the `TestActivityGlyph` cases map:

```go
		"saved_reminder":           "◷",
```

Add `"saved_reminder"` to the type list in `TestActivityGlyphs_SingleCellAndNotEmoji`:

```go
	for _, typ := range []string{"at_user", "thread_v2", "message_reaction", "dm", "bot_dm_bundle", "saved_reminder", ""} {
```

Add to the `TestContextVerb` cases map:

```go
		"saved_reminder": "Reminder",
```

Append:

```go
// A Slack reminder has no message to preview, so its card names it and
// says where to read it instead of leaving line 2 blank (issue #285).
func TestRenderCard_SavedReminder(t *testing.T) {
	m := New(nil, "")
	l1, l2 := m.renderCard(core.ActivityItem{
		Key: "k", Type: "saved_reminder", Subtype: "reminder",
		FeedTS: "1700000000.000000", IsUnread: true,
	}, 80, false)
	line1, line2 := ansi.Strip(l1), ansi.Strip(l2)
	if !strings.Contains(line1, "◷ Reminder") || !strings.Contains(line1, "●") {
		t.Errorf("line 1 = %q, want the reminder glyph, label and unread dot", line1)
	}
	if !strings.Contains(line2, "Open Slack to see this reminder") {
		t.Errorf("line 2 = %q, want the open-in-Slack hint", line2)
	}
}

// SetItemRead changes one item's dot in place. It never removes the
// item, even with the unread-only filter on, so U can undo; and it
// reports a key that is gone (the feed was refetched meanwhile).
func TestSetItemRead(t *testing.T) {
	m := New(nil, "")
	m.ToggleUnreadOnly()
	m.SetItems([]core.ActivityItem{{Key: "a", IsUnread: true}, {Key: "b"}})
	v := m.Version()

	if !m.SetItemRead("a", true) {
		t.Fatal(`SetItemRead("a", true) = false, want true`)
	}
	if m.Items()[0].IsUnread || m.UnreadCount() != 0 {
		t.Errorf("item a still unread after SetItemRead(a, true): %+v", m.Items()[0])
	}
	if m.Version() == v {
		t.Error("SetItemRead did not bump the version, so the frame would not redraw")
	}
	if len(m.Items()) != 2 {
		t.Errorf("items = %d, want 2: marking read must not drop the item", len(m.Items()))
	}
	if !m.SetItemRead("b", false) || !m.Items()[1].IsUnread {
		t.Errorf("SetItemRead(b, false) did not mark b unread: %+v", m.Items()[1])
	}
	if m.SetItemRead("gone", true) {
		t.Error(`SetItemRead("gone", true) = true for a key not in the list`)
	}
}
```

- [ ] **Step 3: Run them to see them fail**

Run: `go test ./internal/ui/activityview -count=1`
Expected: FAIL (`m.SetItemRead undefined`; once that compiles, the glyph/verb/card tests fail).

- [ ] **Step 4: Implement** (`internal/ui/activityview/model.go`)

Add after the `cardStride` const block:

```go
// reminderHint fills line 2 of a Slack reminder's card: the feed carries
// no reminder text.
const reminderHint = "Open Slack to see this reminder"
```

In `renderCard`, directly before `preview = strings.ReplaceAll(preview, "\n", " ")`:

```go
	if it.Type == "saved_reminder" && preview == "" {
		preview = mutedStyle().Render(reminderHint)
	}
```

In `contextVerb`, before `default:`:

```go
	case "saved_reminder":
		return "Reminder"
```

In `activityGlyph`, before `default:`:

```go
	case "saved_reminder":
		return "◷"
```

Update the `activityGlyph` doc's first sentence to end "…the sidebar's DM dot for DMs, and a clock face for reminders."

Add after `UnreadCount`:

```go
// SetItemRead sets the read state of the item with the given key and
// reports whether there was one: the list may have been refetched since
// the caller took the item. The item stays in the list either way; with
// the unread-only filter on, it goes on the next fetch.
func (m *Model) SetItemRead(key string, read bool) bool {
	for i := range m.items {
		if m.items[i].Key != key {
			continue
		}
		if m.items[i].IsUnread == read {
			m.items[i].IsUnread = !read
			m.dirty()
		}
		return true
	}
	return false
}
```

- [ ] **Step 5: Run the package**

Run: `go test ./internal/ui/activityview -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
gofmt -l . && go build ./...
git add internal/ui/activityview
git commit -m "feat(activity): render saved reminders and set an item's read state

Refs #285"
```

---

### Task 5: `U` toggles an Activity item's read state, with rollback

**Files:**
- Modify: `internal/ui/reducer_activity.go`
- Modify: `internal/ui/mode_normal.go` (the `MarkUnread` arm, ~line 351)
- Create: `internal/ui/activity_read_test.go`

**Interfaces:**
- Consumes: `ActivityService.SetRead`, `ui.ActivityReadSetMsg`, `core.ActivityServiceFuncs` (Task 3); `activityview.SetItemRead` (Task 4).
- Produces: `(*App).setActivityItemRead(it core.ActivityItem, read bool) tea.Cmd`, used again by Task 6. Test helpers `activityReadApp`, `setReadCall`, `unreadMention`, used again by Task 6.

Test notes: the toast clear is a real 3 s `tea.Tick`, so these tests never use `pressKey`/`drainBatch` (they run ticks synchronously). They read results with `deliveredMsgs[ActivityReadSetMsg]` (`cmdmsgs_test.go`), which returns as soon as that message arrives. Fake `SetRead` calls go on a buffered channel, so they are race-free under `-race`.

- [ ] **Step 1: Write the failing tests** (`internal/ui/activity_read_test.go`)

```go
package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
)

// setReadCall records one ActivityService.SetRead call.
type setReadCall struct {
	team ids.TeamID
	key  string
	read bool
}

// fakeSetRead records each call on calls and answers with err.
func fakeSetRead(calls chan setReadCall, err error) core.ActivitySetReadFunc {
	return func(team ids.TeamID, it core.ActivityItem, read bool) core.Msg {
		calls <- setReadCall{team, it.Key, read}
		return ActivityReadSetMsg{TeamID: string(team), Key: it.Key, Read: read, Err: err}
	}
}

// activityReadApp is an App in T1 with the Activity view open and
// focused on items. Its SetRead calls arrive on the returned channel and
// are answered with err.
func activityReadApp(t *testing.T, err error, items ...core.ActivityItem) (*App, chan setReadCall) {
	t.Helper()
	a := newTestApp(t, withSize(120, 30), withActiveTeam("T1"))
	calls := make(chan setReadCall, 8)
	a.SetActivityService(core.NewActivityService(core.ActivityServiceFuncs{SetRead: fakeSetRead(calls, err)}))
	a.Update(ActivityViewActivatedMsg{})
	a.Update(ActivityListLoadedMsg{TeamID: "T1", Items: items})
	return a, calls
}

func unreadMention(key string) core.ActivityItem {
	return core.ActivityItem{Key: key, Type: "at_user", ChannelID: "C1", TS: "1.1", FeedTS: "1.1", IsUnread: true}
}

func selectedActivity(t *testing.T, a *App) core.ActivityItem {
	t.Helper()
	it, ok := a.activityView.SelectedItem()
	if !ok {
		t.Fatal("no Activity item selected")
	}
	return it
}

func TestActivityU_TogglesReadState(t *testing.T) {
	a, calls := activityReadApp(t, nil, unreadMention("k1"))

	_, cmd := a.Update(keyPress('U'))
	if selectedActivity(t, a).IsUnread || a.sidebar.ActivityUnreadCount() != 0 {
		t.Fatalf("U did not mark the item read at once (badge %d)", a.sidebar.ActivityUnreadCount())
	}
	a.Update(deliveredMsgs[ActivityReadSetMsg](t, cmd)[0])
	if c := <-calls; c != (setReadCall{"T1", "k1", true}) {
		t.Errorf("first U asked Slack for %+v, want T1/k1 read", c)
	}

	_, cmd = a.Update(keyPress('U'))
	if !selectedActivity(t, a).IsUnread || a.sidebar.ActivityUnreadCount() != 1 {
		t.Fatalf("second U did not mark the item unread again (badge %d)", a.sidebar.ActivityUnreadCount())
	}
	a.Update(deliveredMsgs[ActivityReadSetMsg](t, cmd)[0])
	if c := <-calls; c.read {
		t.Errorf("second U asked Slack for read=%v, want false", c.read)
	}
}

// The channel pane sits hidden behind the Activity view; U used to mark
// its selected message unread.
func TestActivityU_DoesNotMarkHiddenChannelMessage(t *testing.T) {
	a, _ := activityReadApp(t, nil, unreadMention("k1"))
	a.activeChannelID = "C9"
	a.messagepane.SetMessages(testMessageItems(3)) // selects the last one

	_, cmd := a.Update(keyPress('U'))
	if cmd == nil {
		t.Fatal("U in the Activity view returned no command")
	}
	switch got := cmd().(type) {
	case ActivityReadSetMsg:
	case MarkUnreadMsg:
		t.Fatalf("U in the Activity view marked hidden channel %s unread", got.ChannelID)
	default:
		t.Fatalf("U in the Activity view produced %T, want ActivityReadSetMsg", got)
	}
}

func TestActivityReadSetError_RollsBackAndToasts(t *testing.T) {
	a, _ := activityReadApp(t, errors.New("invalid_arguments"), unreadMention("k1"))

	_, cmd := a.Update(keyPress('U'))
	a.Update(deliveredMsgs[ActivityReadSetMsg](t, cmd)[0])

	if !selectedActivity(t, a).IsUnread || a.sidebar.ActivityUnreadCount() != 1 {
		t.Errorf("a failed mark-read was not undone (badge %d)", a.sidebar.ActivityUnreadCount())
	}
	if got := statusbarText(a); !strings.Contains(got, "Couldn't mark activity item read: invalid_arguments") {
		t.Errorf("status bar = %q, want the failure toast", got)
	}
}

func TestActivityReadSet_OtherTeamIgnored(t *testing.T) {
	a, _ := activityReadApp(t, nil, unreadMention("k1"))
	a.Update(keyPress('U'))

	a.Update(ActivityReadSetMsg{TeamID: "T2", Key: "k1", Read: true, Err: errors.New("boom")})
	if selectedActivity(t, a).IsUnread {
		t.Error("another workspace's failure rolled back this workspace's item")
	}
}

// Review focus 1: U, U quickly; the second (unread) request fails. Slack
// holds the first request's state (read), and so must the view.
func TestActivityU_SecondToggleFailsEndsInSlackState(t *testing.T) {
	a, calls := activityReadApp(t, nil, unreadMention("k1"))
	_, first := a.Update(keyPress('U'))

	a.SetActivityService(core.NewActivityService(core.ActivityServiceFuncs{
		SetRead: fakeSetRead(calls, errors.New("ratelimited")),
	}))
	_, second := a.Update(keyPress('U'))

	results := deliveredMsgs[ActivityReadSetMsg](t, first, second)
	a.Update(results[0])
	a.Update(results[1])
	if selectedActivity(t, a).IsUnread || a.sidebar.ActivityUnreadCount() != 0 {
		t.Errorf("item ended unread (badge %d); Slack has it read", a.sidebar.ActivityUnreadCount())
	}
}

// Review focus 2: the feed is refetched between a toggle and its failure.
func TestActivityReadSetError_AfterRefetchIsHarmless(t *testing.T) {
	a, _ := activityReadApp(t, errors.New("boom"), unreadMention("k1"))
	_, cmd := a.Update(keyPress('U'))
	a.Update(ActivityListLoadedMsg{TeamID: "T1", Items: []core.ActivityItem{unreadMention("k2")}})

	a.Update(deliveredMsgs[ActivityReadSetMsg](t, cmd)[0])
	if it := selectedActivity(t, a); it.Key != "k2" || !it.IsUnread {
		t.Errorf("rollback for k1 changed the refetched list: %+v", it)
	}
	if n := a.sidebar.ActivityUnreadCount(); n != 1 {
		t.Errorf("badge = %d, want 1 (the refetched list's count)", n)
	}
}

// Review focus 3.
func TestActivityU_EmptyListIsNoOp(t *testing.T) {
	a, calls := activityReadApp(t, nil)
	if _, cmd := a.Update(keyPress('U')); cmd != nil {
		t.Error("U on an empty Activity list returned a command")
	}
	if len(calls) != 0 {
		t.Error("U on an empty Activity list called Slack")
	}
}

// Review focus 4: with the thread pane focused, U is about the thread's
// replies, not the Activity list. No reply is selected here, so it does
// nothing at all.
func TestActivityU_ThreadFocusedKeepsMessageMeaning(t *testing.T) {
	a, calls := activityReadApp(t, nil, unreadMention("k1"))
	a.focusedPanel = PanelThread

	if _, cmd := a.Update(keyPress('U')); cmd != nil {
		if _, ok := cmd().(ActivityReadSetMsg); ok {
			t.Error("U with the thread pane focused changed the Activity item")
		}
	}
	if len(calls) != 0 || !selectedActivity(t, a).IsUnread {
		t.Error("U with the thread pane focused touched the Activity item")
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/ui -run 'TestActivityU_|TestActivityReadSet' -count=1`
Expected: FAIL. In particular `TestActivityU_TogglesReadState` (item still unread) and `TestActivityU_DoesNotMarkHiddenChannelMessage` ("marked hidden channel C9 unread").

- [ ] **Step 3: Add the helper and the result arm** (`internal/ui/reducer_activity.go`)

Add `"time"` to the imports. Extend the file's header comment list with:

```go
//	ActivityReadSetMsg        - Slack answered a read-state change: on
//	                            error, undo the optimistic change and
//	                            toast.
```

(and change "Five arms" to "Six arms").

Add before `var reduceActivity`:

```go
// setActivityItemRead applies an Activity item's new read state at once
// (its dot and the sidebar badge) and returns the command that asks
// Slack for it. The ActivityReadSetMsg arm undoes the change if Slack
// refuses.
func (a *App) setActivityItemRead(it core.ActivityItem, read bool) tea.Cmd {
	if a.activeTeamID == "" {
		return nil
	}
	a.activityView.SetItemRead(it.Key, read)
	a.sidebar.SetActivityUnreadCount(a.activityView.UnreadCount())
	activity := a.activity
	team := ids.TeamID(a.activeTeamID)
	return func() tea.Msg { return activity.SetRead(team, it, read) }
}
```

Add an arm to `reduceActivity`, after the `ActivityBodiesLoadedMsg` arm:

```go
	case ActivityReadSetMsg:
		if m.TeamID != a.activeTeamID || m.Err == nil {
			return nil, true
		}
		// Undo to the opposite of what failed: if an earlier request
		// for the same item succeeded, that is the state Slack holds.
		a.activityView.SetItemRead(m.Key, !m.Read)
		a.sidebar.SetActivityUnreadCount(a.activityView.UnreadCount())
		state := "unread"
		if m.Read {
			state = "read"
		}
		return toastWithClear(a, "Couldn't mark activity item "+state+": "+m.Err.Error(), 3*time.Second), true
```

- [ ] **Step 4: Add the key arm** (`internal/ui/mode_normal.go`, immediately before `case key.Matches(msg, a.keys.MarkUnread):`)

```go
	case a.view == ViewActivity && a.focusedPanel == PanelMessages && key.Matches(msg, a.keys.MarkUnread):
		// In the Activity view U toggles the selected item's read
		// state. The channel pane is hidden behind the view, so the
		// arm below would mark a message nobody can see.
		it, ok := a.activityView.SelectedItem()
		if !ok {
			return nil
		}
		return a.setActivityItemRead(it, it.IsUnread)
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/ui -run 'TestActivityU_|TestActivityReadSet|TestMarkUnread' -count=1 -race`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
gofmt -l . && go build ./...
git add internal/ui/reducer_activity.go internal/ui/mode_normal.go internal/ui/activity_read_test.go
git commit -m "feat(activity): U toggles the selected item read/unread

The change shows at once and is undone with a toast if Slack refuses.
U in the Activity view no longer marks a message in the hidden channel
pane unread.

Refs #285"
```

---

### Task 6: Enter (or click) on an item slk cannot open

**Files:**
- Modify: `internal/ui/msgs.go` (`ActivitySelectedMsg`)
- Modify: `internal/ui/app.go` (~line 1955, the `ViewActivity` branch of the Enter handler)
- Modify: `internal/ui/reducer_mouse.go` (~line 281, the `ViewActivity` click branch)
- Modify: `internal/ui/reducer_activity.go` (`ActivitySelectedMsg` arm)
- Test: `internal/ui/activity_read_test.go`

**Interfaces:**
- Consumes: `setActivityItemRead`, `activityReadApp`, `selectedActivity` (Task 5).
- Produces: `ActivitySelectedMsg{Item core.ActivityItem}` (replaces its `ChannelID`/`TS`/`ThreadTS` fields); `(*App).activateUnopenableActivity(it core.ActivityItem) tea.Cmd`.

- [ ] **Step 1: Write the failing tests** (append to `internal/ui/activity_read_test.go`; add `"time"` and `tea "charm.land/bubbletea/v2"` to its imports)

```go
func reminder(unread bool) core.ActivityItem {
	return core.ActivityItem{Key: "saved_reminder.1.1.0", Type: "saved_reminder", Subtype: "reminder",
		FeedTS: "1791803337.000000", IsUnread: unread}
}

// enterOnActivity presses Enter and feeds the resulting
// ActivitySelectedMsg back through Update, returning that step's command.
func enterOnActivity(t *testing.T, a *App) tea.Cmd {
	t.Helper()
	_, cmd := a.Update(keyCode(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("Enter in the Activity view returned no command")
	}
	sel, ok := cmd().(ActivitySelectedMsg)
	if !ok {
		t.Fatal("Enter in the Activity view did not select the item")
	}
	_, cmd = a.Update(sel)
	return cmd
}

func TestActivityEnter_UnreadReminderMarksReadAndToasts(t *testing.T) {
	a, calls := activityReadApp(t, nil, reminder(true))

	cmd := enterOnActivity(t, a)
	if selectedActivity(t, a).IsUnread || a.sidebar.ActivityUnreadCount() != 0 {
		t.Errorf("Enter did not mark the reminder read (badge %d)", a.sidebar.ActivityUnreadCount())
	}
	if got := statusbarText(a); !strings.Contains(got, "Reminder: open Slack to see it") {
		t.Errorf("status bar = %q, want the reminder toast", got)
	}
	deliveredMsgs[ActivityReadSetMsg](t, cmd)
	if c := <-calls; c != (setReadCall{"T1", "saved_reminder.1.1.0", true}) {
		t.Errorf("Enter asked Slack for %+v, want the reminder marked read", c)
	}
}

// Enter on a reminder already read only explains; it must not call
// Slack. The command is the toast's 3 s clear tick, so watch for a call
// for a short while instead of waiting the tick out.
func TestActivityEnter_ReadReminderOnlyToasts(t *testing.T) {
	a, calls := activityReadApp(t, nil, reminder(false))

	cmd := enterOnActivity(t, a)
	if got := statusbarText(a); !strings.Contains(got, "Reminder: open Slack to see it") {
		t.Errorf("status bar = %q, want the reminder toast", got)
	}
	go deliverFirst(cmd, make(chan ActivityReadSetMsg, 1))
	select {
	case c := <-calls:
		t.Errorf("Enter on a read reminder asked Slack for %+v", c)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestActivityEnter_UnknownItemMarksReadAndToasts(t *testing.T) {
	a, calls := activityReadApp(t, nil, core.ActivityItem{Key: "x1", Type: "brand_new_type", FeedTS: "1.1", IsUnread: true})

	cmd := enterOnActivity(t, a)
	if got := statusbarText(a); !strings.Contains(got, "slk can't open this activity item yet") {
		t.Errorf("status bar = %q, want the can't-open toast", got)
	}
	deliveredMsgs[ActivityReadSetMsg](t, cmd)
	if c := <-calls; !c.read || c.key != "x1" {
		t.Errorf("Enter asked Slack for %+v, want x1 marked read", c)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/ui -run TestActivityEnter_ -count=1`
Expected: FAIL (no toast; the reminder stays unread).

- [ ] **Step 3: Carry the whole item in `ActivitySelectedMsg`** (`internal/ui/msgs.go`)

```go
	// ActivitySelectedMsg is dispatched when the user presses Enter on
	// (or clicks) an Activity row. The App opens the underlying thread
	// (ThreadTS set) or channel message (jumps to TS); an item with no
	// channel has nothing to open and is acknowledged instead.
	ActivitySelectedMsg struct {
		Item core.ActivityItem
	}
```

In `internal/ui/app.go` (Enter, `ViewActivity` branch) replace

```go
		channelID, ts, threadTS := it.ChannelID, it.TS, it.ThreadTS
		return func() tea.Msg {
			return ActivitySelectedMsg{ChannelID: channelID, TS: ts, ThreadTS: threadTS}
		}
```

with

```go
		return func() tea.Msg { return ActivitySelectedMsg{Item: it} }
```

In `internal/ui/reducer_mouse.go` (`ViewActivity` click branch) replace

```go
					channelID, ts, threadTS := it.ChannelID, it.TS, it.ThreadTS
					return func() tea.Msg {
						return ActivitySelectedMsg{ChannelID: channelID, TS: ts, ThreadTS: threadTS}
					}
```

with

```go
					return func() tea.Msg { return ActivitySelectedMsg{Item: it} }
```

- [ ] **Step 4: Handle it** (`internal/ui/reducer_activity.go`)

Add after `setActivityItemRead`:

```go
// activateUnopenableActivity is Enter on an item with no conversation (a
// Slack reminder, or a type slk does not know): there is nothing to
// open, so acknowledge it by marking it read, and say why nothing
// opened.
func (a *App) activateUnopenableActivity(it core.ActivityItem) tea.Cmd {
	text := "slk can't open this activity item yet"
	if it.Type == "saved_reminder" {
		text = "Reminder: open Slack to see it"
	}
	toast := toastWithClear(a, text, 3*time.Second)
	if !it.IsUnread {
		return toast
	}
	return tea.Batch(a.setActivityItemRead(it, true), toast)
}
```

Replace the start of the `ActivitySelectedMsg` arm, up to and including the `if m.ChannelID == "" { return nil, true }` block, with:

```go
	case ActivitySelectedMsg:
		it := m.Item
		if it.ChannelID == "" {
			return a.activateUnopenableActivity(it), true
		}
		// Thread target: open the thread panel directly, reusing the
		// permalink helper (builds the parent from the pane buffer or
		// thread cache, else a stub the replies-loaded handler
		// backfills). The Activity view stays visible behind the
		// thread panel, mirroring the Threads view.
		if it.ThreadTS != "" {
			return a.openThreadForPermalink(it.ChannelID, it.ThreadTS), true
		}
		// Channel message target: reuse the in-app permalink-jump
		// machinery. Set the pending nav so the ChannelSelectedMsg /
		// MessagesLoadedMsg arms select (or FetchAround) the target ts,
		// then switch to the channel.
```

In the rest of the arm, replace every `m.ChannelID` with `it.ChannelID` and `m.TS` with `it.TS` (6 occurrences: `m.TS` twice, `m.ChannelID` four times); `go build` fails on any you miss. Update the header comment's `ActivitySelectedMsg` entry to add: "An item with no channel is marked read and explained with a toast."

- [ ] **Step 5: Fix any test that read the old fields, then run the package**

Run: `grep -rn "ActivitySelectedMsg" --include=*_test.go internal`
For each hit reading `.ChannelID`, `.TS` or `.ThreadTS` off the message, change it to `.Item.ChannelID` etc.

Run: `go test ./internal/ui -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
gofmt -l . && go build ./... && go vet ./internal/ui
git add internal/ui
git commit -m "fix(activity): acknowledge items slk cannot open

Enter (or a click) on a reminder or an unknown item used to do nothing,
so the item could never be cleared. It now marks the item read and says
why nothing opened.

Fixes #285"
```

---

### Task 7: Docs and full verification

**Files:**
- Modify: `wiki/Keybindings.md` (after line 41, the `u` activity row)

- [ ] **Step 1: Document `U` in the Activity view** — insert after the `| \`u\` | Normal (activity) | …` row:

```markdown
| `U` | Normal (activity) | Toggle the selected Activity item read/unread |
```

- [ ] **Step 2: Full verification**

Run each; all must pass and `gofmt -l .` must print nothing:

```bash
go build ./...
go vet ./...
gofmt -l .
golangci-lint run
go test ./... -race -count=1
```

Expected: build/vet/lint clean, all packages `ok`.

- [ ] **Step 3: Commit**

```bash
git add wiki/Keybindings.md
git commit -m "docs(activity): document U in the Activity view

Refs #285"
```

- [ ] **Step 4: Manual check against the live workspace** (for the human partner, on the dev box)

`go build -o /tmp/opencode/slk ./cmd/slk && SLK_DEBUG=1 /tmp/opencode/slk`, open Activity in Truelist (`Ctrl+a`):
- the top row shows `◷ Reminder` with the hint on line 2;
- `U` toggles its dot; quit, relaunch, and the state is kept;
- Enter on it while unread clears the dot and shows `Reminder: open Slack to see it`;
- `grep "item with no channel ref" slk-debug.log` finds no reminder lines.

