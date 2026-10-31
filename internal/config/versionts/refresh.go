package versionts

import (
	"context"
	"time"

	"github.com/gammons/slk/internal/config"
	slackclient "github.com/gammons/slk/internal/slack"
)

// shouldReloadTimeout bounds the background _x_version_ts refresh.
// Matches slackclient.MintToken's 15s rather than inventing a second
// number for the same job. Nothing waits on this refresh, so a generous
// bound costs nothing, while a tight one would turn a merely slow proxy
// into a lost refresh and a stale build timestamp.
const shouldReloadTimeout = 15 * time.Second

// Refresh refreshes the build timestamp (client.shouldReload), applies
// it to the client's envelope and saves it to configPath's block for
// the client's workspace. Run it in a goroutine after client.Connect,
// with a context that outlives the connect (it adds its own 15 s
// bound). Failure is non-fatal and goes to logf (nil discards): the
// seeded or compiled-in value stays in use.
//
// An empty configPath applies the value without saving it. A caller
// whose config.toml did not load passes "": Save edits the file
// textually, and rewriting a file slk could not read risks the user's
// own edits for one telemetry value.
func Refresh(ctx context.Context, client *slackclient.Client, cfg config.Config, configPath, teamName string, logf func(string, ...any)) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	// The API client's 60 s timeout bounds one request, but this runs
	// unattended: a server that accepts the connection and then never
	// answers (captive portal, wedged corporate proxy — see #111)
	// should cost no more than this bound.
	rctx, cancel := context.WithTimeout(ctx, shouldReloadTimeout)
	defer cancel()
	ts, err := client.ShouldReload(rctx)
	if err != nil {
		logf("shouldReload: %v", err)
		return
	}
	if env := client.Envelope(); env != nil {
		env.SetVersionTS(ts)
	}
	if configPath == "" {
		return
	}
	tomlKey := TOMLKey(cfg, client.TeamID())
	if err := Save(configPath, tomlKey, client.TeamID(), teamName, ts); err != nil {
		logf("saving version_ts: %v", err)
	}
}
