package main

import (
	"fmt"
	"image"
	"io"
	"strconv"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/gui/motion"
	"github.com/gammons/slk/internal/emoji"
)

// Info panel geometry and motion.
const (
	infoOpenDur  = 220 * time.Millisecond
	infoTabDur   = 180 * time.Millisecond
	infoMaxShare = 0.6 // of the window below the header
	infoPadDp    = unit.Dp(16)
	memberAvatar = unit.Dp(28)
	// maxResolving is how many unnamed members the panel asks names
	// for at once, as they scroll into view.
	maxResolving = 8
)

// infoPanel is a chat window's details, opened from its title: it
// grows down out of the header bar, over the timeline, with tabs for
// what the conversation is about, who is in it, and how it notifies.
type infoPanel struct {
	open bool
	prog motion.Tween // 0 closed, 1 open
	// height glides between tabs' heights, in pixels.
	height motion.Tween
	tab    chat.InfoTab
	tabs   [3]widget.Clickable // by chat.InfoTab
	tabAt  [3][2]int           // each tab's x and width, last frame
	under  struct{ x, w motion.Tween }
	// scrim takes clicks on the timeline below; tag is the panel's
	// own area, which keeps the keyboard while open.
	scrim, tag int
	focus      bool
	body       widget.List

	// About
	topic, purpose *msgWidgets
	copyID         widget.Clickable
	creatorHover   widget.Clickable

	// Members
	members   *backend.Members
	loading   bool
	loadErr   string
	filter    widget.Editor
	list      widget.List
	hovers    map[string]*widget.Clickable // avatars, by user ID
	resolving map[string]bool              // names asked for, by user ID
	shown     []backend.Person             // filtered, for query and version
	shownFor  string
	shownVer  int
	version   int // bumped when a member's name arrives

	// Notifications
	levels [3]widget.Clickable // by backend.NotifyLevel
}

type (
	// membersLoaded carries a conversation's member list.
	membersLoaded struct {
		m   backend.Members
		err error
	}
	// notifySetFailed reports a notify level that could not be kept.
	notifySetFailed struct{ err error }
)

// visible reports whether any of the panel is showing.
func (p *infoPanel) visible(now time.Time) bool {
	v, _ := p.prog.Value(now)
	return p.open || v > 0
}

// toggleInfo opens the panel, or closes it when open.
func (c *chatWindow) toggleInfo() {
	p := &c.info
	now := time.Now()
	if p.prog.Target() == 0 {
		p.prog.Set(0, now, 0) // a first Set takes its value at once
	}
	p.open = !p.open
	if p.open {
		p.prog.Set(1, now, infoOpenDur)
		tabs := chat.InfoTabs(c.conv.Kind)
		if !hasTab(tabs, p.tab) {
			p.tab = tabs[0]
		}
		p.focus = true
		c.showTab(p.tab)
	} else {
		p.prog.Set(0, now, infoOpenDur)
		c.focused = false // the composer takes the keyboard back
	}
	c.w.Invalidate()
}

func hasTab(tabs []chat.InfoTab, t chat.InfoTab) bool {
	for _, x := range tabs {
		if x == t {
			return true
		}
	}
	return false
}

// showTab switches to a tab, loading the members the first time they
// are shown.
func (c *chatWindow) showTab(t chat.InfoTab) {
	p := &c.info
	p.tab = t
	if t == chat.TabMembers && p.members == nil && !p.loading {
		p.loading, p.loadErr = true, ""
		id := c.conv.ID
		go func() {
			m, err := c.h.be.Members(c.h.ctx, id)
			c.box.post(membersLoaded{m, err})
		}()
	}
}

// infoApply handles the panel's messages; it reports whether item was
// one.
func (c *chatWindow) infoApply(item any) bool {
	p := &c.info
	switch v := item.(type) {
	case membersLoaded:
		p.loading = false
		if v.err != nil {
			p.loadErr = v.err.Error()
			return true
		}
		p.members = &v.m
		p.version++
		return true
	case notifySetFailed:
		c.notify("Could not change notifications: "+v.err.Error(), true)
		return true
	}
	return false
}

// infoUpdate handles the panel's input before it is drawn.
func (c *chatWindow) infoUpdate(gtx C) {
	p := &c.info
	if !p.visible(gtx.Now) {
		return
	}
	if p.open {
		closing := dismissed(gtx, &p.scrim, &p.tag)
		// Escape from anywhere in the panel: a tab, a choice or the
		// filter may have the keyboard rather than the panel itself.
		for {
			ev, ok := gtx.Event(key.Filter{Focus: &p.filter, Name: key.NameEscape}, key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			if k, ok := ev.(key.Event); ok && k.State == key.Press {
				closing = true
			}
		}
		if closing {
			c.toggleInfo()
			return
		}
	}
	for _, t := range chat.InfoTabs(c.conv.Kind) {
		if p.tabs[t].Clicked(gtx) {
			c.showTab(t)
		}
	}
	if p.copyID.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(c.conv.ID))})
		c.notify("Copied "+c.conv.ID, false)
	}
	for l := range p.levels {
		if p.levels[l].Clicked(gtx) {
			level := backend.NotifyLevel(l)
			if level != c.conv.Notify {
				c.conv.Notify = level
				id := c.conv.ID
				go func() {
					if err := c.h.be.SetNotify(c.h.ctx, id, level); err != nil {
						c.box.post(notifySetFailed{err})
					}
				}()
			}
		}
	}
	for _, w := range []*msgWidgets{p.topic, p.purpose} {
		if w != nil {
			w.handle(gtx, c.setSpanHover, func(r previewReq) { c.startPreview(r) }, nil, c.view.open)
		}
	}
}

// infoLayout draws the panel under the header (top pixels down): the
// timeline dims, and the panel grows down from the bar, its content
// sliding with it.
func (c *chatWindow) infoLayout(gtx C, top int) {
	p := &c.info
	v, moving := p.prog.Value(gtx.Now)
	if moving {
		gtx.Execute(op.InvalidateCmd{})
	}
	if v <= 0 {
		return
	}
	defer op.Offset(image.Pt(0, top)).Push(gtx.Ops).Pop()
	size := image.Pt(gtx.Constraints.Max.X, max(0, gtx.Constraints.Max.Y-top))
	gtx.Constraints = layout.Constraints{Max: size}

	dim := colShadow
	dim.A = uint8(min(255, float64(colShadow.A)*v))
	paint.FillShape(gtx.Ops, dim, clip.Rect{Max: size}.Op())
	if p.open {
		popupScrim(gtx, &p.scrim)
	}

	cgtx := gtx
	cgtx.Constraints = layout.Constraints{Min: image.Pt(size.X, 0), Max: image.Pt(size.X, int(float64(size.Y)*infoMaxShare))}
	rec := op.Record(gtx.Ops)
	dims := c.infoContent(cgtx)
	content := rec.Stop()

	p.height.Set(float64(dims.Size.Y), gtx.Now, infoTabDur)
	full, resizing := p.height.Value(gtx.Now)
	if resizing {
		gtx.Execute(op.InvalidateCmd{})
	}
	h := int(full * v)
	rec = op.Record(gtx.Ops)
	{
		area := clip.Rect{Max: image.Pt(size.X, h)}.Push(gtx.Ops)
		paint.Fill(gtx.Ops, colHeader)
		slide := op.Offset(image.Pt(0, h-dims.Size.Y)).Push(gtx.Ops)
		fade := paint.PushOpacity(gtx.Ops, float32(v))
		content.Add(gtx.Ops)
		fade.Pop()
		slide.Pop()
		area.Pop()
		paint.FillShape(gtx.Ops, colBorder, clip.Rect{Min: image.Pt(0, h-gtx.Dp(1)), Max: image.Pt(size.X, h)}.Op())
	}
	sheet := rec.Stop()
	popupPanel(gtx, image.Point{}, D{Size: image.Pt(size.X, h)}, sheet, &p.tag, &p.focus)
}

// infoContent is the panel at full size: the tabs, then the chosen
// tab's body.
func (c *chatWindow) infoContent(gtx C) D {
	p := &c.info
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(c.infoTabs),
		layout.Rigid(func(gtx C) D {
			paint.FillShape(gtx.Ops, colBorder, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))}.Op())
			return D{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))}
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(14), Left: infoPadDp, Right: infoPadDp}.Layout(gtx, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				if p.tab == chat.TabMembers {
					return c.infoMembers(gtx)
				}
				// About and Notifications scroll when the window is
				// too short for them.
				p.body.Axis = layout.Vertical
				return material.List(c.th, &p.body).Layout(gtx, 1, func(gtx C, _ int) D {
					if p.tab == chat.TabNotify {
						return c.infoNotify(gtx)
					}
					return c.infoAbout(gtx)
				})
			})
		}),
	)
}

// infoTabs is the row of tabs, with an accent underline gliding to the
// chosen one.
func (c *chatWindow) infoTabs(gtx C) D {
	p := &c.info
	tabs := chat.InfoTabs(c.conv.Kind)
	full := gtx.Constraints.Max.X
	gtx.Constraints.Min = image.Point{} // each tab its own width
	pad, gap := gtx.Dp(infoPadDp), gtx.Dp(22)
	x, height := pad, 0
	for _, t := range tabs {
		cl := &p.tabs[t]
		col := colMuted
		if t == p.tab {
			col = colFg
		} else if cl.Hovered() {
			col = colFg
		}
		m := op.Record(gtx.Ops)
		dims := cl.Layout(gtx, func(gtx C) D {
			pointer.CursorPointer.Add(gtx.Ops)
			return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
				lbl := secondaryLabel(c.th, c.tabLabel(t))
				lbl.Color = col
				lbl.Font.Weight = font.SemiBold
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			})
		})
		call := m.Stop()
		st := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		st.Pop()
		p.tabAt[t] = [2]int{x, dims.Size.X}
		x += dims.Size.X + gap
		height = max(height, dims.Size.Y)
	}
	at := p.tabAt[p.tab]
	p.under.x.Set(float64(at[0]), gtx.Now, infoTabDur)
	p.under.w.Set(float64(at[1]), gtx.Now, infoTabDur)
	ux, m1 := p.under.x.Value(gtx.Now)
	uw, m2 := p.under.w.Value(gtx.Now)
	if m1 || m2 {
		gtx.Execute(op.InvalidateCmd{})
	}
	bar := gtx.Dp(2)
	paint.FillShape(gtx.Ops, colAccent, clip.UniformRRect(image.Rect(int(ux), height-bar, int(ux+uw), height), bar/2).Op(gtx.Ops))
	return D{Size: image.Pt(full, height)}
}

// tabLabel names a tab; Members carries the count when it is known.
func (c *chatWindow) tabLabel(t chat.InfoTab) string {
	switch t {
	case chat.TabMembers:
		n := 0
		if c.details != nil {
			n = c.details.Members
		}
		if p := c.info.members; p != nil && !p.Partial {
			n = p.Total
		}
		if n > 0 {
			return "Members · " + strconv.Itoa(n)
		}
		return "Members"
	case chat.TabNotify:
		return "Notifications"
	}
	return "About"
}

// infoSection is a caption heading over its content.
func (c *chatWindow) infoSection(title string, w layout.Widget) layout.FlexChild {
	return layout.Rigid(func(gtx C) D {
		return layout.Inset{Bottom: unit.Dp(14)}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					lbl := captionLabel(c.th, title)
					lbl.Color = colMuted
					lbl.Font.Weight = font.SemiBold
					return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, lbl.Layout)
				}),
				layout.Rigid(w),
			)
		})
	})
}

// infoAbout is what the conversation is: a channel's topic,
// description, creator and ID; a DM's person.
func (c *chatWindow) infoAbout(gtx C) D {
	if c.conv.Kind == backend.KindDM {
		return c.infoPerson(gtx)
	}
	d := c.details
	if d == nil {
		lbl := secondaryLabel(c.th, "Loading…")
		lbl.Color = colMuted
		return lbl.Layout(gtx)
	}
	p := &c.info
	if p.topic == nil {
		p.topic, p.purpose = newMsgWidgets(), newMsgWidgets()
	}
	var rows []layout.FlexChild
	if len(d.TopicBody) > 0 {
		rows = append(rows, c.infoSection("Topic", func(gtx C) D {
			p.topic.begin()
			return c.view.blocks(gtx, p.topic, d.TopicBody)
		}))
	}
	if len(d.PurposeBody) > 0 {
		rows = append(rows, c.infoSection("Description", func(gtx C) D {
			p.purpose.begin()
			return c.view.blocks(gtx, p.purpose, d.PurposeBody)
		}))
	}
	if line := chat.CreatedLine(d.Creator, d.Created); line != "" {
		rows = append(rows, c.infoSection("Created", func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					if d.CreatorID == "" {
						return D{}
					}
					url := ""
					if e := c.profile(d.CreatorID); e.p != nil {
						url = e.p.AvatarURL
					}
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
						dims := p.creatorHover.Layout(gtx, func(gtx C) D {
							return c.view.avatar(gtx, d.CreatorID, url, d.Creator, unit.Dp(20))
						})
						// Read after layout: the card is drawn later
						// this frame.
						if p.creatorHover.Hovered() {
							c.authorHover, c.hoverName = d.CreatorID, d.Creator
						}
						return dims
					})
				}),
				layout.Flexed(1, func(gtx C) D {
					return secondaryLabel(c.th, line).Layout(gtx)
				}),
			)
		}))
	}
	if d.Archived {
		rows = append(rows, layout.Rigid(func(gtx C) D {
			return layout.Inset{Bottom: unit.Dp(14)}.Layout(gtx, func(gtx C) D {
				lbl := secondaryLabel(c.th, "This channel is archived.")
				lbl.Color = colOffline
				return lbl.Layout(gtx)
			})
		}))
	}
	rows = append(rows, layout.Rigid(func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				lbl := captionLabel(c.th, "Channel ID  "+c.conv.ID)
				lbl.Color = colMuted
				lbl.Font = fontMono
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
					return iconButton(gtx, &p.copyID, iconCopy, unit.Dp(24), unit.Dp(14), colMuted, colHeader, colHover)
				})
			}),
		)
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

// infoPerson is a DM's other person: picture, name and title, status,
// handle and local time.
func (c *chatWindow) infoPerson(gtx C) D {
	e := c.profile(c.conv.PeerID)
	if e.p == nil {
		text, col := "Loading…", colMuted
		if e.err != nil {
			text, col = e.err.Error(), colError
		}
		lbl := secondaryLabel(c.th, text)
		lbl.Color = col
		return lbl.Layout(gtx)
	}
	pr := *e.p
	var rows []layout.FlexChild
	rows = append(rows, layout.Rigid(func(gtx C) D {
		return layout.Inset{Bottom: unit.Dp(14)}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
						return c.view.avatar(gtx, pr.ID, pr.AvatarURL, pr.Name, unit.Dp(48))
					})
				}),
				layout.Flexed(1, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							lbl := bodyLabel(c.th, pr.Name)
							lbl.Font.Weight = font.SemiBold
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx C) D {
							var parts []string
							if pr.RealName != "" && pr.RealName != pr.Name {
								parts = append(parts, pr.RealName)
							}
							if pr.Title != "" {
								parts = append(parts, pr.Title)
							}
							if len(parts) == 0 {
								return D{}
							}
							lbl := secondaryLabel(c.th, strings.Join(parts, " · "))
							lbl.Color = colMuted
							return lbl.Layout(gtx)
						}),
					)
				}),
			)
		})
	}))
	if pr.StatusEmoji != "" || pr.StatusText != "" {
		status := strings.TrimSpace(strings.TrimSpace(emoji.Sprint(pr.StatusEmoji)) + " " + pr.StatusText)
		rows = append(rows, c.infoSection("Status", func(gtx C) D { return secondaryLabel(c.th, status).Layout(gtx) }))
	}
	if pr.HasTZ {
		local := gtx.Now.UTC().Add(time.Duration(pr.TZOffset) * time.Second)
		text := local.Format("15:04") + " local time"
		if pr.TZLabel != "" {
			text += " · " + pr.TZLabel
		}
		rows = append(rows, c.infoSection("Local time", func(gtx C) D {
			lbl := secondaryLabel(c.th, text)
			if h := local.Hour(); h < 8 || h >= 20 {
				lbl.Color = colOffline
			}
			return lbl.Layout(gtx)
		}))
	}
	if pr.Handle != "" {
		rows = append(rows, layout.Rigid(func(gtx C) D {
			lbl := captionLabel(c.th, "@"+pr.Handle)
			lbl.Color = colMuted
			return lbl.Layout(gtx)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

// fillNames copies names that profile lookups brought into the member
// list, so they sort and filter like the rest.
func (c *chatWindow) fillNames() {
	p := &c.info
	for id := range p.resolving {
		e := c.profiles[id]
		if e == nil || (e.p == nil && e.err == nil) {
			continue
		}
		delete(p.resolving, id)
		if e.p == nil {
			continue
		}
		for i := range p.members.People {
			if q := &p.members.People[i]; q.ID == id && q.Name == "" {
				q.Name, q.RealName, q.Handle, q.AvatarURL = e.p.Name, e.p.RealName, e.p.Handle, e.p.AvatarURL
				p.version++
			}
		}
	}
}

// infoMembers is who is in the conversation, with a filter. Names not
// yet known are asked for as their rows come into view, a few at a
// time.
func (c *chatWindow) infoMembers(gtx C) D {
	p := &c.info
	switch {
	case p.members == nil && p.loadErr != "":
		lbl := secondaryLabel(c.th, p.loadErr)
		lbl.Color = colError
		return lbl.Layout(gtx)
	case p.members == nil:
		lbl := secondaryLabel(c.th, "Loading members…")
		lbl.Color = colMuted
		return lbl.Layout(gtx)
	}
	if p.resolving == nil {
		p.resolving, p.hovers = map[string]bool{}, map[string]*widget.Clickable{}
	}
	c.fillNames()
	query := p.filter.Text()
	if query != p.shownFor || p.version != p.shownVer || p.shown == nil {
		p.shown = chat.FilterMembers(p.members.People, query)
		p.shownFor, p.shownVer = query, p.version
	}
	shown := p.shown
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				p.filter.SingleLine = true
				return framedEditor(c.th, &p.filter, "Filter members")(gtx)
			})
		}),
		layout.Rigid(func(gtx C) D {
			if !p.members.Partial {
				return D{}
			}
			text := fmt.Sprintf("Showing the first %d members", len(p.members.People))
			if c.details != nil && c.details.Members > len(p.members.People) {
				text = fmt.Sprintf("Showing %d of %d members", len(p.members.People), c.details.Members)
			}
			return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
				lbl := captionLabel(c.th, text)
				lbl.Color = colMuted
				return lbl.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx C) D {
			if len(shown) == 0 {
				lbl := secondaryLabel(c.th, "No one matches.")
				lbl.Color = colMuted
				return lbl.Layout(gtx)
			}
			p.list.Axis = layout.Vertical
			return material.List(c.th, &p.list).Layout(gtx, len(shown), func(gtx C, i int) D {
				return c.memberRow(gtx, shown[i])
			})
		}),
	)
}

// memberRow is one member: picture (its hover shows the profile card),
// name, and real name when it differs.
func (c *chatWindow) memberRow(gtx C, m backend.Person) D {
	p := &c.info
	name := m.Name
	if name == "" {
		if !p.resolving[m.ID] && len(p.resolving) < maxResolving {
			p.resolving[m.ID] = true
			c.profile(m.ID)
		}
	}
	h := p.hovers[m.ID]
	if h == nil {
		h = &widget.Clickable{}
		p.hovers[m.ID] = h
	}
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
					dims := h.Layout(gtx, func(gtx C) D {
						return c.view.avatar(gtx, m.ID, m.AvatarURL, name, memberAvatar)
					})
					if h.Hovered() {
						c.authorHover, c.hoverName = m.ID, name
					}
					return dims
				})
			}),
			layout.Flexed(1, func(gtx C) D {
				if name == "" {
					lbl := secondaryLabel(c.th, m.ID)
					lbl.Color = colMuted
					return lbl.Layout(gtx)
				}
				return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						lbl := secondaryLabel(c.th, name)
						lbl.MaxLines = 1
						if m.ID == c.h.be.Self() && !strings.EqualFold(name, "you") {
							lbl.Text = name + " (you)"
						}
						return lbl.Layout(gtx)
					}),
					layout.Flexed(1, func(gtx C) D {
						if m.RealName == "" || m.RealName == name {
							return D{}
						}
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
							lbl := captionLabel(c.th, m.RealName)
							lbl.Color = colMuted
							lbl.MaxLines = 1
							return lbl.Layout(gtx)
						})
					}),
				)
			}),
		)
	})
}

// notifyChoices are the notify levels as the panel offers them, by
// backend.NotifyLevel.
var notifyChoices = [3]struct{ label, desc string }{
	{"Default", "As configured: mentions, direct messages and your keywords."},
	{"Every message", "Every new message here, even while it is muted in Slack."},
	{"Nothing", "No desktop notifications from here."},
}

// infoNotify chooses how the conversation notifies in slk, with notes
// on what overrides it.
func (c *chatWindow) infoNotify(gtx C) D {
	p := &c.info
	var rows []layout.FlexChild
	for l, ch := range notifyChoices {
		level := backend.NotifyLevel(l)
		rows = append(rows, layout.Rigid(func(gtx C) D {
			return p.levels[l].Layout(gtx, func(gtx C) D {
				pointer.CursorPointer.Add(gtx.Ops)
				bg := colHeader
				if p.levels[l].Hovered() {
					bg = colHover
				}
				return rounded(gtx, bg, unit.Dp(6), func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
						return layout.Flex{}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								return layout.Inset{Top: unit.Dp(2), Right: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
									return radio(gtx, level == c.conv.Notify)
								})
							}),
							layout.Flexed(1, func(gtx C) D {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(secondaryLabel(c.th, ch.label).Layout),
									layout.Rigid(func(gtx C) D {
										lbl := captionLabel(c.th, ch.desc)
										lbl.Color = colMuted
										return lbl.Layout(gtx)
									}),
								)
							}),
						)
					})
				})
			})
		}))
	}
	note := func(text string) layout.FlexChild {
		return layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(14), gtx.Dp(14)))
							return iconMuted.Layout(gtx, colMuted)
						})
					}),
					layout.Flexed(1, func(gtx C) D {
						lbl := captionLabel(c.th, text)
						lbl.Color = colMuted
						return lbl.Layout(gtx)
					}),
				)
			})
		})
	}
	if c.conv.Muted {
		rows = append(rows, note("Muted in Slack: only “Every message” notifies here."))
	}
	if c.h.notes == nil {
		rows = append(rows, note("Desktop notifications are off in your config ([notifications] enabled)."))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

// radio is a round choice marker: a ring, filled with a dot when on.
func radio(gtx C, on bool) D {
	d := gtx.Dp(16)
	box := image.Rectangle{Max: image.Pt(d, d)}
	ring := colMuted
	if on {
		ring = colAccent
	}
	paint.FillShape(gtx.Ops, ring, clip.Ellipse(box).Op(gtx.Ops))
	in := gtx.Dp(2)
	paint.FillShape(gtx.Ops, colHeader, clip.Ellipse(box.Inset(in)).Op(gtx.Ops))
	if on {
		paint.FillShape(gtx.Ops, colAccent, clip.Ellipse(box.Inset(gtx.Dp(5))).Op(gtx.Ops))
	}
	return D{Size: box.Max}
}
