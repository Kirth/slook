package main

import (
	"fmt"
	"os"

	"github.com/gammons/slk/internal/debuglog"
)

// Build-time version info, injected via -ldflags by GoReleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Debug log: when SLK_DEBUG is set, debuglog.Init opens
	// slk-debug.log in cwd (truncating any prior session) and routes
	// both the package-internal logger and the global stdlib log to
	// it. When unset, stdlib log is routed to io.Discard.
	if debugFile, err := debuglog.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "slk: could not open debug log: %v\n", err)
	} else if debugFile != nil {
		// Defer fires only on the clean main() return path; os.Exit
		// in the flag-handling block below skips it. That's fine —
		// the OS reclaims the FD on process exit and stdlib log
		// writes are unbuffered, so no log lines are lost.
		defer debugFile.Close()
		debuglog.General("=== slk debug session started ===")
	}
	// Handle simple flags before anything else
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Printf("slk %s (commit %s, built %s)\n", version, commit, date)
			fmt.Println("Unofficial Slack client. Not affiliated with Slack Technologies, LLC.")
			fmt.Println("Uses Slack's internal browser protocol; may violate Slack's TOS. Use at your own risk.")
			return
		case "--help", "-h", "help":
			printHelp()
			return
		case "--add-workspace", "--browser":
			browser, err := addWorkspaceArgs(os.Args[1:])
			if err == nil {
				err = addWorkspace(browser)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "--remove-workspace":
			if err := removeWorkspace(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "--list-workspaces":
			if err := listWorkspaces(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "export":
			if err := exportChannel(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "--dump-sections":
			if err := dumpSections(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "--dump-prefs":
			if err := dumpPrefs(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	// No argument (or one slk does not know): slk is a CLI now.
	printHelp()
	fmt.Fprintln(os.Stderr, "\nslk is the CLI (workspace setup, export); the desktop app is slk-gio.")
	os.Exit(2)
}

func printHelp() {
	fmt.Printf(`slk %s -- set up Slack workspaces for slk-gio, and export channels

Usage:
  slk --add-workspace     Add a Slack workspace (interactive)
  slk --add-workspace --browser
                          Add one from a browser session (no desktop app)
  slk --remove-workspace  Remove a configured workspace (interactive)
  slk --list-workspaces   List configured workspaces (TeamID, Slug, Name)
  slk --dump-sections     Dump raw users.channelSections.list JSON (diagnostic)
  slk export [flags]      Export a channel's messages and threads to Markdown
                          (see 'slk export --help')
  slk --version          Print version and exit
  slk --help             Show this help

Config:  ~/.config/slk/config.toml
Data:    ~/.local/share/slk/
Cache:   ~/.cache/slk/

Docs:    https://github.com/gammons/slk
`, version)
}
