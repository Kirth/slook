# slk-gio (prototype)

A graphical frontend for slk, built with [Gio](https://gioui.org). Each
conversation opens in its own window; the conversation list is one more
window, and the program exits when the last window closes.

```
make gui                    # builds bin/slk-gio, inside gui/shell.nix when Nix is there
bin/slk-gio -fake           # made-up conversations, no Slack account needed
bin/slk-gio -fake -fake-every 0   # the same, without someone posting every 4 seconds
bin/slk-gio                 # the first workspace from `slk --add-workspace`
bin/slk-gio -team NAME      # a workspace by team ID or name
bin/slk-gio -backfill 1y    # also fill the message cache back a year, slowly (90d, 0 = everything)
```

The conversation list's header has a search button. It opens the search
window: type to search every cached message (thread replies too) as you
type. Results come best match first, with the matched words picked out
in a snippet around them; "Newest" above the results sorts by date
instead. A result opens its conversation scrolled to that message,
loading older pages until it is there. While the box is empty the window lists
each conversation's coverage (how many messages, since when or
"complete", gaps, a dot when it follows the live socket), and the panel
at the bottom shows the backfill's progress, lets you start it with a
horizon (90 days by default, 1 year, everything) or stop it, and lists
what is being fetched right now. Each conversation shows how far toward
the horizon its cache reaches (a bar and a percentage); the panel shows
the same for the whole workspace, and while the backfill runs, how fast
messages are coming in and roughly how long is left.

Colours come from `[gui.theme]` in slk's `config.toml`. `preset` (or
`name`) is one of the shipped palettes:

- `hadal`, the default: the trench floor as an office. Blue-black, lit
  only by content; hairlines; one teal accent.
- `dark`, slk-gio's original look (also `slack-dark`).
- `ink`: ink-blue with an amber accent.
- `light`.

Any slk TUI theme name also works ("nord", "gruvbox dark", …), mapped
onto the GUI's colours. Single colours can be overridden by key, in
the block itself or under `[gui.theme.colors]`. `-theme NAME` picks a
palette for one run.

```toml
[gui.theme]
preset = "hadal"
accent = "#2fb7c9"        # optional overrides, "#rrggbb"
```

The colour keys are `background`, `surface` (header bars and panels),
`overlay` (the switcher, hover card and menus), `text`, `text_muted`,
`on_accent`, `accent`, `mention`, `broadcast`, `border`, `hover`,
`control`, `control_hover`, `error`, `warning`, `success`, `badge`,
`code_text`, `code_background`, `ephemeral` and `shadow`. `shadow`
takes `#rrggbbaa`; its alpha is the shadow's strength.

While no slk-gio window has the keyboard, mentions, direct messages
and `on_keyword` matches raise a desktop notification. These follow
`[notifications]` in `config.toml`, as the TUI does, and muted
conversations stay silent. On Linux with a notification service,
clicking a notification opens the conversation at the message (a
thread reply opens its thread). Each conversation shows one
notification, updated as messages arrive, which goes away once you read
the conversation here or in Slack. With `notify_command` set, or
without a service, the basic notification is shown, which cannot open
anything.

The composer completes as you type:

- `@` suggests people slk has seen, plus `@here`, `@channel` and
  `@everyone`.
- `#` suggests your channels.
- `:` with two or more letters suggests emoji, the workspace's custom
  ones included.

Up and Down choose, Tab or Enter take one, and Escape hides the list.
The composer shows readable text (`@Ada Lovelace`, `#general`), and the
message goes out in Slack's form, so mentions notify and channel names
link. Editing a message shows its mentions the same readable way.

Every message has an actions menu: "⋯" among the actions shown while
the pointer is over it, or a right-click anywhere on it. Copy text and
Copy link put the message or its permalink on the clipboard, and Open
in Slack opens the permalink. For your own messages there is also Edit
message and Delete message…. Edit puts the message in the composer:
Enter saves, and Escape or Cancel puts back what you were writing. Up
in an empty composer edits your last message there. Delete asks first:
Enter or Delete deletes, and Escape or Cancel keeps the message.

Reactions show as pills under a message: the emoji and how many
reacted, outlined in the accent color when you are among them. Click a
pill to add or take back your reaction, and "+" (or the "+ 🙂" that
appears while the pointer is over a message) opens a picker. Type to
filter by name, move with the arrows, and press Enter or click to
react. Reactions others add or remove arrive live. The workspace's
custom emoji are drawn as their pictures, in message text, in pills and
in the picker. `-fake` makes up a picture for each.

Scrolling back in a conversation loads the page before what is shown
as you near the top, one page at a time, from the cache when it holds
it and from Slack otherwise. What you are reading stays in place while
the older messages arrive above it. A "Loading older messages…" row
sits above them during a fetch, and "This is the beginning of #name"
once there is nothing older.

A thread opens in a window of its own. To open one, click a message's
reply count, its "Reply in thread" action (shown while the pointer is
over it), or the "Replied to a thread" line under a reply that was also
sent to the channel. The window shows the parent, the replies, and a
composer that replies in the thread. Slash commands typed there run in
the thread. Tick "Also send to #channel" under the composer to post the
reply to the conversation as well; the box clears after each reply.
Replies from others arrive live, and the thread is marked read in Slack
while its window has the keyboard. A search result that is a thread
reply opens its conversation at the parent and the thread on top, lit
on the reply.

Every window draws its own header bar: the conversation's picture
(with a green dot when the person is active, a grey ring when away),
its name, and a line about it: a channel's topic, member count and
whether it is muted; a person's status, title and local time (amber
with a moon when it is night for them); a group's size. Then its
actions and minimize, maximize and close.

The menu button (or Ctrl+K) opens a quick switcher over the window:
type to filter every conversation, or pick from those waiting to be
read; Enter or a click opens it, "All conversations…" opens the list.
Its corner shows what is waiting elsewhere: a red count of
conversations that mention you, or a dot for other unread. Choosing a
conversation whose window is already open on another workspace brings
it to this one. Wayland lets no app raise or move its own window, but a
new window opens where you are, so the old one hands over its messages,
scroll position, draft and size to a new one and closes; the list
window does the same. Drag the header to move the window; resize from
its edges. When the compositor draws window decorations itself
(server-side decorations, and always on X11), its buttons are used and
the header leaves them out.

The list is arranged by activity, around your Slack sidebar:

- **Unread** first: conversations with unread messages (muted ones only
  when they mention you), mentions before the rest.
- **Starred**, never folded.
- Then **your Slack sections** in Slack's order (custom sections,
  Channels, Direct messages), each with its recently active
  conversations by most recent activity. Conversations with no message
  and no visit from you in 14 days fold away behind "+N quiet"; a
  seasonal channel rises by itself when it gets messages and folds
  again after.

Each row shows unread mentions, or when the conversation last had a
message. Typing filters every conversation, quiet ones included: names
starting with the text first, then the most recently active; Enter opens
the first, or click a row. Viewing a conversation (its window has the
keyboard) marks it read in Slack, as Slack's own clients do. Visits are
kept in slk's cache, which the TUI's channel finder also orders by.

Unread state, newest-message times, sections, stars and mutes each take
one request for the whole workspace (`client.counts`,
`users.channelSections.list`, `stars.list`, `users.prefs.get`), and
stay current from the socket. In a chat, Enter sends, Shift+Enter is a new line, and
Ctrl+K opens the quick switcher. Links open in the browser and
pictures and files in the previewer when clicked, and the pointer says
so. Hovering an author's
name or avatar, or an @-mention, shows a card with their profile:
picture, names, title, status and local time.

Slash commands work as in Slack. Typing `/` lists the workspace's
commands (Slack's own and installed apps', from `client.appCommands`):
Up/Down pick one, Tab, Enter or a click completes it, Escape hides the
list. Enter runs the command through `chat.command`, as Slack's web
client does; what it produces comes back like any message, and a reply
only you can see (`/remind`'s confirmation, say) shows in a tinted box
marked "Only visible to you" until the window reloads. A refused command
("/nope is not a valid command") is reported under the composer with
the text put back. `/open #channel` and `/msg @person [text]` are handled
locally, as Slack's client does, and open that conversation's window
(`/msg` puts the text in its composer). A leading space sends `/text` as
a plain message.

### Editing keys

The composer and the list's filter take the usual keys: Ctrl+A selects
all, Ctrl+Z / Ctrl+Shift+Z undo and redo, Ctrl+C/X/V, Ctrl+arrows move
by word (Shift selects), Ctrl+Home/End. Ctrl+Backspace and Ctrl+Delete
delete a word (Gio's editor misses these on Linux; `readline.go` handles
them). From readline:

| Key | Does |
|---|---|
| Ctrl+E | end of line (Shift selects) |
| Ctrl+W | delete back to the previous space |
| Ctrl+U | delete back to the start of the line |
| Alt+Backspace / Alt+D | delete the word before / after |
| Alt+B / Alt+F | back / forward a word (Shift selects) |
| Ctrl+Y | paste back what the last of those deleted |
| Ctrl+H / Ctrl+D | delete the character before / after |
| Ctrl+T | swap the two characters at the caret |

Ctrl+K stays the conversation switcher, so readline's kill-to-end is not
there; Ctrl+B and Ctrl+F are left free for bold and the like.

It is a separate Go module so Gio's cgo dependencies stay out of slk's
build, CI and the flake's `vendorHash`. The TUI is not a constraint in
this fork: helpers it needs are moved into `internal/` packages the GUI
can import, not copied.

## What a message shows

- **Formatting** from Slack's `rich_text` block when the message has one
  (everything typed in a Slack client does), else from the mrkdwn text
  through `internal/slack/mrkdwn`: bold, italic, inline code, code
  blocks, quotes, bulleted and numbered lists, links, @-mentions,
  #channels, @here, emoji.
- **Block Kit**: section, header, context, divider, image and actions
  blocks (buttons are drawn, not pressable).
- **Legacy attachments** as a card with Slack's color stripe: author,
  title, text, fields, image or thumbnail, footer.
- **Uploads**: image files as thumbnails, other files as a chip with
  name and size. Clicking one previews it in `flux-preview`, with left
  and right stepping through the message's other images and files.
  They are downloaded once, with the workspace's credentials, to
  `$XDG_RUNTIME_DIR/slk-gio/files/<file ID>/<name>` (private to you,
  cleared at logout); without `flux-preview` the downloaded file opens
  in the desktop's default application. Images in link previews and
  Block Kit preview the same way. The previewer is started hidden when
  slk-gio starts, so the first preview is quick.
- **Avatars** beside the first message of an author's run, and in the
  list for direct messages. A colored disc with the initial stands in
  while the picture loads or when there is none.

Pictures and avatars download with the workspace's credentials through
`internal/image`, into the same `~/.cache/slk/images` the TUI uses and
under the same keys, so the two share what they have fetched.

Fonts come from fontconfig (`fonts.go`): the text font in each style,
monospace, a CJK font and the emoji font, loaded explicitly. Gio's own
font scan works on NixOS but picks a variable "Noto Sans" with no bold,
and falls back to it for katakana and emoji, which then draw as nothing.

## Staying connected

The socket is redialled with backoff after every drop (a dead connection
is noticed within a minute), and each drop and recovery is logged to
stderr with the time. On reconnect the list's unread state is refreshed
in one request and every open conversation window catches up through
the cache. No Slack request may take longer than a minute; a hung one
fails instead of freezing a window.

## Messages and the cache

Messages live in slk's SQLite cache (`~/.local/share/slk/cache.db`) and
a window opens from it: the newest hundred cached messages are drawn at
once, and one `conversations.history` page in the official client's
shape (28 messages, `cached_latest_updates` vouching for what is held)
brings the conversation up to date in the background; edits and
deletions made while slk-gio was closed arrive through that page. A
conversation the cache has never seen is fetched first, so the first
open still shows Slack's current state. Live messages, edits, deletions
and reactions from the socket are written through as they arrive, and
the cache records which ranges of each conversation it holds completely
(`message_spans`), which is what a local search will stand on. The
plumbing is `internal/msgsync`; the design is
`docs/superpowers/specs/2026-10-29-message-cache-sync-design.md`.

Conversations the backfill should skip (alert channels, bot output) go
in slk's config, `~/.config/slk/config.toml`; what arrives live in them
is still cached and searchable, and opening one still shows its history:

```toml
[cache]
backfill_ignore = ["leafcloud-alerts-production", "leafcloud-alerts-staging", "noise-*"]
```

Messages that arrive while slk-gio runs are cached as they come. What
arrives while it is closed, or while the connection is down, is caught
up when the socket connects. One `client.counts` names the
conversations with newer messages than the cache holds, and each of
those the cache already follows gets one page, most recently visited
first. That covers at most 20 conversations, 3 seconds apart, and never
anything `backfill_ignore` names. Conversations the cache has never
followed are left alone, so a first run fetches nothing. To tune it:

```toml
[cache]
catch_up = true           # false turns it off
catch_up_limit = 20       # conversations per connect
```

`-backfill` turns on a slow background fill (one request every eight
seconds, paused while a chat window has the keyboard) back to the given
horizon, then the threads in it; the search window starts and stops the
same fill and shows its progress. It is off by default and deliberate
about its pace: Slack has signed slk users out over bulk history
fetching (`docs/superpowers/specs/2026-07-30-enterprise-grid-bootstrap-design.md`).

## Speed

A window should be up in well under 100 ms. The fonts (well over
100 MB of files here) are parsed once, at startup while signing in,
and shared by every window's text shaper; shapers do not rescan the
system's fonts, since fontconfig already chose ours (`fonts.go`); and a
theme with a warmed-up shaper is kept ready for the next window.

`SLK_GIO_TRACE=1 bin/slk-gio` prints, for every window opened, how long
it took to be configured and drawn. To compare Gio's renderers, try it
with `GIORENDERER=opengl` and without.

## Layout

- `rich/` — a message's display form: blocks of styled spans, built from
  rich_text elements, Block Kit, attachments and files. No Gio import, so
  it is tested without a display.
- `backend/` — the `Backend` interface the windows use, with `Slack`
  (real, through `internal/slack`) and `Fake`. A stopgap: `internal/core`'s
  ports push `internal/ui` message types, so they cannot serve a second
  frontend until those move into `core`.
- `chat/` — a chat window's message state (`Timeline`), testable without
  a display.
- `sidebar/` — how the conversation list is arranged (`Arrange`,
  `Search`), testable without a display.
- `textedit/` — readline's editing commands on text and a caret.
- `cmd/slk-gio/` — the windows. `hub.go` owns them and routes events; each
  window runs its own event loop and is reached only through its `inbox`.
  `richview.go` draws `rich` blocks, `images.go` hands decoded pictures
  to every window.

## Not there yet

Dialogs that some commands open (an app's modal) and the buttons on
ephemeral replies (`/giphy`'s Shuffle and Send) do nothing yet; joining
a channel you are not in and starting a new DM through `/open` and
`/msg` are not there either, nor `/leave`. Threads (replies show only
as a count), reactions (cached, not drawn), scrolling back past the
newest hundred messages by hand (a search result pages back for you;
the window does not on scroll yet), custom emoji as pictures
(they show as `:name:`), usergroup names, strike-through (the text is
dimmed instead), selecting message text (code blocks can be selected),
and more than one workspace at a time.
