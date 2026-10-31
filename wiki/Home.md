# slk Wiki

slk is a fast, native Slack desktop client, **`slk-gio`** (built with
[Gio](https://gioui.org), one window per conversation, no Electron), and
**`slk`**, the command-line companion that signs your workspaces in and
exports channels to Markdown. slk's terminal client was retired on
2026-10-31; the pages about it are kept under "Historical".

Marketing site: [getslk.sh](https://getslk.sh) · Source: [github.com/gammons/slk](https://github.com/gammons/slk)

## Getting started

1. **[[Installation]]** — the `slk` CLI from packages or Go; `slk-gio` from source and its prerequisites.
2. **[[Setup]]** — signing in from the Slack desktop app (or a browser session), adding workspaces.
3. **[[Configuration]]** — `config.toml`: the keys slk-gio reads, themes, XDG paths.

## Using slk

- **[[Features]]** — what slk-gio does, and `slk export`.
- The [slk-gio README](https://github.com/gammons/slk/blob/main/gui/README.md) — every feature in detail, with its keys.

## Reference

- **[[Tradeoffs and Non-Goals|Tradeoffs-and-Non-Goals]]** — what's missing, what's out of scope, known caveats.
- **[[Architecture]]** — how the two programs and the shared packages fit together.

## Historical (the retired terminal client)

- [[Keybindings]], [[Commands]], [[Terminal Compatibility|Terminal-Compatibility]], [[Clipboard and OSC 52|Clipboard-and-OSC-52]]

## Project

- License: [MIT](https://github.com/gammons/slk/blob/main/LICENSE)
- `slk` is an independent, unofficial project. Not affiliated with Slack Technologies, LLC. See the [[Tradeoffs and Non-Goals|Tradeoffs-and-Non-Goals]] page for the TOS caveat.
