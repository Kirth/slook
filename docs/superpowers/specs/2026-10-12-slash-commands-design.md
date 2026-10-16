# Slash commands — design (#247 sub-projects 2 and 3)

**Issue:** [#247](https://github.com/gammons/slk/issues/247) ("I can't run `/zoom`")
**Date:** 2026-10-12
**Status:** Design approved in conversation, stage by stage. Awaiting review of
this document.

**Read with:**

- `docs/superpowers/specs/2026-10-09-interactive-ephemeral-messages-design.md`
  — split #247 into sub-projects. Sub-project 1 (interactive ephemerals, Parts A
  and B) is merged (#298, #304). This spec is sub-project 2 (running commands)
  and sub-project 3 (the `/` picker), plus the Block Kit button stage the
  captures showed `/giphy` needs.
- `docs/superpowers/specs/2026-10-10-ephemeral-action-dialog-design.md` — the
  `b` dialog this spec extends; its "Out of scope → Interactive Block Kit →
  1. Pressing Block Kit buttons" is stage 2 here.
- `docs/superpowers/specs/2026-10-12-app-modals-research.md` — app modals
  (bare `/giphy`, any app's forms), deliberately **not** in this spec. That
  note keeps the captures and analysis for a later spec.
- RFC [#236](https://github.com/gammons/slk/issues/236) and the owner's answers
  in its comments: the rule that every stage reduces the number of
  implementations of something and never adds one; which actions a widget may
  call itself; sends stay in `App`; failures go through the shared toast.

## 1. Summary

Today slk has no slash commands at all. Nothing in the composer or send path
treats `/` specially, so typing `/zoom` and pressing Enter posts the literal
text "/zoom" to the conversation through `chat.postMessage`.

This spec makes slash commands work the way Slack's web client does, in three
stages, each its own PR:

1. **Running commands.** Enter on `/name args` runs the command. Slack's
   built-ins that the web client handles locally map onto slk features that
   already exist; everything else goes to `chat.command`. This alone makes
   `/remind`, `/shrug`, `/me` and app commands that answer with a message or an
   ephemeral work, and stops `/…` text being posted by accident. `/zoom` (the
   issue's example) was not captured; as an app command it takes the same
   `chat.command` path.
2. **Block Kit buttons on ephemerals.** `/giphy cats` answers with an ephemeral
   whose Send / Shuffle / Cancel are Block Kit buttons, pressed with
   `blocks.actions`, not the legacy `chat.attachmentAction` slk already
   supports. Shuffle replaces the ephemeral with an ephemeral `message_changed`,
   which slk deliberately drops today. This stage makes `/giphy cats` work end
   to end.
3. **The `/` picker.** Typing `/` at the start of the composer opens a popup of
   the workspace's commands with their usage and description, from
   `client.appCommands`.

**Not in scope:** app modals. Bare `/giphy` opens one. Stage 1 shows a notice
when a command opens a modal; the modals research note has everything needed
to design them later.

## 2. Evidence

The design rests on five HAR captures of Slack's web client taken by the owner
on 2026-10-12 against a non-Grid workspace, not on guesses. Like the earlier
ephemeral specs, slk copies the web client's request shapes deliberately: an
unfamiliar shape is the kind of signature Enterprise Grid anomaly detection
flags (see `postForm`).

The HARs hold live credentials and are not in the repo. Scrubbed excerpts are in
**`docs/superpowers/specs/2026-10-12-slash-commands-captures/`**: tokens are
`xoxc-REDACTED`; workspace, user, channel, bot, app and view IDs are
`*0EXAMPLEnn` placeholders, consistent across files; UUIDs (Giphy session IDs)
are zeroed and Giphy `cid` parameters redacted. Giphy's app ID `A0F827J2C` is
kept because it is the same in every workspace. Each file is
`{method, form: [[name, value], …] in sent order, response}` for an HTTP call,
or the raw frame for a WebSocket event. Marshal and parse tests in this spec
copy these files into the relevant package's `testdata/`.

| File | What it shows |
|---|---|
| `chat_command_channel.json`, `chat_command_dm.json` | `/giphy gorilla` in a channel and in a DM |
| `chat_command_thread.json` | the same in a thread: `thread_ts` added |
| `chat_command_thread_no_args.json` | bare `/giphy`: `blocks` omitted |
| `chat_command_unknown.json` | `/notarealcommand`: `{"ok":false,"error":"unknown_command"}`, no `team_id` sent |
| `chat_command_shrug.json`, `chat_command_me.json`, `chat_command_remind.json` | server-side built-ins: they go through `chat.command` too |
| `presence_set_away.json` | `/away`: the web client calls `presence.set` instead |
| `blocks_actions_ephemeral_{shuffle,send,cancel}.json` | pressing `/giphy`'s buttons |
| `ws_ephemeral_preview.json` | the `/giphy` ephemeral: `bot_message`, `is_ephemeral`, `attachments[0].blocks` = `image` + `actions` |
| `ws_ephemeral_message_changed.json` | Shuffle: `message_changed`, `hidden`, inner `is_ephemeral`, same inner `ts` |
| `ws_ephemeral_deleted.json` | Send / Cancel: `message_deleted` of the ephemeral |
| `ws_posted_gif.json` | the posted GIF: `user` = the current user **and** `bot_id` = Giphy's bot |
| `ws_view_opened.json` | a command opening a modal |
| `ws_me_message.json` | `/me waves`: `subtype: me_message` |
| `ws_remind_ephemeral.json` | `/remind`'s Slackbot ephemeral confirmation |
| `client_app_commands.json` | the command list (`app_actions` omitted) |

What the web client did with each built-in the owner tried:

| Typed | Web client | Seen as |
|---|---|---|
| `/shrug hi` | `chat.command`; the server posts "hi ¯\\\_(ツ)\_/¯" | WS `message` |
| `/me waves` | `chat.command`; the server posts a `me_message` | WS `me_message` |
| `/remind me test in 1 minute` | `chat.command`; Slackbot answers with an ephemeral | WS ephemeral + `saved_added` |
| `/away` (twice) | `presence.set away`, then `presence.set active`: a toggle | `manual_presence_change` |
| `/status away` | `presence.set away`; no `chat.command` | `manual_presence_change` |
| `/msg @self hi` | resolves the user, then a plain `chat.postMessage` to the DM | WS `message` |
| `/open #general` | navigation only (`conversations.history` of #general) | — |

`client.appCommands` (no parameters beyond the `_x_*` envelope, `_x_reason`
`dispatch-client-extras`) returns `{ok, commands[], app_actions[], cache_ts}`.
Each command has `name`, `desc`, `usage`, `type` (`core` / `app` / `service`),
and optionally `alias_of`, `app`, `app_name`. The owner's workspace has 32: 24
core, 7 apps, and `/giphy` (`service`). The aliases are `/dm → /msg`,
`/join → /open`, `/close` and `/part → /leave`, `/kick → /remove`. The web
client caches this list in local storage, which is why it only appears in a
capture taken after clearing it.

## 3. Stage 1 — running commands

### 3.1 When composer text is a command

On Enter in either composer (channel or thread), the text is a slash command
when it **starts** with `/` (no leading whitespace) and its first
whitespace-separated token is `/` followed by one or more of `[A-Za-z0-9_-]`
and nothing else. So `/tmp/foo is full` and `/ hello` are ordinary messages,
and a leading space is the way to post a message that begins with `/name`.
Names are matched case-insensitively and sent lowercased. The arguments are
everything after the name and the whitespace that follows it, newlines kept;
whitespace-only arguments count as none.

It is not a command:

- **while editing a message** — `submitEdit` runs as today and the edit is
  saved as written;
- **with files attached** — toast "Files can't be sent with a command"; nothing
  is sent, the composer keeps its text and files.

Both Enter branches in `mode_insert.go` (thread and channel) call **one** helper,
`(*App).trySlashCommand(c *compose.Model, channelID, threadTS string) (tea.Cmd,
bool)`, placed after the attachment and edit checks and before
`SendThreadReplyMsg` / `SendMessageMsg` are built. It returns `false` for
non-commands, and the existing send path runs unchanged. It uses
`c.TranslateMentionsForSend` on the text first, exactly as a send does, so
`@name` and `#name` arguments arrive as `<@U…>` and `<#C…|name>`.

### 3.2 One command registry, two tables

slk already has a command registry: the vi-style `:` commands in
`internal/ui/command.go` (`commandFunc`, the `commands` map, `executeCommand`,
which splits with `strings.Fields` and toasts on an unknown name). Slash
built-ins use the same shape rather than a second registry:

- `commandFunc` changes from `func(a *App, args []string) tea.Cmd` to
  `func(a *App, args commandArgs) tea.Cmd`, where `commandArgs` holds `Fields
  []string` (what the nine existing handlers use today) and `Rest string` (the
  text after the command name with its whitespace and newlines intact, which
  `/msg` needs). The nine handlers change mechanically.
- `executeCommand` becomes a thin call to `dispatchCommand(a, table, line,
  unknown)`, which parses once and calls the table entry, or `unknown` when
  there is none. `:` passes `commands` and its existing "Unknown command" toast,
  so `:` behaves exactly as before.
- A second table, `slashCommands`, holds the built-ins in §3.3. Its `unknown` is
  the remote path in §3.4.

`:ws` and `/away` stay in separate namespaces: `:` commands are slk's own, `/`
commands are Slack's.

### 3.3 Built-ins handled in slk

Each entry is backed by the capture it cites (§2). Aliases follow
`client.appCommands`.

| Command | Does | Reuses |
|---|---|---|
| `/away` | toggles: away when active, active when away, as the web client did | `(*App).applyPresenceAction`, **extracted** from `mode_presence_menu.go` (the optimistic `a.presence.Apply` + `statusbar.SetStatus` + `presenceSvc.SetStatus`, lines 48–52 today); the menu then calls it too. Current state from `a.presence.Status(a.activeTeamID)` |
| `/active` | set active | same |
| `/status away`, `/status active` | as `/away` / `/active` but not a toggle. Only `/status away` is captured; `active` is by symmetry | same |
| `/open #c`, `/join #c` | switch to the channel, joining it first if not a member | `(*App).openChannel(…)`, **extracted** from the channel finder's "joined → `sidebar.SelectByID` + `ChannelSelectedMsg`, else `channels.Join`" (`mode_channel_finder.go`, lines 37–50 today); the finder then calls it too. The ID comes from the new `slackfmt.MentionedChannelIDs` (§3.8). No `#c`, or a name that does not resolve → toast "No channel named …" |
| `/msg @u [text]`, `/dm @u [text]` | open the DM; text, if any, is **pre-filled** in the DM's composer, not sent | the new-message flow as is: `channels.OpenConversation(userIDs, reqID)` → `NewMessageOpenedMsg` (in-flight guard, `hydrateNewConversation`, switch, focus). The user ID comes from `slackfmt.MentionedUserIDs`. The text is written with the new `compose.OfferDraft` (§3.8) when the open succeeds, before the switch restores the draft; an existing non-empty draft for that DM is kept and a toast says so. One `@user` only; anything else → toast with the usage |
| bare `/status`, `/status <anything else>`, `/leave`, `/close`, `/part` | toast "/x isn't supported in slk yet" | `toastWithClear` |

Pre-filling instead of sending is a deliberate difference from Slack: it needs
no "send once the DM opens" state, and costs one Enter.

Everything not in the table — app commands, `/giphy`, and the server-side
built-ins (`/shrug`, `/me`, `/remind`, `/topic`, `/invite`, …) — takes the
remote path.

### 3.4 The remote path: `chat.command`

**Wire.** `slackclient.(*Client).RunCommand(ctx, CommandRequest) error`, in a new
`internal/slack/command.go` following `attachment_action.go`. The form fields
below match the capture by name and value. Their order on the wire is
alphabetical, like every body slk sends (`url.Values.Encode()`; the accepted
divergence recorded on `applyEnvelopeBody` in `internal/slackhttp/transport.go`
and pinned by `TestPostForm_BodyFieldOrderIsAlphabeticalThenEnvelope`); the
captured order is listed for reference:

| Field | Value |
|---|---|
| `command`, `disp` | `/name`, both the same |
| `blocks` | the arguments as a one-element array holding the rich_text block from the **existing** `mrkdwn.Convert` (which already emits `user` / `channel` elements for `<@U…>` / `<#C…>`); **omitted** when there are no arguments (`Convert` returns a nil block only for empty input). Serialized as captured, with no `block_id` key; the marshal test pins this, since `Convert` builds the block with `slack.NewRichTextBlock("")` |
| `channel` | the conversation |
| `client_token` | `web-<unix ms>`, from a new helper `newClientToken()` in `internal/slack`, the only generator of it (stage 2 uses it too) |
| `thread_ts` | only in a thread composer |
| `team_id` | the workspace. Stage 3 omits it when the command is not in the loaded list, matching the `unknown_command` capture |

`slackhttp`'s `defaultReasons` gains `"chat.command": "executeCommand"`.
Responses: `{"ok":true,"keep_input":false}` → `nil`; `{"ok":false,"error":e}` →
`*slackclient.CommandRejectedError{Code: e}`, which the `cmd/slk` adapter
translates to `*core.CommandError{Code: e}` — the same pattern as
slackclient's `rateLimitError` becoming `core.RateLimitedError`, so the UI never
parses Slack error strings. Transport failures pass through unchanged. slk sends urlencoded bodies
where the web client sends multipart; that is the known, accepted difference
recorded in `internal/slackhttp/transport.go` and is not changed here.

Tests: the marshalled form equals each `chat_command_*.json` capture's form
(minus `token` and the `_x_*` envelope, which `postForm` and `slackhttp` own) as
a set of fields — same names, same values, JSON values byte for byte, nothing
extra — except `client_token`, which is checked for the `web-<digits>` shape;
`unknown_command` becomes an error whose text names it.

**Port.** A new `core.CommandService`, built with `NewCommandService(
CommandServiceFuncs{…})` like the other ports: `Run(channelID ids.ChannelID,
threadTS ids.ThreadTS, command, args string) error` (blocking; callers run it
from a `tea.Cmd`). Stage 3 adds `List`. `cmd/slk` wires it to the active
workspace's client, as `InteractionService` is wired.

**In `App`.** For a remote command, `trySlashCommand` clears the composer and
leaves insert mode exactly as a send does, then returns a `tea.Cmd` that calls
`Run` and yields an unexported result message, `commandRanMsg{teamID,
channelID, threadTS, text, err}` — the same pattern as the action dialog's
`actionPressedMsg`. There is no optimistic placeholder and no
`selfSend.MarkInFlight`: whatever the command produces arrives over the
WebSocket through code that exists (messages, ephemerals, the action dialog).

On `commandRanMsg` with an error, the reducer:

- toasts with `toastWithClear`: "/notarealcommand is not a valid command" for
  `unknown_command`, otherwise "/name failed: <error>";
- puts `text` back with `compose.OfferDraft` on the composer it came from, keyed
  by the conversation it was sent from: visible again if the user is still
  there, the stored draft if they moved away, and nothing if that conversation
  has a draft again by then.

Putting text back is new behaviour: a failed `chat.postMessage` does not do it
today (`MessageSendFailedMsg` only removes the placeholder and toasts). It is one
small function, and the web client keeps the text in the same case.

**Self-send.** The GIF `/giphy` posts arrives with `user` = the current user and
`bot_id` = Giphy (`ws_posted_gif.json`). Because a command never marks the
channel in flight, `reduceNewMessage`'s self-send guards do not swallow it; a
test pins this.

### 3.5 When a command opens a modal

`events.go` decodes `view_opened` (`ws_view_opened.json`) and calls a new
`EventHandler.OnViewOpened(title string)` with the view's title. `cmd/slk`
sends the UI a message that toasts "*Giphy* opened a dialog slk can't show
yet". Nothing else is done with the view. The modals research note describes
replacing this.

### 3.6 `me_message`

`/me waves` arrives as `subtype: me_message` (`ws_me_message.json`). Today the
WebSocket path **drops it**: `dispatchWebSocketEvent` only passes subtypes `""`,
`bot_message`, `thread_broadcast` and `file_share` to `OnMessage`, so a `/me`
line appears only after a history refetch (history does not filter subtypes).
Stage 1 adds `me_message` to that list. slk keeps `Subtype` on `MessageItem` but
renders it like any message. `messages.RenderMessage`
— the one renderer both panes use — renders a `me_message` body in italics, the
way the web client does. A `RenderMessage` test in `internal/ui/messages` covers
it (italic for `me_message`, unchanged otherwise); no App golden needs
re-blessing.

### 3.7 Tests (stage 1)

- `dispatchCommand`: `:` behaviour unchanged (existing `command_test.go` passes
  after the mechanical signature change); slash table entries and the unknown
  fallback.
- Command detection: the token rule, case, `/tmp/foo`, `/ hello`, edit mode,
  attachments.
- Built-ins: `/away` toggles from each state; `/status away|active`;
  `/open`/`/join` joined and not joined, unresolvable; `/msg` with and without
  text, existing draft kept, wrong arguments; unsupported list.
- Remote, through `App.Update`: Enter on `/giphy cats` in the channel and thread
  composers calls `Run` with the right channel, thread, command and args;
  failure toasts and puts the text back — visibly when the user stayed, as the
  stored draft when they moved away, not at all over a newer draft.
- Wire marshal against every `chat_command_*.json`.
- The posted-GIF frame is not dropped as a self-send.
- `view_opened` decodes and toasts.
- Presence menu and channel finder tests pass unchanged after the extractions.

### 3.8 Small additions stage 1 makes to existing packages

- `slackfmt.MentionedChannelIDs(text) []string`, next to `MentionedUserIDs`,
  reusing slackfmt's existing `channelMentionRe` (extended to capture the ID).
  It is not a fourth channel-mention regex (see §8).
- `(*compose.Model).OfferDraft(teamID, channelID, threadTS, text string) bool`:
  makes `text` the draft of that conversation — the visible composer when the
  key is the one it is bound to, otherwise the existing inactive `drafts` map —
  but only when that conversation's draft is empty (no text, no attachments).
  Returns whether it wrote. Both the `/msg` pre-fill and putting a failed
  command's text back use it, so neither needs its own "same conversation and
  empty?" check.

## 4. Stage 2 — Block Kit buttons on ephemerals

### 4.1 Scope

Buttons in a Block Kit `actions` block **nested inside an attachment** of an
**ephemeral** message — what `/giphy` sends, and the shape captured. Buttons in
a message's top-level `blocks` would be pressed with `container.type: "message"`,
which no capture shows; they keep "↗ open in Slack to interact" until one does.
Selects, and buttons on non-ephemeral messages, are unchanged.

### 4.2 Data

`blocks.ActionElement` (`internal/core/blocks/blocks.go`) has only `Kind` and
`Label`. It gains what a press echoes: `ActionID`, `BlockID` (the enclosing
block's), `Value`, `Style`, and `TextEmoji` (the label's `emoji` flag, echoed in
`text`). `blockkit/parse.go` fills them for buttons in `actions` blocks,
including those in `LegacyAttachment.Blocks`. Rendering does not change.

### 4.3 Finding and pressing

- `messages.ButtonAction` gains a Block Kit variant beside the legacy one, and
  `messages.ButtonActions` / `HasButtonActions` list, in order, each
  attachment's legacy `Actions` buttons then the buttons of its `Blocks`.
  `InteractiveHint`, the auto-open in `reduceNewMessage`, the queue, the
  interruption rule and `bubbles/buttonrow` work unchanged on top.
- `action_prompt.go`'s press step calls `PressAttachmentAction` or the new
  `PressBlockAction` according to the variant; both report through the existing
  `actionPressedMsg`.
- `core.InteractionService` gains `PressBlockAction(channelID ids.ChannelID,
  messageTS ids.MessageTS, attachmentID int, botID string, action
  blocks.ActionElement, ephemeral bool) error`.

### 4.4 The wire call

`slackclient.(*Client).BlockAction(ctx, BlockActionRequest) error`, in a new
`internal/slack/block_action.go` beside `attachment_action.go`. Form fields,
matching the capture by name and value (wire order alphabetical, as in §3.4):

| Field | Value |
|---|---|
| `service_id` | the message's `bot_id` |
| `app_id` | the bot's app (§4.5) |
| `service_team_id` | the workspace |
| `actions` | a one-element JSON array echoing the button with keys in the captured order `action_id`, `block_id`, `text` (`{"type":"plain_text","text":…,"emoji":…}`), `value`, `style`, `type`; `value` and `style` omitted when empty. HTML not escaped, as for `chat.attachmentAction` |
| `container` | `{"type":"message_attachment","message_ts":…,"attachment_id":…,"channel_id":…,"is_ephemeral":true}` |
| `client_token` | `newClientToken()` (stage 1) |
| `state` | `{"values":{}}` |

`defaultReasons` gains `"blocks.actions": "dispatch_action_to_developer"`.
Tests: the marshalled form equals each `blocks_actions_ephemeral_*.json` capture
as a set of fields, byte for byte for the JSON values `actions`, `container` and
`state`; `client_token` checked for shape.

### 4.5 `app_id`

The ephemeral frame carries `bot_id` but no `app_id` (`ws_ephemeral_preview.json`).
slk already calls `bots.info` through `(*slackclient.Client).GetBotInfo`, and
slack-go's `slack.Bot` has `AppID`; `userResolver.RequestBot` just does not keep
it. The `cmd/slk` adapter behind `PressBlockAction` resolves the app with
`GetBotInfo` on first press and remembers it in a per-workspace map that only
the adapter touches, under a mutex (the AGENTS.md shared-map rule). A lookup
failure is a press error, and so a toast.

### 4.6 Shuffle: ephemeral `message_changed`

- `events.go` stops dropping an ephemeral `message_changed` (today: "decision=
  dropped") and dispatches the inner message through the existing
  `dispatchEphemeral`, so it reaches `OnEphemeralMessage` → `NewMessageMsg{…
  Ephemeral: true}` like a new ephemeral, and is never cached. The existing
  `events_test.go` cases for both flag placements flip from dropped to
  dispatched.
- In `reduceNewMessage`'s ephemeral branch, an ephemeral whose `ts` is already
  shown in a pane is **replaced in place** instead of appended. No new message
  type and no "(edited)" mark (`UpdateMessageInPlace` replaces only the text and
  sets `IsEdited`, so it is not used).
- The replacement uses a new general `ReplaceMessage(ts, msg) bool` on
  `messages.Model` and `thread.Model`. The bodies of `SwapLocalSent` and
  `SwapLocalSentReply`, which already do exactly "find by TS, replace the whole
  item, refresh", become calls to it, so there is one implementation per pane.
- The replacement goes through the same branch as a new ephemeral, so
  `enqueueActionPrompt` reopens the dialog with the new GIF's buttons.
- Send and Cancel end in `message_deleted` (`ws_ephemeral_deleted.json`), which
  slk already handles.

### 4.7 Tests (stage 2)

- Parse: an attachment's `actions` block yields elements with `ActionID`,
  `BlockID`, `Value`, `Style`, `TextEmoji`.
- `ButtonActions` order across legacy and Block Kit buttons on one message.
- Wire marshal against the three captured presses.
- `App` flow from the captured frames: preview arrives → dialog → Shuffle →
  `PressBlockAction` arguments → `message_changed` replaces the item in place,
  without "(edited)", dialog reopened → Send → `message_deleted` removes it.
  In both the channel pane and a thread.
- `app_id` lookup failure becomes the dialog's error toast.
- Existing action dialog tests pass unchanged.

## 5. Stage 3 — the `/` picker

### 5.1 Data

- `slackclient.(*Client).AppCommands(ctx) ([]core.SlashCommand, error)` calls
  `client.appCommands` (`defaultReasons`: `"client.appCommands":
  "dispatch-client-extras"`) and parses `commands[]` into the neutral
  **`core.SlashCommand{Name, Usage, Desc, AppName, AliasOf string}`**.
  `app_actions` is ignored. Tested against `client_app_commands.json`.
- `cmd/slk` fetches it once per workspace at connect, the way custom emoji are
  fetched (`customemoji.go`), and publishes it with
  `wctx.SetAppCommands(list)`: a whole slice swapped through an
  `atomic.Pointer` and never mutated, per the AGENTS.md shared-map rule. Not
  stored in SQLite: about 12 KB, refetched on every connect. A failed fetch
  leaves the list empty, so the picker does not open and commands still run.
- `core.CommandService` gains `List() []core.SlashCommand`: a non-blocking read
  of the active workspace's pointer. The picker **pulls** the list when it
  opens (RFC #236's "pull, not push"): no `…LoadedMsg`, no copy in the UI.

### 5.2 The widget

**`internal/bubbles/commandpicker`**, the first composer popup built in the RFC
#236 shape:

- `New(source SourceFunc, opts ...Option) Model`, where `type SourceFunc func()
  []core.SlashCommand` is declared in the package; options for `Styles`,
  `KeyMap`, width.
- Own `Styles` and `KeyMap`, following `bubbles/confirmprompt`: the App maps the
  theme onto `Styles` and pushes them from `applyTheme`.
- Drawn through `bubbles/chrome` (`Popup`, `List`, `PopupMarker`) — no box or
  list window of its own.
- Rows: `/name  usage — desc`, truncated to the width. Matching is a
  `text.Fold` prefix match on the name, as `channelpicker` does. Entries with
  `AliasOf` set are hidden (they still run, through §3).
- Imports `core` only; no `internal/ui`, no `slack-go`
  (`TestBubblesReachTheAppOnlyThroughCore`).

### 5.3 In the composer

- `compose` hosts it beside the emoji, mention and channel popups, with the same
  structure as `channelActive`. It opens when `/` is typed **at position 0** of
  an empty composer, as in Slack; the query is the text after `/` up to the
  first whitespace. Tab or Enter replaces the token with `/name ` and closes; Esc
  or a space closes; Enter therefore never runs a command while the picker is
  open. If the source returns no commands it does not open.
- Two duplications that a fourth popup would deepen are removed first:
  - `mode_insert.go` writes `IsEmojiActive() || IsMentionActive() ||
    IsChannelActive()` three times. It becomes **`compose.PickerActive()`**,
    which includes the new picker.
  - `view_messages.go` and `view_thread.go` each have the same three-way chain
    to draw the open popup. It becomes **`compose.PopupView(width)`**, called
    once from each; the new popup is added only there.
- `App` passes `a.commandSvc.List` (via a closure reading whichever service is
  wired) as the source to both composers.
- `RunCommand` (stage 1) omits `team_id` when the command is not in `List()`.

### 5.4 Tests (stage 3)

- Parser against `client_app_commands.json`: count, types, aliases.
- `commandpicker`: filtering, alias hiding, Tab and Enter completion, Esc,
  empty source → stays closed, row truncation.
- Compose: `/` opens only at position 0 of an empty composer; space closes;
  completion text; precedence with the other popups.
- `PickerActive` / `PopupView`: the existing emoji, mention and channel popup
  tests pass unchanged.
- A `dialogGoldens()` scenario for the open picker over the standard fixture.
- `team_id` omitted for an unlisted command, sent for a listed one.

## 6. What this reuses, and what is new

The audit behind this spec found these in the tree. Each row is reused or
extended, never copied.

| Need | Existing | Used as |
|---|---|---|
| Name → handler registry | `:` registry, `internal/ui/command.go` | shared dispatcher, second table |
| Presence change | presence menu apply step | extracted `applyPresenceAction` |
| Switch / join a channel | channel finder selection | extracted `openChannel` |
| Open a DM | new-message flow, `OpenConversation` | as is |
| `@`/`#` → IDs | `TranslateMentionsForSend`, `slackfmt.MentionedUserIDs` | as is, plus `MentionedChannelIDs` beside it |
| Rich text arguments | `mrkdwn.Convert` | as is |
| Form posts, `ok` parsing | `postForm`, `parseOKResponse` | as is |
| `_x_reason` | `slackhttp` `defaultReasons` | three entries |
| Port shape | `NewXService(XServiceFuncs)` | `CommandService` |
| Toasts | `toastWithClear`, `ToastMsg` | as is |
| Ephemerals, the `b` dialog, `buttonrow` | sub-project 1 | extended with a Block Kit variant |
| Bot → app | `GetBotInfo` (`bots.info`) | adapter lookup |
| Replace a message by TS | `SwapLocalSent` / `SwapLocalSentReply` | generalised to `ReplaceMessage` |
| Per-workspace data at connect | custom emoji fetch, `WorkspaceContext` atomics | `SetAppCommands` |
| Popup drawing | `bubbles/chrome`, `messages.ChromeStyles()` | as is |
| Matching | `text.Fold` prefix, as `channelpicker` | as is |
| Widget `Styles` / theme push | `bubbles/confirmprompt` precedent | as is |

Genuinely new, with nothing to reuse: `client_token` generation;
`core.CommandService`; `RunCommand`, `BlockAction`, `AppCommands`; the
`view_opened` event; putting a failed command's text back;
`compose.OfferDraft`; `commandpicker`.

## 7. Delivery

Three PRs, in order, each passing the AGENTS.md gate (`go build`, `go vet`,
`go test ./... -race`, `gofmt -l .` empty, `golangci-lint`) at every commit.

1. **Stage 1.** Fixes the live bug on its own. The plan should reach a build the
   owner can run against a real workspace (remote path working) before the
   built-ins, the restore and the polish.
2. **Stage 2.** `/giphy cats` end to end. Same early-runnable rule.
3. **Stage 3.** The picker.

Each PR updates the AGENTS.md shared-code tables in the same commit as the
helper it adds: `dispatchCommand`/`slashCommands`, `applyPresenceAction`,
`openChannel`, `slackfmt.MentionedChannelIDs`, `compose.OfferDraft`,
`CommandService`, `newClientToken`, `RunCommand`; then `PressBlockAction`,
`BlockAction`, `ReplaceMessage`; then `AppCommands`, `commandpicker`,
`PickerActive`/`PopupView`. Each also lists new test helpers there.

## 8. Found while planning

Recorded, not fixed here:

- **Three channel-mention regexes** already exist: `slackfmt.channelMentionRe`,
  `messages/render.go`'s `channelMentionRe`, and `mrkdwn/tokens.go`'s
  `reChannel`. This spec extends the first and adds no fourth; converging them
  is separate work.
- **The ephemeral spec's assumption was wrong.** It said that with sub-project 1
  in place "`/giphy` needs no special handling". The captures show `/giphy`'s
  buttons are Block Kit buttons pressed with `blocks.actions`, and that Shuffle
  needs ephemeral `message_changed`. Stage 2 is the correction.

## 9. Out of scope

- App modals (bare `/giphy`, any app's forms): see the research note.
- Block Kit buttons in a message's top-level `blocks`, selects, and buttons on
  non-ephemeral messages.
- `app_actions` (global shortcuts) from `client.appCommands`.
- Ranking the picker by the user's command history (`users.prefs`
  `frecency`).
- Sending `/msg` text immediately rather than pre-filling.
- Built-ins beyond §3.3 that the web client may handle locally but no capture
  shows: they take the remote path, and Slack's answer decides.
- Enterprise Grid: no capture. The requests are scoped exactly as captured;
  `team_id` / `service_team_id` are the workspace's.

## 10. Risks

- **Request shape.** `chat.command` and `blocks.actions` are new endpoints for
  slk, sent urlencoded where the web client sends multipart. This is the same
  accepted gap every slk request has; field names and order match the captures.
- **Extra `bots.info`.** The first press per bot makes one `bots.info` call the
  web client probably serves from its local cache. One call per bot per
  session.
- **`/away` as a toggle** reads slk's cached presence; if that is stale the
  toggle goes the wrong way once, and the next `manual_presence_change`
  corrects the status bar.

## Addenda (implementation, stage 1)

- `_x_reason` for `chat.command` is set with `slackhttp.WithReason(ctx, "executeCommand")` in `RunCommand`, not via `defaultReasons`, because that table is pinned to an older attested capture set by `TestGenericReasonIsAValueTheOfficialClientSends`. Stages 2 and 3 (`blocks.actions`, `client.appCommands`) must do the same; §3.4's and §6's mentions of `defaultReasons` entries are superseded.
- `/open` falls back to `ChannelService.SearchRemote` when the name is not among the finder's rows (production finder rows hold only the user's own conversations), via `channelLookupMsg`.
- Refused or failed built-ins (and a failed `/msg` open) give the typed text back, like the remote path.
- The `me_message` render test asserts the body line, because timestamps are always italic.
- Built-in tests guard "runs locally" with `TestSlashBuiltinsAreLocal` (table membership), because a remote run happens inside a tea.Cmd the tests do not execute.
- `/msg` pre-fill text keeps mentions in wire form (`<@U…>`); it still sends correctly.
- Review (Greptile, PR #309): `CommandService.Run` takes the `teamID` the command was typed in, and `cmd/slk` resolves that workspace's client with `router.ByID`, as `MessageService.Forward` does; §3.4's `Run` signature is superseded. Stage 2's `PressBlockAction` should do the same.
- Review: `conversations.open` request IDs come from a counter that never resets. A `/msg` pre-fill whose open is superseded by a picker submit or cancelled by Esc in the picker is given back like a failed one, and a second `/msg` while one is opening is refused.
- Review: any Unicode space (as in `strings.Fields`) separates a command's name from its arguments, so a pasted non-breaking space still runs the command; §3.1's ASCII wording is superseded.
