// Package paths locates slk's XDG directories. Every binary that reads
// slk's tokens, cache or config goes through it, so they agree.
package paths

import (
	"os"
	"path/filepath"
)

// Config is $XDG_CONFIG_HOME/slk, defaulting to ~/.config/slk.
func Config() string { return xdg("XDG_CONFIG_HOME", ".config") }

// Data is $XDG_DATA_HOME/slk, defaulting to ~/.local/share/slk. Tokens
// and the SQLite cache live here.
func Data() string { return xdg("XDG_DATA_HOME", filepath.Join(".local", "share")) }

// Cache is $XDG_CACHE_HOME/slk, defaulting to ~/.cache/slk. Downloaded
// images live here.
func Cache() string { return xdg("XDG_CACHE_HOME", ".cache") }

func xdg(env, fallback string) string {
	if dir := os.Getenv(env); dir != "" {
		return filepath.Join(dir, "slk")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback, "slk")
}
