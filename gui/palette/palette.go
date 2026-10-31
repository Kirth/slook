// Package palette is the Gio frontend's colours: three palettes it
// ships (dark, ink, light), any of the TUI's named themes mapped onto
// the same roles, and per-colour overrides, as config.toml's
// [gui.theme] asks for them. It knows nothing about drawing.
package palette

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/gammons/slk/internal/theme"
)

// Palette is every colour the GUI draws with, by role.
type Palette struct {
	Background color.NRGBA // the window
	Surface    color.NRGBA // header bars and panels
	Overlay    color.NRGBA // overlays lifted over the window: switcher, hover card, menus
	Text       color.NRGBA
	TextMuted  color.NRGBA
	OnAccent   color.NRGBA // text on an accent or badge fill
	Accent     color.NRGBA
	Mention    color.NRGBA // @someone and #channel in text
	Broadcast  color.NRGBA // @here, @channel
	Border     color.NRGBA
	Hover      color.NRGBA // a row or control under the pointer
	Control    color.NRGBA // window control buttons
	// ControlHover is a window control under the pointer.
	ControlHover   color.NRGBA
	Error          color.NRGBA
	Warning        color.NRGBA // offline, night-time
	Success        color.NRGBA // someone active
	Badge          color.NRGBA // unread mentions elsewhere
	CodeText       color.NRGBA // inline code
	CodeBackground color.NRGBA
	Ephemeral      color.NRGBA // behind a message only the user sees
	Shadow         color.NRGBA // under overlays; its alpha is the shadow's strength
}

// Default is the palette when config names none.
const Default = "hadal"

// aliases are other names for shipped palettes.
var aliases = map[string]string{"slack-dark": "dark"}

// Presets are the palettes slk-gio ships. Their names win over the
// TUI's themes of the same name.
var Presets = map[string]Palette{
	// hadal is the trench floor as an office: a blue-black abyss lit
	// only by what the content gives off. Hairlines, not fills; one
	// bioluminescent teal accent, used sparingly; presence green, a
	// pale glow for @you and coral for what needs you are the only
	// other light. Text on the teal is the abyss itself.
	"hadal": {
		Background: hex(0x0b1016), Surface: hex(0x101720), Overlay: hex(0x151e29),
		Text: hex(0xd7dde3), TextMuted: hex(0x7d8a97), OnAccent: hex(0x0b1016),
		Accent: hex(0x2fb7c9), Mention: hex(0x8ed0f5), Broadcast: hex(0xe6c070),
		Border: hex(0x1f2a36), Hover: hex(0x16202b), Control: hex(0x1a2531), ControlHover: hex(0x223040),
		Error: hex(0xe5586f), Warning: hex(0xd9a24e), Success: hex(0x3ddc97), Badge: hex(0xe5586f),
		CodeText: hex(0xb9c4cf), CodeBackground: hex(0x0e1620), Ephemeral: hex(0x121c26),
		Shadow: color.NRGBA{A: 0x59}, // 35 %
	},
	// dark is slk-gio's original look (the brief's "slack-dark").
	"dark": {
		Background: hex(0x1a1d21), Surface: hex(0x222529), Overlay: hex(0x2a2d31),
		Text: hex(0xd1d2d3), TextMuted: hex(0x8b8d91), OnAccent: hex(0xffffff),
		Accent: hex(0x1d9bd1), Mention: hex(0x5fb3e8), Broadcast: hex(0xf2c744),
		Border: hex(0x35373b), Hover: hex(0x27292d), Control: hex(0x2f3236), ControlHover: hex(0x3d4045),
		Error: hex(0xe0526b), Warning: hex(0xe8a33d), Success: hex(0x2bac76), Badge: hex(0xe0526b),
		CodeText: hex(0xf0a3a3), CodeBackground: hex(0x222529), Ephemeral: hex(0x1f262d),
		Shadow: color.NRGBA{A: 0x55},
	},
	// ink is ink-blue surfaces under warm, paper-coloured text, with
	// an amber accent (links, reply counts, your reactions, unread
	// badges), blue mentions, jade presence and coral code. Text on the
	// amber is the ink itself.
	"ink": {
		Background: hex(0x17161f), Surface: hex(0x201e2b), Overlay: hex(0x2a2737),
		Text: hex(0xe6dccb), TextMuted: hex(0x9a90a8), OnAccent: hex(0x17161f),
		Accent: hex(0xe0a458), Mention: hex(0x7fb7e6), Broadcast: hex(0xf2c14e),
		Border: hex(0x34304a), Hover: hex(0x221f2e), Control: hex(0x332f45), ControlHover: hex(0x3f3a55),
		Error: hex(0xe5677b), Warning: hex(0xe8a35a), Success: hex(0x6cc69a), Badge: hex(0xe5677b),
		CodeText: hex(0xf0a58a), CodeBackground: hex(0x1e1c29), Ephemeral: hex(0x1c2231),
		Shadow: color.NRGBA{A: 0x77},
	},
	// light is white with grey panels, after Slack's own light theme.
	"light": {
		Background: hex(0xffffff), Surface: hex(0xf4f4f5), Overlay: hex(0xffffff),
		Text: hex(0x1d1c1d), TextMuted: hex(0x616061), OnAccent: hex(0xffffff),
		Accent: hex(0x1264a3), Mention: hex(0x1264a3), Broadcast: hex(0x8a6100),
		Border: hex(0xdddddd), Hover: hex(0xf0f0f1), Control: hex(0xe6e6e8), ControlHover: hex(0xd9d9dc),
		Error: hex(0xd0284a), Warning: hex(0xb26b00), Success: hex(0x007a5a), Badge: hex(0xd0284a),
		CodeText: hex(0xc01343), CodeBackground: hex(0xf6f6f7), Ephemeral: hex(0xeef4fa),
		Shadow: color.NRGBA{A: 0x22},
	},
}

// Names lists the shipped palettes, sorted.
func Names() []string {
	names := make([]string, 0, len(Presets))
	for n := range Presets {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// Resolve is the palette config asks for: a shipped palette by name,
// else one of the TUI's themes mapped onto the roles, else dark; then
// overrides (palette key → "#rrggbb", or "#rrggbbaa" for shadow). Each
// warning names something it could not use; the rest still applies.
func Resolve(name string, overrides map[string]string) (Palette, []string) {
	var warns []string
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = Default
	}
	if a, ok := aliases[key]; ok {
		key = a
	}
	p, ok := Presets[key]
	if !ok {
		if tc, found := theme.ColorsByName(key); found {
			var bad []string
			p, bad = fromTUI(tc)
			for _, b := range bad {
				warns = append(warns, fmt.Sprintf("theme %q: %s", name, b))
			}
		} else {
			warns = append(warns, fmt.Sprintf("unknown theme %q, using %s (shipped: %s; or any slk theme name)",
				name, Default, strings.Join(Names(), ", ")))
			p = Presets[Default]
		}
	}
	fields := p.fields()
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		target, ok := fields[strings.ToLower(k)]
		if !ok {
			warns = append(warns, fmt.Sprintf("unknown colour %q in [gui.theme.colors]", k))
			continue
		}
		c, err := parseHex(overrides[k])
		if err != nil {
			warns = append(warns, fmt.Sprintf("colour %s: %v", k, err))
			continue
		}
		*target = c
	}
	return p, warns
}

// Keys are the palette's colour names as [gui.theme.colors] takes them.
func Keys() []string {
	var p Palette
	keys := make([]string, 0, 21)
	for k := range p.fields() {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// fields maps each config key to the colour it sets.
func (p *Palette) fields() map[string]*color.NRGBA {
	return map[string]*color.NRGBA{
		"background": &p.Background, "surface": &p.Surface, "overlay": &p.Overlay,
		"text": &p.Text, "text_muted": &p.TextMuted, "on_accent": &p.OnAccent,
		"accent": &p.Accent, "mention": &p.Mention, "broadcast": &p.Broadcast,
		"border": &p.Border, "hover": &p.Hover, "control": &p.Control, "control_hover": &p.ControlHover,
		"error": &p.Error, "warning": &p.Warning, "success": &p.Success, "badge": &p.Badge,
		"code_text": &p.CodeText, "code_background": &p.CodeBackground, "ephemeral": &p.Ephemeral,
		"shadow": &p.Shadow,
	}
}

// fromTUI maps a TUI theme onto the palette's roles, deriving what the
// TUI has no colour for by mixing. bad lists colours it could not read
// (they fall back to dark's).
func fromTUI(tc theme.Colors) (Palette, []string) {
	base := Presets[Default]
	var bad []string
	get := func(name, s string, fallback color.NRGBA) color.NRGBA {
		if s == "" {
			return fallback
		}
		c, err := parseHex(s)
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s: %v", name, err))
			return fallback
		}
		return c
	}
	bg := get("background", tc.Background, base.Background)
	text := get("text", tc.Text, base.Text)
	dark := IsDark(bg)
	surface := get("surface", tc.Surface, mix(bg, text, 0.06))
	if surface == bg {
		surface = mix(bg, text, 0.06)
	}
	accent := get("primary", tc.Primary, base.Accent)
	errc := get("error", tc.Error, base.Error)
	warn := get("warning", tc.Warning, base.Warning)
	p := Palette{
		Background: bg, Surface: surface,
		Text: text, TextMuted: get("text_muted", tc.TextMuted, mix(text, bg, 0.45)), OnAccent: hex(0xffffff),
		Accent: accent, Mention: accent, Broadcast: warn,
		Border:       get("border", tc.Border, mix(bg, text, 0.15)),
		Hover:        mix(bg, text, 0.06),
		Control:      mix(surface, text, 0.08),
		ControlHover: mix(surface, text, 0.14),
		Error:        errc, Warning: warn, Success: get("accent", tc.Accent, base.Success), Badge: errc,
		CodeText:       mix(errc, text, 0.4),
		CodeBackground: get("surface_dark", tc.SurfaceDark, surface),
		Ephemeral:      mix(bg, accent, 0.08),
	}
	if dark {
		p.Overlay = mix(surface, hex(0xffffff), 0.05)
		p.Shadow = color.NRGBA{A: 0x55}
	} else {
		p.Overlay = hex(0xffffff)
		p.Shadow = color.NRGBA{A: 0x22}
	}
	return p, bad
}

// IsDark reports whether c is a dark colour (relative luminance under
// 0.5), so text on it should be light.
func IsDark(c color.NRGBA) bool { return Luminance(c) < 0.5 }

// Luminance is c's relative luminance (WCAG), 0 for black to 1 for
// white.
func Luminance(c color.NRGBA) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// Contrast is the WCAG contrast ratio between a and b, 1 to 21.
func Contrast(a, b color.NRGBA) float64 {
	la, lb := Luminance(a), Luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// mix is a moved toward b by t (0 is a, 1 is b), opaque.
func mix(a, b color.NRGBA, t float64) color.NRGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return color.NRGBA{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B), A: 0xff}
}

// parseHex reads "#rrggbb" or "#rrggbbaa" (the "#" optional).
func parseHex(s string) (color.NRGBA, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) != 6 && len(h) != 8 {
		return color.NRGBA{}, fmt.Errorf("%q is not #rrggbb", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("%q is not #rrggbb", s)
	}
	if len(h) == 6 {
		return hex(uint32(v)), nil
	}
	return color.NRGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)}, nil
}

func hex(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}
