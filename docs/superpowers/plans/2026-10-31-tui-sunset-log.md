# TUI sunset — execution log

Plan: `/home/kirth/.claude/plans/cozy-gliding-swing.md`. Start commit (before any of this work): `9a774d1a`.

Environment for every command: Go via `nix-shell gui/shell.nix` (root) / `cd gui && nix-shell shell.nix` (gui module).

## Phase 0 — commit the pre-existing WIP

- Commit: `e8a8e5d4` — `gui: followed threads, typing, structured search, text selection, tray and single instance`
- Content: the user's uncommitted work as found (52 modified files, 28 untracked paths incl. `internal/searchq/`, `internal/threadsubs/`, `internal/typing/`, `gui/selection/`, `gui/backend/threads.go`, `gui/cmd/slk-gio/{textselect,threadswin,tray_*,instance_*,typingline}.go`, `slook-32x32.png`). Nothing of the executor's included.
- Checks run before committing (both green):
  - root: `go build ./... && go vet ./... && gofmt -l . && go test ./...` → build/vet clean, gofmt empty, `go test` exit 0
  - gui: `go build ./... && go vet ./... && gofmt -l . && go test ./...` → clean, gofmt empty, exit 0

## Phase 1.1 — `blockkit: move the parse half to internal/blockkit`

- Commit: `4397d7f8`
- `git mv` (all `R` in `git show --name-status -M`, 13 renames): `parse.go`, `richtext.go`, `files.go`, `parse_test.go`, `richtext_test.go`, `rendersbody_test.go`, `testdata/*.json` (7) from `internal/ui/messages/blockkit/` to `internal/blockkit/`.
- New: `internal/blockkit/{types.go,rendersbody.go,message.go,message_test.go,fixtures_test.go,types_test.go}`; shim `internal/ui/messages/blockkit/shim.go`; `messages.MessageTextSource` → `bkparse.MessageText(msg)`.
- Survivors rewritten (import path only): `cmd/slk/attachments.go`, `gui/backend/slack.go`, `gui/rich/blocks.go`, `gui/rich/files.go`.
- Checks before commit: root build/vet/gofmt clean; `go test ./internal/blockkit ./internal/ui/... ./cmd/slk ./internal/export` all ok; gui build/vet/gofmt clean, `go test ./...` all ok.
- Deviations:
  1. `internal/ui/messages/blockkit/integration_test.go` (TUI render assertions over the fixtures) was kept, not deleted; its `loadFixture` now reads `../../../blockkit/testdata`. Reason: keeps the TUI renderer's fixture coverage until Phase 5 deletes the package; `internal/blockkit/fixtures_test.go` carries its own `loadFixture` and the parse-only fixture tests (7 `TestFixture_*`, one per fixture).
  2. `capturedMentionAttachments` stays in the moved `parse_test.go` as planned; the TUI's `attachments_test.go` render tests also use it, so a copy (const + `decodeCapturedAttachments`) is in the new test file `internal/ui/messages/blockkit/captured_test.go`. Dies with the package in Phase 5.
  3. `blockType` (unexported, used by the moved `TestBlockTypesImplementInterface`) is also declared in `internal/blockkit/types.go`.
  4. The three `TestMessageTextSource_*` tests moved out of `internal/ui/messages/blockkit_integration_test.go` into `internal/blockkit/message_test.go` as `TestMessageText_*`.
  5. AGENTS.md: added an `internal/blockkit` row; `RendersBody` / `FileThumbs` rows point at `internal/blockkit`.

## Phase 1.2 — `slackfmt, datefmt: move the plain-text mrkdwn and date helpers out of the TUI`

- Commit: `56fc5c56`
- `git mv` (all `R`): `internal/ui/messages/{datetoken.go,datetoken_test.go,flatten.go,flatten_test.go}` → `internal/slackfmt/`; `internal/ui/messages/dateseparator_test.go` → `internal/datefmt/datefmt_test.go`.
- New: `internal/slackfmt/commonmark.go` + `commonmark_test.go` (the 17 `TestCommonMark_*` from `render_test.go` + `TestCommonMarkUsergroupMention` from `render_usergroup_test.go`), `slackfmt.ChannelGlyph`, `slackfmt.ExpandDateTokens`, `internal/datefmt/datefmt.go` (`DateSeparator`, `SetNow`), shims in `internal/ui/messages/shim.go`.
- `flatten.go`: `usergroupDisplay` dropped (calls `usergroups.Display`), `subteamMentionRe` replaces `usergroupMentionRe`, optional-label `channelRefRe` replaces render's `channelMentionRe`; slackfmt's `broadcastRe` deleted, `StripMarkupWithUserGroups` uses `specialMentionRe`.
- Survivors rewritten with the plan's three `sed`s plus imports: `internal/export/{channel,markdown,save,channel_test,markdown_test}.go`, `cmd/slk/export_collect{,_test}.go`, gui `notifications/center.go`, `cmd/slk-gio/{chatwin,commands,listwin,msgview,searchwin,switcher,threadswin,threadwin}.go`, and the comment in `gui/backend/backend.go` (`messages.ChannelGlyph` → `slackfmt.ChannelGlyph`, by the plan's sed).
- Checks before commit: root build/vet/gofmt clean; `go test ./internal/blockkit ./internal/slackfmt ./internal/datefmt ./internal/export ./internal/ui/... ./cmd/slk` all ok; gui build/vet/gofmt clean, `go test ./...` all ok; `grep -rn 'internal/ui' internal/export | wc -l` = 0.
- Deviations:
  1. `internal/export/markdown.go` called `messages.SlackMrkdwnToCommonMark(messages.MessageTextSource(msg), …)`, which the plan's first sed only half-rewrites; the inner call was rewritten to `blockkit.MessageText(msg)` by hand.
  2. `dateTokenRe` lives in `internal/slackfmt/datetoken.go` (next to its only users), not `commonmark.go`: the CommonMark code does not use it.
  3. render.go (TUI) still uses `specialMentionRe`, `usergroupMentionRe`, `usergroupDisplay` and `renderDateToken`, which were declared in the moved `flatten.go`/`datetoken.go`. They are re-declared in the shim file `internal/ui/messages/shim.go` (`renderDateToken` delegates to `slackfmt.ExpandDateTokens`), so `render.go` is unchanged except for the removed CommonMark functions. `model.go` lost `ChannelGlyph`, `SetNowFunc`, `FormatDateSeparator` (now in `shim.go`); `nowFunc` stays (FormatShortDate, LongTimestamp).
  4. `TestRenderSlackMarkdown_CalendarStyleRange` stays in `internal/ui/messages`, in the new file `calendar_token_test.go` (a new name so git keeps the `datetoken_test.go` rename).
  5. `TestHeaderGlyph_ByChannelType` renders the messages pane header (`m.View`), so it cannot move; it stays, and `internal/slackfmt/slackfmt_test.go` gets a direct `TestChannelGlyph` with the same table.
  6. "the 4 usergroup cases from render_usergroup_test.go": that file has one CommonMark and one Flatten usergroup test; both moved (`TestCommonMarkUsergroupMention` → `commonmark_test.go`, `TestFlattenUsergroupBareResolved` → `flatten_test.go`). Its four renderer cases (labeled, bare resolved, bare unresolved, label without `@`) stay with the renderer; `commonmark_test.go` gets CommonMark twins of them (`TestCommonMarkUsergroupMention{Labeled,BareResolved,BareUnresolved,LabelWithoutAt}`).
  7. `TestEmojiAfterSkinTonedEmojiKept` (render + CommonMark in one test) stays in `render_test.go`, going through the shim.
  8. `datefmt_test.go` cannot call `messages.DateFromTS`; it derives the local-day string as `time.Unix(t.Unix(), 0).Format("2006-01-02")` (what `DateFromTS` does) via a `localDay` helper.
  9. AGENTS.md rows updated (FlattenMrkdwn, CommonMark, ExpandDateTokens, DateSeparator, SetDateTokenTimeLayout, ChannelGlyph).

## Phase 1.3 — `theme: move the palette table and user colours out of internal/ui/styles`

- Commit (move): `80fc94f9`
- `git mv` (both `R`): `internal/ui/styles/themes.go` → `internal/theme/themes.go`, `internal/ui/styles/themes_test.go` → `internal/theme/themes_test.go`. Renames per plan (`Colors`, `ColorsByName`, `LoadCustom`, `RegisterCustom`, `Names`, `Resolve`).
- New: `internal/theme/usercolor.go` (`UserPalette []color.NRGBA` via `hex()`, `UserColor`), `usercolor_test.go` (`TestUserColorDeterministic`, `TestUserColorPaletteSpread` moved from `styles_test.go`, + `TestUserPaletteValues`), `TestLoadCustom` in `themes_test.go`; shim `internal/ui/styles/theme_shim.go`; `UserColorPalette` deleted (only used in styles.go).
- Tests that stay in styles (lipgloss/Apply): `TestLoadCustomThemes`, `TestANSIDarkThemeRegistered`, `TestANSILightThemeRegistered`, `TestANSIThemesSelectionTintPaletteInherited` → `internal/ui/styles/themes_tui_test.go`. `TestChannelsPanelContrast` is pure (hex maths over `builtinThemes`) and moved to `internal/theme`.
- gui rewritten with the plan's sed (`gui/palette/palette.go`, `palette_test.go`, `gui/cmd/slk-gio/theme.go`); `authorColor` returns `theme.UserColor(name)` directly. `gui/backend/backend.go` package comment reworded ("internal/ui message types" → "the TUI's own message types") so the `internal/ui` grep is 0. `cd gui && go mod tidy` dropped `charm.land/bubbletea/v2` (indirect) from `gui/go.mod` and 4 lines from `gui/go.sum`.
- Checks before commit: root build/vet/gofmt clean; `go test -count=1 ./internal/theme ./internal/ui/... ./cmd/slk ./internal/demo` all ok (TUI goldens included); gui build/vet/gofmt clean, `go test ./...` ok.
- Deviations:
  1. Shim file is `internal/ui/styles/theme_shim.go`, not `themes.go`, and the staying tests are in `themes_tui_test.go`: a new file at the old path would make git record the moves as add + modify instead of renames.
  2. styles tests assigned the unexported `customThemes` map directly (`seams_test.go`, `styles_test.go`, `TestLoadCustomThemes`). `internal/theme` exports two test helpers, `SnapshotCustom() (restore func())` and `ClearCustom()`, and those three tests call them (clear where they cleared, snapshot/restore where they saved/restored). Clearing had to be kept: `TestComposeInsertBG_StableAcrossBuiltinThemes` (tint_test.go) iterates every listed theme and fails on the all-black custom "seltest" if the registry is restored rather than cleared.
  3. AGENTS.md: new `internal/theme` row; the gui palette row now names `theme.ColorsByName`.

- Commit (behaviour): `9d6876ca` — `gui: custom themes and <!date> tokens from the shared packages`
  - `gui/cmd/slk-gio/main.go`: `slackfmt.SetDateTokenTimeLayout(cfg.Appearance.TimestampFormat)` and `theme.LoadCustom(os.DirFS(filepath.Join(paths.Config(), "themes")))` before `palette.Resolve` (`LoadCustom` already ignores a missing dir).
  - `internal/slackfmt/datetoken.go`: `FormatDateToken(ts int64, format, fallback string) string` (FORMAT in local time; fallback only when FORMAT is unusable; `""` when neither). Tests `TestFormatDateToken_CalendarPrefersFormat`, `TestFormatDateToken_FallbackWhenFormatUnusable`.
  - `gui/rich/richtext.go` `RichTextSectionDateElement` (was line 142): uses `FormatDateToken`; the fallback when the timestamp is 0; the old `Jan 2, 2006 15:04` local rendering only when both give nothing. Test `TestFromRichTextDateFormatFirst` (calendar element with ts + `{time}` + date-only fallback renders the time); the existing `TestFromRichTextBlocks` case (ts 1, no format, fallback "yesterday") still renders "yesterday".
  - `gui/README.md` colours paragraph: custom theme files line. AGENTS.md: `FormatDateToken`, gui seeding of the time layout.
  - Checks before commit: root build/vet/gofmt clean, `go test ./internal/slackfmt` ok; gui build/vet/gofmt clean (after `gofmt -w gui/cmd/slk-gio/main.go`), `go test ./...` ok.

## Verify (Phase 1) — run at `9d6876ca`

Commands as in the plan; `-count=1` added to the `go test` invocations so nothing is served from the test cache, and the bare `go test` lines run under `nix-shell gui/shell.nix --run`. Verbatim output:

```
$ nix-shell gui/shell.nix --run 'go build ./... && go vet ./... && gofmt -l . && go test ./internal/blockkit ./internal/slackfmt ./internal/datefmt ./internal/theme ./internal/export ./internal/ui/... ./cmd/slk'
ok  	github.com/gammons/slk/internal/blockkit	0.012s
ok  	github.com/gammons/slk/internal/slackfmt	0.015s
ok  	github.com/gammons/slk/internal/datefmt	0.002s
ok  	github.com/gammons/slk/internal/theme	0.010s
ok  	github.com/gammons/slk/internal/export	0.017s
ok  	github.com/gammons/slk/internal/ui	88.641s
ok  	github.com/gammons/slk/internal/ui/activityview	0.879s
ok  	github.com/gammons/slk/internal/ui/channelfinder	0.007s
ok  	github.com/gammons/slk/internal/ui/channelpicker	0.008s
ok  	github.com/gammons/slk/internal/ui/compose	0.065s
ok  	github.com/gammons/slk/internal/ui/emojipicker	0.005s
ok  	github.com/gammons/slk/internal/ui/help	0.017s
ok  	github.com/gammons/slk/internal/ui/imgrender	0.008s
ok  	github.com/gammons/slk/internal/ui/linkpicker	0.006s
ok  	github.com/gammons/slk/internal/ui/marks	0.008s
ok  	github.com/gammons/slk/internal/ui/mentionpicker	0.005s
ok  	github.com/gammons/slk/internal/ui/messages	1.373s
ok  	github.com/gammons/slk/internal/ui/messages/blockkit	0.007s
ok  	github.com/gammons/slk/internal/ui/newmessagepicker	0.012s
ok  	github.com/gammons/slk/internal/ui/overlay	0.008s
ok  	github.com/gammons/slk/internal/ui/peerstatus	0.005s
ok  	github.com/gammons/slk/internal/ui/presencemenu	0.005s
ok  	github.com/gammons/slk/internal/ui/reactionpicker	0.052s
ok  	github.com/gammons/slk/internal/ui/reactionsview	0.422s
ok  	github.com/gammons/slk/internal/ui/searchresults	0.025s
ok  	github.com/gammons/slk/internal/ui/sidebar	0.043s
ok  	github.com/gammons/slk/internal/ui/statusbar	0.008s
ok  	github.com/gammons/slk/internal/ui/styles	0.007s
ok  	github.com/gammons/slk/internal/ui/themeswitcher	0.009s
ok  	github.com/gammons/slk/internal/ui/thread	0.681s
ok  	github.com/gammons/slk/internal/ui/threadsview	0.911s
ok  	github.com/gammons/slk/internal/ui/userprofile	0.376s
ok  	github.com/gammons/slk/internal/ui/wintree	0.004s
ok  	github.com/gammons/slk/internal/ui/workspace	0.006s
ok  	github.com/gammons/slk/internal/ui/workspacefinder	0.006s
ok  	github.com/gammons/slk/cmd/slk	8.787s
exit=0
$ cd gui && nix-shell shell.nix --run 'go build ./... && go vet ./... && go test ./...'
ok  	github.com/gammons/slk/gui/backend	1.704s
ok  	github.com/gammons/slk/gui/chat	0.005s
ok  	github.com/gammons/slk/gui/cmd/slk-gio	1.691s
?   	github.com/gammons/slk/gui/internal/richtext	[no test files]
?   	github.com/gammons/slk/gui/internal/styledtext	[no test files]
ok  	github.com/gammons/slk/gui/motion	0.002s
ok  	github.com/gammons/slk/gui/notifications	0.009s
ok  	github.com/gammons/slk/gui/palette	0.002s
ok  	github.com/gammons/slk/gui/rich	0.004s
ok  	github.com/gammons/slk/gui/search	0.006s
ok  	github.com/gammons/slk/gui/selection	0.002s
ok  	github.com/gammons/slk/gui/sidebar	0.003s
ok  	github.com/gammons/slk/gui/textedit	0.002s
exit=0
$ grep -rln 'internal/ui' gui --include='*.go' | wc -l
0
$ grep -rn 'internal/ui' internal/export | wc -l
0
$ grep -c 'bubbletea\|bubbles' gui/go.mod
0
$ ls internal/blockkit/testdata/*.json | wc -l
7
$ go test ./internal/blockkit -run Fixture -v | grep -c '^--- PASS'
8
$ go test ./internal/slackfmt -run 'TestFormatDateToken|TestFlatten|TestCommonMark' -v | grep -c '^--- PASS'
28
$ grep -n 'theme.LoadCustom\|SetDateTokenTimeLayout' gui/cmd/slk-gio/main.go
78:	slackfmt.SetDateTokenTimeLayout(cfg.Appearance.TimestampFormat)
82:	theme.LoadCustom(os.DirFS(filepath.Join(paths.Config(), "themes")))
```

Additionally: full root `go test ./...` at `9d6876ca` → exit 0, every package ok. Capture check: `git diff e8a8e5d4..HEAD -M --name-status -- internal/blockkit/testdata internal/ui/messages/blockkit/testdata internal/slack/testdata internal/slackhttp/testdata docs/superpowers/specs/2026-10-12-slash-commands-captures` → only the 7 `R100` fixture renames.

Not run in Phase 1 (not required by the hand-over contract until Phases 5/6): `go test ./... -race`, `golangci-lint` (not in the nix shell).

## Phase 2.1 — `internal/onboard`

- Commit: `3b21a6f6` — `onboard: move workspace sign-in and token re-mint out of the TUI binary`
- `git mv` (all `R`): `cmd/slk/onboarding_core.go` → `internal/onboard/onboard.go` (+ `_test`), `remint.go` → `internal/onboard/remint.go` (+ `_test`), `add_workspace_config.go` → `internal/onboard/config.go` (+ `_test`), `onboarding_error_test.go` → `internal/onboard/desktop_error_test.go`, `save_atomic_test.go` → `internal/config/write_test.go`.
- Exported: `BuildWorkspaceTokens`, `Minter`, `WorkspaceChoice{Label, TeamID}`, `DesktopChoices`, `RemintTokens` (the injectable core, unchanged), `UniqueSlug`, `ExistingSlugs`, `AppendWorkspaceConfigBlock`. New: `Remint(ctx, store, mint) ([]Token, error)` (lists the store, `RemintTokens` with `slackdesktop.Cookie`/`Tokens` and `store.Save`), `SaveWorkspace(store, tok, configPath) (SaveResult, error)` (the UI-free half of `saveWorkspace`; `cmd/slk/onboarding.go saveWorkspace` prints from `SaveResult`, same texts), `DesktopErrorMessage` (moved from `onboarding.go`, `desktop_error.go`). Imports: config, slack, slackdesktop; no huh/lipgloss/bubbletea (`paths` not needed: `SaveWorkspace` takes the config path).
- `internal/config/write.go` (Phase 3.3's placement, done here): `WriteAtomic`, `TOMLString`, `SanitizeComment`, unexported `writeMu`, from `cmd/slk/save_theme.go`; cmd/slk call sites rewritten (`save_theme.go`, `save_width.go`, `save_version_ts.go`, onboard's `config.go`).
- Checks before commit: root build/vet/gofmt clean, `go test ./internal/onboard ./internal/config ./cmd/slk` ok; gui build/vet ok.
- Deviations:
  1. The mutex cannot be shared across packages unexported, and Phase 3's `versionts.Save` (another package) must hold it across its read-modify-write, so `config.LockWrites() (unlock func())` exports it (`defer config.LockWrites()()`); `writeMu` stays unexported.
  2. `AppendWorkspaceConfigBlock` keeps its plain `os.WriteFile` and takes no lock (unchanged behaviour; a move commit). The plan's "shared by versionts.Save and --add-workspace" is not done: `--add-workspace` runs alone in its process.
  3. `TestConfigSavers_ReaderNeverSeesPartialFile` moved with the file into `internal/config`; its writers were `saveGlobalTheme`/`saveWorkspaceTheme`/`saveWorkspaceWidth`/`saveWorkspaceVersionTS`, which config cannot call, so they are three read-modify-write writers over a `rewriteLine` test helper (lock, read, regexp-replace one line, `WriteAtomic`). Added `TestTOMLString`, `TestSanitizeComment` (the theme-saver tests that covered them die in 2.2). `TestWriteConfigAtomic_*` renamed `TestWriteAtomic_*`.
  4. `DesktopErrorMessage` moved too (not in the plan's list): the GUI needs it for the no-workspace message, and it is UI-free. `onboarding_error_test.go` moved with it rather than staying in cmd/slk.
  5. The `saveWorkspace` tests in `onboarding_browser_test.go` (`TestSaveWorkspaceTwiceKeepsConfigLoadable`, `TestSaveWorkspaceLeavesUnloadableConfigAlone`) stay in cmd/slk; they exercise `onboard.SaveWorkspace` through the printing wrapper.
  6. AGENTS.md: rows for the config write helpers, `internal/onboard`, and `Remint`.

- Commit: `fc6133a1` — `gui: re-mint the token from the desktop session at startup`
  - `gui/cmd/slk-gio/main.go`: `remintStored(store)` runs `onboard.Remint(ctx 10 s, store, slackclient.MintToken)` before `pickToken` (which now takes its result); failures are logged to stderr by onboard's `log.Printf` (the GUI does not redirect `log`) and the stored token is used. No tokens: the message gains a second line, `onboard.DesktopErrorMessage(err)` when `slackdesktop.Cookie()` fails. The connect-failure hint no longer says "run slk once to refresh it" (slk no longer refreshes on launch after 2.2): "sign in to the Slack desktop app and start slk-gio again, or run slk --add-workspace".
  - `gui/go.mod`/`go.sum`: `go mod tidy` adds slackdesktop's deps (goleveldb, gosecret, x/crypto, dpapi, …) and moves `golang.org/x/net` 0.48 → 0.56. `gui/README.md`: a paragraph on the startup refresh. AGENTS.md `Remint` row names `remintStored`.
  - Checks: gui build/vet/gofmt clean, `go test ./...` ok.
  - Deviation: remint runs before the single-instance claim, so a second launch (which then exits) also re-mints. Harmless (desktop tokens are read locally; mint only for workspaces without one) and it keeps the plan's "before pickToken" order.

- Commit: `6a64733c` — `blocks: move ApplyActionIDs out of cmd/slk` (Phase 4.7's placement, done here): `internal/core/blocks/action_ids.go` + `action_ids_test.go` (`TestApplyActionIDs_Misaligned`, moved from `cmd/slk/event_handler_test.go`); `rtm_handler.go` called `blocks.ApplyActionIDs` until 2.2 deleted it. AGENTS.md row updated. Separate commit so 2.1 stays a pure onboarding move.

## Phase 2.2 — `cmd/slk: keep the CLI, drop the TUI composition root`

- Commit: `99c4edba`
- `git rm` 76 files (every `cmd/slk/*.go` not on the keep/edit/pending lists). 36 `.go` files remain.
- Kept as the CLI: `main.go`, `main_scope_test.go`, `dump.go`, `export_command{,_test}.go`, `export_collect{,_test}.go` (not in the plan's keep list, but export needs it), `onboarding.go`, `onboarding_browser{,_test}.go`, `paths.go`, `remove_workspace.go`, `flushinput_{bsd,linux,other,windows}.go`, `usernames{,_test}.go`, `users.go`, `attachments{,_test}.go`.
- Edits per plan: `main.go` (run, newImageHTTPClient, --demo, config/emoji pre-probe gone; no argument → help + "slk is the CLI (workspace setup, export); the desktop app is slk-gio." on stderr + exit 2; imports fmt, os, debuglog); `printHelp` (header "set up Slack workspaces for slk-gio, and export channels", no "Launch the TUI"); `mainGoAllowed` = version, commit, date, main, printHelp; `usernames.go` (Get/Set/Snapshot only; tests SnapshotIsIndependent, ConcurrentAccess, NilIsEmpty kept); `users.go` (resolveUser, resolveDMNames gone; `messageAuthor` without `router`); `attachments.go` (`core.Attachment`, `blockkit.FileThumbs` inline, `collectThumbs` gone; tests unchanged); `export_command.go` `GetChannels` comment added.
- Also edited: "Run slk to start." → "Run slk-gio to start." in `onboarding.go` / `onboarding_browser.go` (slk no longer starts a client); `export_command.go` comments no longer mention the TUI.
- **Kept for Phases 3/4, trimmed** (Phase 3's executor: these are what is left):
  - `bootstrap_adapters.go`: `applyBootUsers` removed (wrote `WorkspaceContext`), with `TestApplyBootUsers_FillsTheMapsTheSidebarReads`. `bootUserDisplayName` stays (now unused outside its doc; Phase 3.1 decides). `bootstrap_adapters_test.go` gains `newTestDB` (was in the deleted `reconnect_sync_test.go`; `user_resolver_test.go` uses it).
  - `starred_conversations.go`: `reconcileStarredConversations` removed (an `rtmEventHandler` method); `missingStarredConversations` stays. Tests `TestBuildChannelItem_StarredDM` and `TestReconcileStarredConversations_ReconnectInactive` removed; the five `TestMissingStarredConversations_*` stay.
  - `user_resolver.go`: `send func(tea.Msg)` → `send func(any)` carrying local `userResolvedMsg` / `userStatusMsg` / `userExternalMsg` (same fields as the TUI's `UserResolvedMsg` / `UserStatusChangeMsg` / `UserExternalMsg`), so the batching tests keep their synchronisation. `applyProfileStatus` and `statusExpiry` (were in the deleted `peer_status.go`) live here now with `send func(any)`. `queueResolvedPeer` and the `resolvedPeers`/`resolvedWake` fields removed (only the deleted rtm handler drained them). `user_resolver_test.go`: `TestResolveDMNames`, `TestResolveDMNames_FallbackPersistsStatusAndEmitsMessage` removed (resolveDMNames is gone); the rest pass with the type renames.
  - `workspace.go`: only `mostRecentlyVisitedChannel` (WorkspaceContext, workspaceRouter, UnresolvedDM gone; they referenced the rtm handler, peer-status refresher and `ui/sidebar`).
  - `connect.go`: only `shouldReloadTimeout` and `refreshVersionTS(ctx, client, cfg, configPath, teamName)`, the body of connectWorkspace's background `client.ShouldReload` goroutine (lines 47-69) extracted as a function so it compiles without connectWorkspace. Seeding is `seedVersionTS` in `save_version_ts.go`.
  - `clipboard_wayland.go`: `nativeClipboardRead` (the X11 reader over `golang.design/x/clipboard`) removed; `IsWayland`, `HasWlPaste`, `WaylandClipboardReader`, `wlPasteBytes` stay. Needed for the dependency to drop.
  - Unchanged: `save_version_ts{,_test}.go`, `channel_search{,_test}.go`, `interactions{,_test}.go`.
- `go mod tidy` (root): `golang.design/x/clipboard`, `golang.org/x/exp/shiny`, `golang.org/x/mobile` dropped. `charm.land/bubbletea/v2` stays a direct requirement: `internal/ui`, `internal/bubbles`, `internal/demo`, `internal/editor` import it; `cmd/slk` reaches it only through `charm.land/huh/v2` (`go list -deps ./cmd/slk` shows bubbletea and atotto/clipboard, both huh's).
- Deviations:
  1. The tidy would not drop `golang.design/x/clipboard` while three `internal/ui` tests imported it for `clipboard.Format`: `app_test.go`, `mode_insert_keys_test.go`, `services_helpers_test.go` now use `core.ClipboardFormat` / `ClipboardImage` / `ClipboardText` (`setClipboardReaderForTest` takes the core type and assigns it directly). Test-only edits to the TUI; `go test ./internal/ui` ok.
  2. `flake.nix` `vendorHash` updated to `sha256-lt+/XOExh/8/K4NOzQtVv8TolpsuMcBnhmIh08m1YV0=` (from `nix build .#slk.goModules`'s mismatch report), since go.mod changed. `buildInputs = [pkgs.libX11]` and the flake description stay for Phase 5.3.
  3. `ls cmd/slk/*.go | wc -l` is 36, not ≈25–30: the 13 files kept for Phases 3/4 (listed above) are the difference.
  4. Not touched (Phase 5.3/6): `docs/assets/demo/*.tape` run `slk --demo`, which no longer exists; `Makefile`/`.goreleaser.yaml` still build `./cmd/slk` (now the CLI); README still describes the TUI.
  5. AGENTS.md: the `cmd/slk` architecture line; the map invariant (`Snapshot()`, no `WorkspaceContext`); the known-duplication bullet (msgsync is the only writer); rows fixed or removed for symbols deleted here (`WorkspaceContext.IsBotUser`, `queueResolvedPeer`/rtm drain, `upsertFinderItem`, rtm `messageAuthor`/`messageItemFromEvent` → export's `messageAuthor`, `newClipboardWriter`, `launchOS`, `collectThumbs`, `runCommand`/`pressAttachmentAction` wiring, `applyProfileStatus` location, the user-name store row). Other TUI rows are Phase 6's rewrite.
- Checks before commit: root `go build ./... && go vet ./... && gofmt -l .` clean, `go test ./...` every package ok; gui `go mod tidy` (no change), build/vet/gofmt clean, `go test ./...` ok.

## Verify (Phase 2) — run at `99c4edba`

Commands as in the plan; `-count=1` added to the `go test` invocations, bare `go` lines run under `nix-shell gui/shell.nix --run`. Verbatim output:

```
$ nix-shell gui/shell.nix --run 'go build ./... && go vet ./... && gofmt -l . && go test ./cmd/slk ./internal/onboard -race'
ok  	github.com/gammons/slk/cmd/slk	5.603s
ok  	github.com/gammons/slk/internal/onboard	1.019s
exit=0
$ nix-shell gui/shell.nix --run 'go run ./cmd/slk --help && go run ./cmd/slk --version && go run ./cmd/slk --list-workspaces && go run ./cmd/slk export --help'
slk dev -- set up Slack workspaces for slk-gio, and export channels

Usage:
  slk --add-workspace     Add a Slack workspace (interactive)
  slk --add-workspace --browser
                          Add one from a browser session (no desktop app)
  slk --remove-workspace  Remove a configured workspace (interactive)
  slk --list-workspaces   List configured workspaces (TeamID, Slug, Name)
  slk --dump-sections     Dump raw users.channelSections.list JSON (diagnostic)
  slk export [flags]      Export a channel's messages and threads to Markdown
                          (see 'slk export --help')
  slk --version          Print version and exit
  slk --help             Show this help

Config:  ~/.config/slk/config.toml
Data:    ~/.local/share/slk/
Cache:   ~/.cache/slk/

Docs:    https://github.com/gammons/slk
slk dev (commit none, built unknown)
Unofficial Slack client. Not affiliated with Slack Technologies, LLC.
Uses Slack's internal browser protocol; may violate Slack's TOS. Use at your own risk.
TEAM ID    SLUG       NAME
---------  ---------  ---------
TGRHBURAP  leafcloud  Leafcloud
Usage of slk export:
  -channel string
    	channel name or ID (required)
  -output string
    	directory to write, which must be empty or absent (default: under the slk exports directory)
  -overlap int
    	calendar days to add before --since and after --until
  -since string
    	first day to export, YYYY-MM-DD, inclusive (required)
  -timezone string
    	IANA timezone the dates are read in (default: local)
  -until string
    	day to stop before, YYYY-MM-DD, exclusive (default: tomorrow)
  -workspace string
    	workspace slug, team ID or name (required when more than one is configured)
exit=0
$ ls cmd/slk/*.go | wc -l
36
$ grep -ln 'internal/ui\|bubbletea' cmd/slk/*.go | wc -l
0
$ grep -n 'onboard.Remint' gui/cmd/slk-gio/main.go
226:	toks, err := onboard.Remint(ctx, store, slackclient.MintToken)
$ grep -c 'golang.design/x/clipboard' go.mod
0
$ go test ./cmd/slk -run TestMainGoHoldsOnlyTheEntrypoint
ok  	github.com/gammons/slk/cmd/slk	0.004s
```

Additionally: `go run ./cmd/slk` (no argument) prints the help, then "slk is the CLI (workspace setup, export); the desktop app is slk-gio." on stderr, exit status 2. gui at `99c4edba`: `go build ./... && go vet ./... && gofmt -l . && go test ./...` → all ok. `-race` for the whole root module not run (contract: before Phases 5 and 6).

## Phase 3 — GUI boots on `internal/bootstrap`; batched resolver; `version_ts`; starred closed DMs

### 3.1 + 3.5 — `bootstrap: move the boot adapters out of cmd/slk; share the fake Slack server`

- Commit: `d0a00836`
- `git mv` (all `R`): `cmd/slk/bootstrap_adapters.go` → `internal/bootstrap/adapt/deps.go` (R067), `bootstrap_adapters_test.go` → `adapt/adapt_test.go` (R078), `starred_conversations.go` → `adapt/starred.go` (R091), `starred_conversations_test.go` → `adapt/starred_test.go` (R090). `cmd/slk/workspace.go` deleted (`mostRecentlyVisitedChannel` → `adapt.MostRecentlyVisited`).
- Exported: `NewDeps(c, db, accessToken, openChannelID, health, logf)` (`Log` is the parameter; `Revalidate: edge.New(accessToken, c.TeamID(), c.HTTPClient())` verbatim), `Conversations`, `IMConversation`, `DropClosedMPIMs(chans, isOpen, logf)`, `Muted{Res}`, `UserDisplayName`, `HydrateFirstSight(db, ws, res, logf)`, `MostRecentlyVisited`, `MissingStarredConversations` (verbatim, `log.Printf` kept). Adapters (`bootAdapter`, `viewAdapter`, `countsAdapter`, `historyAdapter`, `rawMessages`, `storeAdapter`, `boundedMessageVersions`) stay unexported with their comments, the `rawMessages` lossiness comment included.
- New: `adapt.FillFromCounts(ctx, db, e, teamID, res, logf)` in `adapt/conversations.go`: ids in `res.Counts.Unreads` absent from `res.Channels`/`res.IMs` are the gap; cache rows first (`db.GetChannel`; channel/private/group_dm rows become list entries, cached DM rows are skipped: the row has no peer ID), the rest in one `edge.ChannelsInfo(ctx, teamID, {id: 0})` (archived and IM results dropped), each result upserted with `IsMember: true`. One log line: `boot: gap size N (conversations client.counts names that userBoot did not list): A from the cache, B from channels/info, C left out`. Tests `TestFillFromCounts_FillsTheGapFromCacheThenEdge`, `TestFillFromCounts_NoGapNoRequest`; also `TestMostRecentlyVisited`.
- `bootstrap.Unread.Latest` added; `countsAdapter` fills it and now calls `GetUnreadCountsContext(ctx)` (was the context-free `GetUnreadCounts`; its comment said it would switch once a ctx form existed). `TestCountsAdapter_CarriesUnreadsAndTheThreadRollup` asserts `Latest`.
- `internal/slack/slacktest`: `NewServer(t, bodies)`, `NewClient(t, srv *httptest.Server)`, `(*Server).RequestTo`, `Paths`, `Count`, `UserBootBody`; `Request` records `Body` (raw) as well as `Form`, because edgeapi posts JSON as `text/plain`. Removed from `adapt_test.go`.
- `cmd/slk/user_resolver_test.go` (until commit c) got `newTestDB` and `slacktest.NewClient`.
- AGENTS.md: rows for `internal/bootstrap/adapt` and `slacktest`; `boundedMessageVersions`, `IMConversation`, `MissingStarredConversations` rows point at `adapt`.

### 3.3 — `versionts: move _x_version_ts upkeep out of cmd/slk`

- Commit: `701f2537`
- `git mv`: `cmd/slk/save_version_ts.go` → `internal/config/versionts/versionts.go` (R066), `save_version_ts_test.go` → `versionts_test.go` (R066). `cmd/slk/connect.go` deleted; its `refreshVersionTS` + `shouldReloadTimeout` are `versionts.Refresh` in the new `refresh.go`.
- API: `Seed(env, cfg, teamID)`, `TOMLKey(cfg, teamID)`, `Save(configPath, tomlKey, teamID, teamName, ts)`, `Refresh(ctx, client, cfg, configPath, teamName, logf)`. `Refresh` with `configPath == ""` applies the value to the envelope and does not write (the GUI passes "" when `config.Load` returned an error). Tests: the nine moved (`TestSaveWorkspaceVersionTS_*` ×5, `TestSeedVersionTS_*` ×4) + `TestRefresh_AppliesAndSaves`, `TestRefresh_EmptyConfigPathDoesNotWrite`.
- `onboard.AppendWorkspaceConfigBlock`: `defer config.LockWrites()()` and `config.WriteAtomic` instead of `os.WriteFile` (Phase 2 deviation 2 closed).
- AGENTS.md: `versionts` row.

### 3.4 — `resolve: move the batched user resolver out of cmd/slk`

- Commit: `2a237c5e`
- `git mv`: `cmd/slk/user_resolver.go` → `internal/slack/resolve/resolve.go` (git records it as D + A: the file was rewritten to 25 % similarity, see deviation 4), `user_resolver_test.go` → `resolve_test.go` (R074).
- API as planned: `Profile`, `Batcher`, `Single`, `New(workspaceID, db, batch, single, degraded, onResolved)`, `(*Resolver).Resolve(ctx, ids) map[string]Profile` (cache → one edge batch → users.info per id, semaphore 8, synchronous), `Request(id)` (200 ms window, inflight dedupe, cache gate). Plus `Resolver.Logf` (diagnostics; the plan's `New` had no logger).
- Tests kept (renamed only where the API changed): `TestUserResolver_{BoundsConcurrentRequests,RequestDoesNotBlockTheCaller,BatchesMissesThroughEdge,EdgeMissFallsBackToPerUser,EmptyNameEdgeRecordFallsBackToPerUser,EdgeErrorFallsBackToPerUser,DegradedWorkspaceSkipsEdge,ResolveNowBatchesAndApplies,ResolveNowReturnsNilWhenDegraded,ResolveNowReturnsNilOnError,ResolveNowSkipsEmptyInput}` (the `ResolveNow*` tests call `Resolve`; "nil" is now "empty map", with no `Single`); `newResolvedWatch` is fed by `onResolved`. Dropped: `TestUserResolver_RecordsNamesInStore`. New: `TestResolve_CacheHitsMakeNoRequest`, `TestResolve_EdgeMissFallsBackPerUserBeforeReturning`.
- AGENTS.md: the resolver row and the profile-status row point at `resolve`.

### 3.2 + 3.6 — `gui: boot through client.userBoot, never users.conversations`

- Commit: `488975f6`
- `gui/backend/slack.go`: `ConnectSlack(ctx, tok, db, cacheDir, ConnectOptions{Config, ConfigLoadErr, ConfigPath, Log})`: `versionts.Seed` before `client.Connect`, then `newSlack` (the part after auth.test, also the test seam): `health`, `edge`, `resolver`, `go versionts.Refresh(context.Background(), …)`. `Conversations` = `bootOnce` (once per process, `bootMu`; `booted` flag) + a rebuild from `bootChans`/`counts`/visits/sections/mutes/notify levels with no request. `bootOnce`: visits → `adapt.MostRecentlyVisited` → errgroup(`bootstrap.Run(adapt.NewDeps(…))`, `sections.Bootstrap`); on error the loud `FALLING BACK` line (to `s.errOut`, stderr) + `GetChannels` + `GetUnreadCountsContext` + `mutes.Bootstrap(ctx, s.client)`; on success `HydrateFirstSight`, `Conversations`, `FillFromCounts`, `seedBootUsers`, `mutes.Bootstrap(ctx, adapt.Muted{res})`, counts + `noteThreadsAggregate` from `res.Counts`; then `MissingStarredConversations` (10 s, `res.IMs` or nil), `resolveAll(peers)`, state stored, `bootDone` closed, `SLK_DEBUG` → `slk-gio: boot requests:` + `DefaultCounter.Report()`. `opened()` appends to `bootChans`. `afterConnect(first)`: waits for `bootDone` (≤ `listWait`) and uses the boot's counts when `countsOK`; reconnects unchanged. `resolveAll` → `resolver.Resolve` (errgroup-of-8 gone); `userName` → `resolveAll` on a miss. `Profile()` unchanged (the only `GetUserProfileContext`).
- `gui/cmd/slk-gio/main.go`: `ConnectOptions` from `cfg`/`cfgErr`/`configPath`, `Log: debugf` (stderr with `SLK_DEBUG`); `reportRequests()` in the hub's exit closure and the tray's quit closure.
- `gui/backend/boot_test.go`: `TestConversations_BootsWithoutEnumerating`, `TestConversations_FallsBackToEnumerationOnlyWhenBootFails`, `TestConversations_SecondCallMakesNoRequest`, `TestConversations_FillsCountsGapFromEdge`; `cache.New(":memory:")`, `slacktest`.
- Risk 3 done here (`bootstrap.Result.RevalidatedUsers`): 1 field + 1 assignment in `internal/bootstrap`, 26 lines in `adapt.HydrateFirstSight` (rows for revalidated users without one, via `UpsertUserFromEdge` with the version) — 24 non-comment production lines in all. Tests `TestRevalidate_ReportsTheUsersEdgeReturned`, `TestHydrateFirstSight_CachesRevalidatedUsersWithoutARow`. Effect: on a cold cache the DM peers come back from boot's own `users/info` and the resolver asks nothing.
- `slacktest.Server.Requests(path)` added (the gap test reads each `channels/info` body).
- `gui/README.md`: the list paragraph describes the boot; AGENTS.md: a row for the Gio boot.
- `make gui` built `bin/slk-gio` (37.7 MB) at `488975f6`. The `-fake` backend (`gui/backend/fake.go`) is untouched; `go test ./backend` (fake tests included) passes.

### Request shape (Conversations + first connect)

Before (`76b0f18b`, read from the code; `Conversations` + `afterConnect(first)`):

| Endpoint | Requests |
|---|---|
| auth.test | 1 |
| users.conversations | 1 per 200 conversations |
| client.counts | 2 (Conversations, then the first connect) |
| users.channelSections.list | 1 |
| stars.list | 1 |
| users.prefs.get | 1 |
| users.info | 1 per DM peer the cache did not name (8 at a time); again on every list reopen for peers still unnamed |
| every list reopen | users.conversations + client.counts + channelSections + stars + prefs again |

After (`488975f6`), as `TestConversations_BootsWithoutEnumerating` recorded it (empty cache, no visit; `client.shouldReload` arrives in the background and is filtered from the list):

```
boot requests: [/api/auth.test /api/users.channelSections.list /api/client.userBoot /api/stars.list /api/client.counts /cache/T1/channels/info /cache/T1/users/info]
```

| Endpoint | Requests |
|---|---|
| auth.test | 1 |
| client.shouldReload | 1 (background) |
| client.userBoot | 1 |
| client.counts | 1 (the first connect reuses it) |
| conversations.view | 1 when a visit exists (the last-visited conversation), else 0 |
| edge channels/info | revalidation: 1 per context team (≤ 80 ids a request); + 1 batch for the counts gap when the cache lacks some |
| edge users/info | revalidation of the view's authors + DM peers (conditional; nothing back when current) |
| users.channelSections.list, stars.list | 1 each |
| conversations.info | 1 per starred conversation absent from the list (none in the test) |
| users.conversations, users.prefs.get, users.info | 0 (users.info only when edge is degraded or misses an id) |
| every list reopen | 0 |

### Verify (Phase 3) — run at `488975f6`

`-count=1` added; bare `go`/`grep` lines run from the repo root, `go` ones under `nix-shell gui/shell.nix --run`. The runtime line is the user's (below). Verbatim output:

```
$ nix-shell gui/shell.nix --run 'go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./internal/bootstrap/... ./internal/slack/... ./internal/config/... -race'
ok  	github.com/gammons/slk/internal/bootstrap	1.016s
ok  	github.com/gammons/slk/internal/bootstrap/adapt	2.004s
ok  	github.com/gammons/slk/internal/slack	1.305s
ok  	github.com/gammons/slk/internal/slack/boot	1.062s
ok  	github.com/gammons/slk/internal/slack/edge	1.139s
ok  	github.com/gammons/slk/internal/slack/membership	3.239s
ok  	github.com/gammons/slk/internal/slack/mrkdwn	1.028s
ok  	github.com/gammons/slk/internal/slack/resolve	4.715s
?   	github.com/gammons/slk/internal/slack/slacktest	[no test files]
ok  	github.com/gammons/slk/internal/config	3.526s
ok  	github.com/gammons/slk/internal/config/versionts	1.065s
exit=0
$ cd gui && nix-shell shell.nix --run 'go build ./... && go vet ./... && go test -count=1 ./backend/... -race'
ok  	github.com/gammons/slk/gui/backend	3.649s
exit=0
$ grep -rn "GetChannels(" gui/
gui/backend/slack.go:513:		if chans, err = s.client.GetChannels(ctx); err != nil {
$ grep -rn "GetUnreadCountsContext" gui/backend/
gui/backend/slack.go:516:		if unreads, agg, err := s.client.GetUnreadCountsContext(ctx); err != nil {
gui/backend/slack.go:1857:		unreads, agg, err = s.client.GetUnreadCountsContext(cctx)
$ grep -rn "GetUserProfileContext" gui/backend/
gui/backend/slack.go:1369:		u, err := s.client.GetUserProfileContext(ctx, id)
$ grep -rn 'mutes.Bootstrap(.*s.client)' gui/
gui/backend/slack.go:522:		if err := s.mutes.Bootstrap(ctx, s.client); err != nil {
$ grep -rn '"github.com/gammons/slk/internal/slack"' internal/bootstrap/*.go | wc -l
0
$ grep -rln "bootConversations\|newUserResolver\|saveWorkspaceVersionTS\|newBootstrapDeps" cmd/ gui/ internal/ | wc -l
0
$ grep -rn "debuglog\|bubbletea\|internal/ui" internal/bootstrap/adapt internal/slack/resolve internal/config/versionts | wc -l
0
$ go test -count=1 -v -run 'TestRun_NeverEnumerates|TestRun_BootCallBudget|TestBootstrapDeps_|TestBootConversations_|TestMissingStarredConversations_|TestFillFromCounts_|TestUserResolver_BatchesMissesThroughEdge|TestUserResolver_DegradedWorkspaceSkipsEdge|TestSaveWorkspaceVersionTS_|TestSlackAPI_DeclaresNoWorkspaceEnumeration' ./internal/bootstrap/... ./internal/slack/... ./internal/config/... | grep -E '^(--- |ok|FAIL|PASS)'
--- PASS: TestRun_NeverEnumerates (0.00s)
--- PASS: TestRun_BootCallBudget (0.00s)
--- PASS: TestRun_BootCallBudgetWithAChannelOpen (0.00s)
PASS
ok  	github.com/gammons/slk/internal/bootstrap	0.007s
--- PASS: TestBootstrapDeps_PopulatesEveryDependency (0.00s)
--- PASS: TestBootstrapDeps_RevalidatorUsesTheBrowserShapedHTTPClient (0.01s)
--- PASS: TestBootConversations_MapsWhatTheSidebarBuilderReads (0.00s)
--- PASS: TestBootConversations_SkipsArchivedAndClosed (0.00s)
--- PASS: TestBootConversations_SkipsClosedMPIMs (0.00s)
--- PASS: TestBootConversations_EmptyIsOpenDisablesMPIMFilter (0.00s)
--- PASS: TestBootConversations_NilSafe (0.00s)
--- PASS: TestFillFromCounts_FillsTheGapFromCacheThenEdge (0.01s)
--- PASS: TestFillFromCounts_NoGapNoRequest (0.00s)
--- PASS: TestMissingStarredConversations_BootClosedIM (0.00s)
--- PASS: TestMissingStarredConversations_ReadOnlyLookup (0.00s)
--- PASS: TestMissingStarredConversations_InvalidOrUnavailable (0.00s)
--- PASS: TestMissingStarredConversations_Cancellation (0.00s)
--- PASS: TestMissingStarredConversations_ContinuesAfterFailure (0.00s)
PASS
ok  	github.com/gammons/slk/internal/bootstrap/adapt	0.032s
--- PASS: TestSlackAPI_DeclaresNoWorkspaceEnumeration (0.00s)
PASS
ok  	github.com/gammons/slk/internal/slack	0.012s
PASS
ok  	github.com/gammons/slk/internal/slack/boot	0.006s [no tests to run]
PASS
ok  	github.com/gammons/slk/internal/slack/edge	0.008s [no tests to run]
PASS
ok  	github.com/gammons/slk/internal/slack/membership	0.004s [no tests to run]
PASS
ok  	github.com/gammons/slk/internal/slack/mrkdwn	0.007s [no tests to run]
--- PASS: TestUserResolver_BatchesMissesThroughEdge (0.23s)
--- PASS: TestUserResolver_DegradedWorkspaceSkipsEdge (0.03s)
PASS
ok  	github.com/gammons/slk/internal/slack/resolve	0.267s
PASS
ok  	github.com/gammons/slk/internal/config	0.004s [no tests to run]
--- PASS: TestSaveWorkspaceVersionTS_UpdatesExisting (0.00s)
--- PASS: TestSaveWorkspaceVersionTS_AddsToExistingSection (0.00s)
--- PASS: TestSaveWorkspaceVersionTS_DoesNotTouchOtherWorkspaces (0.00s)
--- PASS: TestSaveWorkspaceVersionTS_CreatesFile (0.00s)
--- PASS: TestSaveWorkspaceVersionTS_ConcurrentSavesDoNotLoseUpdates (0.00s)
PASS
ok  	github.com/gammons/slk/internal/config/versionts	0.011s
$ cd gui && go test -count=1 -v -run 'TestConversations_' ./backend | grep -E '^(--- |ok|FAIL|PASS)'
--- PASS: TestConversations_BootsWithoutEnumerating (0.01s)
--- PASS: TestConversations_FallsBackToEnumerationOnlyWhenBootFails (0.01s)
--- PASS: TestConversations_SecondCallMakesNoRequest (0.01s)
--- PASS: TestConversations_FillsCountsGapFromEdge (0.01s)
PASS
ok  	github.com/gammons/slk/gui/backend	0.046s
```

Additionally at `488975f6`: root `go test -count=1 ./...` → no `FAIL` line, every package `ok`; gui `go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./...` → all `ok`, gofmt empty. Capture check `git diff 76b0f18b..HEAD -M --stat -- internal/slack/testdata internal/slackhttp/testdata` → empty (no capture touched). `ls cmd/slk/*.go | wc -l` → 26 (`bootstrap_adapters*`, `starred_conversations*`, `user_resolver*`, `workspace.go`, `connect.go`, `save_version_ts*` gone).

### Runtime check for the user (not run by the executor)

```
make gui
SLK_DEBUG=1 ./bin/slk-gio 2>boot.log      # open the list, wait for it to fill, then quit
grep -A30 "boot requests" boot.log
grep "gap size" boot.log
grep "FALLING BACK" boot.log              # expect nothing
```

Expected in the `boot requests:` block (`slackhttp.DefaultCounter.Report()`, highest count first): `client.userBoot` 1, `client.counts` 1, `users.channelSections.list` 1, `stars.list` 1, `auth.test` 1, `conversations.view` ≤ 1, edge `channels/info` / `users/info` a few; absent (0): `users.conversations`, `users.prefs.get`, `users.list`, `conversations.list`; `users.info` absent unless the log also has `marking edge degraded` (then one per unnamed DM peer). `client.shouldReload` may or may not be in it yet (background). The `requests this session:` block printed on quit should still have `client.counts` 1 if no reconnect happened.

The gap line reads `slk-gio: boot: gap size N (conversations client.counts names that userBoot did not list): A from the cache, B from channels/info, C left out`. N is the subset gap (67/218 and 60/71 on the two workspaces measured). C is expected to hold DMs whose cached row has no peer and, on Enterprise Grid, conversations owned by another team. **Stop condition (hand-over contract): if C counts channels the user sees in Slack's sidebar** (compare the list with Slack's), FillFromCounts is not enough and the next step needs deciding.

### Deviations (Phase 3)

1. `MissingStarredConversations` lives in `adapt/starred.go`, not `conversations.go`, and `Muted`/`UserDisplayName`/`HydrateFirstSight` stay in `deps.go` (no `prefs.go`/`hydrate.go`): whole-file `git mv` keeps the renames (`starred.go` R091; `deps.go` R067 only because the conversation half moved out to the new `conversations.go`).
2. `FillFromCounts` leaves out gap DMs: `cache.Channel` has no peer user ID, and edge fails every IM, so a DM userBoot omits cannot be listed without a per-id `conversations.info`. Counted in the gap line's "left out".
3. `versionts.Refresh` is in `refresh.go` (same package) so `versionts.go` stays a rename (R066); with it in one file git recorded D + A.
4. `resolve.go` is recorded as D + A (R025 at `-M10%`): the plan drops more than half of `user_resolver.go` (messages, name store, avatars, bots). The test file is R074. `resolveOneContext` stores the 72px image (`Image72`→`48`→`32`), the TUI stored `Image32`; the GUI used `Image72`.
5. `TestUserResolver_FirstSight{PerUser,Edge}DeliversPeerStatus` were kept (the plan said drop `*DeliversPeerStatus`): they now assert the cache row and the reported `Profile`'s status, which the resolver still writes.
6. `Resolver.Logf` field added (the plan's `New` signature has no logger and `debuglog` is not allowed there).
7. Read state after the boot is kept current from the socket (`noteRead` on `channel_marked` and a reconnect's counts, `noteActivity` on a new message from someone else), not in the plan: without it a list reopened from the tray would show the boot's unread state (the old code re-fetched counts on every reopen). A new message marks a conversation unread until `channel_marked`; mention counts are not raised by it.
8. `bootOnce` uses a scratch `cache.New(":memory:")` when slk's cache did not open: `bootstrap.Store` and `HydrateFirstSight` need one.
9. Risk 3 (`RevalidatedUsers`) went into the GUI commit (d), since that is where it is used.
10. `afterConnect(first)` waits for the boot via a `bootDone` channel (≤ `listWait`, 1 min) rather than checking `s.counts` once: the first connect usually happens while the boot is still running.

### Follow-ups (Phase 3)

- `Result.Messages` is not ingested (plan); the opened conversation costs one `conversations.history` when its window opens.
- Gap DMs (deviation 2): if the runtime gap line shows DMs left out, list them via `conversations.info` (one per id) or carry the peer ID in the cache's channel row.
- `Profile()` still asks `users.info` per hover card (plan: needs Title/TZ/Image192).
- `(*slackclient.Client).GetUnreadCounts` (the context-free form) has no non-test caller left; delete it with Phase 5's orphan sweep.

## Phase 4 — Feature ports into slk-gio (4.1, 4.2)

### 4.1 — `gui: find and join channels you are not in`

- Commit: `c43a06d8`
- Backend: `ChannelResult{ID, Name, Kind, Member}`, `SearchChannels`, `Join` (after `Conversation`). `gui/backend/channels.go`: no edge → "channel search is not available"; empty query → nil, no request; `top_channels` = first 25 of `s.convOrder` (`maxTopChannels`, comment carried from `cmd/slk/channel_search.go`); archived/IM/MPIM skipped; `Member` = `s.kinds` has the ID; `member_channels` ignored. `Join` = `client.JoinChannel` + `s.Conversation` (which queues the `ConversationOpened`).
- Fake (`fake_channels.go`): `fakeUnjoined` C20 platform-sre, C21 design-crit, C22 cat-pictures in `f.unjoined`; `SearchChannels` fold-contains over joined channels (Member) then unjoined (by ID); `Join` moves to `convs`/`order`, seeds two lines (read), emits `ConversationOpened`; member → no event; unknown → `channel_not_found`.
- `gui/chat/remote.go`: `RemoteDelay` (300 ms), `RemoteSearch` (`Type`, `Due`, `Done`, `Results`, `Searching`, `Wake`), `Unjoined`, `ExactChannel`.
- GUI: switcher "Channels you're not in" rows (`joinLine`), `update` returns a `switcherPick`; list-window `itemJoin` rows under a heading, Enter with no local match joins the first remote, a note line under the filter for a failed search/join; `hub.joinAndOpen`, `hub.searchChannels` (`channelsFound`); compose `/open`/`/join` → `openChannel`: `<#C…|name>` by ID (listed → open; else join by ID), listed by name, else one `SearchChannels` + `ExactChannel` + join.
- `cmd/slk/channel_search{,_test}.go` deleted: no non-test caller (Phase 2 removed the TUI composition root); the rules and a test moved to `gui/backend/channels_test.go` (`TestSearchChannels_MapsResultsAndHintsFromTheList`, `TestSearchChannels_WithoutEdge`, slacktest).
- README: paragraph after the switcher paragraph; slash-command paragraph mentions `/join`; "Starting conversations" shrank to `/msg` + `/leave`. AGENTS.md: one Gio row.

### 4.2 — `gui: start a DM or group DM`

- Commit: `b5cc11bb`
- Backend: `OpenConversation` (after `Join`); `gui/backend/dm.go`: `MaxRecipients = 8`, `recipients` (drops self, repeats, empty; 0 → "choose someone to message"; >8 → "at most 8 people in a group message"), `client.OpenConversation` + `s.Conversation`. Test `TestOpenConversation_LeavesOutSelfAndRepeats` (slacktest: `users=U2,U3`, one request, refusals make none).
- Fake (`fake_dm.go`): one ID → DM by `PeerID`, no event; several → group by sorted member key (D2 = U2,U4 seeded); else `D<seq>` (from 101) named from `fakeNames`/`fakeMembers`, group names joined ", ", empty log, `ConversationOpened`.
- `gui/chat/newmsg.go`: `MaxRecipients` (= backend's), `Recipients` (`Toggle(Person)`, `Has`, `Pop`, `Full`, `IDs`, `Counter`), `PeopleChoices`.
- GUI: `newMessagePicker` (`gui/cmd/slk-gio/newmsg.go`; editor "To: a name", pills via `wrapRow`, `popupList`/`choiceRow`, counter caption; Up/Down, Space/Tab toggle, Backspace on empty pops, Enter opens, Escape/outside click via `dismissed`); hosted by `chatWindow` (switcher's "New message…" row, shown when the filter is empty) and `listWindow` (header action `iconCompose`). `hub.openDM` (a DM in `h.convs` opens with no request; else `OpenConversation`; text via `openChatWith`, which fills only an empty composer). Compose `/msg`/`/dm`: listed DM/group DM by name, else `findPerson(people, mentions.Wire(target))` → `openDM`; nobody → "No one called @name". `TestPopupTagsAreDistinct` covers the picker.
- README paragraph after 4.1's; "Starting conversations" bullet became "**Leaving conversations.** `/leave` is not there." AGENTS.md: one Gio row.

### Verify (4.1, 4.2) — run at `b5cc11bb`, gui module (`cd gui && nix-shell shell.nix --run ...`), `-count=1 -v` added

```
$ go test -count=1 -v ./backend -run 'TestFakeSearchChannels|TestFakeJoin|TestSearchChannels_' | grep -E "^(--- |ok|FAIL)"
--- PASS: TestSearchChannels_MapsResultsAndHintsFromTheList (0.01s)
--- PASS: TestSearchChannels_WithoutEdge (0.01s)
--- PASS: TestFakeSearchChannelsFindsUnjoined (0.00s)
--- PASS: TestFakeJoinOpensTheChannel (0.00s)
ok  	github.com/gammons/slk/gui/backend	0.031s
$ go test -count=1 -v ./chat -run 'TestRemoteSearch|TestUnjoined|TestExactChannel' | grep -E "^(--- |ok|FAIL)"
--- PASS: TestRemoteSearchFiresOncePerPause (0.00s)
--- PASS: TestRemoteSearchIgnoresStaleAnswers (0.00s)
--- PASS: TestUnjoinedDropsMembersAndListed (0.00s)
--- PASS: TestExactChannel (0.00s)
ok  	github.com/gammons/slk/gui/chat	0.003s
$ go test -count=1 -v ./backend -run 'TestFakeOpenConversation|TestOpenConversation_' | grep -E "^(--- |ok|FAIL)"
--- PASS: TestOpenConversation_LeavesOutSelfAndRepeats (0.01s)
--- PASS: TestFakeOpenConversationReusesTheDM (0.00s)
--- PASS: TestFakeOpenConversationMakesAGroup (0.00s)
ok  	github.com/gammons/slk/gui/backend	0.022s
$ go test -count=1 -v ./chat -run 'TestRecipients|TestPeopleChoices' | grep -E "^(--- |ok|FAIL)"
--- PASS: TestRecipientsCapAtEight (0.00s)
--- PASS: TestRecipientsPop (0.00s)
--- PASS: TestPeopleChoicesLeaveOutSpecialsAndBots (0.00s)
ok  	github.com/gammons/slk/gui/chat	0.003s
$ go test -count=1 -v ./cmd/slk-gio -run 'TestFindPerson|TestFindConversation|TestPopupTagsAreDistinct|TestSwitcherMatches' | grep -E "^(--- |ok|FAIL)"
--- PASS: TestFindConversation (0.00s)
--- PASS: TestSwitcherMatches (0.00s)
--- PASS: TestFindPerson (0.00s)
--- PASS: TestPopupTagsAreDistinct (0.00s)
ok  	github.com/gammons/slk/gui/cmd/slk-gio	0.007s
$ go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./... -race
ok  	github.com/gammons/slk/gui/backend	4.200s
ok  	github.com/gammons/slk/gui/chat	1.019s
ok  	github.com/gammons/slk/gui/cmd/slk-gio	9.716s
ok  	github.com/gammons/slk/gui/motion	1.010s
ok  	github.com/gammons/slk/gui/notifications	1.032s
ok  	github.com/gammons/slk/gui/palette	1.011s
ok  	github.com/gammons/slk/gui/rich	1.018s
ok  	github.com/gammons/slk/gui/search	1.018s
ok  	github.com/gammons/slk/gui/selection	1.015s
ok  	github.com/gammons/slk/gui/sidebar	1.015s
ok  	github.com/gammons/slk/gui/textedit	1.010s
exit=0
```

Root module at `b5cc11bb`: `go build ./... && go vet ./... && gofmt -l .` clean; `go test -count=1 ./...` every package `ok` (`cmd/slk` included after the `channel_search` deletion).

### Manual (`-fake`), performed

Built to the scratchpad (`go build -o <scratch>/bin/slk-gio`, not `make gui`: the user's `slk-gio` was running from `bin/slk-gio`), launched as `./bin/slk-gio -fake -fake-every 0` with `WAYLAND_DISPLAY=` so it ran under XWayland, driven with `xdotool` key/type, frames captured with `ffmpeg -f x11grab -window_id`; stopped with `pkill -f "^\./bin/slk-gio -fake"`. The user's `slk-gio` (pid 450995) was untouched.

- 4.1: list filter `sre` → after the pause "CHANNELS YOU'RE NOT IN: # platform-sre  Join"; Enter → window "# platform-sre". In that window Ctrl+K, `crit` → "Channels you're not in: # design-crit". `/open #nope` → "No conversation named nope" with the text put back. `/join #cat-pictures` → window "# cat-pictures".
- 4.2: Ctrl+K shows "New message…" first (the first conversation lit); Up, Enter → picker; `gra` Space `mar` Space → pills Grace Hopper, Margaret Hamilton, "2 / 8"; Enter → window "● Grace Hopper, Margaret Hamilton". `/msg @Ada hello` → Ada's DM with "hello" in the composer; with "my draft" typed there, `/msg @Ada other` → the window comes back with "my draft". `/msg @Linus hey` → new DM window "Linus Torvalds". `/msg @nobody hey` → "No one called @nobody". Not exercised by hand: clicking the list header's pencil (pointer clicks would move the user's mouse); the same picker is exercised from the switcher.

### Deviations (4.1, 4.2)

1. `switcher.update` returns a `switcherPick` struct (conv / join / all / newMsg / closed) instead of a growing tuple with `joinPick`; it takes a `search func(q)` (`hub.searchChannels`).
2. `chat.RemoteSearch` gained `Searching()` (the list shows "Searching channels…" instead of "No conversation matches." while an answer is due) and keeps answers by query (≤ 32), so going back to an answered query makes no request; a failed one (nil) is not kept and is asked again only when typed again. Stale answers are kept under their own query, never shown for another.
3. `chat.Recipients.Toggle` takes a `backend.Person` (the pills need names), not an ID; `Has` and `IDs` added. `MaxRecipients` lives in `backend` (`dm.go`) and `chat.MaxRecipients` aliases it, so the backend's refusal and the picker's cap are one number.
4. `/msg` target parsing is `findPerson` in `cmd/slk-gio/commands.go` (with `namePrefix`, factored out of `findConversation`, unchanged behaviour): a `<@U…>` mention, else a whole name at the start ("@Grace Hopper see you"), else `chat.BestPerson` on the first word. The plan's "`MentionedUserIDs(first)` or `BestPerson`" alone would cut "@Grace Hopper hi" after "Grace". The rest is in wire form when it contains a completed mention (as the TUI's `/msg`).
5. `hub.openDM` opens a DM the list already has without `conversations.open` + `conversations.info` (one person only; a group DM's members are not in `Conversation`).
6. The switcher lights the first conversation, not "New message…", when it opens or the filter is emptied, so Ctrl+K Enter still opens the first unread conversation.
7. `/open` of a `<#C…|name>` mention the list does not have joins by ID directly, without a search.
8. `joinAndOpen`'s failure note does not give the typed `/open` text back (as planned: `sendFailed{note}` only); `/open` failures before the join (no match, search error) do.
9. Picker Space is taken from the editor's text (a space typed is removed and toggles), not a key filter: Gio delivers a typed space as an EditEvent, and names are matched by their start, so the filter never needs one.
10. Added Slack-side tests not in the plan: `TestSearchChannels_*` (the rules from the deleted `cmd/slk/channel_search_test.go`), `TestOpenConversation_LeavesOutSelfAndRepeats`, `TestFindPerson`.

### Follow-ups (4.1, 4.2)

- `internal/core.ChannelService.SearchRemote` and the AGENTS.md row "Resolve a command's channel argument" describe the TUI path; both go with Phase 5.
- The picker lists only people slk has seen (`Backend.People`, no request); edgeapi `users/search` (`edge.UsersSearch`) could find the rest with the same `RemoteSearch` debounce.
- `/msg`'s text dropped when the DM already has a draft is not given back to the composer it was typed in (the TUI's `offerTypedBack`).

## Phase 4.3 — File upload / paste

### Commit 1 — `clipboard, attach, filepick: desktop clipboard, attachments and a file chooser without a toolkit`

- Commit: `aab7c562`
- `internal/clipboard`: `ReadImage() ([]byte, bool)`, `ReadText() string`. Wayland (`WAYLAND_DISPLAY` set, `wl-paste` installed): `wl-paste --no-newline --type image/png` / `--type text`; X11 (`DISPLAY`, `xclip`): `xclip -selection clipboard [-t image/png] -o`; neither → nothing. 3 s timeout per read; an image must start with the PNG signature. Environment (`getenv`, `lookPath`, `run`) swappable for tests.
- `internal/attach`: `MaxSize` (10 MB), `ResolvePath` (the TUI's `resolveFilePath`, plus `file://` URLs), `HumanSize`, `FromPath` (`ErrNotAFile` = treat as text, `ErrEmpty`), `FromImage(png, now)` → `slk-paste-2006-01-02-15-04-05.png`.
- `internal/filepick`: `Choose(ctx)` — zenity `--file-selection --multiple --separator=\n`, else kdialog `--getopenfilename --multiple --separate-output`, else yad `--file --multiple`; exit 1 (and yad's 252) = cancelled = no files, no error; `ErrNoChooser` "no file chooser found (install zenity)".
- `cmd/slk/clipboard_wayland.go` deleted (no caller in the CLI).
- AGENTS.md: three rows (clipboard, attach, filepick) and the "Copy text to the clipboard" row points at the reader.

### Commit 2 — `gui: attach and upload files from the clipboard, a path or a file chooser`

- Commit: `528fe2ec`
- Backend: `Upload(ctx, channelID, threadTS, files []core.PendingAttachment, caption, progress func(done, total int)) error` after `SendReply`. `gui/backend/upload.go`: files one at a time, `progress(i+1, n)` as each starts, 60 s per file, bytes or `os.Open(Path)` (size re-read from the open file), caption on the last only, error `uploading %s (%d/%d): %w`, then `go s.fetchThread` / `go s.catchUp` (only with a cache). Not optimistic. Test `TestUpload_SendsEachFileWithTheCaptionOnTheLast` (slacktest: `files.getUploadURLExternal` ×2, the PUT, `files.completeUploadExternal` ×2, `initial_comment` only on the second; a missing path names the file and its place).
- Fake (`fake_upload.go`): one "You" message (reply in a thread) with the caption's mrkdwn + `rich.Image` (`fake://<name>`, 640×400) for `image/*` or `rich.File`, progress per file, `MessagePosted`. `TestFakeUploadPostsFilesWithCaption`.
- `gui/chat/attach.go`: `Attachments` (`Add` with "Image|File too large (12.3 MB > 10 MB limit)" and `-2`/`-3` names, `Remove`, `Empty`, `Total`), `UploadNote`.
- GUI (`gui/cmd/slk-gio/attach.go`, hooks in `compose.go`): Ctrl+V key filter before readline and the editor → off the UI goroutine `clipboard.ReadImage` → `attach.FromImage`, else `attach.ResolvePath(clipboard.ReadText())` → `FromPath`, else `pasteText` → next frame `clipboard.ReadCmd{Tag: &composer}` so the editor pastes the text itself. Paperclip `iconButton` (`iconAttach`, `icons.EditorAttachFile`) → `filepick.Choose`. Chips (name, size, ×) above the editor via `wrapRow`; while uploading the editor is read-only, chips muted without ×, hint "Uploading…", the note line shows `UploadNote`, Enter does nothing. Enter with files → `submitFiles` (caption = trimmed text in wire form; a slash command is not run). Success clears files + text; failure keeps both, "Not sent: …". `chatState`/`threadState` carry `uploadState` (files, uploading, note); the composer's `deliver` is `hub.toChat` / new `hub.toThread`, so progress and the end of an upload in flight reach the replacement window.
- README: paragraph after the composer-completion section; "Sending files" bullet → "**Dropping files** onto a window does nothing: Gio has no drag-and-drop from the desktop." AGENTS.md: one Gio row.

### Verify (4.3) — run at `528fe2ec` (`-count=1 -v` added)

```
$ cd gui && go test -count=1 -v ./backend -run 'TestFakeUpload|TestUpload_' | grep -E "^(--- |ok|FAIL)"
--- PASS: TestFakeUploadPostsFilesWithCaption (0.00s)
--- PASS: TestUpload_SendsEachFileWithTheCaptionOnTheLast (0.02s)
ok  	github.com/gammons/slk/gui/backend	0.021s
$ go test -count=1 -v ./chat -run 'TestAttachments|TestUploadNote' | grep -E "^(--- |ok|FAIL)"
--- PASS: TestAttachmentsRefuseLargeFiles (0.00s)
--- PASS: TestAttachmentsRenameDuplicates (0.00s)
--- PASS: TestUploadNote (0.00s)
ok  	github.com/gammons/slk/gui/chat	0.005s
$ go test -count=1 -v ./cmd/slk-gio -run 'TestComposer' | grep -E "^(--- |ok|FAIL)"
--- PASS: TestComposerUploadsAttachedFilesWithTheCaption (0.00s)
--- PASS: TestComposerKeepsFilesAndCaptionWhenTheUploadFails (0.00s)
--- PASS: TestComposerRefusesLargeFilesAndCarriesFilesOnMove (0.00s)
ok  	github.com/gammons/slk/gui/cmd/slk-gio	0.008s
$ go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./... -race
ok  	github.com/gammons/slk/gui/backend	4.474s
ok  	github.com/gammons/slk/gui/chat	1.023s
ok  	github.com/gammons/slk/gui/cmd/slk-gio	9.704s
ok  	github.com/gammons/slk/gui/motion	1.011s
ok  	github.com/gammons/slk/gui/notifications	1.029s
ok  	github.com/gammons/slk/gui/palette	1.012s
ok  	github.com/gammons/slk/gui/rich	1.017s
ok  	github.com/gammons/slk/gui/search	1.017s
ok  	github.com/gammons/slk/gui/selection	1.014s
ok  	github.com/gammons/slk/gui/sidebar	1.014s
ok  	github.com/gammons/slk/gui/textedit	1.013s
exit=0
$ cd .. && go test -count=1 -v ./internal/attach ./internal/clipboard ./internal/filepick | grep -E "^(--- |ok|FAIL)"
--- PASS: TestResolvePath (0.00s)
--- PASS: TestHumanSize (0.00s)
--- PASS: TestFromPath (0.00s)
--- PASS: TestFromImage (0.00s)
ok  	github.com/gammons/slk/internal/attach	0.003s
--- PASS: TestReadImageOnWayland (0.00s)
--- PASS: TestReadImageOnX11 (0.00s)
--- PASS: TestReadImageWithTextOnly (0.00s)
--- PASS: TestReadImageRefusesWhatIsNotAPNG (0.00s)
--- PASS: TestReadWithoutATool (0.00s)
--- PASS: TestReadText (0.00s)
ok  	github.com/gammons/slk/internal/clipboard	0.001s
--- PASS: TestChooseReadsOnePathPerLine (0.00s)
--- PASS: TestChooseFallsBackToKdialogThenYad (0.00s)
--- PASS: TestChooseCancelledIsNothing (0.00s)
--- PASS: TestChooseFailure (0.00s)
--- PASS: TestChooseWithoutAChooser (0.00s)
ok  	github.com/gammons/slk/internal/filepick	0.002s
```

Root module at `528fe2ec`: `go build ./... && go vet ./... && gofmt -l .` clean; `go test -count=1 ./...` every package `ok`. At `aab7c562` both modules were also build/vet/gofmt-clean and `cd gui && go test ./... -race` all `ok`.

### Manual (`-fake`), performed

Run in a private X server so neither the user's clipboard nor screen was touched: `Xvfb :97` (from `nix-shell -p xvfb`), the binary built to the scratchpad, launched as `<scratch>/slk-gio -fake -fake-every 0` with `WAYLAND_DISPLAY` unset, `DISPLAY=:97`, zenity (`nix-shell -p zenity`) on `PATH`, and for the chooser `GDK_DEBUG=no-portals` plus a dead `DBUS_SESSION_BUS_ADDRESS` (see deviation 6). Driven with `xdotool` (key, type, and clicks — on `:97` they do not move the user's pointer), frames with `ffmpeg -f x11grab -i :97`, clipboard set with `xclip -display :97`. Stopped with `pkill -f "^<scratch>/slk-gio -fake"`; the user's `slk-gio` (pid 450995) was untouched.

- Image paste: `xclip -selection clipboard -t image/png -i shot.png`, Ctrl+V in #general → chip "slk-paste-2026-10-31-12-31-57.png 48 KB ×", hint "Add a message, or press Enter to send the files".
- Path paste: clipboard = `<scratch>/some.pdf` (300 000 bytes), Ctrl+V → second chip "some.pdf 292 KB" (before the HumanSize change, deviation 4). Plain text on the clipboard, Ctrl+V → "hello from the clipboard" in the composer, no chip.
- Send: Ctrl+A, `photos`, Enter → "You: photos" with the picture and a "some.pdf 293 KB application/pdf" chip; composer emptied. The fake uploads instantly, so "Uploading 1/2…" was not on any captured frame at 30 fps; it is covered by `TestComposerUploadsAttachedFilesWithTheCaption` (notes "Uploading 1/2…", "Uploading 2/2…").
- Chooser: paperclip → zenity "Attach files"; a typed name GTK autocompleted to `some.pdfpdf` → note "Not a file: …/some.pdfpdf"; picking a.png and ac.png (Ctrl+click) → two chips.
- `moveHere`: clicking #general in the list again replaces the window; the new window shows both chips. × on ac.png removes it; Enter with no text → "You" message with the picture only.
- Thread: opened a thread in #general, Ctrl+V an image, typed "thread pic", Enter → reply "thread pic" with the picture, "2 replies".
- Limit: path of a 12 900 000-byte file, Ctrl+V → "File too large (12.3 MB > 10 MB limit)", no chip.

### Deviations (4.3)

1. Ctrl+V is taken from the editor (a Gio key filter consumes the event; the plan assumed the editor still pastes the same press). When the clipboard holds neither an image nor an existing file's path, the composer asks Gio for the text (`clipboard.ReadCmd{Tag: &composer}`) next frame, so the editor pastes it as before. A path to a file is attached and not also pasted (the TUI's rule).
2. The clipboard and path checks run off the UI goroutine (each is a process); results come back as `attachPicked` / `pasteText` items.
3. `attach.ResolvePath` also accepts `file://` URLs (a file manager's copy), %-unescaped. `FromPath` leaves the size limit to `chat.Attachments.Add`, so one message ("Image|File too large (…)") covers both sources.
4. `attach.HumanSize` (commit 2) took the GUI's existing `humanSize` rules ("812 B", rounded KB, "" for unknown) and `richview.go`'s copy was deleted; the TUI's version said "<1 KB" and truncated KB, which disagreed with the message file chips by 1 KB.
5. AGENTS.md rows for the three `internal/` packages went into commit 1 (AGENTS.md: a helper's row goes in its own commit); the Gio row in commit 2.
6. A first chooser attempt ran zenity with the session bus reachable; GTK4's file dialog then goes through xdg-desktop-portal, which draws on the user's real desktop, not on `:97`. Three such dialogs (two probes, one from slk-gio) may have appeared on the user's screen for a few seconds each before they were killed. Rerun with `GDK_DEBUG=no-portals` and no session bus kept it on `:97`. In real use the portal dialog is what is wanted.
7. Extra tests not in the plan: `TestUpload_SendsEachFileWithTheCaptionOnTheLast` (Slack side, slacktest), `TestComposer*` (headless composer: upload flow, failure keeps files, size refusal, `uploadState` across `moveHere`), the `internal/clipboard` tests (`ReadImage`/`ReadText` behind the injectable runner, as asked for the image step).

### Follow-ups (4.3)

- A failure part-way through a multi-file upload keeps every file; a retry re-sends the ones that went. `Upload` could report how many were posted so the composer drops those chips.
- macOS has no clipboard image reader (`pngpaste` would be the tool); `internal/clipboard` finds nothing there.
- `wiki/Installation.md`'s clipboard section still describes the TUI (Phase 6).

## Phase 4.4 — Mark unread; Phase 4.5 — Own presence / DND

### 4.4 — `gui: mark a conversation or thread unread from a message`

- Commit: `5cb7d95b`
- Backend: `MarkUnread(ctx, channelID, threadTS, boundaryTS)` after `MarkThreadRead`. `gui/backend/read.go`: channel → `client.MarkChannelUnread` then queue `rawMarked{unread: 1, mentions: n}`, `n` = cached rows after the boundary (`GetMessagesSince`, the boundary row itself and the user's own skipped) that are in a DM/group DM or match `mention.InText` (the TUI's `countMentionsSince` rule); thread → `client.MarkThreadUnread` then `rawThreadUnread` → cache `UpdateThreadLastReadIfExists(boundary)`, `ThreadState{Unread: true}`, `ThreadsChanged`, `recountThreads`. Not optimistic (as `MarkThreadRead`).
- `threads.go threadMarked`: a `thread_marked` whose read mark is before a cached reply that is not the user's own now emits `ThreadState{Unread: true}` (was always false), via `repliesAfter`. Slack echoes a mark unread as `thread_marked`; without this the echo would clear what was just marked. See deviation 2.
- Fake (`fake_read.go`): channel → `Unread`, `Mentions` (non-self, not thread-only, after the boundary; every message in a DM/group DM, else `mention.InText`), `ReadChanged{Unread, Mentions, LastRead}`; unknown → `channel_not_found`; thread (C1 only) → `followed[ts] = true`, `ThreadState{Unread}`, `ThreadsChanged`, `threadsUnreadLocked()`.
- `gui/chat/readgate.go`: `ReadGate` (`Hold(newest)`, `Blur()`, `Arrived(ts)`, `Open()`; zero value open), `UnreadBoundary`, `ThreadUnreadBoundary`.
- Windows: `msgView.markUnread func(m) func()` (nil action → no item); `menuItems` adds "Mark unread from here" after "Open in Slack" for non-ephemerals (`msgMenu.items` now `[8]`). `chatwin.go`: `gate`, carried in `chatState`; `markRead` returns while `!gate.Open()`; `focusChanged(on)` (was inline in `run`) blurs the gate on focus loss; a `MessagePosted` for this channel, not ephemeral, not thread-only → `Arrived` before `markRead`; `markUnreadFrom(b)` → `Hold(newestKept)`, `marked = b`, `go be.MarkUnread`; a refusal comes back as `unreadFailed` → `Blur`, `marked` restored, note "Could not mark unread: …". `threadwin.go` the same with `ThreadUnreadBoundary` (the parent offers no item). `listwin.go` unchanged.
- Tests: `TestReadGateReleasesOnBlur`, `TestReadGateReleasesOnNewerMessage`, `TestUnreadBoundary`, `TestThreadUnreadBoundary`, `TestFakeMarkUnreadChannel`, `TestFakeMarkUnreadThread`; added: `TestMarkUnread_CountsMentionsAfterTheBoundary` and `TestThreadMarkedEchoOfAnUnreadStaysUnread` (slacktest + in-memory cache), and headless window tests `TestChatWindowKeepsAMarkUnreadUntilBlurOrNewMessage` / `TestThreadWindowKeepsAMarkUnreadUntilBlurOrNewReply` (`cmd/slk-gio/readgate_test.go`: a real hub over the fake with `MarkRead`/`MarkThreadRead`/`MarkUnread` recorded, windows built with `newChatWindow`/`newThreadWindow` and never run). With the gate check removed from `chatWindow.markRead` the chat test fails ("marked \"read C1 …\", want \"\"").
- README: a sentence in the actions-menu paragraph. AGENTS.md: one Gio row.

### 4.5 — `gui: set your own presence and do-not-disturb`

- Commit: `63f42e2a`
- Backend: `SelfStatus{Away, DND, DNDUntil}`, event `SelfPresenceChanged{Status}` (with `isEvent`), `SelfStatus(ctx)`, `SetPresence(ctx, away)`, `Snooze(ctx, minutes)`, `EndSnooze(ctx)`, `EndDND(ctx)` after `Presence`.
- `gui/backend/presence.go`: `Slack.self` under `mu`; `SelfStatus` = `users.getPresence` (Away = `manual_away` or `presence == "away"`) + `dnd.info` → `DNDStateFromStatus`. The `OnSelfPresenceChange` / `OnDNDChange` stubs in `slack.go` are replaced by handlers queuing `rawSelfPresence` / `rawSelfDND`, applied in `Run` (`applySelf`) → `SelfPresenceChanged`. Setters go through `setSelf`: queue the new status, call with `reactTimeout`, on error queue the old one and log `slk-gio: presence:`. `SetPresence` → `users.setPresence` `auto`/`away`; `Snooze` → `dnd.setSnooze`; `EndSnooze` → `dnd.endSnooze`; `EndDND` → `dnd.endSnooze` then `dnd.endDnd`, failing only when both fail (deviation 5).
- Fake (`fake_presence.go`): `self`; setters apply and emit; `Run` emits `SelfPresenceChanged` right after `ConnectionChanged` (`nextPosted` in `fake_test.go` skips it). `TestFakePresenceAndSnooze`. Slack side: `TestSelfStatus_ReadsPresenceAndDND`, `TestSetPresence_TakesBackWhatSlackRefuses` (slacktest).
- `gui/chat/presence.go`: `PresenceItem{Label, Away, Snooze, End, Minutes, Current}`, `PresenceItems(st, now)` (Active, Away, six snoozes, "Snooze until tomorrow morning", "End do not disturb" only while DND; Active/Away marked current only outside DND), `MinutesUntilTomorrowMorning`, `SelfText` ("Away", "Do not disturb until 15:20", "Away · do not disturb until …", "" when simply active; built on `DNDText`). Tests `TestPresenceItemsMarkTheCurrentOne`, `TestPresenceItemsOfferEndOnlyWhileDND`, `TestMinutesUntilTomorrowMorning`, `TestSelfText`.
- Hub: `self` + `selfStatus()`, `loadSelf()` (one `be.SelfStatus` after `showList()`, from `main.go`), `SelfPresenceChanged` in `dispatch` (to the list; `onSelf` called after the lock is released, in order), `setPresence(it, from)` (End → `EndDND`, Snooze → `Snooze`, else `SetPresence(it.Away)`; error → `sendFailed{note: "Could not change presence: …"}` to `from`, or the list when `from` is nil).
- List: `headerSpec(gtx)` tile wrapped in `withSelf` (`withPresence` dot, or an `iconDND` badge during DND), `headerSpec.OnLeading` (+ `headerBar.leading`, laid over the tile so the dot is not clipped) opens `presenceMenu` (`gui/cmd/slk-gio/presencemenu.go`: `popupScrim`/`popupPanel`/`popupFrame`, rules between groups, `dismissed`); the subtitle is the unread summary + " · " + `SelfText` when that is not empty. `menuRow` lifted to `menuRowAt(gtx, th, cl, label, danger, current)` (a ✓ after the current one). `TestPopupTagsAreDistinct` covers `presenceMenu`.
- Tray (`tray_linux.go`): Active / Away (checkboxes), Snooze for 1 hour, End do not disturb (shown only during DND), from `trayPresenceItems` (`tray_items.go`, untagged); the tooltip from `trayTooltip` follows `h.onSelf`. `TestTrayPresenceItems` asserts the items, checks and tooltip.
- Compose: `/away` (toggle), `/active`, `/status away|active` → `hub.setPresence`; any other `/status` → "/status isn't supported in slk-gio yet", text kept. `TestComposerPresenceCommands`.
- README: paragraph after the tray paragraph, the tray paragraph mentions presence. AGENTS.md: one Gio row.

### Verify (4.4, 4.5) — run at `63f42e2a`, gui module (`cd gui && nix-shell shell.nix --run ...`), `-count=1 -v` added

```
$ go test -count=1 -v ./backend -run "TestFakeMarkUnread|TestMarkUnread_|TestThreadMarkedEcho"
--- PASS: TestFakeMarkUnreadChannel (0.00s)
--- PASS: TestFakeMarkUnreadThread (0.00s)
--- PASS: TestMarkUnread_CountsMentionsAfterTheBoundary (0.01s)
--- PASS: TestThreadMarkedEchoOfAnUnreadStaysUnread (0.01s)
ok  	github.com/gammons/slk/gui/backend	0.028s
$ go test -count=1 -v ./chat -run "TestReadGate|TestUnreadBoundary|TestThreadUnreadBoundary"
--- PASS: TestReadGateReleasesOnBlur (0.00s)
--- PASS: TestReadGateReleasesOnNewerMessage (0.00s)
--- PASS: TestUnreadBoundary (0.00s)
--- PASS: TestThreadUnreadBoundary (0.00s)
ok  	github.com/gammons/slk/gui/chat	0.004s
$ go test -count=1 -v ./cmd/slk-gio -run KeepsAMarkUnread
--- PASS: TestChatWindowKeepsAMarkUnreadUntilBlurOrNewMessage (1.43s)
--- PASS: TestThreadWindowKeepsAMarkUnreadUntilBlurOrNewReply (0.21s)
ok  	github.com/gammons/slk/gui/cmd/slk-gio	1.742s
$ go test -count=1 -v ./backend -run "TestFakePresence|TestSelfStatus_|TestSetPresence_"
--- PASS: TestFakePresenceAndSnooze (0.00s)
--- PASS: TestSelfStatus_ReadsPresenceAndDND (0.01s)
--- PASS: TestSetPresence_TakesBackWhatSlackRefuses (0.01s)
ok  	github.com/gammons/slk/gui/backend	0.030s
$ go test -count=1 -v ./chat -run "TestPresence|TestMinutesUntil|TestSelfText"
--- PASS: TestPresenceItemsMarkTheCurrentOne (0.00s)
--- PASS: TestPresenceItemsOfferEndOnlyWhileDND (0.00s)
--- PASS: TestMinutesUntilTomorrowMorning (0.00s)
--- PASS: TestSelfText (0.00s)
ok  	github.com/gammons/slk/gui/chat	0.004s
$ go test -count=1 -v ./cmd/slk-gio -run "TestComposerPresence|TestTrayPresence|TestPopupTags"
--- PASS: TestPopupTagsAreDistinct (0.00s)
--- PASS: TestComposerPresenceCommands (0.02s)
--- PASS: TestTrayPresenceItems (0.00s)
ok  	github.com/gammons/slk/gui/cmd/slk-gio	0.022s
$ go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./... -race
ok  	github.com/gammons/slk/gui/backend	4.992s
ok  	github.com/gammons/slk/gui/chat	1.018s
ok  	github.com/gammons/slk/gui/cmd/slk-gio	10.345s
ok  	github.com/gammons/slk/gui/motion	1.010s
ok  	github.com/gammons/slk/gui/notifications	1.027s
ok  	github.com/gammons/slk/gui/palette	1.010s
ok  	github.com/gammons/slk/gui/rich	1.020s
ok  	github.com/gammons/slk/gui/search	1.021s
ok  	github.com/gammons/slk/gui/selection	1.011s
ok  	github.com/gammons/slk/gui/sidebar	1.019s
ok  	github.com/gammons/slk/gui/textedit	1.012s
exit=0
```

At `5cb7d95b` the gui module was also build/vet/gofmt-clean and `go test -count=1 ./... -race` all `ok`. Root module at `63f42e2a` (and `5cb7d95b`): `go build ./... && go vet ./... && gofmt -l .` clean; `go test -count=1 ./...` every package `ok`.

### Manual (`-fake`), performed

Built to the scratchpad (`go build -o <scratch>/slk-gio ./cmd/slk-gio`, at the 4.5 tree, which contains 4.4 unchanged), run in a private X server: `Xvfb :97` (`nix-shell -p xvfb`), `WAYLAND_DISPLAY` unset, `DISPLAY=:97`, `DBUS_SESSION_BUS_ADDRESS` pointing at a socket that does not exist (no tray registration, no portal, no notifications on the user's desktop), `XDG_CONFIG_HOME`/`XDG_DATA_HOME`/`XDG_CACHE_HOME` in the scratchpad with `[gui] tray = false` and `[notifications] enabled = false`; `<scratch>/slk-gio -fake -fake-every 0`. Driven with `xdotool` (key, type, clicks, `windowfocus` to move the keyboard between windows, `windowmove`/`windowsize`), frames with `ffmpeg -f x11grab -i :97`. Stopped with `pkill -f "^<scratch>/slk-gio -fake"` and `pkill -f "Xvfb :97"`; the user's `slk-gio` (pid 450995) was untouched.

- 4.5: list tile shows a green dot. Tile → menu (Active ✓, Away, rule, six snoozes, until tomorrow morning) → Away → grey ring, subtitle "2 unread conversations · Away". Tile → Snooze for 20 minutes → paused-bell badge, subtitle (window widened to 640 px) "… · Away · do not disturb until 13:15". In #general `/active` → subtitle "… · Do not disturb until 13:15", composer emptied. Tile → menu now ends with a rule and "End do not disturb" → clicked → green dot, plain subtitle. `/away` → grey ring "· Away"; `/away` again → green. `/status lunch` → "/status isn't supported in slk-gio yet", text left in the composer.
- 4.4: #general focused, right-click the third-newest (Margaret's "Following up in #eng-backend") → menu has "Mark unread from here" → clicked → the list shows "general" under UNREAD ("3 unread conversations") and the focused window does not clear it (2 s later still unread). Focus the list (`windowfocus`), then #general again → general leaves UNREAD. Marked unread again, then typed "hello" + Enter in #general → cleared by the new message. Thread window on Margaret's broadcast: the parent's "⋯" menu has no "Mark unread from here", the reply's right-click menu has it → Threads count 1 → 2; focus away and back → 1. Group DM "Grace Hopper, Linus Torvalds": Mark unread from the third-newest → UNREAD row with badge 3 (every message counts in a DM).
- Not exercised by hand: the tray menu (disabled on purpose; `TestTrayPresenceItems` asserts its items, checks and tooltip); Slack refusing a change (covered by `TestSetPresence_TakesBackWhatSlackRefuses`); the real workspace (left to the user/verifier).

### Deviations (4.4, 4.5)

1. Mark unread's refusal is a window item `unreadFailed{err, restore}` rather than a bare `sendFailed`: it also puts the window's `marked` back to the newest message (Slack still has it read), then shows the note through `failed(sendFailed{note})`.
2. `threadMarked` (existing) now derives `ThreadState.Unread` from the cache (`repliesAfter`) instead of always false: Slack echoes `subscriptions.thread.mark read=0` as `thread_marked`, which would otherwise clear the thread just marked unread. A behaviour change to an existing path, in the 4.4 commit because 4.4 needs it; covered by `TestThreadMarkedEchoOfAnUnreadStaysUnread`.
3. `mentionsAfter` counts rows strictly after the boundary; the TUI's `countMentionsSince` counted "at or after", which included the boundary message, which stays read.
4. `MinutesUntilTomorrowMorning` was copied, not moved: `internal/ui/presencemenu` cannot import the gui module, and the TUI's copy goes with Phase 5.1. There was no test of it in the TUI (the plan's "with its test"); `TestMinutesUntilTomorrowMorning` is new.
5. `EndDND` fails only when both `dnd.endSnooze` and `dnd.endDnd` fail; the TUI reported `endSnooze`'s error whenever `endDnd` succeeded, which made ending a scheduled DND (no snooze to end) look like a failure.
6. `chat.SelfDot` not added: the dot is `withSelf` in `headerbar.go` (the `presence` type it maps to lives in package main). The list's subtitle keeps the unread summary and appends `SelfText` ("2 unread conversations · Away") instead of being replaced by it; `SelfText` is "" while simply active.
7. During DND no menu row is marked current (the header says it); a ✓ on "End do not disturb" read wrongly in the first manual run and was removed before the commit.
8. The tray's "End snooze" is labelled "End do not disturb" and calls `EndDND` (the menu's item), so one label means one thing; it is hidden outside DND. `Backend.EndSnooze` exists as planned but no UI uses it yet.
9. The presence menu is mouse-driven (Escape closes it), like `msgMenu`; no arrow-key navigation.
10. `headerBar.leading` is laid over the tile rather than around it: a `Clickable` clips what it wraps, which cut the presence dot in the first manual run.

### Follow-ups (4.4, 4.5)

- Slack's own `channel_marked` echo of a mark unread may arrive after the queued `rawMarked` with a mention count of 0 (the TUI's comment says the echo carries none), turning the list's badge into a plain dot. Not observable on `-fake`; check on the real workspace.
- Usergroup mentions (`mention.InGroups`) are not counted in `mentionsAfter`, as in the TUI.
- `SetPresence(false)` sends `auto`: Slack may still report the user away while idle elsewhere; the optimistic green dot then goes grey on the next `presence_change` only if one arrives for the user.
- A custom `/status` (emoji + text) is not there; `users.profile.set` would be the call.


## Phase 4.6 — Forward; 4.7 — Attachment buttons; 4.8 — Permalinks in-app

### 4.7 step 1 — `rich: buttons carry which attachment action they press`

- Commit: `cbe77471`
- `rich.Buttons{Items []Button}`, `Button{Label, Style, URL string; Action int}`. `FromBlocks` (actions block, section label accessory): `Action: -1`. `FromAttachments` numbers every action of every attachment in order (unlabelled ones and menus counted, so the number indexes `Message.Actions`); a menu (`Type != "button"`) gets -1; Style and URL carried. `PlainBlocks` unchanged (buttons were never in it; its test updated to the new shape). `richView.buttons` takes the items, still drawing labels only. Test added: `TestFromAttachmentsNumbersActionsAcrossAttachments`.

### 4.6 — `gui: forward a message to another conversation`

- Commit: `5d49f925`
- Root: `(*slackclient.Client).ForwardMessage(ctx, src, ts, dst, comment)`; text = comment + "\n" + permalink when the comment is not empty, else the permalink as before; the untested alternative (`text=comment`, `attachments=[{"from_url": permalink}]`) is a comment inside it. `forward_test.go`: a "with comment" case (posted `text` = `"fyi\n"+permalink`), the other cases pass `""`.
- Backend: `Forward(ctx, m, toChannelID, comment)` after `Permalink`. `gui/backend/forward.go`: `ForwardMessage`, then queue `rawMessage{to, self, postedTS, comment + "\n" + "<"+permalink+">"}` as `Send` does. `TestForward_QueuesThePostedMessage` (slacktest: `chat.getPermalink`, `chat.postMessage` with `text=fyi\n<link>`, the queued raw message). Fake (`fake_forward.go`): "You" message with the comment and `rich.Card{Blocks: author line + original body, Footer: "Shared from #general"}`, `Text` = comment + `<permalink>`; unknown message → `message_not_found`, unknown destination → `channel_not_found`. `TestFakeForwardPostsALinkCard`.
- `gui/chat/actions.go`: `ForwardedLink(text)` — the text is exactly `<url>` or `<url|label>` with `url` an archive permalink (`&amp;` decoded); a comment or other text → false. `TestForwardedLink`.
- Windows: `confirmDelete` → `confirmDialog{title, body, okLabel, cancelLabel, danger, ed *widget.Editor, hint, onOK(text), onCancel}` with its own `update`/`layout` (Enter in the editor is OK, Escape in it is Cancel; the editor takes the keyboard); `msgView.deleteDialog(m)` is the delete confirmation, unchanged in look. Menu: "Forward…" (non-ephemerals) after "Open in Slack" → `msgView.onForward`. `switcher.togglePick(title)`: pick mode lists the user's conversations, the window's own included, no "New message…", no channel search, no "All conversations…"; hint = title. `chatWindow.startForward(m)` → pick "Forward to…" → `msgView.forwardDialog(m, to)` (title "Forward to #name" / "Forward to Ada Lovelace", body "Author: preview", editor "Add a message (optional)", OK "Forward") → `go be.Forward` → `forwarded{to, err}`: "Forwarded to …" or "Could not forward: …". Thread window: `hub.forward(conv, m)` raises (or opens) the conversation's chat window and posts `forwardRequest{m}`. `msgMenu.items` is `[9]`.
- README: actions-menu paragraph. AGENTS.md: one Gio row; the "Edit, delete, link to a message" row now names `confirmDialog` / `msgView.deleteDialog`.

### 4.7 — `gui: press an attachment's buttons on a reply only you can see`

- Commit: `d48709bc`
- Root: `slackclient.legacyActionIDs` exported as `LegacyActionIDs` (no other change); `TestLegacyActionIDs`. `cmd/slk/interactions.go` (+ test) deleted: no caller in `cmd/slk` (`grep pressAttachmentAction` finds only itself). `attachment_action.go` untouched; `TestMarshalAttachmentAction_MatchesWebClientCapture` (`capturedDismissPayload`) passes.
- Backend: `Message.Actions []Action`; `Action{Index, Label, Type, Style, URL, Confirm; legacy, attachmentID, callbackID, serviceID}`; `PressAction(ctx, m, a)` after `RunCommand`. `gui/backend/actions.go`: `messageActions(atts, serviceID)` (every action, flat, as `FromAttachments` numbers them), `pressable` (ephemeral, a button, not a URL, the same action at `a.Index`), `PressAction` → `client.AttachmentAction` with `Ephemeral: true`, 10 s timeout. `rawMessage.actionIDs` from `EphemeralMessage.ActionIDs`; `message()` parses the attachments once, and for an ephemeral applies `blocks.ApplyActionIDs` and builds `Actions` with `serviceID = r.botID`. `TestPressAction_EchoesTheEphemeralsButton` (slacktest: the payload carries action id "2", `attachment_id` "1", the callback, `is_ephemeral`; `service_id=B9`; a menu is refused; a non-ephemeral has no actions).
- Fake (`fake_actions.go`): `/standup <notes>` → Standup Bot (`B1`) ephemeral "Here is your standup. Send it to the channel?" with the notes and Send (primary) / Shuffle / Cancel (danger, `Confirm{Title: "Cancel?", Text: "Discard the preview?"}`); `PressAction`: Send posts the notes as Standup Bot + `MessageDeleted`; Shuffle → the same ts edited with the words rotated; Cancel → `MessageDeleted`; a preview no longer there or an unknown value → `invalid_action`; non-ephemeral → refused. `TestFakePressActionDeletesTheEphemeral`.
- `gui/chat/actions.go`: `ButtonActions`, `HasButtonActions` (Type "button", in order). `TestButtonActionsSkipSelects`.
- Windows: `msgWidgets.msg` (the message drawn), `openClick.action`; `richView.button`: a URL button opens its URL, a button of `msg.Actions` with Type "button" is clickable (pointer, `colControl` hover, `colError` frame for danger, `colAccent` for primary), anything else drawn as before. `msgWidgets.handle` gains `press`; `msgView.pressAction(m, a)`: with `Confirm` → `confirmDialog` (title/OK/Cancel defaulting to "Are you sure?"/"Okay"/"Cancel", red OK for a danger button), else `press` → `go be.PressAction`; a refusal → "Couldn't send <label>: <err>". The ephemeral's caption reads "Only visible to you · buttons work here" when `chat.HasButtonActions`.
- README: Block Kit bullet says Block Kit buttons are not pressable; new bullet for an ephemeral's buttons; "Interactive apps" → "Dialogs some commands open (an app's modal) and Block Kit buttons do nothing yet." AGENTS.md: one Gio row; "Legacy attachment action ids slack-go drops" mentions `LegacyActionIDs`; "Press a legacy attachment button from the UI" no longer names the deleted `cmd/slk/pressAttachmentAction` and points at `backend.Backend.PressAction`.

### 4.8 — `gui: open a link to a message in this workspace in its window`

- Commit: `7881c48c`
- Backend: `Domain()` after `WebURL` (Slack: `client.TeamSubdomain()`; Fake: "demo").
- `gui/chat/links.go`: `LinkTarget(url, domain) (channelID, ts, threadTS, ok)` over `slackurl.Parse`; ok only for the workspace's subdomain (case-blind) and never for an empty domain. Tests `TestLinkTargetAcceptsOwnWorkspaceOnly`, `TestLinkTargetReadsThreadReplies`, `TestLinkTargetRejectsNonArchiveURLs`. `TestFakeDomainMatchesItsPermalinks` (`package backend_test`, so it can use `chat`): a fake permalink and a forwarded message's text (through `chat.ForwardedLink`) both lead back to the message.
- `hub.openLink(url)`: miss → `openURL`; hit → the list's conversation, else `be.Conversation` off the event loop (error → stderr + browser) → `openChatAt(conv, ts)`, or for a reply (`threadTS != ts`) `openChatAt(conv, threadTS)` + `openThread(conv, threadTS, ts)`. `richView.open` (set to `h.openLink` in `newMsgView`), passed to `msgWidgets.handle` for span links and URL buttons (chat, thread and info-panel views); the search and list windows keep the browser; the header's and menu's "Open in Slack" keep `openURL`.
- README: the "Links open in the browser…" sentence. AGENTS.md: one Gio row.

### Verify (4.6, 4.7, 4.8) — run at `7881c48c`, gui module (`cd gui && nix-shell shell.nix --run ...`)

```
$ go test -count=1 -v ./backend -run "TestFakeForward|TestForward_|TestFakePressAction|TestPressAction_|TestFakeDomain"
--- PASS: TestPressAction_EchoesTheEphemeralsButton (0.01s)
--- PASS: TestFakeForwardPostsALinkCard (0.00s)
--- PASS: TestFakePressActionDeletesTheEphemeral (0.00s)
--- PASS: TestForward_QueuesThePostedMessage (0.01s)
--- PASS: TestFakeDomainMatchesItsPermalinks (0.00s)
ok  	github.com/gammons/slk/gui/backend	0.036s
$ go test -count=1 -v ./chat -run "TestForwardedLink|TestButtonActions|TestLinkTarget"
--- PASS: TestForwardedLink (0.00s)
--- PASS: TestButtonActionsSkipSelects (0.00s)
--- PASS: TestLinkTargetAcceptsOwnWorkspaceOnly (0.00s)
--- PASS: TestLinkTargetReadsThreadReplies (0.00s)
--- PASS: TestLinkTargetRejectsNonArchiveURLs (0.00s)
ok  	github.com/gammons/slk/gui/chat	0.005s
$ go test -count=1 -v ./rich
--- PASS: TestFromRichTextStylesAndEntities (0.00s)
--- PASS: TestFromRichTextBlocks (0.00s)
--- PASS: TestUserIDs (0.00s)
--- PASS: TestFromMrkdwn (0.00s)
--- PASS: TestFromMrkdwnQuoteRestAndCode (0.00s)
--- PASS: TestBodyPrecedence (0.00s)
--- PASS: TestFromBlocks (0.00s)
--- PASS: TestFromAttachments (0.00s)
--- PASS: TestFromAttachmentsNumbersActionsAcrossAttachments (0.00s)
--- PASS: TestFromFiles (0.00s)
--- PASS: TestSpansFlattens (0.00s)
--- PASS: TestFromMrkdwnKeepsBackslashes (0.00s)
--- PASS: TestFromMrkdwnDecodesEntitiesOnce (0.00s)
--- PASS: TestItalicize (0.00s)
--- PASS: TestEmojiGlyph (0.00s)
--- PASS: TestPlainBlocks (0.00s)
--- PASS: TestFromRichTextDateFormatFirst (0.00s)
ok  	github.com/gammons/slk/gui/rich	0.003s
$ go test -count=1 -v ./cmd/slk-gio -run TestPopupTags
--- PASS: TestPopupTagsAreDistinct (0.00s)
ok  	github.com/gammons/slk/gui/cmd/slk-gio	0.006s
```

Root module at `7881c48c`:

```
$ go build ./... && go vet ./... && gofmt -l .
clean
$ go test -count=1 -v ./internal/slack -run "Forward|ActionIDs|AttachmentAction"
--- PASS: TestMarshalAttachmentAction_MatchesWebClientCapture (0.00s)
--- PASS: TestMarshalAttachmentAction_EchoesConfirm (0.00s)
--- PASS: TestAttachmentAction_PostsPayloadForm (0.00s)
--- PASS: TestAttachmentAction_SendsServiceID (0.00s)
--- PASS: TestAttachmentAction_OmitsEmptyServiceID (0.00s)
--- PASS: TestAttachmentAction_OKFalseIsAnError (0.00s)
--- PASS: TestDispatchMessageForwardsBlocks (0.00s)
--- PASS: TestDispatchEphemeralMessage_NumericActionIDs (0.00s)
--- PASS: TestLegacyActionIDs (0.00s)
--- PASS: TestForwardMessageRequests (0.00s)
--- PASS: TestForwardMessageContextAndErrors (0.00s)
ok  	github.com/gammons/slk/internal/slack	0.009s
$ go test -count=1 ./... (non-ok lines)
exit=1
```

(`exit=1` is grep's: no line other than `ok` / "no test files".) `git diff ace9995f..HEAD -- internal/slack/attachment_action*.go internal/slack/testdata internal/slackhttp/testdata internal/blockkit/testdata docs/superpowers/specs` is empty. At `cbe77471`, `5d49f925` and `d48709bc` both modules were build/vet/gofmt-clean and `cd gui && go test ./... -race` all `ok`; the root tests run there were `./internal/slack` (`Forward|ActionIDs|AttachmentAction`), and at `d48709bc` `./cmd/... ./internal/slack/... ./internal/core/...`.

### Verify (Phase 4, whole) — run at `7881c48c`

```
$ cd gui && nix-shell shell.nix --run 'go build ./... && go vet ./... && gofmt -l . && go test ./... -race'
ok  	github.com/gammons/slk/gui/backend	5.211s
ok  	github.com/gammons/slk/gui/chat	1.019s
ok  	github.com/gammons/slk/gui/cmd/slk-gio	10.337s
?   	github.com/gammons/slk/gui/internal/richtext	[no test files]
?   	github.com/gammons/slk/gui/internal/styledtext	[no test files]
ok  	github.com/gammons/slk/gui/motion	1.011s
ok  	github.com/gammons/slk/gui/notifications	1.029s
ok  	github.com/gammons/slk/gui/palette	1.010s
ok  	github.com/gammons/slk/gui/rich	1.018s
ok  	github.com/gammons/slk/gui/search	1.018s
ok  	github.com/gammons/slk/gui/selection	1.010s
ok  	github.com/gammons/slk/gui/sidebar	1.016s
ok  	github.com/gammons/slk/gui/textedit	1.011s
exit=0
$ grep -c "^- \*\*" gui/README.md
16
$ sed -n "/^## Not there yet/,\$p" gui/README.md | grep "^- \*\*"
- **Dropping files** onto a window does nothing: Gio has no drag-and-drop
- **Leaving conversations.** `/leave` is not there.
- **Interactive apps.** Dialogs some commands open (an app's modal) and
- **Strike-through** is drawn dimmed instead.
- **More than one workspace** at a time.
$ grep -n "SearchChannels\|OpenConversation\|Upload(\|MarkUnread\|SetPresence\|Forward(\|PressAction\|Domain()" gui/backend/backend.go
592:	Upload(ctx context.Context, channelID, threadTS string, files []core.PendingAttachment, caption string, progress func(done, total int)) error
608:	Forward(ctx context.Context, m Message, toChannelID, comment string) error
618:	// MarkUnread moves the read mark of a conversation (threadTS empty)
624:	MarkUnread(ctx context.Context, channelID, threadTS, boundaryTS string) error
656:	// PressAction presses a button of a message only the user can see
660:	PressAction(ctx context.Context, m Message, a Action) error
672:	// SearchChannels asks Slack which channels match query, joined or
676:	SearchChannels(ctx context.Context, query string) ([]ChannelResult, error)
681:	// OpenConversation opens the DM with one person, or the group DM
685:	OpenConversation(ctx context.Context, userIDs []string) (Conversation, error)
700:	Domain() string
708:	// SetPresence sets the user away, or back to active (Slack's
713:	SetPresence(ctx context.Context, away bool) error
$ for t in ...; do grep -q "func $t" gui/backend/*_test.go || echo MISSING $t; done
(no output: none missing)
```

The "Not there yet" list: Dropping files, `/leave`, Interactive apps (modals + Block Kit buttons), Strike-through, more than one workspace. Strike-through was there before Phase 4 and is not a Phase 4 item; the plan's expected list leaves it out.

### Manual (`-fake`), performed

Same isolation as 4.4–4.5: binary built to the scratchpad, `Xvfb :97` (`nix-shell -p xvfb`), `WAYLAND_DISPLAY` unset, `DISPLAY=:97`, `DBUS_SESSION_BUS_ADDRESS` at a socket that does not exist, scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`/`XDG_CACHE_HOME` with `[gui] tray = false` and `[notifications] enabled = false`, `GDK_DEBUG=no-portals`; additionally a stub `xdg-open` first on `PATH` that only appends its argument to `<scratch>/opened.log`, so no browser was started. `<scratch>/slk-gio -fake -fake-every 0`, driven with `xdotool`, frames with `ffmpeg -f x11grab -i :97`. The user's `slk-gio` (pid 450995) was untouched throughout.

- 4.6: #general, right-click Margaret's "Can someone review the cache migration…" → menu Copy text / Copy link / Open in Slack / Forward… / Mark unread from here. Forward… → switcher with the filter hint "Forward to…", rows incidents, Ada Lovelace, general, Grace Hopper + Linus Torvalds, eng-backend, random, release-train, office-amsterdam; no "New message…", no "All conversations…". Typed "ada", Enter → dialog "Forward to Ada Lovelace", body "Margaret Hamilton: Can someone review the cache migration…", editor "Add a message (optional)", Cancel / Forward. Typed "fyi", Enter → note "Forwarded to Ada Lovelace" in #general; Ada's window shows "You: fyi" and the card "Margaret Hamilton / Can someone review … / Shared from #general". From a thread window (Margaret's broadcast reply): Forward… raised #general with the pick switcher; chose random, note "from the thread", Forward → #random shows it with the card.
- 4.7: `/standup shipped the cache` in #general → ephemeral "Only visible to you · buttons work here", Standup Bot, "Here is your standup. Send it to the channel?", "shipped the cache", buttons Send (accent frame), Shuffle, Cancel (red frame). Cancel → dialog "Cancel?" / "Discard the preview?" / Cancel / Okay (red); Escape → preview kept. Shuffle → "the cache shipped", "(edited)". Send → "Standup Bot: the cache shipped" posted in #general and the ephemeral gone.
- 4.8: #general, right-click Grace's "Morning! Anyone else seeing the deploy queue back up?" → Copy link → clipboard `https://demo.slack.com/archives/C1/p1793442385000100` (`xclip -display :97 -o`). In Ada's DM, Ctrl+V, Enter → link message. #general scrolled to its newest message, then the link clicked → #general (moved, as `openChatAt` does for an open window) scrolled to "Morning! …" and lit it; `opened.log` stayed absent. Sent `https://other.slack.com/archives/C1/p1793442385000100` and clicked it → `opened.log`: `xdg-open https://other.slack.com/archives/C1/p1793442385000100`.
- Not exercised by hand: a link to a thread reply (the fake's permalinks carry no `thread_ts`; `hub.openLink`'s thread branch is the `openNote` shape and `TestLinkTargetReadsThreadReplies` covers the parsing); a link to a conversation the list lacks (`Backend.Conversation` path); a URL button (the fake has none); Slack refusing a press or a forward; the real workspace.
- Observed, not part of these sub-phases: (1) the jump target ended under the header (author line hidden, body line at the top edge) after `openChatAt` moved an already open window; the same path serves search results and notifications. (2) The first Ctrl+V of text that slk-gio itself had put on the clipboard landed in the composer more than a second later (after Enter had already sent "see"); a second Ctrl+V landed at once. (3) `xdotool windowclose` on a chat window made the whole process exit with an X `BadWindow` error (an external destroy, not a user action; noted for completeness).

### Deviations (4.6–4.8)

1. `ForwardMessage` had no TUI caller to update: the TUI's composition root was removed in Phase 2, and `grep -rn ForwardMessage` finds only `internal/slack`. Only the test changed.
2. The forwarding flag (does Slack unfurl a permalink after comment text?) was not stopped on as the hand-over contract says: as instructed for this run, the comment + newline + permalink shape is implemented and the `from_url` alternative is a comment in `ForwardMessage`; the question is a follow-up below.
3. `confirmDialog` has `cancelLabel` and `onCancel` beyond the plan's fields (Slack's `dismiss_text` for 4.7); `onOK` gets the editor's trimmed text. A dialog with an editor gives the keyboard to the editor and reads Escape/Enter from it.
4. The pick-mode switcher includes the window's own conversation (the plan said nothing); forwarding a thread reply to its channel is a real choice. It never searches Slack (the plan's "no remote section").
5. `Message.Actions` is set only for ephemerals, and `PressAction` refuses anything else: the action-dialog spec restricts presses to ephemerals (non-ephemeral history has no action ids; a press without one is a request shape the web client never sends). The plan's `LegacyActionIDs` wiring in `applyEvent` and `fromCache` was therefore not added (neither path carries ephemerals); `LegacyActionIDs` is exported and listed in AGENTS.md as planned, its caller still `dispatchEphemeral`.
6. `Action` keeps the whole `blocks.LegacyAction` (`legacy`) rather than separate `id/name/value` fields, so a press echoes every field Slack sent (`AttachmentActionRequest.Action` is that struct); it also carries `Type`, which `chat.ButtonActions` filters on.
7. `msgWidgets.handle`'s press callback takes the message too (`press(m, a)`), and handle also takes `open` (4.8) instead of reading a richView field, since `handle` is a `msgWidgets` method.
8. A legacy button with a URL is a link and opens on any message (as any link does; nothing is sent to Slack); the TUI offered URL buttons only in an ephemeral's dialog.
9. The fake's Forward card starts with the original's author line (Slack's preview shows the author too); its destination must be a conversation the fake has (`channel_not_found` otherwise).
10. `TestFakeDomainMatchesItsPermalinks` is in `package backend_test` (`gui/backend/fake_domain_test.go`): `chat` imports `backend`, so the internal test package cannot import `chat`.
11. Tests beyond the plan: `TestFromAttachmentsNumbersActionsAcrossAttachments`, `TestForward_QueuesThePostedMessage`, `TestPressAction_EchoesTheEphemeralsButton`, `TestLegacyActionIDs`.

### Follow-ups (4.6–4.8)

- **For the user, on the real workspace:** forward a message with a note and check that Slack still shows the original's preview under the note. If it does not, switch `ForwardMessage` to `text=comment` + `attachments=[{"from_url": permalink}]` (the comment in `internal/slack/forward.go`). Without a note the request is unchanged from the TUI's.
- The forward note is sent as typed (no `@name` → `<@U…>` conversion as the composer does with `chat.Mentions`).
- `chat.ForwardedLink` exists but no window uses it yet (the TUI opened the original on Enter); a click on the link itself now opens it in-app.
- Enterprise Grid permalinks may carry a different host than `TeamSubdomain()` (e.g. an org subdomain); such links still open in the browser.
- The two observations under "Manual" (jump target under the header after `moveHere`; a delayed self-owned clipboard paste) are outside these sub-phases and unchanged by them.

## Phase 5 — Delete the TUI; build, CI, release; GUI boundary test

### Before 5.1: full `-race`, both modules, at `1347b723`

- root: `nix-shell gui/shell.nix --run 'go test ./... -race -count=1'` → 82 packages `ok`, no other line but "no test files", exit 0.
- gui: `cd gui && nix-shell shell.nix --run 'go test ./... -race -count=1'` → 11 packages `ok`, exit 0.

### 5.1 — `ui: retire the bubbletea TUI`

- Commit: `44e6f30a`
- `git rm -r internal/ui internal/bubbles internal/demo internal/editor docs/assets/demo internal/wake` (`internal/ui/boundary_test.go` copied to the scratchpad first).
- `internal/core` pruned (each checked with grep over `cmd/`, `gui/`, `internal/` outside the deleted trees): deleted `ports.go` (all 18 interfaces: the Reaction, Interaction, Command, Thread, Activity, Message, Channel, Search, File, Desktop, Editor, Presence, Settings, Unread, Profile, Workspace and Avatar services and `ImageFetcher`; plus `ClipboardFormat`/`ClipboardText`/`ClipboardImage`), `adapters.go` (every `New*Service`, `*ServiceFuncs` and adapter, and the func types `ChannelFetchFunc` … `ChannelLookupFunc`), `core.go` (`Msg`, `Cmd`), and from `types.go`: `ForwardResult`, `ChannelFinderItem`, `EmojiEntry`, `PresenceAction` (+ constants), `ThemeScope` (+ constants), `UserProfile`, `RateLimitedError`. Kept (used outside): `MessageItem`, `Attachment`, `ThumbSpec`, `ReactionItem`, `PendingAttachment`, `ReadState`, `ThreadSummary`, `ActivityItem`, `ActivityMessage`, `ActivityMsgKey`, `CommandError`, `Theme`; package doc moved into `types.go`. Tests deleted with the adapters: `command_adapter_test.go`, `interaction_adapter_test.go`, `message_forward_test.go`, `profile_test.go`; `TestCommandErrorMessageIsTheCode` kept, in new `types_test.go`.
- `(*slackclient.Client).GetUnreadCounts` (context-free) deleted; its doc moved onto `GetUnreadCountsContext`; four test calls in `client_test.go` now `GetUnreadCountsContext(context.Background())`.
- Root `go mod tidy`: `github.com/muesli/reflow` dropped; `charm.land/bubbles/v2` direct → `// indirect`; go.sum lost reflow, `go-runewidth v0.0.12/go.mod`, `rivo/uniseg v0.1.0/v0.2.0 /go.mod`. gui `go mod tidy -diff`: no change.
- Checks: both modules build/vet/gofmt clean; root `go test ./... -race` green (all `ok`).

Orphan sweep (importers outside the package's own directory, the deleted trees excluded; non-test count in brackets):

| package | importers | gui | cmd | action |
|---|---|---|---|---|
| `internal/wake` | 0 | 0 | 0 | deleted (TUI suspend detector) |
| `internal/slack/membership` | 0 | 0 | 0 | Phase 6 backlog |
| `internal/version` | 0 | 0 | 0 | Phase 6 backlog (`ModalFooter`, the TUI help modal's line) |
| `internal/ids` | 7 (4) | 0 | 0 | kept; only `internal/slackurl` outside tests now that the ports are gone — backlog |
| `internal/slack/slacktest` | 11 (0) | 8 | 0 | kept (test fixture by design) |
| every other `internal/*` | ≥ 1 non-test | | | kept |

`internal/image`'s terminal renderers (sixel, sixelframe, preview; kitty/half-block still reached via `internal/avatar` and `internal/emoji`) are terminal-only code for the backlog. Everything the plan guessed might be orphaned (`launch`, `typing`, `threadsubs`, `searchq`, `mention`, `notify`, `slashcmd`) has gui importers.

### 5.2 — `gui: enforce that the GUI never reaches the retired TUI`

- Commit: `c07d4b5e`
- `gui/boundary_test.go`, package `gui_test` at the gui module root. A test-only directory compiles under `go test .`, `go build ./...` and `go vet ./...`; no `doc.go` needed. Walks every non-test `.go` file (skips `testdata`, non-`.go` files), `parser.ImportsOnly`, fails on a direct import of `internal/ui`, `internal/bubbles`, `internal/demo`, `internal/editor` (or beneath), `charm.land/`, `github.com/charmbracelet/`; fails if it walked no files. `TestBoundaryBannedMatchesTreesNotNeighbours` pins the prefix rule (`internal/usergroups`, `internal/editorconfig` not banned). Checked to fail with a throwaway `gui/zztmp/x.go` importing `internal/ui/messages` and `lipgloss` (removed).

### 5.3 — `build: drop the TUI from make, flake, CI and goreleaser; build and test gui in CI`

- Commit: `47a68bcc`
- `Makefile`: `demo-gifs` removed; `run: gui` then `./bin/slk-gio`; `test`/`lint` also `cd gui && …`; `.PHONY` updated.
- `flake.nix`: description "Slack CLI and Gio desktop client"; `buildInputs = [pkgs.libX11]`, `libwebsockets-fixed`, `ttyd-fixed`, `vhs-sixel` and the dev shell's `packages` removed (dev shell keeps `inputsFrom = [ slk ]`); `vendorHash = "sha256-kcaddhrDhI9TfADvtmjmMxdlyGXifxyItjImFWp4lyI="` (from `nix build .#slk.goModules`'s `got:` with a placeholder hash); `excludedPackages = [ "gui" ]` (deviation 3).
- `ci.yml`: both `libx11-dev` steps removed; jobs `gui` (apt list as planned, setup-go `cache-dependency-path: gui/go.sum`, build/vet/`test -race` each with `working-directory: gui`) and `lint-gui` (golangci-lint-action `working-directory: gui`, same apt step). Parsed with `yq`: jobs test, lint, gui, lint-gui, nix.
- `.goreleaser.yaml`: one build `id: slk`, `CGO_ENABLED=0`, goos linux/darwin/windows, amd64/arm64 (windows/arm64 ignored as before), `main: ./cmd/slk`, `binary: slk`; header, nfpm description and cask description → "Slack CLI (workspace setup and export) for slk-gio". `release.yml` untouched (still `macos-latest`).

### Verify (Phase 5) — run at `47a68bcc`

`golangci-lint` is on neither PATH nor in `gui/shell.nix`; those two `&& golangci-lint run` were left off (lint runs in CI's `lint` / `lint-gui`). `-count=1` added to the test lines.

```
$ git ls-files internal/ui internal/bubbles internal/demo internal/editor docs/assets/demo | wc -l
0
$ grep -rln 'internal/ui\|internal/bubbles\|internal/demo\|internal/editor' --include='*.go' . gui | wc -l
2
$ grep -rl 'charm.land\|charmbracelet' --include='*.go' . | grep -v _test | sort
cmd/slk/onboarding_browser.go
cmd/slk/onboarding.go
cmd/slk/remove_workspace.go
internal/emoji/width.go
internal/image/preview.go
$ grep -c 'bubbletea' go.mod
1
6:	charm.land/bubbletea/v2 v2.0.6
$ grep -c 'bubbletea' gui/go.mod
0
$ command -v golangci-lint; nix-shell gui/shell.nix --run 'command -v golangci-lint'
exit=1
$ nix-shell gui/shell.nix --run 'go build ./... && go vet ./... && go test ./... -race && gofmt -l .'
ok  	github.com/gammons/slk/cmd/slk	1.264s
ok  	github.com/gammons/slk/internal/attach	1.036s
ok  	github.com/gammons/slk/internal/avatar	1.152s
ok  	github.com/gammons/slk/internal/blockkit	1.055s
ok  	github.com/gammons/slk/internal/bootstrap	1.042s
ok  	github.com/gammons/slk/internal/bootstrap/adapt	2.256s
ok  	github.com/gammons/slk/internal/cache	29.550s
ok  	github.com/gammons/slk/internal/clipboard	1.011s
ok  	github.com/gammons/slk/internal/config	4.701s
ok  	github.com/gammons/slk/internal/config/versionts	1.062s
ok  	github.com/gammons/slk/internal/core	1.011s
ok  	github.com/gammons/slk/internal/core/blocks	1.012s
ok  	github.com/gammons/slk/internal/datefmt	1.012s
ok  	github.com/gammons/slk/internal/debuglog	1.019s
ok  	github.com/gammons/slk/internal/emoji	1.044s
ok  	github.com/gammons/slk/internal/export	1.039s
ok  	github.com/gammons/slk/internal/filedl	1.020s
ok  	github.com/gammons/slk/internal/filepick	1.010s
?   	github.com/gammons/slk/internal/ids	[no test files]
ok  	github.com/gammons/slk/internal/image	3.858s
?   	github.com/gammons/slk/internal/launch	[no test files]
ok  	github.com/gammons/slk/internal/mention	1.014s
ok  	github.com/gammons/slk/internal/msgsync	7.602s
ok  	github.com/gammons/slk/internal/notify	1.076s
ok  	github.com/gammons/slk/internal/onboard	1.025s
?   	github.com/gammons/slk/internal/paths	[no test files]
ok  	github.com/gammons/slk/internal/searchq	1.013s
ok  	github.com/gammons/slk/internal/service	1.994s
ok  	github.com/gammons/slk/internal/slack	1.402s
ok  	github.com/gammons/slk/internal/slack/boot	1.101s
ok  	github.com/gammons/slk/internal/slack/edge	1.178s
ok  	github.com/gammons/slk/internal/slack/membership	4.099s
ok  	github.com/gammons/slk/internal/slack/mrkdwn	1.030s
ok  	github.com/gammons/slk/internal/slack/resolve	5.412s
?   	github.com/gammons/slk/internal/slack/slacktest	[no test files]
ok  	github.com/gammons/slk/internal/slackdesktop	1.075s
ok  	github.com/gammons/slk/internal/slackfmt	1.042s
ok  	github.com/gammons/slk/internal/slackhttp	1.245s
ok  	github.com/gammons/slk/internal/slackurl	1.012s
ok  	github.com/gammons/slk/internal/slashcmd	1.013s
ok  	github.com/gammons/slk/internal/text	1.019s
ok  	github.com/gammons/slk/internal/theme	1.014s
ok  	github.com/gammons/slk/internal/threadsubs	1.015s
ok  	github.com/gammons/slk/internal/typing	1.014s
ok  	github.com/gammons/slk/internal/usergroups	1.012s
ok  	github.com/gammons/slk/internal/version	1.013s
exit=0
$ nix-shell gui/shell.nix --run 'cd gui && go build ./... && go vet ./... && go test ./... -race && gofmt -l .'
ok  	github.com/gammons/slk/gui	1.019s
ok  	github.com/gammons/slk/gui/backend	5.355s
ok  	github.com/gammons/slk/gui/chat	1.019s
ok  	github.com/gammons/slk/gui/cmd/slk-gio	10.446s
?   	github.com/gammons/slk/gui/internal/richtext	[no test files]
?   	github.com/gammons/slk/gui/internal/styledtext	[no test files]
ok  	github.com/gammons/slk/gui/motion	1.012s
ok  	github.com/gammons/slk/gui/notifications	1.029s
ok  	github.com/gammons/slk/gui/palette	1.011s
ok  	github.com/gammons/slk/gui/rich	1.020s
ok  	github.com/gammons/slk/gui/search	1.019s
ok  	github.com/gammons/slk/gui/selection	1.010s
ok  	github.com/gammons/slk/gui/sidebar	1.019s
ok  	github.com/gammons/slk/gui/textedit	1.011s
exit=0
$ cd gui && nix-shell shell.nix --run 'go test . -run Boundary -v'
=== RUN   TestBoundaryGUINeverImportsTheRetiredTUI
--- PASS: TestBoundaryGUINeverImportsTheRetiredTUI (0.00s)
=== RUN   TestBoundaryBannedMatchesTreesNotNeighbours
--- PASS: TestBoundaryBannedMatchesTreesNotNeighbours (0.00s)
PASS
ok  	github.com/gammons/slk/gui	0.004s
exit=0
$ grep -n 'demo-gifs\|vhs\|libX11' Makefile flake.nix .github/workflows/ci.yml | wc -l
0
$ grep -n 'working-directory: gui' .github/workflows/ci.yml | wc -l
4
$ nix build .#slk --print-build-logs
exit=0
slk
```

The `nix build` printed no log lines because the derivation was already built by the run just before (same inputs), which built `./cmd/slk` and ran the root tests in the sandbox (46 "Building subPackage" lines, exit 0); `result/bin` holds `slk`.

Against the plan's expected values:
- `grep -rln 'internal/ui…' … | wc -l` is 2, not 0: both lines are `gui/boundary_test.go` (listed once via `.` and once via `gui`), which has to name the banned paths. No other `.go` file mentions them.
- `grep -c bubbletea go.mod` is 1 but **direct**, not `// indirect`: `internal/image/sixelframe_integration_test.go` runs a bubbletea program around `SixelFrameStore` (a test of TUI-only code in a package that stays). gui/go.mod: 0.
- The charm/charmbracelet list is exactly the five expected files.

### Deviations (Phase 5)

1. `charm.land/bubbletea/v2` stays a direct requirement (`internal/image/sixelframe_integration_test.go`); `github.com/mattn/go-sixel` stays (`internal/image/sixel.go`); only `muesli/reflow` dropped and `charm.land/bubbles/v2` went indirect. Removing `internal/image`'s terminal renderers (with that test) would drop both; left to the Phase 6 backlog per the "delete only `internal/wake`" rule.
2. Comments in kept packages that pointed into the deleted trees (`internal/theme/themes.go`, `internal/image/{sixel,preview,preview_test}.go`, `internal/ids/ids.go`, `internal/blockkit/rendersbody{,_test}.go`, `internal/core/blocks/blocks.go`, `internal/mention/mention{,_test}.go`) now say "the retired TUI's …" (or name `gui/rich`), so the Verify grep finds no path into the deleted trees. Comments only.
3. `flake.nix` gained `excludedPackages = [ "gui" ]`: `buildGoModule` finds packages with `find`, so it tried `go install ./gui` from the root module ("main module does not contain package github.com/gammons/slk/gui"). It would have failed the same way on `./gui/backend` before this phase (the flake's full build was not run since gui/ was added; Phase 2 only built `.#slk.goModules`); the new test file at `gui/` made it the first hit.
4. AGENTS.md (5.1): only the "Resolve a command's channel argument" row and the `(or GetUnreadCounts())` mention were removed, since their symbols are gone; every other row naming `internal/ui`, `internal/bubbles` or the deleted `core` ports is left for Phase 6's rewrite.
5. The plan's 5.1 says report the orphan sweep and delete nothing; per this run's instructions `internal/wake` (no importer) was deleted, and the `core` ports and `GetUnreadCounts` pruned in the same commit.
6. `TestCoreDoesNotImportTUIOrDirectIO` lived in the deleted `internal/ui/boundary_test.go` and went with it; `internal/core` now has no boundary test (it has no imports beyond `internal/core/blocks`).
7. `release.yml` still runs on `macos-latest`, and its comment (darwin cgo for the clipboard) is now stale; not changed, as instructed.

### Follow-ups (Phase 5)

- Phase 6 backlog (orphans): `internal/slack/membership`, `internal/version`, `internal/ids` (one non-test importer), `internal/image`'s terminal renderers and `sixelframe_integration_test.go` (would make bubbletea indirect and drop go-sixel); `internal/emoji/width.go` / `internal/image/preview.go` lipgloss uses (O1).
- `release.yml` can move to `ubuntu-latest` and drop its cgo comment.
- `.goreleaser.yaml`'s pre-release footer still says "run it with `SLK_DEBUG=1 slk`" and points at the request tally; the CLI does not run a session. Phase 6 wording.
- `MinutesUntilTomorrowMorning`'s TUI copy went with `internal/ui/presencemenu`; the gui copy is now the only one.

## Phase 5b — Trim the terminal image stack

- Commit: `66461d8e` — `image: keep the fetcher and cache, drop the terminal renderers`
- Each deletion checked first with grep over `gui/`, `cmd/` and the surviving `internal/` for the exported symbols of the file (outside callers of `internal/image` were only `imgpkg.Fetcher`, `FetchRequest`, `NewCache`, `NewFetcher`, `PickThumb`, `ThumbSpec`; of `internal/avatar` only `SizedURL`, `FetchKey`; of `internal/emoji` only `BuildCustomEmojiURL`, `CanonicalSlackName`, `CodeMap`, `EmojiCacheKey`, `Sprint`, `StripSkinTone`, `StripSkinToneFromText`).
- `internal/image`: `git rm` of kitty (+ `kitty_cellpx_test`, `kitty_upload_atomic_test`), sixel (+ `sixel_cellpx_test`), sixelframe (+ `sixelframe_integration_test.go`, the bubbletea program), sixelpaint, halfblock, preview, probe (+ `_unix`, `_other`), capability, cellmetrics (+ `_unix`, `_windows`), registry, renderer, and `migrate.go` (+ test; `MigrateAvatars` had no caller and the cache does not use it). `fetcher.go` lost the prerender pipeline (`ConfigurePrerender`, `ConfigurePrerenderKitty`, `Prerendered`, `maybePrerender`, `prerenderKey`, the `prerendered*` fields, `FetchRequest.CellTarget`) and `TestFetcher_FetchPopulatesPrerender`; comments naming deleted TUI functions reworded; the package doc (was in `renderer.go`) is now on `fetcher.go`. `testdata/tiny.webp` stays (`TestFetcher_DecodesWebP`).
- `internal/avatar`: `avatar.go` is now the package doc + `FetchKey`; the TUI's `Cache` (Preload worker pool, ANSI render memo, kitty / half-block cells) and its tests (`avatar_test.go`, `kitty_test.go`, `parity_test.go`, `sized_test.go`) deleted; `TestFetchKey` kept in a new `avatar_test.go`. `SizedURL` untouched.
- `internal/emoji`: `place.go` (kitty placement; its `EmojiCacheKey` moved to `url.go`, + `TestEmojiCacheKey`), `width.go`, `imagemode.go` and their tests deleted.
- `gui/boundary_test.go`: `charm.land/lipgloss` and `github.com/mattn/go-sixel` added to `guiBannedImports`; `TestBoundaryBannedMatchesTreesNotNeighbours` gained `go-sixel` (banned) and `go-sqlite3` (not).
- `internal/slack/resolve/resolve_test.go`: one comment that explained a test body by `avatar.Cache.Preload` reworded.
- AGENTS.md: the `emoji.Width` row removed (its symbol is gone).
- go.mod (root, `go mod tidy`): `charm.land/bubbletea/v2` direct → `// indirect` (via huh); `github.com/charmbracelet/x/ansi`, `github.com/charmbracelet/x/term` direct → `// indirect`; `github.com/mattn/go-sixel` and `github.com/soniakeys/quant` removed; go.sum −4 lines. `charm.land/lipgloss/v2` stays direct (`cmd/slk/remove_workspace.go`). gui/go.mod (`go mod tidy`): `charm.land/lipgloss/v2`, `charmbracelet/{colorprofile,ultraviolet,x/ansi,x/term,x/termios,x/windows}`, `clipperhouse/{displaywidth,uax29/v2}`, `lucasb-eyer/go-colorful`, `mattn/go-runewidth`, `mattn/go-sixel`, `muesli/cancelreader`, `soniakeys/quant`, `xo/terminfo` removed (all indirect); gui/go.sum −32 lines.
- `flake.nix` `vendorHash` = `sha256-3zdIahgS0Z8hcuUO/Z6YR5MhTIruhXJwDKd1KUMJJH4=` (`nix build .#slk.goModules` with a placeholder, the `got:` value); `nix build .#slk` then exit 0, `result/bin/slk` (the `result` link removed afterwards).
- Checks before commit: both modules `go build ./... && go vet ./...` clean, `gofmt -l .` empty (after `gofmt -w internal/image/fetcher.go`, field alignment), `go test -count=1 ./... -race` every package `ok` in both.

### Deviations (5b)

1. `internal/emoji/width.go` deleted rather than rewritten on `go-runewidth`/`uniseg`: its only caller was `emoji.Place` (kitty placement, deleted with the renderers), so `emoji.Width` had no caller left; `imagemode.go` was read only by `width.go` and went with it.
2. `internal/emoji/place.go` deleted (the plan named only `width.go`): `Place` needs `Fetcher.Prerendered` and `image.Protocol`, both deleted; the GUI used only its `EmojiCacheKey`, now in `url.go`.
3. `internal/avatar`'s `Cache` deleted whole: the plan's "keep the download/cache" is `internal/image`'s `Fetcher`/`Cache`; `avatar.Cache` was the TUI's per-user ANSI render memo and had no caller outside its tests.
4. `migrate.go` deleted: the plan keeps it only "if the cache uses it"; nothing calls `MigrateAvatars`.

### Verify (Phase 5b) — run at `66461d8e`

```
$ grep -rl 'charm.land\|charmbracelet' --include='*.go' . | grep -v _test | sort
cmd/slk/onboarding_browser.go
cmd/slk/onboarding.go
cmd/slk/remove_workspace.go
$ grep -n 'bubbletea\|go-sixel' go.mod
29:	charm.land/bubbletea/v2 v2.0.6 // indirect
$ grep -c 'charmbracelet\|lipgloss\|go-sixel\|bubbletea' gui/go.mod
0
$ ls internal/image internal/avatar
internal/avatar:
avatar.go
avatar_test.go
sizedurl.go
sizedurl_test.go

internal/image:
cache.go
cache_test.go
fetcher.go
fetcher_reqid_test.go
fetcher_test.go
testdata
thumb_test.go
$ nix-shell gui/shell.nix --run 'go build ./... && go vet ./... && test -z "$(gofmt -l .)" && go test -count=1 ./... -race | grep -v "^ok\|no test files"'
(no output; grep exit 1)
$ cd gui && nix-shell shell.nix --run 'go build ./... && go vet ./... && test -z "$(gofmt -l .)" && go test -count=1 ./... -race | grep -v "^ok\|no test files"'
(no output; grep exit 1)
$ cd gui && go test -count=1 . -run Boundary -v
=== RUN   TestBoundaryGUINeverImportsTheRetiredTUI
--- PASS: TestBoundaryGUINeverImportsTheRetiredTUI (0.00s)
=== RUN   TestBoundaryBannedMatchesTreesNotNeighbours
--- PASS: TestBoundaryBannedMatchesTreesNotNeighbours (0.00s)
PASS
ok  	github.com/gammons/slk/gui	0.004s
$ nix build .#slk
exit=0
```

Full `-race` output at `66461d8e` (also the timing run for AGENTS.md: wall clock root 32.9 s, gui 11.8 s; without `-race` 4.4 s and 2.8 s; `-count=1`, warm cache, 8-core/16-thread Ryzen AI 7 PRO 350):

```
ok  	github.com/gammons/slk/cmd/slk	1.522s
ok  	github.com/gammons/slk/internal/attach	1.049s
ok  	github.com/gammons/slk/internal/avatar	1.017s
ok  	github.com/gammons/slk/internal/blockkit	1.067s
ok  	github.com/gammons/slk/internal/bootstrap	1.032s
ok  	github.com/gammons/slk/internal/bootstrap/adapt	2.541s
ok  	github.com/gammons/slk/internal/cache	31.144s
ok  	github.com/gammons/slk/internal/clipboard	1.017s
ok  	github.com/gammons/slk/internal/config	5.009s
ok  	github.com/gammons/slk/internal/config/versionts	1.093s
ok  	github.com/gammons/slk/internal/core	1.013s
ok  	github.com/gammons/slk/internal/core/blocks	1.016s
ok  	github.com/gammons/slk/internal/datefmt	1.011s
ok  	github.com/gammons/slk/internal/debuglog	1.032s
ok  	github.com/gammons/slk/internal/emoji	1.051s
ok  	github.com/gammons/slk/internal/export	1.042s
ok  	github.com/gammons/slk/internal/filedl	1.021s
ok  	github.com/gammons/slk/internal/filepick	1.011s
?   	github.com/gammons/slk/internal/ids	[no test files]
ok  	github.com/gammons/slk/internal/image	1.094s
?   	github.com/gammons/slk/internal/launch	[no test files]
ok  	github.com/gammons/slk/internal/mention	1.010s
ok  	github.com/gammons/slk/internal/msgsync	8.135s
ok  	github.com/gammons/slk/internal/notify	1.046s
ok  	github.com/gammons/slk/internal/onboard	1.018s
?   	github.com/gammons/slk/internal/paths	[no test files]
ok  	github.com/gammons/slk/internal/searchq	1.012s
ok  	github.com/gammons/slk/internal/service	1.955s
ok  	github.com/gammons/slk/internal/slack	1.434s
ok  	github.com/gammons/slk/internal/slack/boot	1.114s
ok  	github.com/gammons/slk/internal/slack/edge	1.203s
ok  	github.com/gammons/slk/internal/slack/membership	4.355s
ok  	github.com/gammons/slk/internal/slack/mrkdwn	1.032s
ok  	github.com/gammons/slk/internal/slack/resolve	6.205s
?   	github.com/gammons/slk/internal/slack/slacktest	[no test files]
ok  	github.com/gammons/slk/internal/slackdesktop	1.081s
ok  	github.com/gammons/slk/internal/slackfmt	1.031s
ok  	github.com/gammons/slk/internal/slackhttp	1.273s
ok  	github.com/gammons/slk/internal/slackurl	1.013s
ok  	github.com/gammons/slk/internal/slashcmd	1.015s
ok  	github.com/gammons/slk/internal/text	1.022s
ok  	github.com/gammons/slk/internal/theme	1.018s
ok  	github.com/gammons/slk/internal/threadsubs	1.022s
ok  	github.com/gammons/slk/internal/typing	1.013s
ok  	github.com/gammons/slk/internal/usergroups	1.016s
ok  	github.com/gammons/slk/internal/version	1.016s
ok  	github.com/gammons/slk/gui	1.024s
ok  	github.com/gammons/slk/gui/backend	5.377s
ok  	github.com/gammons/slk/gui/chat	1.021s
ok  	github.com/gammons/slk/gui/cmd/slk-gio	10.567s
?   	github.com/gammons/slk/gui/internal/richtext	[no test files]
?   	github.com/gammons/slk/gui/internal/styledtext	[no test files]
ok  	github.com/gammons/slk/gui/motion	1.011s
ok  	github.com/gammons/slk/gui/notifications	1.031s
ok  	github.com/gammons/slk/gui/palette	1.011s
ok  	github.com/gammons/slk/gui/rich	1.022s
ok  	github.com/gammons/slk/gui/search	1.020s
ok  	github.com/gammons/slk/gui/selection	1.011s
ok  	github.com/gammons/slk/gui/sidebar	1.016s
ok  	github.com/gammons/slk/gui/textedit	1.011s
```

## Phase 6 — Docs

- Commit `1583d21e` — `docs: describe the CLI + GUI split`: README.md (slk-gio + the CLI; features from gui/README.md; install: the CLI as before, slk-gio from source with CI's apt list and `gui/shell.nix`'s libraries, run-time tools; Setup; Debugging: `SLK_DEBUG` for slk-gio — boot requests, gap size, session tally, FALLING BACK — `SLK_GIO_TRACE`, the CLI's log; `hero.gif` and the bubbletea/lipgloss sentence gone); AGENTS.md rewritten (below); gui/README.md (title without "(prototype)"; the custom-themes and `SLK_DEBUG` boot-requests lines were already there from 1.3 and 3.2; sentences about the TUI corrected: the token refresh, theme names, notifications keys, visits, the module note, the shared image cache, the layout list, `message_spans`); `.goreleaser.yaml` pre-release footer (`SLK_DEBUG=1 slk-gio 2>slk-gio.log`, grep for the boot and session tallies); `release.yml` comment (no darwin cgo build any more; runner unchanged).
- Commit `9bbb82ea` — `docs: wiki for slk-gio and the slk CLI`: `Architecture.md` (the new layout), `Home.md`, `_Sidebar.md`, `Features.md` (slk-gio, plus `slk export`), `Installation.md` (CLI packages, build from source, slk-gio prerequisites and run-time tools), `Setup.md`, `Tradeoffs-and-Non-Goals.md`, `Configuration.md` (keys slk-gio/slk read, ignored TUI keys, keys nothing reads, the old reference under a historical heading); `Keybindings.md`, `Commands.md`, `Terminal-Compatibility.md`, `Clipboard-and-OSC-52.md` get the historical header, content left.
- Commit `b5c76a3d` — `docs: specs index and the slk-gio feature backlog`: `docs/superpowers/specs/README.md` (all 95 design docs + the captures directory in the two lists, every plan likewise: 54 still useful, 41 archived, 2 current; checked by script that each file is listed once), `docs/superpowers/plans/2026-10-31-gui-feature-backlog.md`, `docs/reviewing-prs.md` points 7–8, `docs/STATUS.md` historical header, a `wiki/Features.md` fix (slk-gio does not show huddles).
- AGENTS.md rewrite: build block for both modules with the timings above (root ~4 s / ~33 s `-race`, gui ~3 s / ~12 s; `internal/cache` 31 s, `msgsync` 8 s, `resolve` 6 s; gui `cmd/slk-gio` 11 s, `backend` 5 s); the nix-shell note; the layout from `ls internal gui gui/cmd` (including `gui/sidebar`, `gui/textedit`, `gui/internal/*`, `internal/core`, `service`, `image`, … and the orphans marked); invariants: gui boundary test, `Backend` as the one port (a feature = method + Slack + Fake + tests + window), windows reached only through their inbox, boot never enumerates (`TestRun_NeverEnumerates`, `TestSlackAPI_DeclaresNoWorkspaceEnumeration`, `TestConversations_BootsWithoutEnumerating`), SQLite a cache with msgsync its only writer, the shared-map rule; shared-code tables regrouped (text and formatting; Slack, cache and sync; workspace setup and config; desktop integration; the Gio frontend), every row into `internal/ui`, `internal/bubbles`, the deleted `core` ports or deleted `cmd/slk` files removed, survivors re-pointed (`slackfmt`, `datefmt`, `theme`, `blockkit`, `bootstrap.Run` + `adapt`, `resolve`, `versionts`, `onboard`, `threadsubs` via `RefreshThreads`, `slashcmd` via the Gio composer, `PressAction`/`RunCommand` via `Backend`), new rows for `internal/image`, `emoji.EmojiCacheKey`, `ForwardMessage`, `GetUsersInConversationMax`, Gio's `clipboard.WriteCmd`, `notify.ShouldNotify`; every Gio row kept (one reworded: "an slk theme" for "a TUI theme"); rows whose code is now unused say so. Test helpers rewritten for `backend.NewFake`, `nextPosted`, `slacktest`, `bootSlack`, `gateTestHub`, `newComposeBox`/`TestComposer*`, `searchTestWindow`, the capture helpers; "Manual checks that cannot reach the user's desktop" with the recipe (scratchpad build, Xvfb `:97`, dead session bus, `GDK_DEBUG=no-portals`, scratch XDG dirs with tray and notifications off, stub `xdg-open`, `-fake -fake-every 0`, xdotool/ffmpeg/xclip, `pkill` of the scratch binary only) and the zenity leak it prevents; known duplication (msgsync, export's `GetChannels`, fake vs Slack lockstep); Conventions and Workflow kept, with Gio examples. A script checked every backticked identifier in the new AGENTS.md against `cmd/`, `internal/`, `gui/` (all found).

### Deviations (Phase 6)

1. Spec classification: the plan's two lists did not name `2026-04-23-slack-tui-design` (put under TUI-only), `inline-emoji-selector` (TUI-only), `compose-mentions` and `reconnection-and-new-message-landmark` (protocol: messages; cache/sync), and named `emoji-as-images` under protocol (kept there for its custom-emoji URL rules, though its plan directory is archived).
2. Plans: the plan's keep/archive lists were partial; the rest were sorted by whether their content is protocol/engine (kept) or the TUI's UI, rendering or refactors (archived). The list is in the specs README.
3. Beyond the plan's file list: `wiki/Setup.md` (it said running `slk` starts onboarding, that all workspaces stay connected with `:ws`/`1`–`9`, and that slk re-mints mid-session — none true now), `wiki/Tradeoffs-and-Non-Goals.md` (roadmap listed search and uploads as missing; image caveats were the terminal's), `wiki/_Sidebar.md`, `docs/reviewing-prs.md` (pointed reviewers at the `internal/ui` boundary and goldens), `docs/STATUS.md` (historical header).
4. Backlog: `Result.RevalidatedUsers` is recorded as done (Phase 3 did it), not as backlog. The plan's O2 and O3 are not defined anywhere in it; the backlog says so. O1 is done by 5b.
5. The two release-file edits are the ones named (pre-release footer; `release.yml` comment only).
6. README's install keeps Homebrew/AUR/`go install` for the CLI and says release packages carry only the CLI; slk-gio is from source (no package exists; O4).

### Found in the docs, contradicting the code, not resolved

- `internal/config/config.go`'s `GUITheme` comment and `internal/theme` comments still say "TUI theme name"; comments, left (listed in the backlog).
- `wiki/Configuration.md` keeps the terminal client's reference below a historical heading; its custom-theme section is current, the sections after the heading are not.
- The `docs/superpowers/specs/` and `plans/` documents themselves are unchanged and still describe `internal/ui` etc.; the specs README is the guide to which still apply.

### Verify (Phase 6) — run at `b5c76a3d`

```
$ test -f docs/superpowers/specs/README.md && test -f docs/superpowers/plans/2026-10-31-gui-feature-backlog.md; echo $?
0
$ grep -ci 'tui' README.md .goreleaser.yaml
README.md:0
.goreleaser.yaml:1
$ grep -ni 'tui' README.md .goreleaser.yaml
.goreleaser.yaml:14:# (macOS paste in the retired TUI, issue #156) and was retired with it. slk is
$ grep -c 'internal/ui\|internal/bubbles' AGENTS.md
0
$ grep -n 'bubbletea\|lipgloss' AGENTS.md | wc -l
0
$ grep -n 'gui/backend/fake.go\|slacktest' AGENTS.md
70:    slacktest/              fake Slack HTTP server for tests
142:  `slacktest` for the Slack side), and the window. Logic a window needs goes in
303:| The Gio frontend's conversation list and boot: Slack's own boot once per run (no enumeration; `users.conversations` only as the loud fallback when `client
326:| A whole made-up workspace behind `backend.Backend` (conversations, people, threads, usergroups, unjoined channels C20–C22, a `/standup` app with buttons, 
328:| A fake Slack HTTP server: canned bodies per path, every request recorded (form and raw body, since edgeapi posts JSON as `text/plain`), and a real `*slack
329:| A `backend.Slack` over a slacktest server with an empty in-memory cache, signed in, not booted (warnings captured) | `bootSlack(t, srv)` (`gui/backend/boo
$ grep -rn 'wiki/Architecture' README.md
README.md:147:  [Architecture](https://github.com/gammons/slk/wiki/Architecture)
```

The `gui/backend/fake.go|slacktest` and `wiki/Architecture` lines were cut at 160/200 columns when captured. `.goreleaser.yaml`'s one `tui` hit is the build comment "macOS paste in the retired TUI", which the plan allows. `yq` parses `.goreleaser.yaml` and `release.yml`.
