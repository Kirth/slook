package main

import (
	"testing"

	"gioui.org/io/event"
)

// A popup's scrim (a click there closes it) and its panel (clicks on it
// stay) must be different event tags. Two zero-size fields side by side
// share an address, so &p.scrim == &p.panel: a click on an emoji or a
// menu item then reached the scrim and closed the popup unchosen.
func TestPopupTagsAreDistinct(t *testing.T) {
	var picker reactPicker
	var menu msgMenu
	var confirm confirmDelete
	var info infoPanel
	var newMsg newMessagePicker
	for _, c := range []struct {
		name         string
		scrim, panel event.Tag
	}{
		{"reaction picker", &picker.scrim, &picker.panel},
		{"message menu", &menu.scrim, &menu.panel},
		{"delete confirmation", &confirm.scrim, &confirm.panel},
		{"info panel", &info.scrim, &info.tag},
		{"new message", &newMsg.scrim, &newMsg.panel},
	} {
		if c.scrim == c.panel {
			t.Errorf("%s: scrim and panel are the same tag", c.name)
		}
	}
}
