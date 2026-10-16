# Activity: saved reminders and per-item read state

Fixes #285.

## Purpose

Some Activity rows render blank, do nothing on Enter, and can never be
cleared. Make every Activity item readable and clearable:

1. Recognise `saved_reminder` items and render them as reminders.
2. Label any other unrecognised item type instead of drawing a blank row.
3. Let the user mark any Activity item read or unread, with Slack's own
   activity read-state endpoints.

### Success criteria

- The Truelist reminder that triggered this (top of Activity, key
  `saved_reminder.12285215116245.1.0`) shows as `◷ Reminder` with its time
  and unread dot, not a blank row.
- `U` on any Activity row toggles its unread dot, and the change survives
  a refetch (it is Slack's state, not local state).
- Enter on the unread reminder clears its dot and toasts an explanation.
- `U` in the Activity view no longer touches the hidden channel pane.

## Background

### Why the row is blank

`flattenActivityItem` (`internal/slack/activity.go`) extracts each
item's channel and ts with a per-type rule. `activityFeedTypes` asks for
`saved_reminder`, but no rule handles it, so the item lands with an empty
`ChannelID` and `TS`. Downstream:

- `activityRefs` skips it, so no body is hydrated (line 2 empty).
- `contextVerb` returns `""` for the type, and there is no author, so
  line 1 is only the `•` glyph and the time.
- `reduceActivity`'s `ActivitySelectedMsg` arm returns early on an empty
  `ChannelID`, so Enter does nothing.
- Read state can only change by opening the conversation, which is
  impossible, so the item stays unread forever.

Commit `87d34ab4` fixed the same symptom for `channel` items, against a
hand-built payload. This time the payload is a live capture.

### The `saved_reminder` payload (captured 2026-10-12, Truelist)

```json
{
  "feed_ts": "1791803337.000000",
  "is_archived": false,
  "is_unread": true,
  "item": {
    "saved_reminder_payload": {
      "date_due": 1791803337,
      "item_id": "Sa0C8D6B3E77",
      "item_id_2": "0000000000.000000",
      "item_id_3": "",
      "item_type": "reminder"
    },
    "type": "saved_reminder"
  },
  "key": "saved_reminder.12285215116245.1.0",
  "version": "…"
}
```

There is no message ref and no reminder text. Paging the entire Truelist
feed found this as the only `saved_reminder` and the only type
`flattenActivityItem` did not recognise (`at_channel` 7, `at_user` 563,
`bot_dm_bundle` 8, `dm` 5, `message_reaction` 481, `saved_reminder` 1,
`thread_v2` 935).

### Slack's activity read-state endpoints (probed 2026-10-12, Truelist)

Both take one form field, `entries`, a JSON array of objects with the
required fields `type`, `key` and `feed_ts` (the validator's own error
messages name them):

| Method | Effect | Response |
|---|---|---|
| `activity.markRead` | clears `is_unread` | `{"ok":true,"undo_key":<int>}` |
| `activity.markUnread` | sets `is_unread` | `{"ok":true}` |

A round trip on the reminder (read → unread → read) was reflected by
`activity.feed` within 1.5 s each time. Every feed item carries
`type`, `key` and `feed_ts`, so read state works for every item,
including types slk cannot open or describe.

### Prior work

PR #296 (closed unmerged) added two commits that apply unchanged to the
current code:

- `6f8d5ed1` labels an unrecognised type from the type itself
  (`fallbackVerb`: `saved_reminder` → "Saved reminder", empty →
  "Unknown activity"), with tests.
- `b57958ba` debug-logs the raw JSON of any item left with no channel
  ref.

They are cherry-picked first, then built on.

### `U` in the Activity view today

`U` reaches `markUnreadOfSelected` → `selectedMessageContext` →
`focusedContentPane()`, which returns the channel pane for
`PanelMessages` even when the Activity view is drawn over it. `U` there
silently marks a message in the hidden channel unread.

## Design

### Slack client (`internal/slack/activity.go`)

- `flattenActivityItem` gets a `saved_reminder` case that decodes
  `item.saved_reminder_payload` and sets `ActivityItem.Subtype` to its
  `item_type`. `ChannelID`/`TS` stay empty: no observed reminder points
  at a message, and `item_id_2` is a zero ts.
- New `(*Client).SetActivityRead(ctx, typ, key, feedTS string, read bool) error`
  posts `activity.markRead` (read) or `activity.markUnread` (unread)
  through `postForm`, with `entries` set to a one-element array. `ok:false`
  becomes an error carrying Slack's `error` string.
- The `TODO(activity): activity.markRead` comment on `GetActivityFeed`
  is removed.

### Core (`internal/core`)

- `ActivityItem` gains `Subtype string` (`saved_reminder`'s `item_type`;
  empty for other types).
- `ActivityService` gains
  `SetRead(teamID ids.TeamID, item ActivityItem, read bool) Msg`.
  Its doc comment loses "Marking items read is out of scope".
- `NewActivityService(fetch, hydrate)` becomes
  `NewActivityService(ActivityServiceFuncs{Fetch, Hydrate, SetRead})`,
  matching the Thread/Message/Channel services. Any nil func no-ops
  (nil Msg). Callers: `cmd/slk/main.go`, `internal/ui/service_defaults.go`.

### Wiring (`cmd/slk/activity.go`)

`activitySetReadFunc(router)` resolves the workspace by `teamID` (as the
two existing closures do), calls `SetActivityRead` with a 15 s timeout,
and always returns `ui.ActivityReadSetMsg{TeamID, Key, Read, Err}`: the
UI needs the error to roll back. A nil workspace returns the message with
an error too.

### View (`internal/ui/activityview`)

- `saved_reminder` card: glyph `◷` (one column, no emoji presentation,
  like the other glyphs), context label `Reminder`, the usual relative
  time and unread dot. Line 2 is a muted hint, `Open Slack to see this
  reminder`, because the feed carries no reminder text.
- Other unrecognised types use `fallbackVerb` (from #296).
- `SetItemRead(key string, read bool) bool` sets one item's `IsUnread`
  to `!read`, bumps the version, and returns false when no item has that
  key (the list was refetched in between).

### App (`internal/ui`)

New message in `msgs.go`:

```go
ActivityReadSetMsg struct {
	TeamID string
	Key    string
	Read   bool  // the state that was requested
	Err    error
}
```

A shared helper, `(*App).setActivityItemRead(it core.ActivityItem, read bool) tea.Cmd`,
applies the change optimistically (`SetItemRead`, then
`sidebar.SetActivityUnreadCount(activityView.UnreadCount())`) and returns
the `activity.SetRead` command. Both entry points below use it.

At most one request per item is in flight (amended after PR review:
two concurrent requests could reach Slack in either order and leave it
holding the older state). The App keeps `activityReads`, keyed by team and
item key, each entry holding the item's workspace, the item, `want` (the state the user asked
for last), `confirmed` (the state Slack holds as far as we know: the
item's state when the first change began, then each success) and
`inFlight`. A change while a request is in flight only moves the dot and
updates `want`; nothing is sent. Requests always go to the entry's own
workspace, whichever one is on screen.

**`U` toggles.** In `mode_normal.go`, ahead of the general `MarkUnread`
arm: when `a.view == ViewActivity && a.focusedPanel == PanelMessages` and
`MarkUnread` matches, take the selected item and call
`setActivityItemRead(it, it.IsUnread)` (unread → read, read → unread).
Nothing selected: no-op. With the thread pane focused in the Activity
view, `U` keeps its message meaning.

**Enter on an item slk cannot open.** In the `ActivitySelectedMsg` path,
when the item has no `ChannelID`:

- if it is unread, mark it read via `setActivityItemRead`;
- always toast: `Reminder: open Slack to see it` for `saved_reminder`,
  else `slk can't open this activity item yet`.

`ActivitySelectedMsg` carries the whole item, and the `TeamID` it was
selected in; a selection delivered after a workspace switch is ignored.
Enter on openable items is unchanged; opening the conversation already
marks it read.

**Result.** `reduceActivity` handles `ActivityReadSetMsg` for its
`activityReads` entry: clear `inFlight`; on success set `confirmed`.
Then, if `want == confirmed`, drop the entry. Otherwise, on success send
`want` to the entry's workspace (the user changed it meanwhile), even if
another workspace is now on screen. On error drop the entry; if its
workspace is the active one, `SetItemRead(Key, confirmed)`, refresh the
sidebar badge, and `toastWithClear`
`Couldn't mark activity item read|unread: <err>`. Errors are not retried.

### Error handling and edge cases

- **Failure:** rolled back to the state Slack holds; the user sees the
  dot return and a toast. If the user already toggled back while the
  failed request was in flight, the dot already matches Slack and
  nothing is shown.
- **In-flight fetch:** a fetch already in flight when the user toggles
  can return the pre-toggle state and overwrite it. Fetches happen only
  on view activation and on the unread-only toggle, and the next one
  shows Slack's real state. Accepted.
- **Unread-only filter on:** marking an item read leaves it in the list
  until the next fetch, so the user can undo with `U`. No client-side
  filtering is added.
- **Workspace switch mid-request:** a change queued behind the request is
  still sent, to the item's own workspace. A failure there has no dot on
  screen to undo, so it only ends the entry; that workspace's feed
  reloads from Slack on activation.

## Out of scope

- Reminder text (would need the saved-items API, another undocumented
  endpoint).
- Opening the message a reminder is attached to; none was observed.
- Marking an openable item read when Enter opens it (the conversation
  read already covers it).
- `activity.markAllRead` and the `undo_key` undo flow.

## Testing

`internal/slack`:
- Parse test with the captured reminder payload above: `Type`,
  `Subtype`, `IsUnread`, `Key`, `FeedTS` set; `ChannelID`/`TS` empty.
- `SetActivityRead` against an `httptest` server: method path per
  `read`, `entries` decodes to exactly `[{type,key,feed_ts}]`, `ok:false`
  is an error containing Slack's error string.

`internal/ui/activityview`:
- Reminder card shows `◷`, `Reminder` and the hint; fallback label for an
  unknown type (from #296).
- `SetItemRead` flips `IsUnread`, bumps `Version`, returns false for an
  unknown key.

`internal/ui`, through the real `Update` chain (`pressKey`, `arrive`,
`toastText`; `SetRead` faked on the `App`):
- `U` on an unread item marks it read (dot and badge drop, `SetRead`
  called with `read=true`); `U` again marks it unread.
- `U` in Activity produces no `MarkUnreadMsg`.
- An error result restores the dot and badge and toasts.
- A result for another team is ignored.
- Enter on an unread reminder marks it read and toasts; on a read
  reminder it toasts and does not call `SetRead`.

## Docs

- `wiki/Keybindings.md`: add `U` (Normal, activity) "Toggle the selected
  Activity item read/unread".
- `internal/ui/keys.go` help for `U` stays "mark unread"; the Activity
  meaning is documented in the wiki row.
- `AGENTS.md` gets no new helper rows: nothing added is meant for reuse
  outside the Activity view.
