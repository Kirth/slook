# Design docs

One design doc per feature, written before it was built. slk's terminal
client (the TUI) was retired on 2026-10-31 (commit `44e6f30a`); its code is in
git history at that commit. The docs are left where they are and as they were,
and sorted here into two lists:

- **TUI-only** docs describe the terminal client's own UI and rendering. They
  are archived: nothing in the tree implements them now.
- **Protocol and engine** docs record how Slack behaves, what the official
  clients send, and how slk's cache and sync work. Their protocol findings are
  still authoritative even where they describe the TUI's screens; slk-gio and
  the `internal/` packages build on them. Read one before changing the
  behaviour it covers.

New design docs go here as `YYYY-MM-DD-<topic>-design.md`; add each to a list
below. Implementation plans are in `../plans/` (sorted the same way at the end
of this page).

## Protocol and engine knowledge (still authoritative)


### Auth and sign-in

- [2026-04-24-browser-cookie-auth.md](2026-04-24-browser-cookie-auth.md)
- [2026-07-23-desktop-app-auth-design.md](2026-07-23-desktop-app-auth-design.md)
- [2026-09-30-browser-session-login-design.md](2026-09-30-browser-session-login-design.md)

### Boot and Enterprise Grid

- [2026-07-30-enterprise-grid-bootstrap-design.md](2026-07-30-enterprise-grid-bootstrap-design.md)
- [2026-07-31-grid-parity-phase2b-design.md](2026-07-31-grid-parity-phase2b-design.md)
- [2026-08-05-edge-health-and-batched-resolver-design.md](2026-08-05-edge-health-and-batched-resolver-design.md)
- [2026-08-05-grid-edge-team-scoping-design.md](2026-08-05-grid-edge-team-scoping-design.md)

### Cache, sync and read state

- [2026-05-03-cache-first-message-loading-design.md](2026-05-03-cache-first-message-loading-design.md)
- [2026-05-10-messages-cache-integrity-design.md](2026-05-10-messages-cache-integrity-design.md)
- [2026-05-11-reconnect-backfill-and-threads-sort-design.md](2026-05-11-reconnect-backfill-and-threads-sort-design.md)
- [2026-04-26-reconnection-and-new-message-landmark.md](2026-04-26-reconnection-and-new-message-landmark.md)
- [2026-05-17-read-state-sync-design.md](2026-05-17-read-state-sync-design.md)
- [2026-09-04-focus-gated-read-marking-design.md](2026-09-04-focus-gated-read-marking-design.md)
- [2026-05-01-mark-unread-design.md](2026-05-01-mark-unread-design.md)
- [2026-10-29-message-cache-sync-design.md](2026-10-29-message-cache-sync-design.md)

### Threads

- [2026-04-28-threads-view-design.md](2026-04-28-threads-view-design.md)
- [2026-05-12-subscriptions-thread-design.md](2026-05-12-subscriptions-thread-design.md)
- [2026-10-29-gui-threads-design.md](2026-10-29-gui-threads-design.md)

### Messages

- [2026-04-30-block-kit-rendering-design.md](2026-04-30-block-kit-rendering-design.md)
- [2026-05-01-commonmark-compose-design.md](2026-05-01-commonmark-compose-design.md)
- [2026-04-26-compose-mentions-design.md](2026-04-26-compose-mentions-design.md)
- [2026-10-09-interactive-ephemeral-messages-design.md](2026-10-09-interactive-ephemeral-messages-design.md)
- [2026-10-10-ephemeral-action-dialog-design.md](2026-10-10-ephemeral-action-dialog-design.md)
- [2026-10-12-slash-commands-design.md](2026-10-12-slash-commands-design.md)
- [2026-10-12-slash-commands-captures](2026-10-12-slash-commands-captures/)
- [2026-10-12-app-modals-research.md](2026-10-12-app-modals-research.md)
- [2026-09-23-message-forwarding-design.md](2026-09-23-message-forwarding-design.md)
- [2026-04-29-paste-image-upload-design.md](2026-04-29-paste-image-upload-design.md)
- [2026-08-18-file-attachment-downloads-design.md](2026-08-18-file-attachment-downloads-design.md)
- [2026-04-28-copy-message-permalink-design.md](2026-04-28-copy-message-permalink-design.md)
- [2026-06-09-open-links-keybinding-design.md](2026-06-09-open-links-keybinding-design.md)
- [2026-04-29-edit-delete-messages-design.md](2026-04-29-edit-delete-messages-design.md)
- [2026-06-08-list-message-reactions-design.md](2026-06-08-list-message-reactions-design.md)
- [2026-04-26-reaction-picker-design.md](2026-04-26-reaction-picker-design.md)
- [2026-05-27-emoji-as-images-design.md](2026-05-27-emoji-as-images-design.md)

### People

- [2026-04-29-presence-and-dnd-design.md](2026-04-29-presence-and-dnd-design.md)
- [2026-04-26-typing-indicators-design.md](2026-04-26-typing-indicators-design.md)
- [2026-09-30-user-profile-dialog-design.md](2026-09-30-user-profile-dialog-design.md)
- [2026-10-05-external-user-cache-design.md](2026-10-05-external-user-cache-design.md)
- [2026-10-05-external-user-notifications-design.md](2026-10-05-external-user-notifications-design.md)
- [2026-10-05-profile-status-retries-design.md](2026-10-05-profile-status-retries-design.md)
- [2026-10-05-bot-classification-concurrency-design.md](2026-10-05-bot-classification-concurrency-design.md)
- [2026-10-05-sized-avatar-urls-design.md](2026-10-05-sized-avatar-urls-design.md)
- [2026-05-20-mention-picker-channel-membership-design.md](2026-05-20-mention-picker-channel-membership-design.md)
- [2026-10-05-sidebar-starred-people-design.md](2026-10-05-sidebar-starred-people-design.md)

### Workspace, sidebar and the rest

- [2026-05-02-slack-native-sections-design.md](2026-05-02-slack-native-sections-design.md)
- [2026-04-30-per-workspace-config-sections-design.md](2026-04-30-per-workspace-config-sections-design.md)
- [2026-05-01-workspace-ordering-design.md](2026-05-01-workspace-ordering-design.md)
- [2026-04-26-workspace-switching.md](2026-04-26-workspace-switching.md)
- [2026-10-10-workspace-ready-snapshot-design.md](2026-10-10-workspace-ready-snapshot-design.md)
- [2026-05-25-new-message-feature-design.md](2026-05-25-new-message-feature-design.md)
- [2026-09-09-mention-badges-design.md](2026-09-09-mention-badges-design.md)
- [2026-04-26-desktop-notifications-design.md](2026-04-26-desktop-notifications-design.md)
- [2026-10-12-activity-reminders-read-state-design.md](2026-10-12-activity-reminders-read-state-design.md)
- [2026-10-14-huddle-links-design.md](2026-10-14-huddle-links-design.md)
- [2026-09-17-channel-export-design.md](2026-09-17-channel-export-design.md)
- [2026-06-10-search-design.md](2026-06-10-search-design.md)
- [2026-05-19-accent-insensitive-matching-design.md](2026-05-19-accent-insensitive-matching-design.md)
- [2026-05-09-comprehensive-debug-logging-design.md](2026-05-09-comprehensive-debug-logging-design.md)

## TUI-only (archived)

- [2026-04-23-slack-tui-design.md](2026-04-23-slack-tui-design.md)
- [2026-04-25-fuzzy-channel-finder.md](2026-04-25-fuzzy-channel-finder.md)
- [2026-04-25-qol-ui-fixes.md](2026-04-25-qol-ui-fixes.md)
- [2026-04-25-reflow-viewport-refactor.md](2026-04-25-reflow-viewport-refactor.md)
- [2026-04-25-thread-panel-design.md](2026-04-25-thread-panel-design.md)
- [2026-04-26-customizable-themes-design.md](2026-04-26-customizable-themes-design.md)
- [2026-04-27-emoji-width-findings.md](2026-04-27-emoji-width-findings.md)
- [2026-04-27-emoji-width-investigation.md](2026-04-27-emoji-width-investigation.md)
- [2026-04-27-emoji-width-probing-design.md](2026-04-27-emoji-width-probing-design.md)
- [2026-04-27-lipgloss-v2-migration-design.md](2026-04-27-lipgloss-v2-migration-design.md)
- [2026-04-27-ui-fixes-batch.md](2026-04-27-ui-fixes-batch.md)
- [2026-04-28-inline-emoji-selector-design.md](2026-04-28-inline-emoji-selector-design.md)
- [2026-04-28-message-text-selection-design.md](2026-04-28-message-text-selection-design.md)
- [2026-04-28-themes-and-per-workspace-design.md](2026-04-28-themes-and-per-workspace-design.md)
- [2026-04-29-inline-image-rendering-design.md](2026-04-29-inline-image-rendering-design.md)
- [2026-05-02-mode-and-selection-and-thread-images-design.md](2026-05-02-mode-and-selection-and-thread-images-design.md)
- [2026-05-03-1.0-readiness-design.md](2026-05-03-1.0-readiness-design.md)
- [2026-05-09-channel-history-and-recency-design.md](2026-05-09-channel-history-and-recency-design.md)
- [2026-05-21-tab-title-unread-indicator-design.md](2026-05-21-tab-title-unread-indicator-design.md)
- [2026-05-24-emoji-shortcode-fallback-design.md](2026-05-24-emoji-shortcode-fallback-design.md)
- [2026-05-25-ansi-palette-themes-design.md](2026-05-25-ansi-palette-themes-design.md)
- [2026-05-27-version-and-attribution-design.md](2026-05-27-version-and-attribution-design.md)
- [2026-06-07-cursor-follows-scroll-design.md](2026-06-07-cursor-follows-scroll-design.md)
- [2026-06-07-more-themes-design.md](2026-06-07-more-themes-design.md)
- [2026-06-11-window-management-design.md](2026-06-11-window-management-design.md)
- [2026-09-06-phase0-test-safety-net-design.md](2026-09-06-phase0-test-safety-net-design.md)
- [2026-09-22-phase1-main-go-splits-design.md](2026-09-22-phase1-main-go-splits-design.md)
- [2026-09-23-macos-clipboard-design.md](2026-09-23-macos-clipboard-design.md)
- [2026-09-24-thread-stacked-layout-design.md](2026-09-24-thread-stacked-layout-design.md)
- [2026-09-27-demo-mode-design.md](2026-09-27-demo-mode-design.md)
- [2026-09-30-selected-message-timestamp-design.md](2026-09-30-selected-message-timestamp-design.md)
- [2026-10-05-conversation-finder-sync-design.md](2026-10-05-conversation-finder-sync-design.md)
- [2026-10-05-sidebar-cursor-identity-design.md](2026-10-05-sidebar-cursor-identity-design.md)
- [2026-10-10-phase3-messages-thread-fork-design.md](2026-10-10-phase3-messages-thread-fork-design.md)
- [2026-10-12-phase4-modal-chrome-design.md](2026-10-12-phase4-modal-chrome-design.md)

## Implementation plans (`../plans/`)

Current: [2026-10-31-tui-sunset-log.md](../plans/2026-10-31-tui-sunset-log.md)
(the migration, with every deviation and follow-up) and
[2026-10-31-gui-feature-backlog.md](../plans/2026-10-31-gui-feature-backlog.md)
(what was not ported, orphans, dead config keys, checks owed).

### Still useful (protocol, engine, Enterprise Grid)

- [2026-04-24-browser-cookie-auth.md](../plans/2026-04-24-browser-cookie-auth.md)
- [2026-04-26-compose-mentions.md](../plans/2026-04-26-compose-mentions.md)
- [2026-04-26-desktop-notifications.md](../plans/2026-04-26-desktop-notifications.md)
- [2026-04-26-reaction-picker.md](../plans/2026-04-26-reaction-picker.md)
- [2026-04-26-reconnection-and-landmark.md](../plans/2026-04-26-reconnection-and-landmark.md)
- [2026-04-26-typing-indicators.md](../plans/2026-04-26-typing-indicators.md)
- [2026-04-26-workspace-switching.md](../plans/2026-04-26-workspace-switching.md)
- [2026-04-28-copy-message-permalink.md](../plans/2026-04-28-copy-message-permalink.md)
- [2026-04-28-threads-view.md](../plans/2026-04-28-threads-view.md)
- [2026-04-29-edit-delete-messages.md](../plans/2026-04-29-edit-delete-messages.md)
- [2026-04-29-paste-image-upload.md](../plans/2026-04-29-paste-image-upload.md)
- [2026-04-29-presence-and-dnd.md](../plans/2026-04-29-presence-and-dnd.md)
- [2026-04-30-block-kit-rendering](../plans/2026-04-30-block-kit-rendering/)
- [2026-04-30-per-workspace-config-sections.md](../plans/2026-04-30-per-workspace-config-sections.md)
- [2026-05-01-commonmark-compose.md](../plans/2026-05-01-commonmark-compose.md)
- [2026-05-01-mark-unread.md](../plans/2026-05-01-mark-unread.md)
- [2026-05-01-mpdm-unread-indicator.md](../plans/2026-05-01-mpdm-unread-indicator.md)
- [2026-05-01-workspace-ordering.md](../plans/2026-05-01-workspace-ordering.md)
- [2026-05-02-slack-native-sections.md](../plans/2026-05-02-slack-native-sections.md)
- [2026-05-03-cache-first-message-loading.md](../plans/2026-05-03-cache-first-message-loading.md)
- [2026-05-09-debug-logging-cache.md](../plans/2026-05-09-debug-logging-cache.md)
- [2026-05-09-debug-logging-foundation.md](../plans/2026-05-09-debug-logging-foundation.md)
- [2026-05-10-messages-cache-integrity.md](../plans/2026-05-10-messages-cache-integrity.md)
- [2026-05-11-reconnect-backfill-and-threads-sort.md](../plans/2026-05-11-reconnect-backfill-and-threads-sort.md)
- [2026-05-12-subscriptions-thread.md](../plans/2026-05-12-subscriptions-thread.md)
- [2026-05-17-read-state-sync.md](../plans/2026-05-17-read-state-sync.md)
- [2026-05-19-accent-insensitive-matching.md](../plans/2026-05-19-accent-insensitive-matching.md)
- [2026-05-20-browser-like-headers-verification-notes.md](../plans/2026-05-20-browser-like-headers-verification-notes.md)
- [2026-05-20-browser-like-headers.md](../plans/2026-05-20-browser-like-headers.md)
- [2026-05-25-new-message-feature](../plans/2026-05-25-new-message-feature/)
- [2026-05-25-new-message-feature.md](../plans/2026-05-25-new-message-feature.md)
- [2026-06-08-list-message-reactions.md](../plans/2026-06-08-list-message-reactions.md)
- [2026-06-09-open-links-keybinding.md](../plans/2026-06-09-open-links-keybinding.md)
- [2026-06-10-search.md](../plans/2026-06-10-search.md)
- [2026-07-23-desktop-app-auth.md](../plans/2026-07-23-desktop-app-auth.md)
- [2026-07-30-grid-parity-phase1-outcomes.md](../plans/2026-07-30-grid-parity-phase1-outcomes.md)
- [2026-07-30-grid-parity-phase1-request-envelope.md](../plans/2026-07-30-grid-parity-phase1-request-envelope.md)
- [2026-07-30-grid-parity-phase2a-foundations.md](../plans/2026-07-30-grid-parity-phase2a-foundations.md)
- [2026-07-30-grid-parity-phase2a-outcomes.md](../plans/2026-07-30-grid-parity-phase2a-outcomes.md)
- [2026-07-31-grid-parity-phase2b-bootstrap-rewrite.md](../plans/2026-07-31-grid-parity-phase2b-bootstrap-rewrite.md)
- [2026-07-31-grid-parity-phase2b-outcomes.md](../plans/2026-07-31-grid-parity-phase2b-outcomes.md)
- [2026-08-01-grid-parity-STATE.md](../plans/2026-08-01-grid-parity-STATE.md)
- [2026-08-03-grid-parity-qa-checklist.md](../plans/2026-08-03-grid-parity-qa-checklist.md)
- [2026-08-05-edge-health-and-batched-resolver.md](../plans/2026-08-05-edge-health-and-batched-resolver.md)
- [2026-08-05-grid-edge-team-scoping.md](../plans/2026-08-05-grid-edge-team-scoping.md)
- [2026-08-05-next-session-handoff.md](../plans/2026-08-05-next-session-handoff.md)
- [2026-08-18-file-attachment-downloads.md](../plans/2026-08-18-file-attachment-downloads.md)
- [2026-09-04-focus-gated-read-marking.md](../plans/2026-09-04-focus-gated-read-marking.md)
- [2026-09-09-mention-badges.md](../plans/2026-09-09-mention-badges.md)
- [2026-10-05-sized-avatar-urls.md](../plans/2026-10-05-sized-avatar-urls.md)
- [2026-10-09-interactive-ephemeral-messages-part-a.md](../plans/2026-10-09-interactive-ephemeral-messages-part-a.md)
- [2026-10-10-ephemeral-action-dialog.md](../plans/2026-10-10-ephemeral-action-dialog.md)
- [2026-10-12-activity-reminders-read-state.md](../plans/2026-10-12-activity-reminders-read-state.md)
- [2026-10-12-slash-commands-stage1.md](../plans/2026-10-12-slash-commands-stage1.md)

### Archived (the terminal client's UI, rendering and refactors)

- [2026-04-23-slack-tui-mvp.md](../plans/2026-04-23-slack-tui-mvp.md)
- [2026-04-25-fuzzy-channel-finder.md](../plans/2026-04-25-fuzzy-channel-finder.md)
- [2026-04-25-qol-ui-fixes.md](../plans/2026-04-25-qol-ui-fixes.md)
- [2026-04-25-reflow-viewport-refactor.md](../plans/2026-04-25-reflow-viewport-refactor.md)
- [2026-04-25-thread-panel.md](../plans/2026-04-25-thread-panel.md)
- [2026-04-26-customizable-themes.md](../plans/2026-04-26-customizable-themes.md)
- [2026-04-27-customizable-themes.md](../plans/2026-04-27-customizable-themes.md)
- [2026-04-27-displaywidth-fix.md](../plans/2026-04-27-displaywidth-fix.md)
- [2026-04-27-emoji-width-probing.md](../plans/2026-04-27-emoji-width-probing.md)
- [2026-04-27-lipgloss-v2-migration.md](../plans/2026-04-27-lipgloss-v2-migration.md)
- [2026-04-27-ui-fixes-batch.md](../plans/2026-04-27-ui-fixes-batch.md)
- [2026-04-28-inline-emoji-selector.md](../plans/2026-04-28-inline-emoji-selector.md)
- [2026-04-28-message-text-selection.md](../plans/2026-04-28-message-text-selection.md)
- [2026-04-28-themes-and-per-workspace.md](../plans/2026-04-28-themes-and-per-workspace.md)
- [2026-04-29-inline-images](../plans/2026-04-29-inline-images/)
- [2026-04-30-thread-perf-fixes.md](../plans/2026-04-30-thread-perf-fixes.md)
- [2026-05-01-image-pipeline-followups.md](../plans/2026-05-01-image-pipeline-followups.md)
- [2026-05-01-image-pipeline-perf.md](../plans/2026-05-01-image-pipeline-perf.md)
- [2026-05-02-mode-and-selection-and-thread-images.md](../plans/2026-05-02-mode-and-selection-and-thread-images.md)
- [2026-05-09-channel-history-and-recency.md](../plans/2026-05-09-channel-history-and-recency.md)
- [2026-05-09-debug-logging-images.md](../plans/2026-05-09-debug-logging-images.md)
- [2026-05-21-tab-title-unread-indicator.md](../plans/2026-05-21-tab-title-unread-indicator.md)
- [2026-05-23-app-go-solid-refactor.md](../plans/2026-05-23-app-go-solid-refactor.md)
- [2026-05-24-emoji-shortcode-fallback.md](../plans/2026-05-24-emoji-shortcode-fallback.md)
- [2026-05-25-ansi-palette-themes.md](../plans/2026-05-25-ansi-palette-themes.md)
- [2026-05-27-emoji-as-images](../plans/2026-05-27-emoji-as-images/)
- [2026-05-27-version-and-attribution.md](../plans/2026-05-27-version-and-attribution.md)
- [2026-06-07-more-themes.md](../plans/2026-06-07-more-themes.md)
- [2026-06-11-window-management-phase1-command-mode.md](../plans/2026-06-11-window-management-phase1-command-mode.md)
- [2026-06-11-window-management-phase2-window-tree.md](../plans/2026-06-11-window-management-phase2-window-tree.md)
- [2026-06-11-window-management-phase3-per-window-models.md](../plans/2026-06-11-window-management-phase3-per-window-models.md)
- [2026-09-06-architecture-refactor.md](../plans/2026-09-06-architecture-refactor.md)
- [2026-09-06-phase0-test-safety-net.md](../plans/2026-09-06-phase0-test-safety-net.md)
- [2026-09-22-phase1-main-go-splits.md](../plans/2026-09-22-phase1-main-go-splits.md)
- [2026-09-24-thread-stacked-layout.md](../plans/2026-09-24-thread-stacked-layout.md)
- [2026-09-27-demo-mode.md](../plans/2026-09-27-demo-mode.md)
- [2026-09-30-selected-message-timestamp.md](../plans/2026-09-30-selected-message-timestamp.md)
- [2026-09-30-user-profile-dialog.md](../plans/2026-09-30-user-profile-dialog.md)
- [2026-10-11-phase3-messages-thread-fork.md](../plans/2026-10-11-phase3-messages-thread-fork.md)
- [2026-10-12-phase4-modal-chrome.md](../plans/2026-10-12-phase4-modal-chrome.md)
- [phase3-baseline](../plans/phase3-baseline/)
