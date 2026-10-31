# Tradeoffs and Non-Goals

slk is intentionally not a 1:1 port of the desktop client. Some Slack features are deferred or out of scope.

## Not there yet

`slk-gio` has no activity feed, no server-side search of history older than
its cache, no `/leave`, no custom `/status`, no app dialogs (modals), no
pressable Block Kit buttons and no drag-and-drop of files, and serves one
workspace per process. The full list, with the Slack calls each would need,
is the [GUI feature backlog](https://github.com/gammons/slk/blob/main/docs/superpowers/plans/2026-10-31-gui-feature-backlog.md).

## Not planned

- Joining or starting huddles (slk only shows who is in one, and can copy a conversation's huddle link — opening that link happens in Slack's own clients), Slack Connect, Workflow Builder
- Bot/app management, custom emoji management
- Animated reactions, link unfurls, in-app toasts

## Markdown caveats

The composer's text goes out through a CommonMark → mrkdwn + rich_text
conversion:

- Editing a message you originally formatted with markdown may flatten the rich_text formatting on Slack clients that prefer blocks. The mrkdwn fallback (`*bold*`, etc.) still renders correctly everywhere.
- Headings (`# Title`) and blockquotes (`> quote`) are passed through verbatim — Slack has no heading construct and `>` is already valid mrkdwn.
- Tables, footnotes, task lists, and reference-style links are not translated.

## Image caveats

- Animated GIFs show as a static first frame.

## Auth caveat

slk signs in with the Slack desktop app's session (or a browser session with
`--browser`), not an OAuth app. `slk-gio` refreshes desktop tokens at every
start; a browser-session token lasts as long as that browser session, after
which `slk --add-workspace --browser` refreshes it. See [[Setup]].

## Unofficial / TOS caveat

slk talks to Slack via the same internal browser protocol the official web
client uses. This is unofficial and not sanctioned by Slack — using it may
violate Slack's [API](https://slack.com/terms-of-service/api) and
[user](https://slack.com/terms-of-service/user) Terms of Service, and Slack
may break the protocol or invalidate tokens at any time. Use at your own risk
on workspaces where that's acceptable to you and your admins.
