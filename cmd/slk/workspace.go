package main

// What is left of the TUI's per-workspace state (WorkspaceContext,
// workspaceRouter): the last-visited channel choice, kept for Phase 3 of
// the TUI sunset (docs/superpowers/plans/2026-10-31-tui-sunset-log.md),
// where the GUI's boot opens that channel's conversations.view.

// mostRecentlyVisitedChannel returns the channel ID with the latest
// last-visited timestamp, or "" when there are no recorded visits (e.g.
// the first run). Drives last-channel restoration on startup.
func mostRecentlyVisitedChannel(visits map[string]int64) string {
	var bestID string
	var bestTS int64
	for id, ts := range visits {
		if ts > bestTS {
			bestTS = ts
			bestID = id
		}
	}
	return bestID
}
