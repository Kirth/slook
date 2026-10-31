package main

import (
	"fyne.io/systray"
	"github.com/godbus/dbus/v5"
)

// startTray puts slk-gio's icon in the desktop's tray: a click opens the
// conversation list, the menu opens the list, Threads or Search, or
// quits. It reports whether a tray host is there to show it; without
// one (GNOME without the AppIndicator extension) it does nothing, so
// the caller keeps exiting when the last window closes.
func startTray(h *hub, quit func()) bool {
	if !trayHostPresent() {
		return false
	}
	start, _ := systray.RunWithExternalLoop(func() {
		systray.SetOnTapped(h.showList)
		open := systray.AddMenuItem("Open", "The conversation list")
		threads := systray.AddMenuItem("Threads", "Followed threads")
		search := systray.AddMenuItem("Search", "Search messages")
		systray.AddSeparator()
		stop := systray.AddMenuItem("Quit", "Quit slk")
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					h.showList()
				case <-threads.ClickedCh:
					h.showThreads()
				case <-search.ClickedCh:
					h.showSearch()
				case <-stop.ClickedCh:
					quit()
					return
				}
			}
		}()
	}, nil)
	// Set before start, which publishes them. Set from the ready callback
	// instead, they race start: one that lands after start reads them
	// but before it can update them is lost (fyne.io/systray v1.12.2).
	systray.SetIcon(trayIcon())
	systray.SetTitle("slk")
	systray.SetTooltip("slk — " + h.be.Workspace())
	start()
	return true
}

// trayHostPresent is whether the session bus has a StatusNotifierWatcher
// with a host registered: something that will draw the icon.
func trayHostPresent() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer conn.Close()
	v, err := conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher").
		GetProperty("org.kde.StatusNotifierWatcher.IsStatusNotifierHostRegistered")
	if err != nil {
		return false
	}
	ok, _ := v.Value().(bool)
	return ok
}
