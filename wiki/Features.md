# Features

What `slk-gio` does, in short. The [slk-gio README](https://github.com/gammons/slk/blob/main/gui/README.md)
describes each feature in full, with its keys. slk's terminal client, which
this page used to describe, was retired on 2026-10-31.

## Windows and the conversation list

- Each conversation opens in a window of its own; the conversation list is one
  more window. Every window draws its own header (picture, presence, name, topic
  or status and local time) unless the compositor decorates it.
- The list follows your Slack sidebar: **Unread** first (mentions before the
  rest), **Starred**, then your Slack sections, each by recent activity;
  conversations quiet for 14 days fold behind "+N quiet". Typing filters every
  conversation.
- The quick switcher (`Ctrl+K`) filters every conversation and lists what is
  waiting to be read; channels you are not in show up under "Channels you're
  not in" (one edgeapi search per pause) and join when chosen.
- "New message…" in the switcher (or the list's pencil) starts a DM or group DM
  with up to eight people; `/msg @person [text]` does the same.
- On Linux a tray icon keeps slk-gio running with no window open; a second
  launch brings the running one's list forward.

## Messages

- Real time over Slack's WebSocket: messages, edits, deletions, reactions,
  typing ("Ada is typing") and apps' "working" status.
- Formatting from `rich_text` (or mrkdwn), Block Kit sections, headers,
  context, images and actions, legacy attachments as cards, image thumbnails
  and file chips that open in a previewer, avatars, custom emoji as pictures.
- Threads open in their own window, with "Also send to #channel"; a Threads
  window lists the threads you follow, unread ones marked.
- Every message has an actions menu: copy text or link, open in Slack, forward
  (with an optional note) to another conversation, mark unread from here, and
  for your own messages edit and delete. Text can be selected and copied.
- Reactions show as pills; click to toggle, "+" for a picker, hover to see who
  reacted.
- An app's reply only you can see shows its legacy buttons, which work (a
  confirmation first when the app asks for one).
- A link to a message in this workspace opens its window scrolled to it (and
  its thread); other links open in the browser.
- Scrolling back loads older pages from the cache, then from Slack.

## Compose

- Enter sends, Shift+Enter is a new line; Up in an empty composer edits your
  last message.
- Completion for @people and usergroups, #channels, :emoji: and the
  workspace's slash commands; the composer shows readable names and sends
  Slack's form.
- Slash commands run through `chat.command` as in Slack's web client;
  `/open`, `/join`, `/msg`, `/dm`, `/away`, `/active` and `/status away|active`
  are handled locally.
- Files: Ctrl+V attaches a picture on the clipboard or a pasted file path, the
  paperclip opens the desktop's file chooser; each file up to 10 MB, the text
  as the caption.
- readline keys in every text field (Ctrl+W, Ctrl+U, Alt+B/F, …).

## Search and the cache

- Messages live in a SQLite cache; windows open from it at once and catch up
  with one request in the official client's shape.
- The search window searches every cached message as you type, best match or
  newest first, with a query language (`from:`, `in:`, `mentions:`, `is:`,
  `has:`, `before:`/`after:`/`since:`, phrases, `!exclusions`, `a | b`).
- A paced backfill (one request every eight seconds, paused while a chat window
  has the keyboard) fills the last 90 days for search; the search window shows
  each conversation's coverage and the backfill's progress.

## Presence and notifications

- Your presence and do-not-disturb on the list's header tile: Active, Away,
  snooze for 20 minutes to 24 hours or until tomorrow morning, end DND. The
  tray menu offers the common ones.
- Other people's custom status and DND show in headers and hover
  cards, and follow changes live.
- Desktop notifications for mentions (usergroups you are in included), DMs
  and keywords while no slk-gio window has the keyboard; per conversation:
  default, every message, or nothing. Clicking one opens the message.

## Sign-in and connection

- Workspaces are signed in with `slk --add-workspace`, from the Slack desktop
  app's session (or `--browser`); slk-gio refreshes the tokens from the desktop
  app at every start.
- The boot is the one Slack's web client makes; slk-gio never lists the
  workspace's conversations. The socket redials with backoff, and reconnects
  catch up through the cache.

## Export

- Export one channel's messages and threads for a date range with the
  `slk` CLI:

  ```sh
  slk export \
    --workspace example-workspace \
    --channel project_alpha \
    --since 2026-04-01 \
    --until 2026-07-01 \
    --timezone America/New_York \
    --output ./project-alpha-export \
    --overlap 14
  ```

  Each conversation (a standalone message, or a thread with its replies)
  becomes one Markdown file, and `index.md` links them by day.
  - `--since` is inclusive and `--until` exclusive, both read as midnight in
    `--timezone` (default: local). `--until` defaults to tomorrow.
  - `--overlap N` widens the range by N calendar days on each side.
  - A reply inside the range is exported even when its thread started
    earlier; the older parent is kept, labelled, as context. Finding those
    threads means scanning the channel's whole earlier history, so a first
    export of a long-lived channel takes a while.
  - `--workspace` takes a slug, team ID or name, and may be omitted with a
    single workspace. `--channel` takes a name or ID. `--output` must be
    empty or absent, and defaults to a folder under the exports directory.

## Customization

- Four palettes of slk-gio's own (`hadal`, the default; `dark`; `ink`;
  `light`), every slk theme by name, custom theme files in
  `~/.config/slk/themes/`, and single colours overridden in `[gui.theme]`.
- See [[Configuration]] for the keys slk-gio reads.

## Not there yet

- One workspace per running slk-gio (start another with `-team NAME`).
- `/leave`, a custom `/status`, app dialogs (modals) and Block Kit buttons,
  drag-and-drop of files, strike-through (drawn dimmed).
- The terminal client's activity feed, server-side search of history older
  than the cache, and other features listed in the
  [GUI feature backlog](https://github.com/gammons/slk/blob/main/docs/superpowers/plans/2026-10-31-gui-feature-backlog.md).
