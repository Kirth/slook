# Message cache sync for the Gio frontend

**Date:** 2026-10-29
**Status:** implemented (`internal/msgsync`, `internal/cache` spans, `gui/backend`)

## Problem

The Gio frontend fetched the newest 100 messages from Slack on every window
open and cached nothing. The TUI's caching code (`cmd/slk/history.go`,
`cmd/slk/rtm_handler.go`) was `package main`, written three times over, and
wrong in ways a cache cannot afford: a WebSocket edit overwrote a row's
`subtype` and `reply_count`, `edited_at` and `version` were never written (so
`cached_latest_updates` always sent `{}`), a live thread reply never changed its
parent's count, and nothing recorded which ranges of a channel were complete.

The cache is also meant to become the base for local indexing and search that
does not depend on Slack's. A search over an incomplete cache is only useful if
the cache can say what it is complete *for*.

## Constraints

- **No sweeps.** `docs/superpowers/specs/2026-07-30-enterprise-grid-bootstrap-design.md`
  ties Slack signing users out (issues #5, #111) to fetching
  `conversations.history` for every channel. Everything here is one request
  per user action, in the official web client's request shape
  (`HistoryWithVersions`: 28 per page, `inclusive`, `ignore_replies`,
  `cached_latest_updates`). The only autonomous fetching is the backfill, which
  is opt-in, slow and bounded, and never runs on reconnect.
- **The cache is a cache.** Slack stays authoritative; every open still issues
  one revalidation page, and spans only ever record what Slack returned.
- The TUI need not keep working, but the root module builds and vets.

## Design

### Coverage spans

`message_spans(channel_id, workspace_id, oldest_ts, latest_ts, reaches_start,
is_live)`, primary key `(channel_id, oldest_ts)`: every channel-feed message
(the rows `conversations.history` returns: top-level, parents, broadcasts) with
`oldest_ts <= ts <= latest_ts` is in `messages`, possibly soft-deleted.
`reaches_start` means a backwards page ended with `has_more=false`; `is_live`
means the span reaches the head and the socket is up, so live messages extend
it. `ClearLiveSpans` on disconnect: the socket replays nothing.

`AddSpan` merges spans that share a ts. Adjacency is the caller's knowledge, and
the caller (`msgsync.Ingest`) expresses it by the bounds it records for a page:

| anchor | range recorded |
|---|---|
| head (none) | `[oldest returned, newest returned]`; `has_more=false` ⇒ reaches start |
| `Latest=X` | `[oldest returned, X]` (X may be deleted; the page was asked inclusive of it) |
| `Oldest=X`, `has_more=false` | `[X, newest returned]` |
| `Oldest=X`, `has_more=true` | `[oldest returned, newest returned]` only: Slack anchors such a page at the head, not at X (`GetHistoryAround`'s note), so the hole below it stays a hole |

A hole is filled by the next `Older` page across it (a scroll back, or the
backfill), which overlaps both neighbours and merges them.

### Write semantics (`internal/cache/messages.go`)

- `UpsertMessages`: one transaction; authoritative for everything the API
  returns, including `is_deleted=0` and `edited_at`; `version` from
  `latest_updates`, kept when a write carries none.
- `ApplyEdit`: a `message_changed` touches `text`, `raw_json`, `edited_at`,
  plus `thread_ts` when the event carries one and `reply_count` as a maximum
  (idempotent with the increment a reply event causes). Never `subtype`, never
  `version`. No row, no write.
- `IncrementReplyCount` for a live reply; `PruneChannelRange` soft-deletes feed
  rows inside a page's own bounds that it neither returned nor confirmed
  unchanged, guarded by `created_at < requestStart` (a row cached mid-request is
  newer, not gone); broadcasts are not candidates because whether
  `ignore_replies` hides them is unmeasured.

### `internal/msgsync`

Importable; depends on `internal/cache`, `internal/slack`, slack-go. A `Source`
interface (`HistoryWithVersions`, `GetReplies`) that `*slackclient.Client`
satisfies and tests fake. `Open` (head page vouching for the newest 28 cached),
`Older` (page at or before a ts), `CatchUp` (page forward from the newest span),
`Thread` (whole thread, pruning stale replies), `ApplyEvent` /
`ApplyReaction` for socket frames, `Ingest` behind them, `Versions` (moved from
`cmd/slk/bootstrap_adapters.go`). `Backfill(ctx, Config)`: per channel, bridge
holes then page back to `reaches_start` or the horizon; then threads whose
parent's `reply_count` exceeds the cached replies; one request per `Interval`
(default 8 s), held while `Paused`, resumable from the spans.

### Socket frames whole (`internal/slack`)

`MessageEventHandler` is an optional interface the dispatcher type-asserts
(like `PendingEventHandler`). It delivers `MessageEvent{Kind, Message
slack.Message, Raw}`: the frame decoded whole, so the cache gets the real
message JSON and a `message_changed` keeps the parent's `reply_count`,
`thread_ts` and the edit's ts, which `OnMessage` flattens away.

### GUI (`gui/backend`, `gui/chat`, `gui/cmd/slk-gio`)

- `History`: spans present ⇒ read the cache, return, and `CatchUp` in the
  background; none ⇒ one `Open` page first. `Older` reads the cache when a
  span covers the page, else fetches one.
- All cache writes from socket frames and all `emit`s happen on `Run`'s
  goroutine; fetches run in their own goroutines and post their results to the
  same queue. SQLite (WAL, `busy_timeout`) serialises the rest.
- `HistoryUpdated{ChannelID, Messages, Deleted}` is a new event: the cache
  catching up. `hub.dispatch` routes it to the conversation's window only,
  never to the list (a `MessagePosted` would badge the channel);
  `chatWindow.apply` handles it before the generic `Event` case so it is not
  marked read. `chat.Timeline.Merge` replaces by ts, drops deleted rows, skips
  thread-only replies and leaves ephemerals alone.
- `ConnectSlack` writes the workspace row, `Conversations` the channel rows,
  profile lookups the user rows (the FKs need the workspace; search needs the
  names).
- `slk-gio -backfill 1y|90d|0`: opt-in backfill, paused while a chat window has
  focus.

## Addendum (same day): the search window

- `gui/cmd/slk-gio/searchwin.go`, opened from the list header's search
  button (`hub.showSearch`, one window, `moveHere` like the others). Search
  as you type over `cache.SearchMessages` (workspace-wide, replies included,
  150 ms debounce, stale results dropped by generation); while the box is
  empty, each conversation's coverage from `SpansForWorkspace` +
  `MessageCounts` (`backend.SyncStatus`); a panel with the backfill's
  progress (`msgsync.Config.OnProgress` → `backend.BackfillStatus`), Start
  with a horizon / Stop (`backend.StartBackfill` / `StopBackfill`; the
  `-backfill` flag calls the same), and the fetches in flight (`Active`,
  from `(*Slack).track`). `backend.SyncChanged` tells the window to re-read
  the status (coalesced, at most one read per 400 ms).
- A result opens its conversation scrolled to the message:
  `hub.openChatAt` → `jumpTo` / `moveHere{jumpTS}`; the chat window's
  `settleJump` pages back with `backend.Older` (cursor = the returned page's
  oldest ts, at most 30 pages), then `list.ScrollTo` with 120 dp of context
  above and a two-second highlight. `ScrollToEnd` is never flipped: Gio's
  list resumes end-anchoring once `Position.BeforeEnd` is false again. A
  reply that lives only in its thread opens its parent.
- The pause hook is `(*Slack).SetPaused`, set once from `main` with the hub's
  `anyFocused`; the search window does not count as focus.

## Addendum (same day): reconnecting

`backend.Slack.Run` dialled the socket once; `slackclient.ConnectionManager`
(the TUI's redial loop with 1 s to 30 s backoff) now owns the connection. `Run`
dedupes the up/down state the handler reports (a failed redial reports down
each attempt), logs each flip with the time, clears live spans on a drop, and
on a reconnect (not the first connect, at most every 30 s) refreshes
`client.counts` into `ReadChanged` events; each open chat window re-reads its
history on reconnect (`chatWindow.resync`, 30 s gate), which runs the usual
catch-up page. Name lookups on `Run`'s goroutine have a 10 s deadline and the
shared HTTP client a 60 s timeout, so a hung request cannot stall event
delivery or a window's loading state. The backfill holds while the socket is
down.

## Not done here

- Scroll-to-top paging in the chat window (a jump pages back; a scroll does
  not yet).
- Reactions are cached, not drawn.
- Ranking or highlighting matched terms in search results (newest first today).
- The TUI's three writers still exist; `msgsync` is their replacement when the
  TUI is next touched.
