// Package slashcmd recognises slash commands in composer text and words
// a command's failure, for every frontend (docs/superpowers/specs/
// 2026-10-12-slash-commands-design.md §3).
package slashcmd

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gammons/slk/internal/core"
)

// Parse splits composer text into a slash command's name and
// arguments. ok is false for anything that is not a command, which then
// sends as an ordinary message: text not starting with "/" (a leading
// space is the escape hatch), "/" alone, or a first token with anything
// but [A-Za-z0-9_-] after the slash ("/tmp/foo", "/giphy:"). Any Unicode
// space separates the name from its arguments, as in strings.Fields, so a
// pasted non-breaking space still runs the command. name is
// lowercased; args keeps inner whitespace and newlines and is "" when
// only whitespace follows the name.
func Parse(text string) (name, args string, ok bool) {
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	end := 1
	for end < len(text) && isNameByte(text[end]) {
		end++
	}
	if end == 1 {
		return "", "", false
	}
	if r, _ := utf8.DecodeRuneInString(text[end:]); end < len(text) && !unicode.IsSpace(r) {
		return "", "", false
	}
	name = strings.ToLower(text[:end])
	args = strings.TrimLeftFunc(text[end:], unicode.IsSpace)
	if strings.TrimSpace(args) == "" {
		args = ""
	}
	return name, args, true
}

// IsNamePrefix reports whether text could still become a command name:
// "/" followed only by name characters. A frontend completes commands
// while this holds.
func IsNamePrefix(text string) bool {
	if !strings.HasPrefix(text, "/") {
		return false
	}
	for i := 1; i < len(text); i++ {
		if !isNameByte(text[i]) {
			return false
		}
	}
	return true
}

func isNameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}

// FailureText words a command that did not run, for a toast or an error
// line under the composer.
func FailureText(name string, err error) string {
	var ce *core.CommandError
	switch {
	case errors.As(err, &ce) && ce.Code == "unknown_command":
		return name + " is not a valid command"
	case errors.As(err, &ce):
		return name + " failed: " + ce.Code
	case errors.Is(err, errors.ErrUnsupported):
		return name + " failed: not available"
	default:
		return name + " failed: " + err.Error()
	}
}
