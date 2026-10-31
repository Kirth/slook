// Package versionts keeps the _x_version_ts telemetry value (Slack's
// build timestamp) current: seeded from config.toml before the first
// request, refreshed from client.shouldReload after connecting, and
// saved back to the workspace's [workspaces.*] block.
//
// _x_version_ts moves (two captures hours apart carried different
// values), and a client that sends one hardcoded value forever is itself
// a signal; see "_x_version_ts sourcing" in
// docs/superpowers/specs/2026-07-30-enterprise-grid-bootstrap-design.md.
// It lived in cmd/slk (save_version_ts.go, connectWorkspace) until the
// TUI's retirement moved the boot into the Gio frontend.
package versionts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/slackhttp"
)

// Seed primes env with the build timestamp cached for teamID
// on a previous run, so the very first request of this session carries
// a current _x_version_ts instead of the compiled-in fallback. A nil
// envelope, an unconfigured workspace, or an empty cached value all
// leave the fallback in place (Envelope.SetVersionTS ignores "").
func Seed(env *slackhttp.Envelope, cfg config.Config, teamID string) {
	if env == nil {
		return
	}
	ws, ok := cfg.WorkspaceByTeamID(teamID)
	if !ok {
		return
	}
	env.SetVersionTS(ws.VersionTS)
}

// TOMLKey returns the literal TOML key of the [workspaces.*]
// block configured for teamID. Blocks are keyed either by a user-chosen
// slug (with team_id set) or, for legacy configs, by the raw team ID.
// When no block exists we fall back to the team ID, which is the key
// Save uses when it appends a new block.
func TOMLKey(cfg config.Config, teamID string) string {
	for k, w := range cfg.Workspaces {
		if w.TeamID == teamID {
			return k
		}
	}
	return teamID
}

// Save rewrites or appends a version_ts entry in
// [workspaces.<tomlKey>]: a textual line rewrite rather than a TOML
// re-marshal, so user comments and field ordering survive the write. It
// holds config.LockWrites across the read-modify-write and replaces the
// file with config.WriteAtomic.
func Save(configPath, tomlKey, teamID, teamName, versionTS string) error {
	defer config.LockWrites()()

	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			return err
		}
		data = nil
	} else if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")

	header := fmt.Sprintf("[workspaces.%s]", tomlKey)

	sectionStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			sectionStart = i
			break
		}
	}

	if sectionStart >= 0 {
		end := len(lines)
		for j := sectionStart + 1; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if t == "" || strings.HasPrefix(t, "[") {
				end = j
				break
			}
		}
		updated := false
		for j := sectionStart + 1; j < end; j++ {
			t := strings.TrimSpace(lines[j])
			if strings.HasPrefix(t, "version_ts") && strings.Contains(t, "=") {
				lines[j] = "version_ts = " + config.TOMLString(versionTS)
				updated = true
				break
			}
		}
		if !updated {
			newLines := make([]string, 0, len(lines)+1)
			newLines = append(newLines, lines[:sectionStart+1]...)
			newLines = append(newLines, "version_ts = "+config.TOMLString(versionTS))
			newLines = append(newLines, lines[sectionStart+1:]...)
			lines = newLines
		}
		return config.WriteAtomic(configPath, []byte(strings.Join(lines, "\n")))
	}

	// No existing section — append a legacy-keyed block.
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}
	safeName := config.SanitizeComment(teamName)
	if safeName == "" {
		safeName = teamID
	}
	commentLine := "# " + safeName
	legacyHeader := fmt.Sprintf("[workspaces.%s]", teamID)
	lines = append(lines, commentLine, legacyHeader, "version_ts = "+config.TOMLString(versionTS))
	return config.WriteAtomic(configPath, []byte(strings.Join(lines, "\n")))
}
