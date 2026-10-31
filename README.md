# slk

> **A fast, native Slack desktop client — and the CLI that signs it in.**
> `slk-gio` is the client: each conversation in its own window, built with
> [Gio](https://gioui.org), no Electron. `slk` is the command-line companion:
> it signs your workspaces in from the Slack desktop app and exports channels
> to Markdown.
>
> Marketing site: [getslk.sh](https://getslk.sh) · Docs: [Wiki](https://github.com/gammons/slk/wiki)

slk began as a terminal client; that client was retired on 2026-10-31 in
favour of `slk-gio`. The wiki's pages about terminal keybindings, terminal
graphics and OSC 52 describe it and are kept for history.

## slk-gio

- **One window per conversation**, plus a conversation list arranged like your
  Slack sidebar (unread, starred, your sections, quiet ones folded away), a
  quick switcher (`Ctrl+K`) and a tray icon that keeps it running.
- **Real time** over Slack's own WebSocket: messages, edits, deletions,
  reactions, typing and apps' "working" status, presence and do-not-disturb.
- **Threads** in windows of their own, and a Threads window for the ones you
  follow.
- **Everything is cached** in SQLite and searchable offline with a small query
  language (`from:ada in:#general "build failed" since:1w`); a slow, paced
  backfill fills the cache for search.
- **Compose** with @people, #channel, :emoji: and slash-command completion,
  edit and delete your messages, upload files from the clipboard, a pasted
  path or the file chooser.
- **Join channels** you are not in, start DMs and group DMs, forward
  messages, mark them unread, react, set your presence and snooze
  notifications, press the buttons on an app's reply only you can see, open
  links to messages in-app.
- **Desktop notifications** for mentions, DMs and keywords, per conversation.
- **Themes**: four palettes of its own, every slk theme, and your own.
- **Signs in from the Slack desktop app**: no tokens to copy, no Slack App,
  and it refreshes them from the desktop app at every start. It boots the way
  Slack's web client does and never lists the workspace's conversations, the
  request pattern that has got Enterprise Grid users signed out.

The full description, feature by feature, is
[gui/README.md](gui/README.md).

## Install

### The `slk` CLI

**Homebrew** (macOS and Linux):

```bash
brew install --cask gammons/tap/slk
```

**Arch Linux** (community-maintained [AUR package](https://aur.archlinux.org/packages/slk)):

```bash
yay -S slk
```

**Go:**

```bash
go install -ldflags="-s -w" -trimpath github.com/gammons/slk/cmd/slk@latest
```

Release packages (tarballs, `.deb` / `.rpm` / `.apk`, Windows) carry the CLI
only; see the [Installation wiki page](https://github.com/gammons/slk/wiki/Installation).

### `slk-gio`

`slk-gio` is built from source for now (Linux is what it is developed and
tested on). It needs Go 1.26, a C compiler, `pkg-config`, and Gio's
libraries:

- **Debian/Ubuntu:**
  ```bash
  sudo apt-get install -y gcc pkg-config libwayland-dev libx11-dev libx11-xcb-dev \
    libxkbcommon-x11-dev libgles2-mesa-dev libegl1-mesa-dev libffi-dev \
    libxcursor-dev libvulkan-dev
  ```
- **Nix / NixOS:** `nix-shell gui/shell.nix` provides Go and the libraries
  (wayland, libxkbcommon, libGL, vulkan-headers, libX11, libXcursor,
  libXfixes, libxcb). It uses your system's `<nixpkgs>` on purpose, so the
  binary loads the same graphics driver your system does; for that reason
  the flake has no `slk-gio` package.
- **Other distributions:** the same libraries under their names; see
  [Gio's Linux notes](https://gioui.org/doc/install/linux).

```bash
git clone https://github.com/gammons/slk.git
cd slk
make gui          # bin/slk-gio (inside gui/shell.nix when nix-shell is there)
make build        # bin/slk, the CLI
```

At run time slk-gio uses, when present: `wl-paste` (wl-clipboard) on Wayland
or `xclip` on X11 to paste pictures, `zenity`, `kdialog` or `yad` for the file
chooser, `xdg-open` for links, and `flux-preview` to preview files. Try it
without a Slack account: `bin/slk-gio -fake`.

## Setup

slk reads your session directly from the **Slack desktop app** — no DevTools,
no tokens to copy. Make sure you're signed in to the desktop app, then:

```bash
slk --add-workspace
```

slk lists the workspaces you're signed in to; pick the ones you want, then
start `slk-gio` (`slk-gio -team NAME` for another than the first).

No desktop app (a server, a container, an SSH session)? `slk --add-workspace
--browser` signs in from your browser session instead: in DevTools > Network,
Copy as cURL any request to `/api/` on app.slack.com, and paste it. slk also
offers this when it cannot read the desktop app's session.

`slk --list-workspaces`, `slk --remove-workspace` and `slk export` (a
channel's messages and threads for a date range, as Markdown) do what they
say; `slk --help` lists everything.

Full walkthrough: [Setup wiki page](https://github.com/gammons/slk/wiki/Setup).

## Debugging

- `SLK_DEBUG=1 slk-gio` writes diagnostics to stderr: the boot's Slack
  requests by endpoint ("boot requests:"), how many conversations the boot had
  to fill in ("gap size"), and the whole session's requests when it quits
  ("requests this session:"). A "FALLING BACK" line (printed with or without
  `SLK_DEBUG`) means the boot failed and slk-gio listed conversations the old
  way; please report it.
- `SLK_GIO_TRACE=1 slk-gio` prints how long each window took to open.
- `SLK_DEBUG=1 slk …` (the CLI) writes `slk-debug.log` to the current
  directory, truncated each run; its lines are categorized (`[cache]`,
  `[general]`, …) so `grep '\[cache\]' slk-debug.log` slices to one area.

## Documentation

- [gui/README.md](gui/README.md) — what slk-gio does and how
- The [**wiki**](https://github.com/gammons/slk/wiki):
  [Installation](https://github.com/gammons/slk/wiki/Installation),
  [Setup](https://github.com/gammons/slk/wiki/Setup),
  [Features](https://github.com/gammons/slk/wiki/Features),
  [Configuration](https://github.com/gammons/slk/wiki/Configuration) (which
  `config.toml` keys slk-gio reads),
  [Tradeoffs and Non-Goals](https://github.com/gammons/slk/wiki/Tradeoffs-and-Non-Goals),
  [Architecture](https://github.com/gammons/slk/wiki/Architecture)
- For contributors: [AGENTS.md](AGENTS.md), and the design docs in
  [docs/superpowers/specs](docs/superpowers/specs/README.md)

## Contributing

Contributions are welcome. A few ground rules:

- **AI-assisted PRs are accepted** — and in fact encouraged — but only if
  they're driven by a **frontier model** (e.g. Claude Opus, GPT-5,
  Gemini Pro) running with **high thinking effort**. Low-effort,
  small-model output that nobody reviewed tends to create more work than
  it saves, and will be closed.
- Ideally, drive the work with the
  [superpowers](https://github.com/obra/superpowers) framework (or an
  equivalent skills/TDD-disciplined workflow). Brainstorm the design
  first, write tests, then implement.
- **For large feature additions, open an issue first.** Before sinking
  time into a big change, file an issue to discuss the idea and approach
  so we can agree on direction. Bug fixes and small improvements can go
  straight to a PR.
- Whether human- or AI-written, **you are responsible for your PR.**
  Understand the diff, make sure both modules (the root and `gui/`) build and
  pass `go vet ./...` and `go test ./... -race`, and be ready to explain your
  choices in review. Read [AGENTS.md](AGENTS.md) first.

## Disclaimer

`slk` is an independent, unofficial project. It is not affiliated with, endorsed by, or sponsored by Slack Technologies, LLC or Salesforce, Inc. "Slack" is a trademark of Slack Technologies, LLC; it is used here only to describe the service this client interoperates with.

slk talks to Slack via the same internal browser protocol the official web client uses. This is unofficial and not sanctioned by Slack — see [Tradeoffs and Non-Goals](https://github.com/gammons/slk/wiki/Tradeoffs-and-Non-Goals#unofficial--tos-caveat) for details.

## License

[MIT](LICENSE) © Grant Ammons
