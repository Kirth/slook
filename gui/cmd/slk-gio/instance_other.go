//go:build !linux

package main

// claimInstance: one instance per workspace is kept over D-Bus, on
// Linux; elsewhere every launch runs.
func claimInstance(string, func()) bool { return true }
