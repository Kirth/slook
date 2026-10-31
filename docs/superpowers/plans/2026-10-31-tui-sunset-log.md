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
