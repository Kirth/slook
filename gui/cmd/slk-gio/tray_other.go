//go:build !linux

package main

// startTray: the tray is Linux's (StatusNotifierItem over D-Bus); on
// other systems slk-gio exits when its last window closes.
func startTray(*hub, func()) bool { return false }
