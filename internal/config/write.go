package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// writeMu serializes the read-modify-write cycles that edit config.toml
// in place (the version_ts refresh, which fires from one background
// goroutine per workspace, and any other textual saver). Without it two
// concurrent saves lose one another's update, and a reader can observe
// a half-written file and persist the truncation. Take it with
// LockWrites.
var writeMu sync.Mutex

// LockWrites takes the process-wide config.toml write lock and returns
// the function that releases it:
//
//	defer config.LockWrites()()
//
// Hold it across the whole read-modify-write, not just the WriteAtomic.
func LockWrites() (unlock func()) {
	writeMu.Lock()
	return writeMu.Unlock
}

// defaultConfigPerm is the mode a config file is created with when it
// does not exist yet. An existing file keeps its own mode.
const defaultConfigPerm os.FileMode = 0644

// WriteAtomic replaces configPath's contents with data by writing
// a temp file in the same directory and renaming it over the target.
//
// writeMu serialises writers inside one process, but nothing
// stops two slk instances sharing one config.toml. With a plain
// os.WriteFile — truncate, then write — the other process can read the
// file in its truncated state, and every saver is a
// read-modify-write, so it then writes that truncation back: the
// user's themes, sections and workspace entries are gone. The risk
// profile changed when version_ts started saving automatically on
// every boot, once per workspace, rather than only on rare user
// actions.
//
// Rename is atomic within a filesystem, so a reader sees either the
// whole old file or the whole new one. That is why the temp file must
// be created in the target's own directory rather than os.TempDir().
//
// This does not make the read-modify-write cycle itself atomic across
// processes — two instances can still lose one another's *update*.
// Preventing that needs file locking; this fixes the destructive half,
// where a partial read is persisted as the whole file.
func WriteAtomic(configPath string, data []byte) error {
	perm := defaultConfigPerm
	if fi, err := os.Stat(configPath); err == nil {
		perm = fi.Mode().Perm()
	}

	f, err := os.CreateTemp(filepath.Dir(configPath), ".config-*.toml.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// No-op once the rename below succeeds; on any earlier return it
	// keeps a failed save from littering the config directory.
	defer os.Remove(tmp)

	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	// Without this a crash between rename and writeback can leave a
	// zero-length config, which is the same data loss by another route.
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	// CreateTemp makes the file 0600. Restore the mode the config
	// actually had, so a save doesn't silently tighten permissions the
	// user chose.
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}

// TOMLString returns s as a properly escaped TOML basic string,
// including the surrounding quotes. Backslashes and double quotes
// are escaped; control characters become their TOML escape forms.
func TOMLString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// SanitizeComment turns arbitrary text into a single-line comment-safe
// string by replacing CR/LF and ASCII control characters with spaces.
// The leading "# " is added by the caller.
func SanitizeComment(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\r' || r == '\n' || r < 0x20 {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
