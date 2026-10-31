package main

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
)

// presenceMenu is the user's own presence menu, opened from the list's
// header tile: Active, Away, the snoozes and, during do-not-disturb, a
// way to end it (chat.PresenceItems), the current one checked.
type presenceMenu struct {
	open  bool
	focus bool
	at    image.Point // its top left, in window pixels
	items []chat.PresenceItem
	rows  [12]widget.Clickable
	// scrim and panel are bytes so their tags differ; see msgMenu.
	scrim, panel byte
}

// show opens the menu at at for the user's status st.
func (pm *presenceMenu) show(st backend.SelfStatus, at image.Point, now time.Time) {
	pm.open, pm.focus, pm.at = true, true, at
	pm.items = chat.PresenceItems(st, now)
}

// update handles clicks before the menu is drawn: the item picked, if
// any, and whether the menu closed (picked, a click outside, Escape).
func (pm *presenceMenu) update(gtx C) (pick *chat.PresenceItem, closed bool) {
	if !pm.open {
		return nil, false
	}
	for i := range pm.items {
		if i < len(pm.rows) && pm.rows[i].Clicked(gtx) {
			pm.open = false
			it := pm.items[i]
			return &it, true
		}
	}
	if dismissed(gtx, &pm.scrim, &pm.panel) {
		pm.open = false
		return nil, true
	}
	return nil, false
}

// layout draws the menu, kept inside the window, with a rule between
// the presence rows and the snoozes and before ending do-not-disturb.
func (pm *presenceMenu) layout(gtx C, th *material.Theme) {
	if !pm.open {
		return
	}
	popupScrim(gtx, &pm.scrim)
	cgtx := gtx
	cgtx.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(250), gtx.Constraints.Max.Y)}
	cgtx.Constraints.Min.X = cgtx.Constraints.Max.X
	rec := op.Record(gtx.Ops)
	dims := popupFrame(cgtx, func(gtx C) D {
		var rows []layout.FlexChild
		for i, it := range pm.items {
			if i >= len(pm.rows) {
				break
			}
			if i == 2 || it.End {
				rows = append(rows, layout.Rigid(menuRule))
			}
			rows = append(rows, layout.Rigid(func(gtx C) D {
				return menuRowAt(gtx, th, &pm.rows[i], it.Label, false, it.Current)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
	call := rec.Stop()
	off := pm.at
	off.X = max(0, min(off.X, gtx.Constraints.Max.X-dims.Size.X))
	off.Y = max(0, min(off.Y, gtx.Constraints.Max.Y-dims.Size.Y))
	popupPanel(gtx, off, dims, call, &pm.panel, &pm.focus)
}

// menuRule is a thin line between groups of a menu's rows.
func menuRule(gtx C) D {
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
		size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))
		paint.FillShape(gtx.Ops, colBorder, clip.Rect{Max: size}.Op())
		return D{Size: size}
	})
}
