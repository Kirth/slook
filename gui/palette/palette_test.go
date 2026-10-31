package palette

import (
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/theme"
)

func TestPresetsSetEveryColour(t *testing.T) {
	for name, p := range Presets {
		v := reflect.ValueOf(p)
		for i := 0; i < v.NumField(); i++ {
			if c := v.Field(i).Interface().(color.NRGBA); c == (color.NRGBA{}) {
				t.Errorf("%s: %s is unset", name, v.Type().Field(i).Name)
			}
		}
		if len(p.fields()) != v.NumField() {
			t.Errorf("fields() has %d keys for %d colours", len(p.fields()), v.NumField())
		}
	}
}

func TestPresetsReadable(t *testing.T) {
	for name, p := range Presets {
		if c := Contrast(p.Text, p.Background); c < 4.5 {
			t.Errorf("%s: text on background contrast %.2f, want ≥ 4.5", name, c)
		}
		if c := Contrast(p.Text, p.Overlay); c < 4.5 {
			t.Errorf("%s: text on overlay contrast %.2f, want ≥ 4.5", name, c)
		}
	}
	if IsDark(Presets["light"].Background) || !IsDark(Presets["dark"].Background) || !IsDark(Presets["ink"].Background) {
		t.Fatal("light is dark or a dark preset is light")
	}
}

func TestDarkIsTheOriginalLook(t *testing.T) {
	p := Presets["dark"]
	if p.Background != hex(0x1a1d21) || p.Surface != hex(0x222529) || p.Accent != hex(0x1d9bd1) || p.Text != hex(0xd1d2d3) {
		t.Fatalf("dark changed: %+v", p)
	}
}

func TestResolve(t *testing.T) {
	if p, w := Resolve("", nil); p != Presets["hadal"] || len(w) != 0 {
		t.Fatalf("default: %v", w)
	}
	if p, w := Resolve(" Ink ", nil); p != Presets["ink"] || len(w) != 0 {
		t.Fatalf("ink: %v", w)
	}
	p, w := Resolve("nonesuch", nil)
	if p != Presets[Default] || len(w) != 1 || !strings.Contains(w[0], "nonesuch") {
		t.Fatalf("unknown: %v", w)
	}

	nord, _ := theme.ColorsByName("nord")
	p, w = Resolve("Nord", nil)
	bg, _ := parseHex(nord.Background)
	fg, _ := parseHex(nord.Text)
	if len(w) != 0 || p.Background != bg || p.Text != fg {
		t.Fatalf("nord: %+v, %v", p, w)
	}
	if !IsDark(p.Background) || p.Overlay == p.Surface || p.Shadow.A == 0 {
		t.Fatalf("nord's derived colours: %+v", p)
	}
	if lp, _ := Resolve("github light", nil); lp.Overlay != hex(0xffffff) || IsDark(lp.Background) {
		t.Fatalf("a light TUI theme: %+v", lp)
	}
}

func TestResolveOverrides(t *testing.T) {
	p, w := Resolve("dark", map[string]string{
		"accent":     "#7aa2f7",
		"Text_Muted": "aabbcc",
		"shadow":     "#00000040",
		"bogus":      "#ffffff",
		"border":     "blue",
	})
	if p.Accent != hex(0x7aa2f7) || p.TextMuted != hex(0xaabbcc) || p.Shadow != (color.NRGBA{A: 0x40}) {
		t.Fatalf("overrides not applied: %+v", p)
	}
	if p.Border != Presets["dark"].Border {
		t.Fatal("a bad colour replaced the border")
	}
	if all := strings.Join(w, "\n"); len(w) != 2 || !strings.Contains(all, "border") || !strings.Contains(all, "bogus") {
		t.Fatalf("warnings %v", w)
	}
	if len(Keys()) != 21 {
		t.Fatalf("%d keys", len(Keys()))
	}
}

func TestTextOnAccentIsReadable(t *testing.T) {
	for name, p := range Presets {
		if c := Contrast(p.OnAccent, p.Accent); c < 3 {
			t.Errorf("%s: on_accent over accent contrast %.2f, want ≥ 3", name, c)
		}
	}
}

func TestHadalIsTheDefault(t *testing.T) {
	if Default != "hadal" {
		t.Fatalf("default %q", Default)
	}
	p := Presets["hadal"]
	for name, c := range map[string]struct {
		got  color.NRGBA
		want uint32
	}{
		"background": {p.Background, 0x0b1016}, "surface": {p.Surface, 0x101720}, "overlay": {p.Overlay, 0x151e29},
		"hover": {p.Hover, 0x16202b}, "border": {p.Border, 0x1f2a36}, "text": {p.Text, 0xd7dde3},
		"muted": {p.TextMuted, 0x7d8a97}, "accent": {p.Accent, 0x2fb7c9}, "mention": {p.Mention, 0x8ed0f5},
		"broadcast": {p.Broadcast, 0xe6c070}, "success": {p.Success, 0x3ddc97}, "badge": {p.Badge, 0xe5586f},
		"error": {p.Error, 0xe5586f}, "code background": {p.CodeBackground, 0x0e1620},
	} {
		if c.got != hex(c.want) {
			t.Errorf("hadal %s is %v, want #%06x", name, c.got, c.want)
		}
	}
	if p, w := Resolve("Slack-Dark", nil); p != Presets["dark"] || len(w) != 0 {
		t.Fatalf("slack-dark: %v", w)
	}
}
