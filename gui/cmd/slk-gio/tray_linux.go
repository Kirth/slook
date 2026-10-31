package main

import (
	"time"

	"fyne.io/systray"
	"github.com/godbus/dbus/v5"

	"github.com/gammons/slk/gui/backend"
)

// startTray puts slk-gio's icon in the desktop's tray: a click opens the
// conversation list, the menu opens the list, Threads or Search, sets
// the user's presence (trayPresenceItems), or quits; the tooltip says
// when they are away or not to be disturbed. It reports whether a tray host is there to show it; without
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
		// One item per choice the tray can offer; which show, and which
		// is checked, follow the user's status.
		pres := make([]*systray.MenuItem, len(trayPresenceLabels))
		for i, label := range trayPresenceLabels {
			if label == "End do not disturb" {
				pres[i] = systray.AddMenuItem(label, "")
			} else {
				pres[i] = systray.AddMenuItemCheckbox(label, "", false)
			}
		}
		show := func(st backend.SelfStatus) {
			items := trayPresenceItems(st, time.Now())
			for i, label := range trayPresenceLabels {
				pres[i].Hide()
				for _, it := range items {
					if it.Label == label {
						pres[i].Show()
						if it.Current {
							pres[i].Check()
						} else {
							pres[i].Uncheck()
						}
					}
				}
			}
		}
		show(h.selfStatus())
		h.mu.Lock()
		h.onSelf = func(st backend.SelfStatus) {
			show(st)
			systray.SetTooltip(trayTooltip(h.be.Workspace(), st, time.Now()))
		}
		h.mu.Unlock()
		pick := func(i int) {
			for _, it := range trayPresenceItems(h.selfStatus(), time.Now()) {
				if it.Label == trayPresenceLabels[i] {
					h.setPresence(it, nil)
				}
			}
		}
		systray.AddSeparator()
		stop := systray.AddMenuItem("Quit", "Quit slk")
		go func() {
			for {
				select {
				case <-pres[0].ClickedCh:
					pick(0)
				case <-pres[1].ClickedCh:
					pick(1)
				case <-pres[2].ClickedCh:
					pick(2)
				case <-pres[3].ClickedCh:
					pick(3)
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
