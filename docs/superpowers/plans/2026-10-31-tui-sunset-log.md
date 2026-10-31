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
