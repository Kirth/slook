package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/onboard"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/slackdesktop"
	"golang.org/x/term"
)

// onboardingStyles are the lipgloss styles shared by the desktop and the
// browser sign-in flows.
type onboardingStyles struct {
	title, subtitle, step, success, errorText, dim lipgloss.Style
}

func newOnboardingStyles() onboardingStyles {
	return onboardingStyles{
		title:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4A9EFF")).MarginBottom(1),
		subtitle:  lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).MarginBottom(1),
		step:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50C878")),
		success:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50C878")).MarginTop(1),
		errorText: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E04040")),
		dim:       lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")),
	}
}

// addWorkspace signs in through the Slack desktop app. When that session
// cannot be read (no desktop app, no keyring, ...) and stdin is a terminal, it
// offers the browser-session flow instead. forceBrowser skips the desktop app.
func addWorkspace(forceBrowser bool) error {
	dataDir := xdgData()
	tokenDir := filepath.Join(dataDir, "tokens")
	tokenStore := slackclient.NewTokenStore(tokenDir)

	st := newOnboardingStyles()
	titleStyle, subtitleStyle, stepStyle := st.title, st.subtitle, st.step
	successStyle, errorStyle, dimStyle := st.success, st.errorText, st.dim

	fmt.Println()
	fmt.Println(titleStyle.Render("slk -- Add Workspace"))
	if forceBrowser {
		return addWorkspaceFromBrowser(tokenStore, st)
	}
	fmt.Println(subtitleStyle.Render("Reading your signed-in workspaces from the Slack desktop app."))
	fmt.Println()

	browser := func() error { return addWorkspaceFromBrowser(tokenStore, st) }

	// Read cookie + workspaces from the desktop app.
	cookie, err := slackdesktop.Cookie()
	if err != nil {
		fmt.Println(errorStyle.Render("  " + onboard.DesktopErrorMessage(err)))
		return offerBrowserFallback(term.IsTerminal(int(os.Stdin.Fd())), askBrowserFallback, browser, err)
	}
	workspaces, err := slackdesktop.Workspaces()
	if err != nil {
		fmt.Println(errorStyle.Render("  " + onboard.DesktopErrorMessage(err)))
		return offerBrowserFallback(term.IsTerminal(int(os.Stdin.Fd())), askBrowserFallback, browser, err)
	}

	// Multi-select (all pre-selected, but for the teams signed in from a
	// browser session: see onboard.DesktopChoices).
	saved, _ := tokenStore.List()
	choices, chosen := onboard.DesktopChoices(workspaces, saved)
	var opts []huh.Option[string]
	for _, c := range choices {
		opts = append(opts, huh.NewOption(c.Label, c.TeamID))
	}
	// huh sizes the MultiSelect option viewport to (Height - title/description
	// lines); when Height is unset the viewport collapses to a row or two and
	// the user has to scroll a 3-item list. Size it to show every workspace
	// (capped so a very long list can't overflow a small terminal). The +4
	// covers the title + description overhead with a little slack.
	visibleRows := len(workspaces)
	if visibleRows > 12 {
		visibleRows = 12
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Workspaces to add").
				Description("All selected by default; space to toggle, enter to confirm.").
				Options(opts...).
				Value(&chosen).
				Height(visibleRows + 4),
		),
	).WithTheme(huh.ThemeFunc(huh.ThemeDracula))
	if err := form.Run(); err != nil {
		return fmt.Errorf("form cancelled")
	}
	selected := map[string]bool{}
	for _, id := range chosen {
		selected[id] = true
	}

	// Resolve tokens for the selected workspaces. Prefer the desktop app's
	// stored tokens (client-v2 no longer inlines them in page HTML); mint is a
	// fallback for older workspaces. A read failure here is non-fatal — mint
	// still covers those cases.
	desktopTokens, _ := slackdesktop.Tokens()

	fmt.Println()
	fmt.Println(stepStyle.Render("Connecting..."))
	tokens, err := onboard.BuildWorkspaceTokens(context.Background(), cookie, desktopTokens, workspaces, selected, slackclient.MintToken)
	if err != nil {
		fmt.Println(errorStyle.Render("  Failed to obtain token: " + err.Error()))
		return err
	}

	// Validate each and save.
	for _, tok := range tokens {
		client := slackclient.NewClient(tok.AccessToken, tok.Cookie)
		if err := client.Connect(context.Background()); err != nil {
			fmt.Println(errorStyle.Render(fmt.Sprintf("  %s: authentication failed: %v", tok.TeamName, err)))
			return fmt.Errorf("authentication failed for %s: %w", tok.TeamName, err)
		}
		if err := saveWorkspace(tokenStore, tok, st); err != nil {
			return err
		}
	}

	fmt.Println()
	fmt.Println(successStyle.Render(fmt.Sprintf("  %d workspace(s) added!", len(tokens))))
	fmt.Println(dimStyle.Render("  Run ") + lipgloss.NewStyle().Bold(true).Render("slk-gio") + dimStyle.Render(" to start."))
	fmt.Println()
	return nil
}

// saveWorkspace saves a validated token and its config.toml block
// (onboard.SaveWorkspace) and reports what it did.
func saveWorkspace(tokenStore *slackclient.TokenStore, tok slackclient.Token, st onboardingStyles) error {
	res, err := onboard.SaveWorkspace(tokenStore, tok, filepath.Join(xdgConfig(), "config.toml"))
	if err != nil {
		return err
	}
	switch {
	case res.LoadErr != nil:
		fmt.Println(st.success.Render("  Added ") + st.dim.Render(tok.TeamName))
		fmt.Println(st.dim.Render("  Note: config.toml does not load (" + res.LoadErr.Error() + "), left as is."))
	case res.Updated:
		fmt.Println(st.success.Render("  Updated ") + st.dim.Render(tok.TeamName))
	default:
		if res.WriteErr != nil {
			fmt.Println(st.dim.Render("  Note: could not write config.toml: " + res.WriteErr.Error()))
		}
		fmt.Println(st.success.Render("  Added ") + st.dim.Render(tok.TeamName))
	}
	return nil
}
