package main

import (
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/palette"
)

// TestColorsComeFromThePalette keeps colour literals in theme.go, where
// the palette is applied (and Slack's fixed attachment colours live):
// anything drawn elsewhere takes a palette colour, so every theme
// reaches it.
func TestColorsComeFromThePalette(t *testing.T) {
	literal := regexp.MustCompile(`rgb\(0x|color\.NRGBA\{R:`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "theme.go" || strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if literal.MatchString(line) {
				t.Errorf("%s:%d: a colour outside the palette: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}

func TestApplyPaletteSetsEveryColour(t *testing.T) {
	defer applyPalette(palette.Presets[palette.Default])
	// A palette whose every colour is the same: each variable must end
	// up as it, or applyPalette missed one.
	mark := color.NRGBA{R: 1, G: 2, B: 3, A: 4}
	p := palette.Palette{
		Background: mark, Surface: mark, Overlay: mark, Text: mark, TextMuted: mark, OnAccent: mark,
		Accent: mark, Mention: mark, Broadcast: mark, Border: mark, Hover: mark, Control: mark,
		ControlHover: mark, Error: mark, Warning: mark, Success: mark, Badge: mark, CodeText: mark,
		CodeBackground: mark, Ephemeral: mark, Shadow: mark,
	}
	applyPalette(p)
	for name, c := range map[string]color.NRGBA{
		"colBg": colBg, "colFg": colFg, "colMuted": colMuted, "colAccent": colAccent, "colOnAccent": colOnAccent,
		"colBorder": colBorder, "colError": colError, "colHover": colHover, "colOffline": colOffline,
		"colCodeFg": colCodeFg, "colCodeBg": colCodeBg, "colBroadcast": colBroadcast, "colMention": colMention,
		"colEphemeral": colEphemeral, "colHeader": colHeader, "colOverlay": colOverlay, "colShadow": colShadow,
		"colPresent": colPresent, "colBadge": colBadge, "colControl": colControl, "colControlHover": colControlHover,
	} {
		if c != mark {
			t.Errorf("%s not set from the palette", name)
		}
	}
}

func TestTextOnPicksAReadableColour(t *testing.T) {
	for _, bg := range []color.NRGBA{rgb(0x3b82f6), rgb(0xe01e5a), rgb(0xc026d3), rgb(0xf97316), rgb(0xffffff), rgb(0xf2c744)} {
		if c := palette.Contrast(textOn(bg), bg); c < 3 {
			t.Errorf("text on #%02x%02x%02x: contrast %.2f", bg.R, bg.G, bg.B, c)
		}
	}
}

func TestSunkAndConversationNames(t *testing.T) {
	defer applyPalette(palette.Presets[palette.Default])
	applyPalette(palette.Presets["hadal"])
	s := sunk(colMuted)
	if palette.Contrast(s, colBg) >= palette.Contrast(colMuted, colBg) || s == colBg {
		t.Fatalf("sunk %v is not between slate and the floor", s)
	}
	unread := backend.Conversation{Unread: true}
	read := backend.Conversation{}
	muted := backend.Conversation{Unread: true, Muted: true}
	if convNameColor(unread) != colFg || convNameColor(read) != colMuted || convNameColor(muted) != s {
		t.Fatal("unread is full text, read is slate, muted sinks")
	}
}
