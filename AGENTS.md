# AGENTS.md

Orientation for anyone — human or agent — writing code in this repo.

**The single most important rule: search before you write.** The most common
defect in this codebase's history is not bugs, it is the same logic implemented
a fourth time because the author did not know the first three existed. See
[Shared code](#shared-code--check-here-before-writing-a-helper) below.

slk is two programs over one set of shared packages: `slk`, a CLI that signs
workspaces in and exports channels (`cmd/slk`), and `slk-gio`, the desktop
client, built with [Gio](https://gioui.org) in its own Go module (`gui/`). The
terminal client (the TUI) that slk started as was retired on 2026-10-31; its
code is in git history (commit `44e6f30a`, "ui: retire the … TUI"), and
`docs/superpowers/plans/2026-10-31-tui-sunset-log.md` records how it went.

## Build, test, lint

Two Go modules: the root (`github.com/gammons/slk`: `cmd/slk` and every
`internal/` package) and `gui/` (`github.com/gammons/slk/gui`, which imports the
root through `replace github.com/gammons/slk => ../`). Run each command in both.

```
go build ./...                # and: cd gui && go build ./...
go test ./...                 # root ~4s,  gui ~3s
go test ./... -race           # root ~33s, gui ~12s; this is what CI runs
go vet ./...
golangci-lint run             # v2.14.0, config in .golangci.yml (CI: lint, lint-gui)
gofmt -l .                    # must be empty; enforced in CI
make gui                      # bin/slk-gio, built inside gui/shell.nix when Nix is there
```

`gui/` needs cgo and Gio's libraries (Wayland, X11, xkbcommon, EGL/GLES,
Vulkan headers): `gui/shell.nix` lists them, and CI's `gui` job installs the
Debian names. On a machine without Go on `PATH`, run everything through
`nix-shell gui/shell.nix --run '…'` (`cd gui && nix-shell shell.nix --run '…'`
for the gui module); the root module builds there too. `golangci-lint` is not
in that shell.

Those timings are wall clock with `-count=1` and a warm build cache on an
8-core Linux box, measured on 2026-10-31 after the TUI was removed (they were
`~10s`/`~47s` for the root module while it still held the TUI). Treat them as
a shape, not a target. Under `-race` one package is most of the root's run:
`internal/cache` 31s (then `internal/msgsync` 8s, `internal/slack/resolve`
6s); in `gui/`, `cmd/slk-gio` 11s and `backend` 5s. If you are iterating, run
the one package you are changing.

Tests are plain `testing.T`, stdlib only. No testify, no gomock, no golden
libraries. White-box (`package backend`, not `package backend_test`) by
convention; an external test package only when the test must import a package
that imports this one (`gui/backend/fake_domain_test.go` imports `gui/chat`).

## Architecture in one screen

```
cmd/slk/                    the `slk` CLI: --add-workspace (desktop app or
                            --browser), --remove-workspace, --list-workspaces,
                            export, --dump-sections. Its interactive prompts
                            (charm's huh) live only here
internal/                   shared by both programs; no Gio, no terminal UI
  core/                     plain values both sides exchange (MessageItem,
                            Attachment, ThumbSpec, PendingAttachment, …);
                            core/blocks: Block Kit and legacy-attachment values
  slack/                    Slack Web API + browser-protocol WebSocket client
    boot/                   the official client's boot calls (userBoot, counts,
                            conversations.view, …)
    edge/                   edgeapi: revalidation, channels/users info and search
    resolve/                batched user-name resolution (edge, then users.info)
    mrkdwn/                 composer text → rich_text
    slacktest/              fake Slack HTTP server for tests
    membership/             (no importer; backlog)
  slackhttp/                browser-shaped transport, request envelope, counter
  slackdesktop/             the Slack desktop app's session (cookie, tokens)
  slackurl/, slackfmt/      permalinks; mrkdwn → plain text / CommonMark,
                            mentions, date tokens, channel glyphs
  blockkit/                 Block Kit / attachment parsing (data only)
  datefmt/, theme/, text/   date separators; colour tables and user colours;
                            case/accent folding
  bootstrap/                the anti-scrape boot (bootstrap.Run)
    adapt/                  bootstrap.Deps over a real client; conversation list
  onboard/                  workspace sign-in and token re-mint, UI-free
  config/                   TOML config; config/versionts: _x_version_ts upkeep
  cache/                    SQLite cache (a cache, not a source of truth)
  msgsync/                  the one writer of the message cache; backfill
  threadsubs/, typing/      followed-thread sync; typing/app-status tracker
  searchq/                  the search query language
  service/                  section and mute stores
  image/, avatar/, emoji/   image fetcher and disk cache; avatar keys and sized
                            URLs; emoji tables, names, URLs
  clipboard/, attach/,      desktop clipboard reads, attachments from paths and
  filepick/, launch/        pictures, the file chooser, xdg-open
  notify/, mention/,        notification rules; mention tests; usergroup helpers
  usergroups/
  export/, filedl/          Markdown export; file downloads
  paths/, debuglog/         XDG paths; SLK_DEBUG log
  ids/, version/            (one / no importer; backlog)
gui/                        module github.com/gammons/slk/gui, binary slk-gio
  backend/                  the Backend interface windows use; Slack (real)
                            and Fake (-fake) implement it
  chat/                     window state and rules without a display (timeline,
                            thread, completion, presence, read gate, …)
  search/                   query resolution, suggestions, highlighting
  sidebar/                  how the conversation list is arranged
  notifications/            desktop notifications (D-Bus, notify_command)
  palette/                  the GUI's colour roles from theme tables
  rich/                     a message's display form (styled blocks)
  selection/, motion/,      text selection; small animations; readline edits
  textedit/
  internal/richtext/,       Gio text layout with per-rune positions
  internal/styledtext/
  cmd/slk-gio/              the windows: hub.go owns them and routes events;
                            each window runs its own event loop
```

Current structural documentation:

- `wiki/Architecture.md` — the same layout, for users
- `gui/README.md` — what slk-gio does, feature by feature, and its layout
- `docs/superpowers/specs/README.md` — which design docs are still
  authoritative protocol knowledge and which describe the retired TUI
- `docs/superpowers/plans/2026-10-31-gui-feature-backlog.md` — TUI features
  not ported, orphan packages, dead config keys, real-workspace checks owed
- `docs/superpowers/plans/2026-10-31-tui-sunset-log.md` — the migration,
  with every deviation and follow-up

### Invariants worth knowing

- **The GUI never reaches the retired TUI or a terminal toolkit.**
  `gui/boundary_test.go` fails on a direct import of any tree listed in its
  `guiBannedImports`: the retired TUI's packages (none exist now; the test
  keeps them from coming back), the charm terminal toolkit (`charm.land/…`,
  `github.com/charmbracelet/…`) and the sixel encoder. The only users of the
  charm toolkit left are `cmd/slk`'s prompts.
- **`gui/backend.Backend` is the one port.** Windows reach Slack, the cache and
  the desktop only through it (and the `internal/` readers it names). `Slack`
  and `Fake` implement it; both are returned as a `backend.Backend` from
  `openBackend` (`gui/cmd/slk-gio/main.go`), so the build fails when either
  lacks a method. A feature is: a `Backend` method (and its event, with an
  `isEvent()` line), the `Slack` implementation (a file per area under
  `gui/backend/`, errors to stderr as `slk-gio: <area>:`), the `Fake` one
  (`fake_<area>.go`), a test of each (`fake_test.go` style for the fake,
  `slacktest` for the Slack side), and the window. Logic a window needs goes in
  `gui/chat` (or `search`, `sidebar`) so it is tested without a display.
- **A window is reached only through its inbox.** Each window runs its own
  event loop; results from goroutines arrive as items through `inbox.post`
  (`box.post(item)`) and are applied on that loop. The hub routes events
  (`hub.dispatch`, `toChat`, `toThread`).
- **Boot never enumerates.** slk-gio boots as Slack's web client does
  (`client.userBoot`, `client.counts`, `conversations.view`, edge
  revalidation) once per process; `users.conversations` is the loud fallback
  when userBoot fails and nothing else. `TestRun_NeverEnumerates`
  (`internal/bootstrap`), `TestSlackAPI_DeclaresNoWorkspaceEnumeration`
  (`internal/slack`) and `TestConversations_BootsWithoutEnumerating`
  (`gui/backend/boot_test.go`) hold it. That enumeration is what got Enterprise
  Grid users signed out (`docs/superpowers/specs/2026-07-30-enterprise-grid-bootstrap-design.md`).
  History sweeps stay paced (`msgsync.Backfill`, one request every eight
  seconds) and honour `[cache] backfill_ignore`; nothing else fetches unasked.
- **SQLite is a cache.** Slack remains authoritative. `internal/msgsync` is
  the only writer of message rows.
- **A map shared across goroutines is never written.** Every map handed to
  another goroutine is either built for it (a copy, like
  `userNameStore.Snapshot()` or the per-message `ExternalUsers` set) or
  published once and never mutated again (swapped whole via
  `atomic.Pointer`). A copy handed over belongs to the receiver from then on:
  the sender must not keep reading it. A plain map written while another
  goroutine reads it is a `fatal error: concurrent map read and map write`,
  which no recover catches; it crashed slk on 2026-10-02 via the user-name
  map. State several goroutines write is locked (`userNameStore`,
  `backend.Slack`'s `mu`) instead.

## Shared code — check here before writing a helper

If you are about to write date formatting, case folding, mention parsing,
mrkdwn conversion, a Slack request, a cache query or a popup: it probably
already exists.

### Text and formatting

| Need | Use |
|---|---|
| Case/accent-insensitive fold for matching | `text.Fold(s)` |
| Slack mrkdwn → plain text | `slackfmt.FlattenMrkdwn`, `slackfmt.FlattenMrkdwnWithUserGroups` (`internal/slackfmt/flatten.go`); the lossier notification form is `slackfmt.StripMarkup` |
| Slack mrkdwn → CommonMark (the Markdown export) | `slackfmt.MrkdwnToCommonMark`, `slackfmt.MrkdwnToCommonMarkWithUserGroups` (`internal/slackfmt/commonmark.go`) |
| Expand `<!date^…>` tokens in text; render a date given as ts/format/fallback (a rich_text `date` element) by the same rule | `slackfmt.ExpandDateTokens(text)`, `slackfmt.FormatDateToken(ts, format, fallback)` (`internal/slackfmt/datetoken.go`; FORMAT in local time, FALLBACK only when the FORMAT is unusable; `FormatDateToken` returns "" when neither gives anything) |
| Clock layout `<!date` tokens render `{time}`/`{time_secs}` with | `slackfmt.SetDateTokenTimeLayout(layout)` — seeded once at startup from `appearance.timestamp_format` (`gui/cmd/slk-gio/main.go`); empty keeps the default `"3:04 PM"`. Package-level so every render path agrees |
| Date-separator label ("Today", "Yesterday", "Monday, Oct 27", …) | `datefmt.DateSeparator(dateStr)` (`internal/datefmt`; test clock `datefmt.SetNow`) |
| Slack ts → `time.Time` (whole seconds, any timezone) | `export.TimeFromTS` |
| A since/until/overlap date range in a timezone, and "is this ts in it?" | `export.NewWindow`, `export.Window.Contains` |
| mpdm channel name → human name | `slackfmt.FormatMPDMName` |
| Channel-type glyph (`#` / `◆` / `●`) | `slackfmt.ChannelGlyph(chType)` |
| Slack permalink parsing | `slackurl.Parse` |
| Huddle deep link for a conversation (`https://app.slack.com/huddle/<TEAM>/<CHANNEL>`) | `slackurl.HuddleURL` (no caller since the TUI; backlog) |
| Emoji shortcode → glyph | `emoji.Sprint`, `emoji.CodeMap`, `emoji.StripSkinTone` / `StripSkinToneFromText`; the name Slack stores for a picked emoji is `emoji.CanonicalSlackName` |
| A custom emoji's picture: its URL, and its key in the image cache | `emoji.BuildCustomEmojiURL`, `emoji.EmojiCacheKey(url)` (`internal/emoji/url.go`) |
| Does message text mention the current user? One of the user's usergroups (`<!subteam^S…>`, bare or labelled)? | `mention.InText(text, selfUserID)`; `mention.InGroups(text, groupIDs)` (`InText` deliberately leaves usergroups out) |
| User IDs mentioned in message text | `slackfmt.MentionedUserIDs(text)` |
| Channel IDs mentioned in message text (`<#C…\|name>`), or each with its label | `slackfmt.MentionedChannelIDs(text)`; `slackfmt.MentionedChannels(text)` returns `ChannelMention{ID, Label}` |
| Parse Block Kit blocks / legacy attachments into `core/blocks` values; mrkdwn back from a rich_text block; a message's body source (the rich_text reconstruction, else `Text`); does Block Kit already carry the body? | `internal/blockkit`: `Parse`, `ParseAttachments`, `RichTextToMrkdwn`, `MessageText(msg)`, `RendersBody(blocks)` (parse-only; `testdata/*.json` are real messages) |
| A `slack.File`'s thumbnails as `core.ThumbSpec`s, smallest first | `blockkit.FileThumbs(f)` (`internal/blockkit/files.go`) |
| Legacy attachment action ids slack-go drops | `slackclient.EphemeralMessage.ActionIDs` (decoded from the raw frame), or `slackclient.LegacyActionIDs(raw)` for any message JSON; applied with `blocks.ApplyActionIDs(atts, ids)` (`internal/core/blocks`) |
| Usergroup map helpers; a usergroup's mention handle (its own, or its name slugified), and the ID → handle map for a `usergroups.list` result | `usergroups.Copy`, `usergroups.Equal`, `usergroups.Display`; `usergroups.Handle(g)`, `usergroups.Handles(groups)` (`internal/usergroups/handles.go`) |
| The theme colour tables (built-in and custom TOML themes, lookup by name, the names list) and the per-user colour | `internal/theme`: `Colors`, `Resolve`, `ColorsByName`, `Names`, `LoadCustom(fsys)`, `RegisterCustom`, `UserColor(id)` / `UserPalette`; tests restore the registry with `SnapshotCustom` / `ClearCustom`. slk-gio loads `~/.config/slk/themes/` at startup |
| A message's display form for a non-terminal frontend (blocks of styled spans from rich_text, mrkdwn, Block Kit, attachments, files) | `gui/rich`: `Body`, `FromRichText`, `FromMrkdwn`, `FromBlocks`, `FromAttachments`, `FromFiles` |
| A message body as plain text for a one-line preview (search result) | `rich.PlainBlocks(blocks)` (`gui/rich`) |

### Slack, the cache and sync

| Need | Use |
|---|---|
| The channel a user's huddle is in (users.info) | `slackclient.UserProfile.HuddleStateChannelID` (wraps `*slack.User`; `GetUserProfile`/`GetUserProfileContext` are hand-rolled POSTs because slack-go's `UserProfile` drops that field). No caller since the TUI |
| Sleep out a slack-go rate-limit error (ctx-aware) | `slackclient.WaitOutRateLimit` |
| Page through all channel history in a ts range / a thread's replies in a ts range | `(*slackclient.Client).WalkHistory`, `GetRepliesBetween` (`slk export`) |
| Press a legacy attachment button (`chat.attachmentAction`, payload byte-identical to the web client's) | `(*slackclient.Client).AttachmentAction(ctx, AttachmentActionRequest)`, with `service_id` = the posting app's bot ID; from the GUI, `backend.Backend.PressAction` |
| Run a slash command (`chat.command`, request shape copied from the web client; arguments as rich text via `mrkdwn.Convert`) | `(*slackclient.Client).RunCommand(ctx, CommandRequest)`; a refusal (`unknown_command`, …) is `*slackclient.CommandRejectedError`. From the GUI, `backend.Backend.RunCommand` (`*core.CommandError{Code}` on refusal) |
| Is composer text a slash command, its name/args, and could it still become one (for a completion popup)? Word a command's failure | `slashcmd.Parse(text)`, `slashcmd.IsNamePrefix(text)`, `slashcmd.FailureText(name, err)` (`internal/slashcmd`); the Gio composer uses them (`gui/cmd/slk-gio/compose.go`) |
| List the workspace's slash commands (name, usage, description, app, alias) for a picker | `(*slackclient.Client).AppCommands(ctx)` (`client.appCommands`, as the web client calls it) |
| Every conversation's read state and newest-message ts in one request (`client.counts`) | `(*slackclient.Client).GetUnreadCountsContext(ctx)`; `UnreadInfo.Latest` is the newest message's ts |
| A web-client `client_token` (`web-<unix ms>`) for any request that sends one | `newClientToken()` (`internal/slack`); the only generator |
| Forward a message: post its permalink, after an optional note, to another conversation | `(*slackclient.Client).ForwardMessage(ctx, src, ts, dst, comment)` (`internal/slack/forward.go`; the untried `from_url` alternative is a comment there) |
| List a conversation's members with a page cap | `(*slackclient.Client).GetUsersInConversationMax` |
| Current DND state from a Slack API result | `slackclient.DNDStateFromStatus` |
| A WebSocket message frame whole (the decoded `slack.Message`, its raw JSON, and for `message_changed` the inner message with `reply_count`/`thread_ts`/`edited`) | implement `slackclient.MessageEventHandler` (`OnMessageEvent(MessageEvent)`) on your `EventHandler`; the dispatcher type-asserts it and then skips `OnMessage`/`OnMessageDeleted`. Ephemerals still go to `OnEphemeralMessage` |
| Wake the serialized WebSocket event owner for local work | Implement `slackclient.PendingEventHandler` (`PendingEvents() <-chan struct{}`, `OnPendingEvents()`). The dispatcher selects the wake-up channel even on an idle socket and serializes the drain with Slack callbacks; nil disables it, and the channel must never close |
| The workspace's usergroups with their members (to know the user's own) | `(*slackclient.Client).GetUserGroupsWithMembers(ctx)` (`include_users`, one request); `GetUserGroups` without members. A usergroup created or changed reaches a handler that implements the optional `slackclient.SubteamEventHandler` (`OnSubteamChanged`) |
| Can the user post in an app DM? (`app_home.messages_tab_enabled` / `messages_tab_read_only_enabled`) | `boot.ConversationsInfo` (`return_app_home=true`) or `ConversationsView`, then `(*boot.AppHome).CanReceiveMessages()` (nil is true). Not wired in slk-gio (backlog) |
| Boot a workspace the way Slack's web client does (client.userBoot, client.counts, conversations.view, edge revalidation; never an enumeration): the `bootstrap.Deps` for a real client, the conversation list from the result, the conversations `client.counts` names that userBoot left out (cache, then one edge `channels/info`), mutes from the boot prefs, first-sight cache rows, the conversation to open | `bootstrap.Run(ctx, deps)`; `internal/bootstrap/adapt`: `NewDeps(client, db, token, openChannelID, health, logf)`, `Conversations(res)`, `FillFromCounts(ctx, db, edgeClient, teamID, res, logf)`, `Muted{res}` (a `service.MutedChannelsClient`), `HydrateFirstSight(db, workspaceID, res, logf)`, `UserDisplayName(boot.User)`, `MostRecentlyVisited(visits)`; `DropClosedMPIMs` for a list from `users.conversations` |
| userBoot IM → conversation metadata | `adapt.IMConversation(im)` (`internal/bootstrap/adapt`); callers decide whether an ordinary open IM or a closed starred IM is eligible |
| Starred conversation membership / recover starred conversations the boot left out | `(*service.SectionStore).StarredConversationIDs()` (locked defensive copy); `adapt.MissingStarredConversations(ctx, knownIDs, starredIDs, ims, resolve)` (`internal/bootstrap/adapt/starred.go`; read-only metadata hydration) |
| Name users the cache does not (DM peers at boot, message authors): cache first, then one edge `users/info` batch (80 ids a request), then `users.info` per id (8 at a time); edge skipped while boot has marked it degraded | `internal/slack/resolve`: `New(workspaceID, db, edgeClient, slackClient, health.Degraded, onResolved)`, `(*Resolver).Resolve(ctx, ids)` (synchronous, returns `map[id]Profile`), `Request(id)` (non-blocking, 200 ms coalescing window, reported through `onResolved`); `Resolver.Logf` for diagnostics |
| Persist a full Web API profile's custom status and huddle (including clears) | `(*cache.DB).UpdateUserStatus` after `UpsertUser` (as `resolve`'s users.info path and `gui/backend/profiles.go` do); `UpsertUser` alone deliberately preserves status on conflict |
| Workspace-relative cached external-user flags | `cache.DB.ExternalUsers(workspaceID)` returns a caller-owned map, derived from `User.HomeTeamID`; the legacy boolean is only a fallback within the row's original workspace. No caller since the TUI |
| A user ID → display name cache, from any goroutine in `cmd/slk` (`slk export`'s) | `*userNameStore` (`cmd/slk/usernames.go`: `Get`/`Set`/`Snapshot`, a copy); `lookupUserCached` / `resolveUserCached` (`users.go`) add the SQLite fallback. Never hand out the map itself |
| A fetched message's author ID and name from local sources (bot-ID fallback) | `cmd/slk/messageAuthor` (`users.go`), for `slk export` |
| Keep the message cache in step with Slack from any frontend: one history page (head, older, catch-up), a whole thread, a WebSocket frame, a reaction; the slow backfill | `internal/msgsync`: `Syncer{DB, Source, WorkspaceID}` with `Open`, `Older`, `CatchUp`, `Thread`, `ApplyEvent`, `ApplyReaction`, `Ingest`, `Backfill(ctx, Config)`; `Changed{Upserted, Deleted}` is what a view redraws. Fetches use `HistoryWithVersions` (official shape, 28/page). Nothing fetches unasked except `Backfill`, explained on `Config` (sign-out history: `docs/superpowers/specs/2026-07-30-enterprise-grid-bootstrap-design.md`; slk-gio runs it from startup, 90 days unless `[cache] backfill` says otherwise via `config.CacheConfig.BackfillOnStart` / `config.ParseBackfill`, paced and skipping `backfill_ignore`), and the bounded catch-up on connect, whose choice is `msgsync.Stale` (followed, changed, not ignored, capped) |
| Which ranges of a channel the cache holds completely (so a reader can tell "cached" from "some rows") | `message_spans`: `(*cache.DB).Spans`, `AddSpan` (merges overlapping), `ExtendLiveSpan`, `ClearLiveSpans`, `Covers` (`internal/cache/spans.go`); the bounds a page covers are `msgsync.Ingest`'s rule, documented there |
| Write a history page's rows atomically, with `version` and `edited_at`; apply a `message_changed` without clobbering `subtype`/`reply_count`; count a live reply on its parent; soft-delete feed rows a page no longer returns | `(*cache.DB).UpsertMessages`, `ApplyEdit(channelID, ts, cache.Edit)`, `IncrementReplyCount`, `PruneChannelRange(channelID, oldest, latest, keep, insertedBefore)` (returns the pruned ts; `PruneThreadRepliesTS` is the thread twin); `GetMessagesBetween`, `OldestCachedTS` |
| Drop cached thread replies a complete `conversations.replies` response no longer contains (missed deletes, Slackbot ephemerals) | `(*cache.DB).PruneThreadReplies(channelID, threadTS, returnedTS, fetchStartUnix)`; soft-deletes, never touches the parent or rows cached after the fetch began |
| `{ts: version}` for the newest 28 cached messages, for `cached_latest_updates` | `msgsync.Versions(store, channelID)` (`internal/bootstrap/adapt`'s `boundedMessageVersions` wraps it) |
| Threads whose cached parent says more replies than the cache holds | `(*cache.DB).ThreadsNeedingReplies(workspaceID, sinceTS, limit)` |
| Search every cached message of a workspace (replies included), bounded, best match (bm25) or newest first | `(*cache.DB).SearchMessages(workspaceID, query, limit, order)` with `cache.SearchRelevance` / `SearchNewest` (FTS5; the LIKE fallback is always newest first); the per-channel feed-only form is `SearchChannelMessages` (no caller since the TUI) |
| The same with structure: whole words (a trailing `*` a prefix), phrases, alternatives, exclusions, authors, conversations, mentions, a ts range, thread/reply/edited/link/file/reaction, emoji | `(*cache.DB).SearchMessagesSpec(workspaceID, cache.SearchSpec, limit, order)` (`SearchMessages` is it with every word required); the Gio backend's form is `backend.SearchQuery` (`Words(s)`, `Empty`, `Ranked`), mapped by `searchSpec`, matched in Go by the fake's `fakeMatches` (`TestSearchFakeMatchesCache` keeps them agreeing) |
| Read a search query (`from:ada "a phrase" !not a \| b since:3d`): clauses with spans, the words it looks for, a time value as a span, the clause at the caret | `internal/searchq`: `Parse`, `Query.TextTerms` / `HasText` / `ClauseAt`, `ParseTime(v, now)` (`Span{Start, End, Relative}`), `Keys`, `IsValues`, `HasValues`, `TimeWords` |
| What the cache holds, for a coverage view: every span of a workspace in one query; non-deleted rows per channel | `(*cache.DB).SpansForWorkspace(workspaceID)`, `(*cache.DB).MessageCounts(workspaceID)`; the GUI's `backend.SyncStatus` is built from them |
| Follow a backfill run (phase, item in flight, counts, requests, paused, last error) | `msgsync.Config.OnProgress(msgsync.Progress)`; the GUI maps it onto `backend.BackfillStatus` (`gui/backend/sync_status.go`) and emits `backend.SyncChanged` |
| Should the search backfill skip a conversation? (`[cache] backfill_ignore`, names or globs) | `config.CacheConfig.IgnoresBackfill(name)`; the GUI hands it to `(*backend.Slack).SetBackfillIgnore` |
| Usergroups in the cache | `(*cache.DB).ReplaceUserGroups(workspaceID, groups)`, `ListUserGroups(workspaceID)` |
| Every cached reaction on a page of messages, in first-added order, in one query | `(*cache.DB).ReactionsFor(channelID, ts)` |
| Keep the cache's followed threads (`thread_subscriptions`) in step with `subscriptions.thread.getView`, at most once per half hour (up to ~60 requests a sweep) | `internal/threadsubs`: `Sync{Client, DB, WorkspaceID, Available}.Run`, `Gate{Window}` (`TryStart`, `Done`, `Running`), `Interval`; slk-gio runs them from `(*Slack).RefreshThreads` (`gui/backend/threads.go`) |
| Who is typing where and which apps are working, with their expiries, and the words for it ("Ada is typing", "Claude is working") | `internal/typing`: `Tracker` (`Add`, `Done`, `Expire`, `Users`, `SetBotStatus`, `ClearBot`, `WorkingBots`, `ThreadBotStatuses`), `Text`, `Working`, `Row(typers, apps, end)`, `Expiry` / `BotExpiry` / `Throttle`. The Gio composer keeps one per window (`gui/cmd/slk-gio/typingline.go`) from the backend's `Typing` and `AppStatus` events, and sends the user's own with `backend.Backend.Typing` |
| Download and cache an image (a file thumbnail, an avatar, a custom emoji) with the workspace's credentials, bounded concurrency and 429 back-off; choose the thumbnail for a size | `internal/image`: `NewCache(dir, maxMB)` (on-disk LRU), `NewFetcher(cache, client)`, `(*Fetcher).SetAuths`, `Fetch(ctx, FetchRequest)`; `PickThumb(thumbs, target)`. `gui/cmd/slk-gio/images.go` hands the decoded pictures to every window |
| Slack original-avatar URL → sized CDN URL (e.g. 72px) | `avatar.SizedURL(orig, px)`; returns non-Slack-original URLs unchanged |
| Disk-cache key for one user's avatar at one URL | `avatar.FetchKey(userID, url)` |
| Download a Slack file (or any URL) to a path of your choosing, once: no-op if it is already there, atomic rename, Slack auth per workspace | `(*filedl.Downloader).Fetch(ctx, url, path)`; `filedl.SafeName(title)` for the file name. `Download` is the save-to-folder form with `-2` suffixes |

### Workspace setup and config

| Need | Use |
|---|---|
| slk's XDG directories (config, data with tokens and cache.db, cache with images) | `paths.Config()`, `paths.Data()`, `paths.Cache()` (`internal/paths`); `cmd/slk/paths.go`'s `xdg*` names wrap them |
| Edit config.toml in place: the process-wide write lock, an atomic replace (temp file + rename, mode kept), a TOML basic string, a comment-safe line | `config.LockWrites()` (`defer config.LockWrites()()` across the whole read-modify-write), `config.WriteAtomic(path, data)`, `config.TOMLString(s)`, `config.SanitizeComment(s)` (`internal/config/write.go`) |
| Keep `_x_version_ts` (Slack's build timestamp) current: seed the envelope from config.toml before the first request, refresh it from `client.shouldReload` after connecting and save it to the workspace's block; the block's TOML key for a team | `internal/config/versionts`: `Seed(client.Envelope(), cfg, teamID)` (before `Connect`), `Refresh(ctx, client, cfg, configPath, teamName, logf)` (a goroutine after `Connect`; 15 s bound; `configPath` "" applies without saving, for a config that did not load), `Save(configPath, tomlKey, teamID, teamName, ts)`, `TOMLKey(cfg, teamID)` |
| Sign a workspace in without a terminal or window: desktop-app workspaces to offer (browser sessions not pre-selected), the tokens to save for the chosen ones, saving one (token + `[workspaces.<slug>]` block, never a second block for a team), a slackdesktop error as advice | `internal/onboard`: `DesktopChoices`, `BuildWorkspaceTokens`, `SaveWorkspace` (`SaveResult`), `UniqueSlug` / `ExistingSlugs` / `AppendWorkspaceConfigBlock`, `DesktopErrorMessage`. The terminal prompts are `cmd/slk/onboarding*.go` |
| Refresh stored tokens from the Slack desktop app's live session at startup (desktop tokens first, mint as the fallback; browser-session tokens left alone; failures keep the stored token) | `onboard.Remint(ctx, store, slackclient.MintToken)`; the injectable core is `onboard.RemintTokens`. `slk-gio` calls `Remint` at startup (`remintStored`, `gui/cmd/slk-gio/main.go`); `slk export` calls `RemintTokens` |

### Desktop integration

| Need | Use |
|---|---|
| Copy text to the clipboard from a Gio window | `gtx.Execute(clipboard.WriteCmd{…})` (Gio; `msgactions.go`, `textselect.go`, `infopanel.go`) |
| Read the desktop clipboard's image (PNG) or text outside any toolkit (Gio's `clipboard.ReadCmd` is text-only): `wl-paste` on Wayland, `xclip` on X11, nothing elsewhere (macOS has no reader yet) | `clipboard.ReadImage()`, `clipboard.ReadText()` (`internal/clipboard`); each runs a process, so off the UI goroutine |
| Turn pasted text or an image into an upload: is the text a file path (`/…`, `~/…`, `./…`, `file://…`), the attachment for a file or a pasted PNG (`slk-paste-<time>.png`), the 10 MB limit, a size for people | `attach.ResolvePath`, `attach.FromPath` (`ErrNotAFile` means "treat it as text", `ErrEmpty`), `attach.FromImage(png, now)`, `attach.MaxSize`, `attach.HumanSize` ("812 B", "243 KB", "12.3 MB"; also the Gio file chips) (`internal/attach`) |
| Let the user choose files with the desktop's chooser, without a toolkit dialog (zenity, else kdialog, else yad; Cancel is no files) | `filepick.Choose(ctx)`, `filepick.ErrNoChooser` (`internal/filepick`) |
| Open a URL or file with the desktop's default handler | `launch.Open(target)` (`internal/launch`) |
| Which messages notify (mentions, DMs, keywords, mutes, DND) | `notify.ShouldNotify` (`internal/notify`); slk-gio adds "no window focused" in `gui/notifications` |

### The Gio frontend

| Need | Use |
|---|---|
| Turn a parsed query into a `backend.SearchQuery` (names matched fuzzily, the best winning) and the line saying how it was read; what to offer at the caret | `search.Resolve(q, search.Sources, now)` returning `[]search.Chip`; `search.Suggest(query, caret, src, limit)` returning `search.Suggestion` (`gui/search/query.go`, `complete.go`) |
| The person, usergroup or conversation a typed name fits best (prefix, word prefix, contains, then letters in order from three letters) | `chat.BestPerson`, `chat.BestGroup`, `chat.BestConversation` (`gui/chat/complete.go`); the ranking is `personRank`, which @-completion shares |
| Draw a completion popup's rows (lit row, clicks) and a completion choice (avatar, glyph or emoji, label, detail) in a Gio window | `popupList`, `choiceRow` (`gui/cmd/slk-gio/commands.go`); the composer and the search window use both |
| Show which words a search matched in a result's text, and cut a long text around the match | `search.Highlight(text, query)` (the query's text terms only, via `searchq`; whole words, or prefixes for terms ending in `*`, case- and accent-blind, as FTS5 matches), `search.Snippet(text, query, width)` (`gui/search/highlight.go`); a coverage bar's colour state is `search.StateOf` / `OverallState` |
| Usergroups in the Gio frontend: names for mentions, the user's own, the hover card, completion | `backend.Backend.UserGroups` (no request; loaded from the cache at startup, refreshed per connect and on subteam events), event `backend.UserGroupsChanged{Mine}`, `backend.GroupProfile`; `hub.isMyGroup`, `richView.myGroup`; `chat.MatchPeople` takes the groups; `notifications.Center.SetGroups` |
| A thread for the Gio frontend: read it (cache first, then one `conversations.replies` fetch arriving as a `HistoryUpdated` with `ThreadTS` set), reply (optionally broadcast), mark it read | `backend.Backend.Thread`, `SendReply`, `MarkThreadRead` (`gui/backend`); `RunCommand` takes a thread ts |
| Reactions in the Gio frontend: apply an add/remove idempotently (the socket's echo of the user's own change is a no-op), list emoji for the picker (common set, prefix-ranked, one per glyph, customs included), toggle one, say who reacted | `chat.ApplyReaction`, `chat.MatchEmoji`, `chat.Reactors` (`gui/chat`); `backend.Backend.React` (shows the change at once, takes it back if Slack refuses or does not answer within `reactTimeout`; `reactWith`), event `backend.ReactionChanged`; pills, picker and the who-reacted card on `msgView` (`gui/cmd/slk-gio/reactions.go`). A picked name goes through `emoji.CanonicalSlackName` before it is sent (Slack keeps `+1`, not `thumbsup`) |
| Draw over a span of rich text in the Gio frontend (a custom emoji's picture over a placeholder) | `richtext.SpanStyle.Inline` (`gui/internal/richtext`); `(*richView).customEmoji(name)` resolves and loads a custom emoji's picture, `emojiPicture` draws it |
| Edit, delete, link to a message from the Gio frontend; the text an edit starts from; the user's last message (for Up-to-edit) | `backend.Backend.Edit` (optimistic, built from the cached row so files survive), `Delete`, `Permalink`; `chat.EditableText`, `chat.LastOwn` (`gui/chat/actions.go`); the composer's edit mode is `composeBox.beginEdit` / `endEdit` (`gui/cmd/slk-gio/compose.go`), the menu `msgMenu` and its dialogs `confirmDialog` (`msgactions.go`; `msgView.deleteDialog` is the delete confirmation) |
| Mark a conversation or thread unread from a message in the Gio frontend, and keep the window from reading it again until the user leaves it or something newer arrives | `backend.Backend.MarkUnread(ctx, channelID, threadTS, boundaryTS)` (`gui/backend/read.go`; mentions after the boundary counted from the cache); `chat.UnreadBoundary`, `chat.ThreadUnreadBoundary`, `chat.ReadGate` (`gui/chat/readgate.go`); `msgView.markUnread` offers the menu item, `markUnreadFrom` / `focusChanged` in `chatwin.go` and `threadwin.go` |
| Forward a message from the Gio frontend: post its permalink (after an optional note) to another conversation; choose where; is a message only a forwarded permalink? | `backend.Backend.Forward(ctx, m, toChannelID, comment)` over `(*slackclient.Client).ForwardMessage(ctx, src, ts, dst, comment)` (`internal/slack/forward.go`); the switcher's pick mode `switcher.togglePick(title)` (only the user's conversations, no joining); the note dialog `msgView.forwardDialog`, a `confirmDialog` with an editor (`gui/cmd/slk-gio/msgactions.go`); a thread window asks its conversation's window through `hub.forward`; `chat.ForwardedLink(text)` (`gui/chat/actions.go`) |
| Press the buttons of a reply only the user can see in the Gio frontend (an app's ephemeral with legacy attachment actions); which buttons a message has | `backend.Message.Actions` (every attachment action in order, set only on ephemerals), `backend.Action`, `backend.Backend.PressAction` (`gui/backend/actions.go`, `chat.attachmentAction` with a 10 s timeout); `rich.Buttons.Items` / `rich.Button.Action` (the index into `Message.Actions`, -1 for Block Kit and menus); `chat.ButtonActions`, `chat.HasButtonActions` (`gui/chat/actions.go`, menus left out); `richView.button`, `msgView.pressAction` (asks through a `confirmDialog` when the action has a confirm) |
| Open a clicked link to a message of this workspace in the Gio frontend instead of the browser (its window scrolled to it; a thread reply's thread too) | `chat.LinkTarget(url, domain)` (`gui/chat/links.go`, over `slackurl.Parse`; ok only for the workspace's own subdomain) with `backend.Backend.Domain`; `hub.openLink` (`gui/cmd/slk-gio/hub.go`, the `openNote` shape; a conversation the list lacks via `Backend.Conversation`, else the browser); windows route link clicks through `richView.open` (`msgWidgets.handle`'s `open`) |
| The user's own presence and do-not-disturb in the Gio frontend: read it, set away/active, snooze, end a snooze or DND (optimistic, taken back on refusal); the menu's choices and the header's words for it | `backend.Backend.SelfStatus`, `SetPresence`, `Snooze`, `EndSnooze`, `EndDND`, event `SelfPresenceChanged` (`gui/backend/presence.go`; the socket's own presence and `dnd_updated` arrive there too); `chat.PresenceItems`, `chat.SelfText`, `chat.MinutesUntilTomorrowMorning` (`gui/chat/presence.go`); `hub.setPresence`, `hub.selfStatus`; `presenceMenu` (`gui/cmd/slk-gio/presencemenu.go`, opened through `headerSpec.OnLeading`), `trayPresenceItems`; a popup menu's row is `menuRowAt` |
| Whether a Gio message starts a new group (its author line and avatar) or a new day (a date separator) | `chat.StartsGroup`, `chat.NewDay`, `chat.SameDay`, `chat.GroupGap` (`gui/chat/group.go`); the list's spacing is the `groupTop`/`lineTop`/`dateTop` constants in `gui/cmd/slk-gio/chatwin.go` |
| A caption centered between two thin rules (date separator) | `ruledCaption` (`gui/cmd/slk-gio/msgview.go`) |
| The Gio frontend's colours (default `hadal`): the shipped palettes, an slk theme (`internal/theme`) mapped onto the palette's colour roles, `[gui.theme]` overrides, contrast and luminance | `gui/palette` (`Resolve`, `Presets`, `Keys`, `Contrast`, `IsDark`); applied once at startup by `applyPalette` (`gui/cmd/slk-gio/theme.go`) into the `col*` variables. Colour literals live only in `theme.go` (`TestColorsComeFromThePalette`). A named theme's colors: `theme.ColorsByName` |
| An overlay in the Gio frontend (switcher, hover card, menu, picker, dialog) | `overlayFrame` (lifted `colOverlay` surface, edge, `cardRadius`, shadow via `lifted`) in `gui/cmd/slk-gio/theme.go`; highlight rows inside with `colControl` |
| A small transition in the Gio frontend: a value gliding to a new target, an overlay's entrance, a highlight's rise and fall | `gui/motion` (`Tween`, `Enter`, `Pulse`, `EaseOut`); `enter(gtx, since, w)` (`gui/cmd/slk-gio/theme.go`) slides and fades an overlay in. Ask for the next frame (`op.InvalidateCmd{}`) only while something moves |
| Desktop notifications in the Gio frontend: which messages notify (internal/notify's `ShouldNotify` plus "no window focused"), their text, and showing them (D-Bus with click-to-open and one per conversation, or `notify_command`/beeep) | `gui/notifications` (`Center`, `NewSink`, `Title`, `Body`); wired in `hub.dispatch` (`h.notes`), cleared by `ReadChanged` and window focus, clicks reach `hub.openNote`. The user's own ID is `backend.Backend.Self` |
| A conversation's details in the Gio frontend: who is in it (cache first, else at most 5 pages of `conversations.members`), its creator, creation date and rich topic/description; which info-panel tabs a conversation has, filtering its members, the "Created by" line | `backend.Backend.Members`, `backend.Details` (`TopicBody`, `PurposeBody`, `Created`, `Creator`); `chat.InfoTabs`, `chat.FilterMembers`, `chat.CreatedLine` (`gui/chat/info.go`); the panel is `infoPanel` (`gui/cmd/slk-gio/infopanel.go`), opened from the title through `headerSpec.OnTitle`. List members with a page cap: `(*slackclient.Client).GetUsersInConversationMax` |
| A conversation's own desktop-notification level in the Gio frontend (default, every message, nothing), kept in slk's cache and never sent to Slack | `backend.NotifyLevel` on `Conversation.Notify`, set with `backend.Backend.SetNotify` (event `NotifyChanged`), stored with `(*cache.DB).SetNotifyLevel` / `NotifyLevels`; applied by `notifications.Center.Message` |
| Followed threads in the Gio frontend: the list, refreshing it, read state, the unread count, a parent's "new replies" mark | `backend.Backend.Threads` (cache, no request), `RefreshThreads` (the gate above), `MarkThreadRead`; events `ThreadState`, `ThreadsUnread` (client.counts' aggregate until a sync this session, then the cache's count), `ThreadsChanged`; `Message.ThreadUnread`; `chat.ThreadMeta`, `chat.UnreadThreads`; the window is `threadsWindow` (`gui/cmd/slk-gio/threadswin.go`), the list's row `listWindow.threadsLine` |
| The Gio frontend's conversation list and boot: Slack's own boot once per run (no enumeration; `users.conversations` only as the loud fallback when `client.userBoot` fails), then the list rebuilt from state with no request; read state kept current from the socket; the boot's `client.counts` reused by the first connect's catch-up | `(*Slack).Conversations` → `bootOnce` (`gui/backend/slack.go`, over `internal/bootstrap` + `adapt`), `noteRead` / `noteActivity`, `afterConnect`; names through `resolveAll` (a `resolve.Resolver`); `ConnectOptions` carries the config for `versionts`. `SLK_DEBUG=1` prints "boot requests:" (`slackhttp.DefaultCounter.Report()`) and the session's on exit (`reportRequests`, `gui/cmd/slk-gio/main.go`). Tests: `gui/backend/boot_test.go` over `slacktest` |
| A conversation the user newly has, in the Gio frontend (a DM started, a channel joined, or one a message came from that the list did not know) | `(*Slack).conversationOfLocked` (the one Slack-channel → `Conversation` conversion), `(*Slack).opened`, `backend.Backend.Conversation` (one `conversations.info`), event `ConversationOpened`; the hub asks once per unknown channel (`askUnknownLocked`), the list buffers its activity until it arrives; `upsertConv` |
| Find and join a channel the user is not in, in the Gio frontend: ask Slack as a filter is typed (one edgeapi `channels/search` per pause, `chat.RemoteDelay`), the results the list does not have, the one an exact name means (`/open`, `/join`), joining and opening it | `chat.RemoteSearch` (`Type`, `Due`, `Done`, `Results`, `Searching`, `Wake`), `chat.Unjoined`, `chat.ExactChannel` (`gui/chat/remote.go`); `backend.Backend.SearchChannels` (empty query: no request; `top_channels` = the first 25 of `convOrder`), `Join` (also a `ConversationOpened`); `hub.joinAndOpen`, `hub.searchChannels` (answer as `channelsFound`); the switcher's and the list's "Channels you're not in" rows |
| Start a DM or group DM in the Gio frontend: who it goes to (at most eight, "2 / 8"), who can be chosen (people, no broadcasts, usergroups or apps), opening it (the user and repeats dropped, an existing one reused), the picker, `/msg`'s target | `chat.Recipients` (`Toggle`, `Has`, `Pop`, `Full`, `IDs`, `Counter`), `chat.PeopleChoices`, `chat.MaxRecipients` (`gui/chat/newmsg.go`); `backend.Backend.OpenConversation` (`gui/backend/dm.go`, a new one also a `ConversationOpened`); `hub.openDM` (a DM the list has opens with no request; text only into an empty composer); `newMessagePicker` (`gui/cmd/slk-gio/newmsg.go`); `findPerson` / `namePrefix` (`commands.go`, the name-at-the-start rule `findConversation` shares) |
| Attach and upload files in the Gio frontend: the files waiting to go (10 MB each, a repeated name made `-2`), the progress line, uploading them with a caption (one at a time, the caption on the last, then a catch-up), Ctrl+V / the paperclip / the chips, carrying them (and an upload in flight) through `moveHere` | `chat.Attachments` (`Add`, `Remove`, `Empty`, `Total`), `chat.UploadNote` (`gui/chat/attach.go`); `backend.Backend.Upload(ctx, channelID, threadTS, files, caption, progress)` (`gui/backend/upload.go`, not optimistic); `composeBox.attachKeys` / `paste` / `choose` / `submitFiles` / `chips`, `uploadState` (`gui/cmd/slk-gio/attach.go`), results through `composeBox.deliver` = `hub.toChat` / `hub.toThread`; the readers are `internal/clipboard`, `internal/attach`, `internal/filepick` |
| Someone's status, DND or profile changing, in the Gio frontend | event `ProfileChanged` (windows drop their cached profile and ask again); `backend.Profile.StatusExpires`, `DND`, `DNDUntil`; `Backend.WatchDND` (one `dnd.info` for a DM's peer); `chat.StatusText`, `chat.DNDText`, `chat.ChipDND` |
| Select and copy a message's text in the Gio frontend: where each rune of styled text was drawn, hit-testing, words, ranges, highlight rectangles, the text to copy | `styledtext.TextStyle.Segments` / `richtext.TextStyle.Segments` (pieces as drawn, with rune x positions); `gui/selection` (`Hit`, `Word`, `Whole`, `Range`, `Rects`, `Text`); in slk-gio `msgView.selectable` around a body and `msgWidgets.tsel` (`textselect.go`). Layout that moves text must keep `tsel.origin` in step: use `stack`, `insetSel`, `barredSel` (`richview.go`), or `tsel.without` for blocks whose places are not tracked |
| Complete @people, #channels and :emoji: in a Gio composer: which token the caret is in, the matches, and readable text ↔ Slack's wire form (`@Ada` ↔ `<@U1>`, `#general` ↔ `<#C1\|general>`, `@here` ↔ `<!here>`, `@eng` ↔ `<!subteam^S1\|@eng>`) | `chat.Trigger`, `chat.MatchPeople` / `MatchChannels` / `EmojiChoices`, `chat.Mentions.Wire`, `chat.Readable` (`gui/chat/complete.go`); people come from `backend.Backend.People` (cache + names seen, no request) through `hub.people`; the popup is `commandPopup` in `gui/cmd/slk-gio` |
| A label in the Gio frontend at the right size | `bodyLabel` (15 sp, read), `secondaryLabel` (13 sp, metadata and controls), `captionLabel` (12 sp, stamps and counts) in `gui/cmd/slk-gio/theme.go`; never `material.Body1/Body2/Caption` or a literal `unit.Sp` (`TestLabelsUseTheTypeScale`). Code is `fontMono`, which falls back to Go Mono (`monoFaces`, `fonts.go`) |
| A popup over a Gio window that closes on a click outside or Escape and keeps clicks off what is under it | `popupScrim`, `popupPanel`, `popupFrame`, `dismissed` (`gui/cmd/slk-gio/msgactions.go`). A popup without its own editor must ask for `key.FocusFilter` every frame to keep the keyboard; `popupPanel` does |
| When a Gio chat window asks for the page before what it shows (near the top, one in flight, stop at the start, back off after a failure); keep the view still while rows are prepended | `chat.Pager` (`gui/chat/pager.go`); `anchorOf` / `(*chatWindow).keepInPlace` (`gui/cmd/slk-gio/chatwin.go`). The list's row 0 is the history edge (`rowMessages`) |
| One thread's messages in the Gio frontend (parent, replies, broadcasts, ephemerals; each reply counted once on the parent), and a channel's | `chat.Thread`, `chat.Timeline` (`gui/chat`); both keep a `sortedMsgs`. `Timeline.Refresh` applies a thread fetch without adding rows the window has not loaded |
| Keep slk-gio running with no window open, and bring a running one's list forward from a second launch | `startTray` (`gui/cmd/slk-gio/tray_linux.go`, `fyne.io/systray`; only when a StatusNotifierWatcher has a host), `lastWindowClosed` (the hub's `onLast`, `main.go`), `claimInstance` (`instance_linux.go`, a session-bus name per workspace); off with `config.GUIConfig.Tray`. Stubs elsewhere than Linux |
| Draw a list of messages in a Gio window (rows, hover card, previews, the lit jump target, thread affordances via `onThread`), and its composer (slash-command popup, send or reply, broadcast box) | `msgView` (`gui/cmd/slk-gio/msgview.go`) and `composeBox` (`compose.go`), embedded by `chatWindow` and `threadWindow`. Which thread windows an event reaches: `threadTargets` (`hub.go`) |

### Test helpers

Unexported helpers live in `_test.go` files, so each is reachable only from
the package that declares it. Symbols are greppable by name; no line numbers,
because these files move.

| Need | Use |
|---|---|
| A whole made-up workspace behind `backend.Backend` (conversations, people, threads, usergroups, unjoined channels C20–C22, a `/standup` app with buttons, uploads, presence) | `backend.NewFake(time.Unix(1_700_000_000, 0))` (`gui/backend/fake.go`, areas in `fake_<area>.go`); set `f.Interval = 0` so nothing is posted on a timer, then `go f.Run(ctx, emit)`. The same backend runs `slk-gio -fake` |
| Take the next `MessagePosted` the fake emitted (skipping connection and presence events) | `nextPosted(t, got)` (`gui/backend/fake_test.go`); a followed thread's parent: `fakeThreadParent(t, f)` (`fake_thread_test.go`) |
| A fake Slack HTTP server: canned bodies per path, every request recorded (form and raw body, since edgeapi posts JSON as `text/plain`), and a real `*slackclient.Client` pointed at it (through `BrowserTransport.Inner`, so the request counter and envelope stay in the path) | `internal/slack/slacktest`: `NewServer(t, map[path]body)`, `NewClient(t, srv.Server)`, `(*Server).RequestTo(t, path)`, `Requests(path)`, `Paths()`, `Count(path)`; `UserBootBody` is a minimal `client.userBoot` answer |
| A `backend.Slack` over a slacktest server with an empty in-memory cache, signed in, not booted (warnings captured) | `bootSlack(t, srv)` (`gui/backend/boot_test.go`); the boot's canned bodies `bootBodies()`, its requests without the background `client.shouldReload` `bootPaths(srv)`. Every Slack-side test in `gui/backend` (`actions_test`, `channels_test`, `dm_test`, `forward_test`, `presence_test`, `read_test`, `upload_test`) builds on it |
| An in-memory cache | `cache.New(":memory:")`; `newTestDB(t)` in `internal/bootstrap/adapt` and `internal/slack/resolve` tests |
| A real hub over the fake whose read marks are recorded, with windows built (`newChatWindow`, `newThreadWindow`) but never run, to test a window's logic headless | `gateTestHub(t)` and `wantMark(t, marks, want)` (`gui/cmd/slk-gio/readgate_test.go`: `TestChatWindowKeepsAMarkUnreadUntilBlurOrNewMessage`, `TestThreadWindowKeepsAMarkUnreadUntilBlurOrNewReply`); a menu's items `hasMenuItem` |
| A composer without a window: attach, upload, refuse, carry through `moveHere`, run slash commands | `newComposeBox(nil, nil, nil, "C1", "")` (or `&hub{ctx: ctx, be: f}` for one that reaches the fake), as in `TestComposer*` (`gui/cmd/slk-gio/attach_test.go`, `compose_test.go`); `pngAttachment(name)` |
| A search window over the fake with people loaded, and waiting for its searches | `searchTestWindow(t)`, `settle(t, s)` (`gui/cmd/slk-gio/searchwin_test.go`) |
| Event tags of popups that must differ (scrim vs panel) | `TestPopupTagsAreDistinct` (`gui/cmd/slk-gio/popuptags_test.go`): add a new popup type there |
| Compare a request this repo builds with a scrubbed web-client capture (`{method, form, response}` JSON in `testdata/`) | `capturedForm(t, name)` (the capture's form minus `token` and the `_x_*` envelope) and `assertFormMatchesCapture(t, got, name)` (same fields and values, nothing extra; `client_token` checked for shape) (`internal/slack/command_test.go`, `package slackclient`) |
| Load a config from a TOML string | `loadConfigString(t, body)` (`internal/config/config_test.go`, `package config`) |

Anything that needs a Gio `layout.Context` is not unit-tested: put the logic
in `gui/chat` (or `search`, `sidebar`, `rich`) and test it there.

### Manual checks that cannot reach the user's desktop

The user runs their own `slk-gio` from `bin/slk-gio` alongside your work.
Never stop it, never `make gui` over it, and never let a check draw on their
screen, take their pointer, read or write their clipboard, or show a
notification or dialog there: one run left zenity dialogs on the user's
screen because GTK's file chooser went through the desktop portal. The recipe
the TUI-sunset executors used:

```
SCRATCH=<your scratchpad>
cd gui && go build -o $SCRATCH/slk-gio ./cmd/slk-gio     # never into bin/
mkdir -p $SCRATCH/{config/slk,data,cache}
printf '[gui]\ntray = false\n[notifications]\nenabled = false\n' > $SCRATCH/config/slk/config.toml
printf '#!/bin/sh\necho "xdg-open $*" >> %s/opened.log\n' $SCRATCH > $SCRATCH/xdg-open; chmod +x $SCRATCH/xdg-open
nix-shell -p xvfb --run 'Xvfb :97 -screen 0 1600x1000x24' &      # a private X server
env -u WAYLAND_DISPLAY DISPLAY=:97 \
    DBUS_SESSION_BUS_ADDRESS=unix:path=$SCRATCH/no-bus \
    GDK_DEBUG=no-portals \
    XDG_CONFIG_HOME=$SCRATCH/config XDG_DATA_HOME=$SCRATCH/data XDG_CACHE_HOME=$SCRATCH/cache \
    PATH=$SCRATCH:$PATH \
    $SCRATCH/slk-gio -fake -fake-every 0 &
DISPLAY=:97 xdotool search --name general windowfocus key ctrl+k type sre       # drive it
ffmpeg -f x11grab -i :97 -frames:v 1 $SCRATCH/frame.png                          # look at it
xclip -display :97 -selection clipboard -t image/png -i shot.png                 # its clipboard
pkill -f "^$SCRATCH/slk-gio -fake"; pkill -f "Xvfb :97"                          # stop only yours
```

- The dead session bus keeps the run off the user's tray, notification
  service, portal and single-instance name (`claimInstance`); `GDK_DEBUG=no-portals`
  keeps zenity's chooser on `:97`. The scratch XDG directories keep the user's
  config, tokens, cache and images untouched, and `tray = false` /
  `enabled = false` say the same thing in config.
- The stub `xdg-open` records what would have opened in a browser.
- `xdotool` clicks on `:97` do not move the user's pointer; `windowfocus` moves
  the keyboard between windows. `xdotool windowclose` on a chat window makes
  the process exit with an X `BadWindow` error; close windows from inside.
- `-fake -fake-every 0` stops the made-up messages arriving every four
  seconds. Nothing in a `-fake` run talks to Slack; anything that must be seen
  against a real workspace is listed for the user in the backlog instead.

### Known duplication — do not add to it

- **The message cache has one writer, `internal/msgsync`.** Do not add
  another: route new code through `msgsync`.
- **`slk export` enumerates.** `cmd/slk/export_command.go` resolves a channel
  name with `client.GetChannels` (`users.conversations`). Acceptable for a
  one-shot, user-initiated CLI export; do not copy it into anything that runs
  at startup or on a timer.
- **Fake and Slack must agree.** The fake's search matcher (`fakeMatches`)
  re-implements the cache's FTS rules; `TestSearchFakeMatchesCache` keeps them
  in step. Any other fake behaviour that mirrors a Slack rule needs the same
  kind of test.

## Conventions

**Adding a reusable helper?** Add it to the tables above in the same commit. An
unlisted helper gets re-implemented. When this file and the code disagree, the
code is right and this file is a bug — fix it.

**Two implementations that must stay parallel?** Express it as an interface with
a compile-time assertion (`var _ I = (*T)(nil)`), a lockstep test
(`TestSearchFakeMatchesCache`), or a test that walks the source
(`TestColorsComeFromThePalette`, `TestLabelsUseTheTypeScale`,
`gui/boundary_test.go`). Not a comment asking humans to keep two things in step
by hand. When you find a comment standing in for a check, replace it with the
check.

**Extract the substrate, not the widget.** Share the uniform part; leave the
divergent part alone. Forcing genuinely different behavior into a common shape
is worse than the duplication it removes.

**Refactoring?** Moving functions is free; moving state is not. Keep moves and
behaviour changes in separate commits, and move files with `git mv` whole so
history follows them.

**Found a bug while refactoring?** Record it, annotate it, raise it separately.
A refactor commit that also changes behavior cannot be reviewed.

## Workflow

Per the README: brainstorm the design first, write tests, then implement.
Designs go to `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md`,
implementation plans to `docs/superpowers/plans/`. Add a design doc to
`docs/superpowers/specs/README.md`.

Before opening a PR, in both modules: `go build ./...`, `go vet ./...`,
`go test ./... -race`, `gofmt -l .` empty.
