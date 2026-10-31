package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gammons/slk/internal/config"
)

// saveGlobalTheme rewrites the [appearance] theme line in config.toml.
// If the file has no theme line, it appends a new [appearance] section.
// Existing comments and ordering are preserved (textual rewrite, not
// TOML re-marshal).
func saveGlobalTheme(configPath, themeName string) error {
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

	// Track current section. Match a "theme = ..." line ONLY when we're
	// currently inside the [appearance] section. This avoids clobbering
	// per-workspace [workspaces.X] theme lines.
	inAppearance := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inAppearance = trimmed == "[appearance]"
			continue
		}
		if !inAppearance {
			continue
		}
		if strings.HasPrefix(trimmed, "theme") && strings.Contains(trimmed, "=") &&
			!strings.HasPrefix(trimmed, "theme.") {
			lines[i] = "theme = " + config.TOMLString(themeName)
			return config.WriteAtomic(configPath, []byte(strings.Join(lines, "\n")))
		}
	}
	// No [appearance] theme line found — append a new section.
	lines = append(lines, "", "[appearance]", "theme = "+config.TOMLString(themeName))
	return config.WriteAtomic(configPath, []byte(strings.Join(lines, "\n")))
}

// saveWorkspaceTheme rewrites or appends a [workspaces.<tomlKey>]
// theme entry. tomlKey is the literal TOML key in the config — for
// slug-keyed blocks that's the slug, for legacy blocks it's the team
// ID. teamID is the underlying Slack team ID; when we are creating a
// brand-new slug-keyed block, teamID is written as the team_id =
// "..." line (currently we only create legacy-keyed blocks here, but
// slug callers update an existing block).
func saveWorkspaceTheme(configPath, tomlKey, teamID, teamName, themeName string) error {
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
			if strings.HasPrefix(t, "theme") && strings.Contains(t, "=") &&
				!strings.HasPrefix(t, "theme.") {
				lines[j] = "theme = " + config.TOMLString(themeName)
				updated = true
				break
			}
		}
		if !updated {
			newLines := make([]string, 0, len(lines)+1)
			newLines = append(newLines, lines[:sectionStart+1]...)
			newLines = append(newLines, "theme = "+config.TOMLString(themeName))
			newLines = append(newLines, lines[sectionStart+1:]...)
			lines = newLines
		}
		return config.WriteAtomic(configPath, []byte(strings.Join(lines, "\n")))
	}

	// No existing section — append at end. We only get here when no
	// block exists for either the slug or the team ID, which means we
	// fall back to a legacy-keyed [workspaces.<teamID>] block.
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}
	safeName := config.SanitizeComment(teamName)
	if safeName == "" {
		safeName = teamID
	}
	commentLine := "# " + safeName
	legacyHeader := fmt.Sprintf("[workspaces.%s]", teamID)
	lines = append(lines, commentLine, legacyHeader, "theme = "+config.TOMLString(themeName))
	return config.WriteAtomic(configPath, []byte(strings.Join(lines, "\n")))
}
