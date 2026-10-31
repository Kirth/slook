package styles

import (
	"image/color"
	"io/fs"

	"github.com/gammons/slk/internal/theme"
)

// The colour tables and per-user colours moved to internal/theme; these
// delegate to it so the TUI's callers are unchanged.

// ThemeColors is theme.Colors.
type ThemeColors = theme.Colors

// RegisterCustomTheme delegates to theme.RegisterCustom.
func RegisterCustomTheme(name string, colors ThemeColors) { theme.RegisterCustom(name, colors) }

// ThemeNames delegates to theme.Names.
func ThemeNames() []string { return theme.Names() }

// ThemeColorsByName delegates to theme.ColorsByName.
func ThemeColorsByName(name string) (ThemeColors, bool) { return theme.ColorsByName(name) }

// LoadCustomThemes delegates to theme.LoadCustom.
func LoadCustomThemes(fsys fs.FS) { theme.LoadCustom(fsys) }

func resolveTheme(name string) (string, ThemeColors) { return theme.Resolve(name) }

// UserColor returns a deterministic color for the given user ID
// (theme.UserColor). Returns Primary for empty IDs (e.g. some bot
// messages) so they fall back to the theme default.
func UserColor(userID string) color.Color {
	if userID == "" {
		return Primary
	}
	return theme.UserColor(userID)
}
