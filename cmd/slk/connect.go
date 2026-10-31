package main

// What is left of the TUI's workspace connect (connectWorkspace): the
// _x_version_ts upkeep, kept for Phase 3 of the TUI sunset
// (docs/superpowers/plans/2026-10-31-tui-sunset-log.md), which moves it
// to internal/config/versionts as Refresh. connectWorkspace seeded the
// envelope with seedVersionTS before client.Connect and then ran the
// body of refreshVersionTS in a goroutine.

import (
	"context"
	"time"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/debuglog"
	slackclient "github.com/gammons/slk/internal/slack"
)

// shouldReloadTimeout bounds the background _x_version_ts refresh.
// Matches slackclient.MintToken's 15s — the only other bounded Slack
// HTTP call in slk — rather than inventing a second number for the
// same job. Nothing waits on this refresh, so a generous bound costs
// nothing, while a tight one would turn a merely slow proxy into a
// lost refresh and a stale build timestamp.
const shouldReloadTimeout = 15 * time.Second

// refreshVersionTS refreshes the build timestamp (client.shouldReload),
// applies it to the client's envelope and saves it to config.toml.
// Failure is non-fatal: the seeded or compiled-in value stays in use.
// Run it in a goroutine after client.Connect.
func refreshVersionTS(ctx context.Context, client *slackclient.Client, cfg config.Config, configPath, teamName string) {
	// The API client sets no http.Client.Timeout, and
	// http.DefaultTransport bounds only the dial and the TLS
	// handshake — not the response headers or body. A server that
	// accepts the connection and then never answers (captive
	// portal, wedged corporate proxy — see #111) would otherwise
	// pin this goroutine and its connection for the whole life of
	// the process, because ctx here is the app root context.
	rctx, cancel := context.WithTimeout(ctx, shouldReloadTimeout)
	defer cancel()
	ts, err := client.ShouldReload(rctx)
	if err != nil {
		debuglog.General("shouldReload: %v", err)
		return
	}
	if env := client.Envelope(); env != nil {
		env.SetVersionTS(ts)
	}
	tomlKey := workspaceTOMLKey(cfg, client.TeamID())
	if err := saveWorkspaceVersionTS(configPath, tomlKey, client.TeamID(), teamName, ts); err != nil {
		debuglog.General("saving version_ts: %v", err)
	}
}
