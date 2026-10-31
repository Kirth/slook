package theme

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestLoadCustom(t *testing.T) {
	t.Cleanup(SnapshotCustom())
	dir := t.TempDir()
	themeData := []byte(`
name = "My Custom"

[colors]
primary = "#AABBCC"
background = "#000000"
`)
	if err := os.WriteFile(filepath.Join(dir, "mycustom.toml"), themeData, 0644); err != nil {
		t.Fatal(err)
	}
	// A non-toml file is ignored.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a theme"), 0644); err != nil {
		t.Fatal(err)
	}

	LoadCustom(os.DirFS(dir))

	if !slices.Contains(Names(), "My Custom") {
		t.Fatalf("expected 'My Custom' in theme names, got %v", Names())
	}
	c, ok := ColorsByName("my custom")
	if !ok || c.Primary != "#AABBCC" {
		t.Fatalf("my custom = %+v, %v; want primary #AABBCC", c, ok)
	}
	if dark := builtinThemes["dark"].Colors; c.Accent != dark.Accent {
		t.Errorf("accent = %q, want dark's %q filled in", c.Accent, dark.Accent)
	}
}

func TestLoadCustomMissingDir(t *testing.T) {
	// Should not panic on non-existent directory
	LoadCustom(os.DirFS("/tmp/nonexistent-theme-dir-12345"))
}

func TestNewBuiltinThemesRegistered(t *testing.T) {
	newThemes := []string{
		"Catppuccin Latte",
		"GitHub Light",
		"Tokyo Night Light",
		"Atom One Light",
		"Catppuccin Frappé",
		"Catppuccin Macchiato",
		"Tokyo Night Storm",
		"Cobalt2",
		"Iceberg",
		"Oceanic Next",
		"Cyberpunk Neon",
		"Material Palenight",
	}

	names := Names()
	have := make(map[string]bool, len(names))
	for _, n := range names {
		have[n] = true
	}

	for _, want := range newThemes {
		if !have[want] {
			t.Errorf("new built-in theme %q not registered (Names: %v)", want, names)
		}
	}
}

func TestNewThemesHaveRequiredColors(t *testing.T) {
	newThemes := []string{
		"catppuccin latte",
		"github light",
		"tokyo night light",
		"atom one light",
		"catppuccin frappé",
		"catppuccin macchiato",
		"tokyo night storm",
		"cobalt2",
		"iceberg",
		"oceanic next",
		"cyberpunk neon",
		"material palenight",
	}
	for _, key := range newThemes {
		_, c := Resolve(key)
		if c.Primary == "" || c.Accent == "" || c.Warning == "" || c.Error == "" ||
			c.Background == "" || c.Surface == "" || c.SurfaceDark == "" ||
			c.Text == "" || c.TextMuted == "" || c.Border == "" {
			t.Errorf("theme %q is missing one or more required color fields: %+v", key, c)
		}
	}
}

func TestLightThemesHaveDarkSidebars(t *testing.T) {
	// Light themes should set SidebarBackground/etc explicitly so the
	// sidebar/rail aren't washed out against the light message pane.
	lightThemes := []string{
		"catppuccin latte",
		"github light",
		"tokyo night light",
		"atom one light",
	}
	for _, key := range lightThemes {
		_, c := Resolve(key)
		if c.SidebarBackground == "" {
			t.Errorf("light theme %q must set SidebarBackground", key)
		}
		if c.RailBackground == "" {
			t.Errorf("light theme %q must set RailBackground", key)
		}
	}
}

// TestANSIThemeLookupViaDisplayName regression-pins the realistic
// theme-switcher path: when the user picks "ANSI Dark" via Ctrl+y,
// the display name is saved verbatim to config.toml. On the next
// render, Resolve must resolve "ANSI Dark" to the ansi-dark
// theme — not fall through to the default "dark" theme.
//
// The key in builtinThemes must therefore lowercase-match "ANSI Dark"
// after strings.ToLower, i.e. it must use a space separator like every
// other multi-word built-in theme ("tokyo night", "gruvbox dark", etc).
func TestANSIThemeLookupViaDisplayName(t *testing.T) {
	_, dark := Resolve("ANSI Dark")
	if dark.Background != "0" {
		t.Errorf("Resolve(\"ANSI Dark\").Background = %q, want \"0\" — likely fell through to default \"dark\" theme", dark.Background)
	}

	_, light := Resolve("ANSI Light")
	if light.Background != "15" {
		t.Errorf("Resolve(\"ANSI Light\").Background = %q, want \"15\" — likely fell through to default \"dark\" theme", light.Background)
	}
}

// --- channels-panel contrast guard ---

// contrastAllowlist are themes intentionally exempt from the
// channels-panel contrast requirement: the ANSI themes use palette
// numbers (not hex), so a CIELAB hex computation doesn't apply to them
// (they inherit the terminal palette). "hot dog stand" is a deliberately
// garish novelty whose hand-picked red-on-yellow palette we never want
// the contrast retune to touch; it already clears the threshold on its
// own, so the exemption is belt-and-suspenders.
var contrastAllowlist = map[string]bool{
	"ansi dark":     true,
	"ansi light":    true,
	"hot dog stand": true,
}

// minChannelsPanelDeltaLstar is the minimum perceptual lightness
// difference (CIELAB L*) between a theme's message-pane Background and
// its channels-panel SidebarBackground. 6.0 is calibrated so the
// slack-default split (ΔL*≈72) passes easily while a 1–2% nudge
// (ΔL*≈3) fails. See spec 2026-06-07-more-themes-design.md.
const minChannelsPanelDeltaLstar = 6.0

func srgbToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// lstar returns the CIELAB L* (0..100) of a "#RRGGBB" hex string.
func lstar(hex string) float64 {
	h := strings.TrimPrefix(hex, "#")
	if len(h) != 6 {
		return 0
	}
	ri, _ := strconv.ParseInt(h[0:2], 16, 0)
	gi, _ := strconv.ParseInt(h[2:4], 16, 0)
	bi, _ := strconv.ParseInt(h[4:6], 16, 0)
	r := srgbToLinear(float64(ri) / 255)
	g := srgbToLinear(float64(gi) / 255)
	b := srgbToLinear(float64(bi) / 255)
	y := 0.2126*r + 0.7152*g + 0.0722*b
	if y > 0.008856 {
		return 116*math.Cbrt(y) - 16
	}
	return 903.3 * y
}

// TestChannelsPanelContrast asserts every non-allowlisted built-in
// theme gives the channels panel a perceptibly distinct surface from
// the message pane. Reports each theme's measured ΔL* on failure so the
// retune is a deterministic adjust-and-rerun loop.
func TestChannelsPanelContrast(t *testing.T) {
	for key, theme := range builtinThemes {
		if contrastAllowlist[key] {
			continue
		}
		bg := theme.Colors.Background
		sb := theme.Colors.SidebarBackground
		if sb == "" {
			sb = bg // falls back to Background -> zero contrast
		}
		if !strings.HasPrefix(bg, "#") || !strings.HasPrefix(sb, "#") {
			t.Errorf("theme %q: non-hex background/sidebar not allowlisted (bg=%q sidebar=%q)", key, bg, sb)
			continue
		}
		delta := math.Abs(lstar(bg) - lstar(sb))
		if delta < minChannelsPanelDeltaLstar {
			t.Errorf("theme %q channels-panel contrast too low: ΔL*=%.1f (bg=%s L*=%.1f, sidebar=%s L*=%.1f), want >= %.1f",
				key, delta, bg, lstar(bg), sb, lstar(sb), minChannelsPanelDeltaLstar)
		}
	}
}

var darkEditorThemes = []string{
	"Zenburn", "Gruvbox Material Dark", "Nightfox", "Carbonfox",
	"Melange Dark", "Vesper", "Flexoki Dark", "Modus Vivendi",
	"Night Owl", "Poimandres", "Ayu Dark", "Kanagawa Dragon",
}

func TestDarkEditorThemesRegistered(t *testing.T) {
	have := map[string]bool{}
	for _, n := range Names() {
		have[n] = true
	}
	for _, want := range darkEditorThemes {
		if !have[want] {
			t.Errorf("dark editor theme %q not registered", want)
		}
	}
}

func TestDarkEditorThemesHaveRequiredColors(t *testing.T) {
	for _, name := range darkEditorThemes {
		_, c := Resolve(strings.ToLower(name))
		if c.Primary == "" || c.Accent == "" || c.Warning == "" || c.Error == "" ||
			c.Background == "" || c.Surface == "" || c.SurfaceDark == "" ||
			c.Text == "" || c.TextMuted == "" || c.Border == "" {
			t.Errorf("theme %q missing required color(s): %+v", name, c)
		}
	}
}

var lightEditorThemes = []string{
	"Rosé Pine Dawn", "Everforest Light", "Flexoki Light",
	"Modus Operandi", "Kanagawa Lotus", "PaperColor Light",
}

func TestLightEditorThemesRegistered(t *testing.T) {
	have := map[string]bool{}
	for _, n := range Names() {
		have[n] = true
	}
	for _, want := range lightEditorThemes {
		if !have[want] {
			t.Errorf("light editor theme %q not registered", want)
		}
	}
}

func TestLightEditorThemesHaveRequiredColors(t *testing.T) {
	for _, name := range lightEditorThemes {
		_, c := Resolve(strings.ToLower(name))
		if c.Primary == "" || c.Accent == "" || c.Warning == "" || c.Error == "" ||
			c.Background == "" || c.Surface == "" || c.SurfaceDark == "" ||
			c.Text == "" || c.TextMuted == "" || c.Border == "" {
			t.Errorf("theme %q missing required color(s): %+v", name, c)
		}
	}
}

func TestLightEditorThemesHaveDarkSidebars(t *testing.T) {
	for _, name := range lightEditorThemes {
		_, c := Resolve(strings.ToLower(name))
		if c.SidebarBackground == "" {
			t.Errorf("light theme %q must set SidebarBackground", name)
		}
		if c.RailBackground == "" {
			t.Errorf("light theme %q must set RailBackground", name)
		}
	}
}

var slackBrandedThemes = []string{
	"Aubergine", "Ochin", "Choco Mint", "Mocha", "Nocturne",
}

func TestSlackBrandedThemesRegistered(t *testing.T) {
	have := map[string]bool{}
	for _, n := range Names() {
		have[n] = true
	}
	for _, want := range slackBrandedThemes {
		if !have[want] {
			t.Errorf("slack-branded theme %q not registered", want)
		}
	}
}

func TestSlackBrandedThemesHaveRequiredColors(t *testing.T) {
	for _, name := range slackBrandedThemes {
		_, c := Resolve(strings.ToLower(name))
		if c.Primary == "" || c.Accent == "" || c.Warning == "" || c.Error == "" ||
			c.Background == "" || c.Surface == "" || c.SurfaceDark == "" ||
			c.Text == "" || c.TextMuted == "" || c.Border == "" {
			t.Errorf("theme %q missing required color(s): %+v", name, c)
		}
	}
}

func TestBuiltinThemeCount(t *testing.T) {
	const want = 59
	if got := len(builtinThemes); got != want {
		t.Errorf("builtinThemes count = %d, want %d (update docs in README.md and wiki/Features.md if this changed intentionally)", got, want)
	}
}

func TestThemeColorsByName(t *testing.T) {
	c, ok := ColorsByName("  Nord ")
	if !ok || c.Background == "" || c.Background != builtinThemes["nord"].Colors.Background {
		t.Fatalf("Nord: %+v, %v", c, ok)
	}
	if _, ok := ColorsByName("no such theme"); ok {
		t.Fatal("found a theme that does not exist")
	}
}
