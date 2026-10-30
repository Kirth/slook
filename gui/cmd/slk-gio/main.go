// Command slk-gio is a prototype graphical frontend for slk, built with
// Gio. Every conversation opens in a window of its own; the conversation
// list is one more window. The program exits when the last window closes.
//
//	slk-gio            the first workspace added with slk --add-workspace
//	slk-gio -team NAME a workspace by team ID or name
//	slk-gio -fake      made-up conversations, no Slack account needed
//	slk-gio -fake -fake-every 0
//	                   the same, without someone posting every few seconds
//	slk-gio -theme ink a palette for this run: dark, ink, light, or any slk theme
//	                   the same, without someone posting every few seconds
//	slk-gio -backfill 1y
//	                   also fill the local message cache back a year, slowly
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/notifications"
	"github.com/gammons/slk/gui/palette"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/paths"
	slackclient "github.com/gammons/slk/internal/slack"
)

// connectTimeout bounds sign-in, so a wedged network fails visibly.
const connectTimeout = 30 * time.Second

func main() {
	fake := flag.Bool("fake", false, "use made-up conversations instead of Slack")
	fakeEvery := flag.Duration("fake-every", 4*time.Second, "with -fake, how often someone posts (0 for never)")
	team := flag.String("team", "", "workspace to open, by team ID or name (default: the first one)")
	backfill := flag.String("backfill", "", "fill the message cache back this far in the background (1y, 90d, 0 for everything); off when absent")
	themeName := flag.String("theme", "", "palette: dark, ink, light, or any slk theme name (default: [gui.theme] in config.toml)")
	flag.Parse()
	horizon, doBackfill, err := parseBackfill(*backfill)
	if err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: -backfill:", err)
		os.Exit(2)
	}

	// slk's config: the GUI's colours, and the conversations the
	// backfill skips. Missing is fine (defaults); unreadable is reported
	// and treated the same.
	cfg, err := config.Load(filepath.Join(paths.Config(), "config.toml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: config:", err)
	}
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

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		be, err := openBackend(ctx, *fake, *fakeEvery, *team)
		if err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio:", err)
			os.Exit(1)
		}
		h := newHub(ctx, be, func(err error) {
			cancel()
			if err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		})
		// Desktop notifications for mentions and DMs while no window
		// has the keyboard; set before any window or event exists.
		h.people() // start reading who @ completes to
		if cfg.Notifications.Enabled {
			h.notes = notifications.New(cfg.Notifications, be.Self(), notifications.NewSink(cfg.Notifications, h.openNote))
		}
		h.seedGroups()
		if s, ok := be.(*backend.Slack); ok {
			s.SetPaused(h.anyFocused)
			s.SetBackfillIgnore(cfg.Cache.IgnoresBackfill)
			s.SetCatchUp(cfg.Cache.CatchUpOnConnect(), cfg.Cache.CatchUpMax())
			if doBackfill {
				s.EnableBackfill(horizon)
			}
		}
		h.showList()
		go func() {
			if err := be.Run(ctx, h.dispatch); err != nil {
				fmt.Fprintln(os.Stderr, "slk-gio:", err)
			}
		}()
	}()
	app.Main()
}

func openBackend(ctx context.Context, fake bool, fakeEvery time.Duration, team string) (backend.Backend, error) {
	if fake {
		f := backend.NewFake(time.Now())
		f.Interval = fakeEvery
		return f, nil
	}
	tok, err := pickToken(slackclient.NewTokenStore(filepath.Join(paths.Data(), "tokens")), team)
	if err != nil {
		return nil, err
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
	be, err := backend.ConnectSlack(cctx, tok, db, paths.Cache())
	if err != nil && db != nil {
		db.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("%w (if the session expired, run slk once to refresh it)", err)
	}
	return be, nil
}

func pickToken(store *slackclient.TokenStore, team string) (slackclient.Token, error) {
	toks, err := store.List()
	if err != nil {
		return slackclient.Token{}, fmt.Errorf("reading workspaces: %w", err)
	}
	if len(toks) == 0 {
		return slackclient.Token{}, errors.New("no workspaces yet: run slk --add-workspace, or try slk-gio -fake")
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

// parseBackfill reads the -backfill flag: "" is off; "0" is everything;
// "1y"/"90d" are years and days; anything else is a Go duration.
func parseBackfill(v string) (horizon time.Duration, on bool, err error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		return 0, false, nil
	case v == "0":
		return 0, true, nil
	case strings.HasSuffix(v, "y") || strings.HasSuffix(v, "d"):
		n, err := strconv.Atoi(v[:len(v)-1])
		if err != nil || n <= 0 {
			return 0, false, fmt.Errorf("%q: want a count of years or days, like 1y or 90d", v)
		}
		days := n
		if strings.HasSuffix(v, "y") {
			days = n * 365
		}
		return time.Duration(days) * 24 * time.Hour, true, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, false, fmt.Errorf("%q: want 1y, 90d, 0, or a duration like 720h", v)
	}
	return d, true, nil
}
