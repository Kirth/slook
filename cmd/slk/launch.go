package main

import "github.com/gammons/slk/internal/launch"

// launchOS starts the platform's default handler for target (a URL or
// file path). The implementation is internal/launch, shared with slk-gio.
func launchOS(target string) error { return launch.Open(target) }
