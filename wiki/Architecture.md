# Architecture

slk is two programs over one set of shared Go packages:

- **`slk-gio`**, the desktop client, built with [Gio](https://gioui.org). It
  lives in its own Go module, `gui/`, so Gio's cgo dependencies stay out of
  the CLI's build.
- **`slk`**, the command-line companion (`cmd/slk`): signs workspaces in from
  the Slack desktop app or a browser session, removes and lists them, and
  exports channels to Markdown.

The terminal client slk started as was retired on 2026-10-31.

```
slk-gio windows (gui/cmd/slk-gio)    one event loop per window; hub.go routes
        │                            events and owns the windows
gui/backend.Backend                  the one port: Slack (real) and Fake (-fake)
        │
internal/bootstrap (+ adapt)         the boot Slack's web client makes: userBoot,
                                     counts, conversations.view, edge revalidation;
                                     never a workspace enumeration
internal/msgsync                     the one writer of the message cache: history
                                     pages, threads, socket frames, paced backfill
internal/slack (+ boot, edge,        Slack Web API, edgeapi and the browser-protocol
  resolve, mrkdwn), slackhttp        WebSocket, with a browser-shaped transport
internal/cache                       SQLite: a cache, not the source of truth
internal/config, onboard, paths      config.toml, sign-in and token re-mint, XDG
```

- Display logic that does not need a window (timelines, completion, search
  queries, the list's arrangement, the read gate) lives in `gui/chat`,
  `gui/search` and `gui/sidebar` and is tested without a display.
- Messages render from `gui/rich`, a display form built from rich_text,
  mrkdwn, Block Kit, attachments and files (`internal/blockkit` parses them).
- Pictures download through `internal/image` into an on-disk LRU cache.
- `gui/boundary_test.go` keeps the retired terminal client's packages and
  terminal toolkits out of the GUI.

## Further reading

- [`gui/README.md`](https://github.com/gammons/slk/blob/main/gui/README.md) — what slk-gio does, feature by feature, and its package layout
- [`AGENTS.md`](https://github.com/gammons/slk/blob/main/AGENTS.md) — the full package map, invariants and shared helpers, for contributors
- Design docs: [`docs/superpowers/specs/`](https://github.com/gammons/slk/tree/main/docs/superpowers/specs/) (its README says which still apply)
