package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gioui.org/font"
	"gioui.org/widget/material"
)

// TestLabelsUseTheTypeScale holds the package to the type scale: a
// label is bodyLabel, secondaryLabel or captionLabel, not one of Gio's
// own styles (which scale off the theme size: Caption is 11.25 sp at
// 15) and not a literal size. Text sized from its box (initials, an
// emoji in a cell) is computed, not literal, and stays allowed.
func TestLabelsUseTheTypeScale(t *testing.T) {
	banned := regexp.MustCompile(`material\.(Body1|Body2|Caption|Subtitle1|Subtitle2|H[1-6])\(|material\.Label\([^,]+,\s*unit\.Sp\(\d|TextSize\s*=\s*unit\.Sp\(\d`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if banned.MatchString(line) {
				t.Errorf("%s:%d: label outside the type scale: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}

func TestTypeScale(t *testing.T) {
	th := material.NewTheme()
	for _, c := range []struct {
		name string
		got  material.LabelStyle
		want float32
	}{
		{"body", bodyLabel(th, "x"), 15},
		{"secondary", secondaryLabel(th, "x"), 13},
		{"caption", captionLabel(th, "x"), 12},
	} {
		if float32(c.got.TextSize) != c.want {
			t.Errorf("%s is %v sp, want %v", c.name, c.got.TextSize, c.want)
		}
	}
}

func TestMonoFacesFallsBackToGoMono(t *testing.T) {
	sans := roleFace{role: fontRoleSans, face: font.FontFace{Font: font.Font{Typeface: "DejaVu Sans"}}}
	mono := roleFace{role: fontRoleMono, face: font.FontFace{Font: font.Font{Typeface: "Iosevka"}}}

	typeface, extra := monoFaces([]roleFace{sans, mono})
	if !strings.HasPrefix(string(typeface), `"Iosevka"`) || extra != nil {
		t.Fatalf("with a monospace face: %q, %d extra faces", typeface, len(extra))
	}

	typeface, extra = monoFaces([]roleFace{sans})
	if !strings.HasPrefix(string(typeface), `"Go Mono"`) || strings.Contains(string(typeface), "DejaVu") {
		t.Fatalf("without one: typeface %q", typeface)
	}
	if len(extra) == 0 {
		t.Fatal("without one: no Go Mono faces to add")
	}
	for _, f := range extra {
		if f.Font.Typeface != goMono {
			t.Fatalf("extra face %q, want only Go Mono", f.Font.Typeface)
		}
	}

	// No fontconfig at all: the theme's collection has Go Mono already.
	if typeface, extra := monoFaces(nil); !strings.HasPrefix(string(typeface), `"Go Mono"`) || extra != nil {
		t.Fatalf("no faces: %q, %d extra", typeface, len(extra))
	}
}

// TestTwoWeights holds the package to two weights: regular for
// everything, semibold for names, titles and headings. Bold has no
// place (and on a font without semibold, semibold falls back to it).
func TestTwoWeights(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "font.Bold") || strings.Contains(line, "font.ExtraBold") || strings.Contains(line, "font.Black") {
				t.Errorf("%s:%d: a third weight: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}
