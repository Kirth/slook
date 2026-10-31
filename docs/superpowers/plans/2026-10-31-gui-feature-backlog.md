# slk-gio feature backlog (after the TUI sunset)

What slk's terminal client (TUI) had that slk-gio does not, the follow-ups the
TUI sunset left open, orphaned code, and dead config keys. Written 2026-10-31
from the sunset analysis and the execution log
(`2026-10-31-tui-sunset-log.md`, which has the reasoning behind each
follow-up).

"TUI reference" paths are in git history: check them out at the commit before
the TUI's removal, `git show 44e6f30a^:<path>` (Phase 5.1, `ui: retire the
bubbletea TUI`, deleted `internal/ui`, `internal/bubbles`, `internal/demo`,
`internal/editor`; Phase 2.2, `99c4edba^`, deleted the TUI half of `cmd/slk`).
A suggested Backend method follows the conventions in `AGENTS.md` (a `Backend`
method + `Slack` + `Fake` + tests + the window).

## Checks owed on a real workspace

None of these can be seen with `-fake`; each needs the user's workspace.

| Check | How | What decides it |
|---|---|---|
| The boot's request shape (Phase 3) | `make gui && SLK_DEBUG=1 ./bin/slk-gio 2>boot.log`, open the list, quit; `grep -A30 "boot requests" boot.log`, `grep "gap size" boot.log`, `grep "FALLING BACK" boot.log` | `client.userBoot` 1, `client.counts` 1, no `users.conversations` / `users.prefs.get` / `users.list`, `users.info` only with "marking edge degraded"; no FALLING BACK. **Stop** if the gap line's "left out" counts channels the user sees in Slack's sidebar: `FillFromCounts` is then not enough |
| Gap DMs (Phase 3 deviation 2) | the same `gap size` line | DMs userBoot omits whose cached row has no peer are left out; if any appear, list them with `conversations.info` per id or carry the peer ID in the cache's channel row |
| A forwarded message with a note still unfurls (4.6) | forward a message with a note; look in Slack | if Slack shows no preview of the original under the note, switch `(*slackclient.Client).ForwardMessage` to `text=comment` + `attachments=[{"from_url": permalink}]` (the comment in `internal/slack/forward.go`) |
| Mark unread keeps its mention badge (4.4) | mark a message with a mention unread; watch the list | Slack's own `channel_marked` echo may carry no mention count and turn the badge into a plain dot; if so, keep the locally counted mentions when the echo's boundary equals the one just set |

## Not ported from the TUI

| Feature | TUI reference | Slack call | Suggested Backend method |
|---|---|---|---|
| Activity feed (mentions, thread replies, reactions, DMs, like Slack's Activity tab; unread-only filter, mark read) | `internal/ui/activityview`, `core.ActivityService` (`internal/core/ports.go`), wired in `cmd/slk` | `activity.feed`, `messages.list` (`(*slackclient.Client).GetActivityFeed`, `GetActivityMessages`, `SetActivityRead` in `internal/slack/activity.go`, still there) | `Activity(ctx, unreadOnly, cursor) ([]ActivityItem, string, error)`, `MarkActivityRead(ctx, item)`; event `ActivityChanged` |
| Server-side search for history older than the cache | `internal/ui/searchresults` | `search.messages` (`(*slackclient.Client).SearchMessages`, still there); edge search exists for channels only | `SearchRemote(ctx, query) ([]SearchHit, error)`, shown below the cache's results when the cache's coverage ends |
| Save a file to `download_dir` (`d`) | `internal/ui` `d` key, `core.FileService`; `filedl.Downloader.Download` (still there) | file download with workspace auth (no API call) | `SaveFile(ctx, file, dir) (path, error)` (the previewer's `Backend.Download` already fetches) |
| Save a thread to Markdown (`S`) | `internal/ui` `S` key; `export.SaveThread` (still there, no caller) | none (cache) | `SaveThread(ctx, channelID, threadTS) (path, error)` |
| Read-only app DMs (composer disabled when the app's Messages tab is off or read-only; a refused send marks it) | `cmd/slk/messaging_capability.go`, `internal/ui/read_only.go` | `conversations.info` with `return_app_home=true` or `conversations.view` (`boot.ConversationsInfo`, `(*boot.AppHome).CanReceiveMessages`, still there); `restricted_action_read_only*` | `MessagingCapability(ctx, channelID) (bool, error)` |
| "New messages" divider at the unread boundary | `internal/ui/messages` landmark | none (`last_read` from `client.counts` / `conversations.view`) | `Conversation.LastRead` already flows; a `chat` rule + `msgView` row |
| Frecent emoji first in the reaction picker | `internal/ui/reactionpicker`; `(*cache.DB).GetFrecentEmoji` / `RecordEmojiUse` (still there) | none | `FrecentEmoji(limit) []string`; record on `React` |
| Who-reacted dialog | `internal/ui/reactionsview` | `reactions.get` | covered in part: slk-gio's pill hover card names who reacted |
| Profile dialog: copy email, copy huddle link | `internal/ui/userprofile` | `users.info` (`email`); `slackurl.HuddleURL` (still there, no caller) | hover-card actions; `Profile.Email` |
| A peer's huddle shown (🎧 on DM rows and authors) | `internal/ui/peerstatus`; `UserProfile.HuddleStateChannelID` | `users.info` `huddle_state` (`cache.UpdateUserStatus` already stores it from `gui/backend/profiles.go`) | `Profile.Huddle`, `chat.StatusText` |
| Unread counts in the window title / tray tooltip | `internal/ui` tab-title indicator | none | the hub's unread summary into `app.Title` and the tray tooltip |
| `[notifications] status_command` (a hook on every unread change) | `cmd/slk` status hook | none | a `notifications` sink fed from the hub's unread summary |
| `/leave` | `internal/ui/slash_builtins.go` | `conversations.leave` | `Leave(ctx, channelID) error`; event `ConversationClosed` |
| A custom `/status` (emoji + text, expiry) | `internal/ui` `/status` | `users.profile.set` | `SetStatus(ctx, emoji, text string, expires time.Time) error` |
| More than one workspace in one process (the rail, `1`–`9`, all connected) | `cmd/slk/workspace*.go`, `internal/ui/workspace`, `internal/wake` (deleted in 5.1) | per workspace as now | one `Backend` per workspace in the hub; today one process per workspace (`-team`) |
| `/msg` text given back to the composer it was typed in when the DM already has a draft (4.1/4.2 follow-up) | `(*App).offerTypedBack` (`internal/ui/slash_command.go`) | none | `hub.openDM` returns whether the text was placed; `compose.go` keeps it |
| People slk has not seen in the new-message picker (4.2 follow-up) | `internal/ui/newmessagepicker` | edgeapi `users/search` (`edge.UsersSearch`) with `chat.RemoteSearch`'s debounce | `SearchPeople(ctx, query) ([]Person, error)` |
| Enterprise Grid permalinks on an org host open in-app (4.8 follow-up) | — | none | `chat.LinkTarget` accepts the org subdomain as well as `TeamSubdomain()` |
| A forwarded message opens its original on Enter (4.6 follow-up) | `internal/ui/forward_navigation.go` | none | `chat.ForwardedLink` exists with no caller; wire it to the message menu or Enter |
| The forward note's `@name` → `<@U…>` (4.6 follow-up) | — | none | send the note through `chat.Mentions.Wire` like the composer |
| A multi-file upload that fails part-way re-sends what went (4.3 follow-up) | — | none | `Upload` reports how many were posted; the composer drops those chips |
| Pictures from the clipboard on macOS (4.3 follow-up) | the TUI read it through `golang.design/x/clipboard` (dropped in Phase 2.2) | none | `internal/clipboard` runs `pngpaste` |
| Usergroup mentions counted by mark unread (4.4 follow-up; the TUI did not either) | `cmd/slk/markread.go countMentionsSince` | none | `mentionsAfter` (`gui/backend/read.go`) also counts `mention.InGroups` |
| `SetPresence(false)` sends `auto`; Slack may still report the user away while idle elsewhere (4.5 follow-up) | — | `users.setPresence` | observe; the dot follows the next `presence_change` |
| A thread reply link where the fake cannot test it, a link to a conversation the list lacks, a URL button, refusals of press/forward (4.8 "not exercised") | — | — | real-workspace checks |

## Deliberately not ported

- **`peer_status.go`'s batched refresh** (`dnd.teamInfo` and eager profile
  refetch on `user_invalidated`): slk-gio drops a cached profile on
  `ProfileChanged` and refetches lazily when a hover card or header needs it,
  which issues fewer requests than the TUI's eager refetch (328
  `user_invalidated` in 12 h). Phase 3.4.
- **`bootstrap.Result.Messages` ingestion at boot**: the boot's messages have
  no `LatestUpdates`, so ingesting them would claim a span the cache does not
  hold; the opened conversation costs one `conversations.history` instead.
  Phase 3.2.
- `bootstrap.Result.RevalidatedUsers` was listed here in the plan; Phase 3
  did it (`adapt.HydrateFirstSight` caches revalidated users without a row).

## Known issues seen during the sunset (not caused by it)

- After `openChatAt` moves an already open window (search result,
  notification, in-app link), the jump target can end under the header: the
  author line hidden, the body at the top edge.
- The first Ctrl+V of text slk-gio itself put on the clipboard landed more than
  a second late (after Enter had sent); a second Ctrl+V was immediate.
- `xdotool windowclose` on a chat window makes the process exit with an X
  `BadWindow` error (an external destroy, not a user action).
- `Profile()` asks `users.info` once per hover card (it needs title, time zone
  and the 192 px image, which edge `users/info` lacks).

## Orphaned code

| Code | State | Suggestion |
|---|---|---|
| `internal/slack/membership` | no importer | delete, or use it for `/leave` / member lists |
| `internal/version` (`ModalFooter`, the TUI help modal's line) | no importer | delete, or use it for an About box |
| `internal/ids` | one importer (`internal/slackurl`); `gui/chat` converts to strings so windows never import it | fold into `slackurl` or keep |
| `internal/service`: `MessageService`, `WorkspaceManager` | tests only (the section and mute stores are used) | delete |
| `internal/emoji`: `ResolveShortcodesInText`, `ShouldRenderUnicode`, `BuildEntries`/`EmojiEntry`, `ResolveEmojiToTokens`/`Token`, `URLForShortcode`, `BuildStandardEmojiURL`, `CodepointsForShortcode`, `ComposeSkinTonedCodepoints` | tests only since the TUI | delete, or use for standard emoji as pictures |
| `internal/cache`: `GetFrecentEmoji`/`RecordEmojiUse`, `ExternalUsers`, `SearchChannelMessages`, marks (`UpsertMark`, `ListMarks`, `DeleteMark`) | no caller outside `cache` | keep the ones the features above would use; delete the rest |
| `internal/image`: `(*Fetcher).Cached`, `Bytes`; `FetchRequest.ReqID` and debuglog's `ImgFetch`/`ImgRender` lines (slk-gio never calls `debuglog.Init`) | no caller / never logged | trim |
| `slackurl.HuddleURL`, `slackclient.UserProfile.HuddleStateChannelID`, `boot.ConversationsInfo` / `AppHome`, `export.SaveThread` | no caller | the features above |
| `config.GUITheme`'s and `internal/theme`'s comments still say "TUI theme" | comments | reword when next touched |

Phase 5b (`image: keep the fetcher and cache, drop the terminal renderers`)
removed the terminal image stack (`internal/image` kitty/sixel/half-block,
`internal/avatar`'s cell renderer, `internal/emoji`'s placement and width), so
the plan's cleanup O1 (lipgloss reached through `internal/emoji`) is done: the
charm toolkit is imported only by `cmd/slk`'s prompts, and `gui/go.mod` has none
of it. O4 below is open; the plan defines no O2 or O3.

## Build and release

- **O4: package `slk-gio`.** Releases (`.goreleaser.yaml`) carry only the
  CLI. slk-gio needs cgo and per-platform libraries, so it needs its own build
  job (Linux first); the flake has no `slk-gio` package because
  `gui/shell.nix` deliberately uses the system's nixpkgs for the GL driver.
- `.github/workflows/release.yml` still runs on `macos-latest`; every build is
  `CGO_ENABLED=0` now, so `ubuntu-latest` would do (the comment says so; the
  runner was left alone).
- `internal/core` has no boundary test since `internal/ui/boundary_test.go`
  went (it imports only `internal/core/blocks`).

## Dead config keys

Read by nothing in either program (candidates for removal from
`internal/config`): `[notifications] quiet_hours`, `[cache]
message_retention_days`, `[cache] max_db_size_mb`, `[appearance] show_avatars`,
`[general] default_workspace`. Read only by the retired TUI (still parsed so
old configs load; `wiki/Configuration.md` lists them as ignored):
`[general] use_slack_sections` / `download_dir`, `[appearance] theme` /
`image_protocol` / `max_image_rows` / `max_image_cols` / `mouse_wheel_lines` /
`emoji_images` / `emoji_cells` / `colored_usernames`, `[animations]` except
`typing_indicators`, `[notifications] status_command`, `[sidebar] *`,
`[compose] editor`, `[marks] *`, `[sections.*]`, `[theme]`,
`[workspaces.<slug>] theme` / `sidebar_width` / `use_slack_sections` /
`sections`, `[cache] max_image_cache_mb` (slk-gio's image cache is a fixed
200 MB).
