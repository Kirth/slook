// Command slk-gio is a prototype graphical frontend for slk, built with
// Gio. Every conversation opens in a window of its own; the conversation
// list is one more window. With a tray icon (on Linux, unless [gui] tray
// = false or the desktop has no tray) the program keeps running when the
// last window closes, and the icon opens the list again; without one it
// exits. A second launch for the same workspace brings the running one's
// list forward and exits.
//
//	slk-gio            the first workspace added with slk --add-workspace
//	slk-gio -team NAME a workspace by team ID or name
//	slk-gio -fake      made-up conversations, no Slack account needed
//	slk-gio -fake -fake-every 0
//	                   the same, without someone posting every few seconds
//	slk-gio -theme ink a palette for this run: dark, ink, light, or any slk theme
//	slk-gio -backfill 1y
//	                   fill the local message cache back a year, slowly,
//	                   for search; by default [cache] backfill, else 90 days
//	                   ("off" to skip)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/app"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/notifications"
	"github.com/gammons/slk/gui/palette"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/onboard"
	"github.com/gammons/slk/internal/paths"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/slackdesktop"
	"github.com/gammons/slk/internal/slackfmt"
	"github.com/gammons/slk/internal/slackhttp"
	"github.com/gammons/slk/internal/theme"
)

// connectTimeout bounds sign-in, so a wedged network fails visibly.
const connectTimeout = 30 * time.Second

// remintTimeout bounds the startup token refresh from the desktop app's
// session; on expiry the stored tokens are used.
const remintTimeout = 10 * time.Second

func main() {
	fake := flag.Bool("fake", false, "use made-up conversations instead of Slack")
	fakeEvery := flag.Duration("fake-every", 4*time.Second, "with -fake, how often someone posts (0 for never)")
	team := flag.String("team", "", "workspace to open, by team ID or name (default: the first one)")
	backfill := flag.String("backfill", "", "fill the message cache back this far in the background (90d, 1y, all, off); default: [cache] backfill in config.toml, else 90d")
	themeName := flag.String("theme", "", "palette: dark, ink, light, or any slk theme name (default: [gui.theme] in config.toml)")
	flag.Parse()

	// slk's config: the GUI's colours, and the conversations the
	// backfill skips. Missing is fine (defaults); unreadable is reported
	// and treated the same.
	configPath := filepath.Join(paths.Config(), "config.toml")
	cfg, cfgErr := config.Load(configPath)
	if cfgErr != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: config:", cfgErr)
	}
	// The search backfill runs from startup, as far back as [cache]
	// backfill says (90 days unset); -backfill overrides it.
	horizon, doBackfill, err := cfg.Cache.BackfillOnStart()
	if err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: [cache] backfill:", err, "(not filling)")
		doBackfill = false
	}
	if *backfill != "" {
		if horizon, doBackfill, err = config.ParseBackfill(*backfill); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: -backfill:", err)
			os.Exit(2)
		}
	}
	// <!date tokens' {time} follows appearance.timestamp_format, as in
	// the TUI.
	slackfmt.SetDateTokenTimeLayout(cfg.Appearance.TimestampFormat)
	// The user's own theme files (config/themes/*.toml), so a custom
	// theme name works in [gui.theme] and -theme. A missing directory
	// is no themes.
	theme.LoadCustom(os.DirFS(filepath.Join(paths.Config(), "themes")))
	name, colors := cfg.GUI.Theme.Settings()
	if *themeName != "" {
		name = *themeName
	}
	p, warns := palette.Resolve(name, colors)
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, "slk-gio: theme:", w)
	}
	applyPalette(p) // before any theme or window is built

	// Parse the fonts and build the first window's theme while signing
	// in, so no window waits for them.
	refillThemes()
	go startPreviewer()

	// One slk-gio per workspace: a second launch brings the running
	// one's list forward and exits, before signing in.
	var tok slackclient.Token
	instanceID := "fake"
	if !*fake {
		if tok, err = pickToken(remintStored(slackclient.NewTokenStore(filepath.Join(paths.Data(), "tokens"))), *team); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio:", err)
			os.Exit(1)
		}
		instanceID = tok.TeamID
	}
	var running atomic.Pointer[hub]
	if !claimInstance(instanceID, func() {
		if h := running.Load(); h != nil {
			h.showList()
		}
	}) {
		fmt.Fprintln(os.Stderr, "slk-gio: already running; showing its list")
		os.Exit(0)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		be, err := openBackend(ctx, *fake, *fakeEvery, tok, backend.ConnectOptions{
			Config: cfg, ConfigLoadErr: cfgErr, ConfigPath: configPath, Log: debugf,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio:", err)
			os.Exit(1)
		}
		var tray atomic.Bool
		h := newHub(ctx, be, lastWindowClosed(tray.Load, func(code int) {
			cancel()
			reportRequests()
			os.Exit(code)
		}))
		// The tray keeps slk-gio running with no window open.
		if cfg.GUI.Tray {
			tray.Store(startTray(h, func() {
				cancel()
				reportRequests()
				os.Exit(0)
			}))
		}
		running.Store(h)
		// Desktop notifications for mentions and DMs while no window
		// has the keyboard; set before any window or event exists.
		h.people() // start reading who @ completes to
		if cfg.Notifications.Enabled {
			h.notes = notifications.New(cfg.Notifications, be.Self(), notifications.NewSink(cfg.Notifications, h.openNote))
		}
		h.seedGroups()
		h.typingOn = cfg.Animations.TypingIndicators
		if s, ok := be.(*backend.Slack); ok {
			s.SetPaused(h.anyFocused)
			s.SetBackfillIgnore(cfg.Cache.IgnoresBackfill)
			s.SetCatchUp(cfg.Cache.CatchUpOnConnect(), cfg.Cache.CatchUpMax())
			if doBackfill {
				s.EnableBackfill(horizon)
			}
		}
		h.showList()
		go h.loadSelf() // the list's own presence, once
		go func() {
			if err := be.Run(ctx, h.dispatch); err != nil {
				fmt.Fprintln(os.Stderr, "slk-gio:", err)
			}
		}()
	}()
	app.Main()
}

// lastWindowClosed is the hub's onLast: with the tray up (tray reports
// it) the program keeps running, the tray its way back; otherwise it
// exits, 1 if the window failed.
func lastWindowClosed(tray func() bool, exit func(code int)) func(error) {
	return func(err error) {
		switch {
		case err != nil:
			exit(1)
		case !tray():
			exit(0)
		}
	}
}

// debugf is the Slack backend's diagnostics: stderr, with SLK_DEBUG set.
func debugf(format string, args ...any) {
	if os.Getenv("SLK_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "slk-gio: "+format+"\n", args...)
	}
}

// reportRequests prints the session's Slack API requests by endpoint
// (slackhttp.DefaultCounter) on the way out, with SLK_DEBUG set.
func reportRequests() {
	if os.Getenv("SLK_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "slk-gio: requests this session:\n%s", slackhttp.DefaultCounter.Report())
	}
}

func openBackend(ctx context.Context, fake bool, fakeEvery time.Duration, tok slackclient.Token, opts backend.ConnectOptions) (backend.Backend, error) {
	if fake {
		f := backend.NewFake(time.Now())
		f.Interval = fakeEvery
		return f, nil
	}
	// The cache (shared with the TUI) seeds user names and keeps the
	// user's channel visits, which order the list. It is created if slk
	// never ran; without it the list still works, without visits.
	db, err := cache.New(filepath.Join(paths.Data(), "cache.db"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: cache:", err)
		db = nil
	}
	cctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	be, err := backend.ConnectSlack(cctx, tok, db, paths.Cache(), opts)
	if err != nil && db != nil {
		db.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("%w (if the session expired, sign in to the Slack desktop app and start slk-gio again, or run slk --add-workspace)", err)
	}
	return be, nil
}

// storedTokens is the token store's contents after a re-mint, or the
// error listing it failed with.
type storedTokens struct {
	toks []slackclient.Token
	err  error
}

// remintStored refreshes every stored token from the Slack desktop app's
// live session, as slk did at each launch: xoxc tokens expire, and the
// desktop app's session is the source of truth. A token that cannot be
// refreshed (no desktop app, offline, a browser-session token) is used as
// stored; onboard logs why to stderr.
func remintStored(store *slackclient.TokenStore) storedTokens {
	ctx, cancel := context.WithTimeout(context.Background(), remintTimeout)
	defer cancel()
	toks, err := onboard.Remint(ctx, store, slackclient.MintToken)
	return storedTokens{toks, err}
}

func pickToken(stored storedTokens, team string) (slackclient.Token, error) {
	toks, err := stored.toks, stored.err
	if err != nil {
		return slackclient.Token{}, fmt.Errorf("reading workspaces: %w", err)
	}
	if len(toks) == 0 {
		msg := "no workspaces yet: run slk --add-workspace, or try slk-gio -fake"
		// Say why the desktop app's session is no help, when it is not.
		if _, derr := slackdesktop.Cookie(); derr != nil {
			msg += "\n" + onboard.DesktopErrorMessage(derr)
		}
		return slackclient.Token{}, errors.New(msg)
	}
	if team == "" {
		return toks[0], nil
	}
	var names []string
	for _, t := range toks {
		if t.TeamID == team || strings.EqualFold(t.TeamName, team) {
			return t, nil
		}
		names = append(names, t.TeamName)
	}
	return slackclient.Token{}, fmt.Errorf("no workspace %q (have: %s)", team, strings.Join(names, ", "))
}
