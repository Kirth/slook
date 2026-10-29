package main

import "github.com/gammons/slk/internal/paths"

// The XDG directories live in internal/paths so other binaries share
// them; these names stay for the many callers in this package.
func xdgConfig() string { return paths.Config() }
func xdgData() string   { return paths.Data() }
func xdgCache() string  { return paths.Cache() }
