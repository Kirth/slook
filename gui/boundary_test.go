package gui_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// guiBannedImports are the import-path trees the GUI must never import:
// the retired bubbletea TUI (removed in 2026-10), the terminal toolkit
// it was built on, and the terminal image encoders (sixel) that went with
// it. lipgloss is also covered by "charm.land/"; it is named on its own
// because cmd/slk's prompts still use it, so it is the one most likely to
// be reached for. A direct import of any of them would bring the TUI's
// code or its dependencies back into slk-gio. Only direct imports are
// checked; what internal/ packages import in turn is theirs to answer.
var guiBannedImports = []string{
	"github.com/gammons/slk/internal/ui",
	"github.com/gammons/slk/internal/bubbles",
	"github.com/gammons/slk/internal/demo",
	"github.com/gammons/slk/internal/editor",
	"charm.land/",
	"charm.land/lipgloss",
	"github.com/charmbracelet/",
	"github.com/mattn/go-sixel",
}

// banned reports whether path is one of guiBannedImports or lies under
// it. An entry ending in "/" is a prefix of module paths; any other entry
// matches itself and the packages beneath it.
func banned(path string) (string, bool) {
	for _, b := range guiBannedImports {
		if strings.HasSuffix(b, "/") {
			if strings.HasPrefix(path, b) {
				return b, true
			}
			continue
		}
		if path == b || strings.HasPrefix(path, b+"/") {
			return b, true
		}
	}
	return "", false
}

func TestBoundaryGUINeverImportsTheRetiredTUI(t *testing.T) {
	fset := token.NewFileSet()
	files := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		files++
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if b, ok := banned(p); ok {
				t.Errorf("%s imports %s; nothing under %s may be imported by the GUI", filepath.ToSlash(path), p, b)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("walked no .go files; the test is not looking at the GUI")
	}
}

func TestBoundaryBannedMatchesTreesNotNeighbours(t *testing.T) {
	for path, want := range map[string]bool{
		"github.com/gammons/slk/internal/ui":             true,
		"github.com/gammons/slk/internal/ui/messages":    true,
		"github.com/gammons/slk/internal/bubbles/chrome": true,
		"github.com/gammons/slk/internal/editor":         true,
		"charm.land/lipgloss/v2":                         true,
		"github.com/charmbracelet/x/ansi":                true,
		"github.com/mattn/go-sixel":                      true,
		"github.com/mattn/go-sqlite3":                    false,
		"github.com/gammons/slk/internal/usergroups":     false,
		"github.com/gammons/slk/internal/editorconfig":   false,
		"github.com/gammons/slk/internal/demographics":   false,
		"github.com/gammons/slk/gui/backend":             false,
		"gioui.org/widget":                               false,
	} {
		if _, got := banned(path); got != want {
			t.Errorf("banned(%q) = %v, want %v", path, got, want)
		}
	}
}
