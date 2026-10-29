package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"gioui.org/font"
	"gioui.org/font/opentype"
)

// Gio finds system fonts itself, but on NixOS it picks badly: a variable
// "Noto Sans" with no bold, and the same font as the "fallback" for
// glyphs it lacks (katakana, emoji), which then draw as nothing.
// Fontconfig, which the rest of the desktop uses, picks well, so the
// fonts come from it: the text font in each style, monospace, a CJK
// font and the emoji font, loaded explicitly and listed in that order
// so each glyph comes from the first that has it.

// fontRole is one kind of font asked of fontconfig.
type fontRole int

const (
	fontRoleSans fontRole = iota
	fontRoleMono
	fontRoleCJK
	fontRoleScripts // other writing systems the text font may lack
	fontRoleEmoji
)

// fontPatterns are the fontconfig queries for each role, every style of
// a role being one query.
var fontPatterns = []struct {
	role    fontRole
	pattern string
}{
	{fontRoleSans, "sans-serif"},
	{fontRoleSans, "sans-serif:bold"},
	{fontRoleSans, "sans-serif:italic"},
	{fontRoleSans, "sans-serif:bold:italic"},
	{fontRoleMono, "monospace"},
	{fontRoleMono, "monospace:bold"},
	{fontRoleCJK, ":lang=ja"},
	// The shaper does not scan the system's fonts (that costs every
	// window time), so scripts the text font lacks come from these.
	// A query fontconfig answers with a file already loaded costs
	// nothing: files are loaded once.
	{fontRoleScripts, ":lang=ru"},
	{fontRoleScripts, ":lang=el"},
	{fontRoleScripts, ":lang=ar"},
	{fontRoleScripts, ":lang=he"},
	{fontRoleScripts, ":lang=hi"},
	{fontRoleScripts, ":lang=th"},
	{fontRoleEmoji, "emoji"},
}

// fontFile is one font file's bytes and the role it was found for.
type fontFile struct {
	role fontRole
	data []byte
}

// fontFiles reads the files fontconfig names, once per process. Nil
// when fc-match is missing; the theme then keeps Gio's own choice.
var fontFiles = sync.OnceValue(func() []fontFile {
	var out []fontFile
	seen := map[string]bool{}
	for _, p := range fontPatterns {
		raw, err := exec.Command("fc-match", "-f", "%{file}", p.pattern).Output()
		if err != nil {
			if len(out) == 0 {
				return nil // no fontconfig at all
			}
			continue
		}
		path := strings.TrimSpace(string(raw))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: font:", err)
			continue
		}
		out = append(out, fontFile{p.role, data})
	}
	return out
})

// roleFace is a parsed face and the role its file was found for.
type roleFace struct {
	role fontRole
	face font.FontFace
}

// systemFaces parses the fontconfig fonts, once per process. Parsing
// is the expensive part (well over 100 MB of font files here); a
// parsed face is safe to share between shapers, as Gio's opentype
// package says, so every window's shaper gets the same faces.
var systemFaces = sync.OnceValue(func() []roleFace {
	var out []roleFace
	for _, f := range fontFiles() {
		parsed, err := opentype.ParseCollection(f.data)
		if err != nil {
			continue
		}
		for _, face := range parsed {
			out = append(out, roleFace{f.role, face})
		}
	}
	return out
})

// collection is the faces alone, for the shaper.
func collection(faces []roleFace) []font.FontFace {
	out := make([]font.FontFace, len(faces))
	for i, f := range faces {
		out[i] = f.face
	}
	return out
}

// faceList is a typeface naming the families of the given roles, in
// order, then fallback: "DejaVu Sans", "Noto Sans CJK JP", "Noto Color
// Emoji", sans-serif.
func faceList(faces []roleFace, fallback string, roles ...fontRole) font.Typeface {
	var names []string
	seen := map[string]bool{}
	for _, want := range roles {
		for _, r := range faces {
			name := string(r.face.Font.Typeface)
			if r.role != want || name == "" || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, `"`+strings.ReplaceAll(name, `"`, `\"`)+`"`)
		}
	}
	names = append(names, fallback)
	return font.Typeface(strings.Join(names, ", "))
}
