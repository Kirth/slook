package styles

import (
	"os"
	"path/filepath"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/theme"
)

// The theme tests that need lipgloss or the applied styles; the rest
// live with the tables in internal/theme/themes_test.go.

func TestLoadCustomThemes(t *testing.T) {
	dir := t.TempDir()

	themeData := []byte(`
name = "My Custom"

[colors]
primary = "#AABBCC"
accent = "#112233"
warning = "#445566"
error = "#778899"
background = "#000000"
surface = "#111111"
surface_dark = "#222222"
text = "#FFFFFF"
text_muted = "#999999"
border = "#555555"
`)
	if err := os.WriteFile(filepath.Join(dir, "mycustom.toml"), themeData, 0644); err != nil {
		t.Fatal(err)
	}

	// Also write a non-toml file that should be ignored
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a theme"), 0644); err != nil {
		t.Fatal(err)
	}

	LoadCustomThemes(os.DirFS(dir))

	// Verify the custom theme was loaded
	names := ThemeNames()
	found := false
	for _, n := range names {
		if n == "My Custom" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'My Custom' in theme names, got %v", names)
	}

	// Verify it can be applied
	Apply("my custom", config.Theme{})
	if !colorEqual(Primary, lipgloss.Color("#AABBCC")) {
		t.Errorf("expected custom primary #AABBCC")
	}

	// Clean up custom themes for other tests
	theme.ClearCustom()
	Apply("dark", config.Theme{})
}

// TestANSIDarkThemeRegistered asserts the ansi-dark theme is present
// in the theme switcher and that every color field is populated with
// a value that resolves to ansi.BasicColor — confirming the theme
// will inherit the user's terminal palette rather than emit truecolor.
func TestANSIDarkThemeRegistered(t *testing.T) {
	names := ThemeNames()
	found := false
	for _, n := range names {
		if n == "ANSI Dark" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected \"ANSI Dark\" in ThemeNames, got %v", names)
	}

	_, c := resolveTheme("ANSI Dark")
	required := map[string]string{
		"Primary":     c.Primary,
		"Accent":      c.Accent,
		"Warning":     c.Warning,
		"Error":       c.Error,
		"Background":  c.Background,
		"Surface":     c.Surface,
		"SurfaceDark": c.SurfaceDark,
		"Text":        c.Text,
		"TextMuted":   c.TextMuted,
		"Border":      c.Border,
	}
	for name, val := range required {
		if val == "" {
			t.Errorf("ansi dark.%s is empty", name)
			continue
		}
		col := lipgloss.Color(val)
		if _, ok := col.(ansi.BasicColor); !ok {
			t.Errorf("ansi dark.%s = %q resolves to %T, want ansi.BasicColor",
				name, val, col)
		}
	}
}

// TestANSILightThemeRegistered: mirror of TestANSIDarkThemeRegistered
// for the light variant.
func TestANSILightThemeRegistered(t *testing.T) {
	names := ThemeNames()
	found := false
	for _, n := range names {
		if n == "ANSI Light" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected \"ANSI Light\" in ThemeNames, got %v", names)
	}

	_, c := resolveTheme("ANSI Light")
	required := map[string]string{
		"Primary":     c.Primary,
		"Accent":      c.Accent,
		"Warning":     c.Warning,
		"Error":       c.Error,
		"Background":  c.Background,
		"Surface":     c.Surface,
		"SurfaceDark": c.SurfaceDark,
		"Text":        c.Text,
		"TextMuted":   c.TextMuted,
		"Border":      c.Border,
	}
	for name, val := range required {
		if val == "" {
			t.Errorf("ansi light.%s is empty", name)
			continue
		}
		col := lipgloss.Color(val)
		if _, ok := col.(ansi.BasicColor); !ok {
			t.Errorf("ansi light.%s = %q resolves to %T, want ansi.BasicColor",
				name, val, col)
		}
	}
}

// TestANSIThemesSelectionTintPaletteInherited regression-pins that
// SelectionTintColor for the ANSI themes returns a palette-inherited
// ansi.BasicColor — not a near-black RGB mix from the default
// mixColors(Accent, Background=ANSI 0, 0.15) path.
//
// Without explicit SelectionBgFocused/SelectionBgUnfocused on the
// theme, ansi-dark's selection tint computes to roughly RGB(0,25,25)
// which renders as effectively black against any dark terminal bg.
// The fix sets the optional theme fields to a palette ANSI color
// ("8" = bright black / gray) so the tint is visible and tracks
// the user's terminal palette.
func TestANSIThemesSelectionTintPaletteInherited(t *testing.T) {
	cases := []struct {
		theme string
	}{
		{"ansi dark"},
		{"ansi light"},
	}
	for _, tc := range cases {
		t.Run(tc.theme, func(t *testing.T) {
			Apply(tc.theme, config.Theme{})
			t.Cleanup(func() { Apply("dark", config.Theme{}) })

			focused := SelectionTintColor(true)
			if _, ok := focused.(ansi.BasicColor); !ok {
				t.Errorf("%s focused selection tint = %T, want ansi.BasicColor (palette-inherited)", tc.theme, focused)
			}

			unfocused := SelectionTintColor(false)
			if _, ok := unfocused.(ansi.BasicColor); !ok {
				t.Errorf("%s unfocused selection tint = %T, want ansi.BasicColor (palette-inherited)", tc.theme, unfocused)
			}
		})
	}
}
