package main

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/gui/sidebar"
	"github.com/gammons/slk/internal/emoji"
	"github.com/gammons/slk/internal/ui/messages"
)

// listWindow is the conversation list: a filter, then the conversations
// arranged by sidebar.Arrange. Choosing a row opens that conversation's
// own window.
type listWindow struct {
	h    *hub
	w    *app.Window
	box  inbox
	th   *material.Theme
	view richView

	convs     []backend.Conversation
	index     map[string]int // conversation ID -> position in convs
	sections  []backend.Section
	loading   bool
	loadErr   string
	connected bool
	filter    widget.Editor
	rl        readline
	list      widget.List
	rows      map[string]*widget.Clickable // by conversation ID
	folds     map[string]*widget.Clickable // "+N quiet", by group key
	expanded  map[string]bool              // groups whose quiet ones are shown
	focused   bool
	cfg       app.Config
	hdr       headerBar
	presence  map[string]presence // DM partners, by user ID
	customs   map[string]string   // the workspace's custom emoji
	sizeDp    image.Point
	moved     bool
	shown     bool // the first frame is out
}

// listState is what the list hands to its replacement when it moves to
// the user's workspace.
type listState struct {
	convs     []backend.Conversation
	sections  []backend.Section
	expanded  map[string]bool
	presence  map[string]presence
	filter    string
	connected bool
	loadErr   string
	sizeDp    image.Point
}

// newListWindowFrom opens a list that carries on where another left
// off.
func newListWindowFrom(h *hub, st listState) *listWindow {
	l := newListWindow(h, st.connected)
	l.loading = false
	l.loadErr = st.loadErr
	l.setConvs(st.convs)
	l.sections, l.expanded, l.presence = st.sections, st.expanded, st.presence
	l.filter.SetText(st.filter)
	if st.sizeDp.X > 0 && st.sizeDp.Y > 0 {
		l.w.Option(app.Size(unit.Dp(st.sizeDp.X), unit.Dp(st.sizeDp.Y)))
	}
	return l
}

// moveHere is chatWindow.moveHere for the list.
func (l *listWindow) moveHere() {
	if l.moved {
		return
	}
	l.moved = true
	l.h.adoptList(listState{
		convs: l.convs, sections: l.sections, expanded: l.expanded, presence: l.presence,
		filter: l.filter.Text(), connected: l.connected, loadErr: l.loadErr, sizeDp: l.sizeDp,
	})
	l.w.Perform(system.ActionClose)
}

// setConvs replaces the conversations and their index.
func (l *listWindow) setConvs(convs []backend.Conversation) {
	l.convs = convs
	l.index = make(map[string]int, len(convs))
	for i, c := range convs {
		l.index[c.ID] = i
	}
}

type conversationsLoaded struct {
	convs []backend.Conversation
	err   error
}

// listItem is one line of the list: a group heading, a conversation,
// or a group's quiet fold.
type listItem struct {
	kind  int
	group *sidebar.Group
	conv  backend.Conversation
	quiet bool // a conversation shown from an expanded fold
}

const (
	itemHeading = iota
	itemConv
	itemFold
)

func newListWindow(h *hub, connected bool) *listWindow {
	l := &listWindow{
		h:         h,
		w:         new(app.Window),
		th:        newTheme(),
		loading:   true,
		connected: connected,
		index:     map[string]int{},
		rows:      map[string]*widget.Clickable{},
		folds:     map[string]*widget.Clickable{},
		expanded:  map[string]bool{},
		presence:  map[string]presence{},
	}
	l.box.w = l.w
	l.view = richView{th: l.th, images: h.images, box: &l.box, myGroup: h.isMyGroup}
	l.filter.SingleLine = true
	l.filter.Submit = true
	l.list.Axis = layout.Vertical
	l.w.Option(app.Title("slk — "+h.be.Workspace()), app.Size(unit.Dp(340), unit.Dp(720)),
		app.Decorated(false)) // the header bar is ours
	return l
}

func (l *listWindow) load() {
	go func() {
		if customs, err := l.h.customEmoji(); err == nil {
			l.box.post(customEmojiLoaded{customs})
		}
	}()
	convs, err := l.h.be.Conversations(l.h.ctx)
	if err == nil {
		l.h.setConversations(convs)
	}
	l.box.post(conversationsLoaded{convs, err})
}

// customEmojiLoaded carries the workspace's custom emoji, for section
// headings.
type customEmojiLoaded struct{ customs map[string]string }

func (l *listWindow) run() {
	var ops op.Ops
	for {
		switch e := l.w.Event().(type) {
		case app.DestroyEvent:
			l.h.closed(l, e.Err)
			return
		case app.ConfigEvent:
			l.cfg = e.Config
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			l.apply()
			l.layout(gtx)
			e.Frame(gtx.Ops)
			if !l.shown {
				l.shown = true
				refillThemes() // for the first conversation opened
			}
		}
	}
}

// conv returns the conversation with id for updating, or nil.
func (l *listWindow) conv(id string) *backend.Conversation {
	if i, ok := l.index[id]; ok {
		return &l.convs[i]
	}
	return nil
}

func (l *listWindow) apply() {
	items := l.box.drain()
	if len(items) > 0 {
		defer l.publish()
	}
	for _, item := range items {
		switch v := item.(type) {
		case conversationsLoaded:
			l.loading = false
			if v.err != nil {
				l.loadErr = v.err.Error()
				continue
			}
			l.setConvs(v.convs)
			l.sections = l.h.be.Sections()
		case activity:
			if c := l.conv(v.channelID); c != nil {
				if v.at.After(c.Latest) {
					c.Latest = v.at
				}
				if v.unread {
					c.Unread = true
				}
			}
		case visited:
			if c := l.conv(v.channelID); c != nil {
				c.Visited = v.at
			}
		case backend.ReadChanged:
			if c := l.conv(v.ChannelID); c != nil {
				c.Unread, c.Mentions = v.Unread, v.Mentions
			}
		case backend.SectionsChanged:
			for i := range l.convs {
				l.convs[i].Section = v.Membership[l.convs[i].ID]
			}
			l.sections = l.h.be.Sections()
		case backend.MutesChanged:
			for i := range l.convs {
				l.convs[i].Muted = v.Muted[l.convs[i].ID]
			}
		case backend.NotifyChanged:
			for i := range l.convs {
				if l.convs[i].ID == v.ChannelID {
					l.convs[i].Notify = v.Level
				}
			}
		case customEmojiLoaded:
			l.customs = v.customs
		case backend.ConnectionChanged:
			l.connected = v.Connected
		case raiseWindow:
			l.w.Perform(system.ActionRaise)
		case moveHere:
			l.moveHere()
		case backend.PresenceChanged:
			l.presence[v.UserID] = presenceAway
			if v.Active {
				l.presence[v.UserID] = presenceActive
			}
		case imageReady:
			// The post already woke the window; the row redraws.
		}
	}
}

// publish gives the hub a copy of the conversations, with their
// current read state and activity, for chat windows' switchers, and
// the unread summary for their badges.
func (l *listWindow) publish() {
	if l.convs == nil {
		return
	}
	l.h.setConversations(append([]backend.Conversation(nil), l.convs...))
	unread, mentions := 0, 0
	for _, c := range l.convs {
		if sidebar.NeedsReading(c) {
			unread++
			if c.Mentions > 0 {
				mentions++
			}
		}
	}
	l.h.setUnread(unread, mentions)
}

// items is what the list shows now: the arrangement, or with a filter
// typed, every matching conversation.
func (l *listWindow) items(now time.Time) []listItem {
	if q := strings.TrimSpace(l.filter.Text()); q != "" {
		var out []listItem
		for _, c := range sidebar.Search(l.convs, q) {
			out = append(out, listItem{kind: itemConv, conv: c})
		}
		return out
	}
	groups := sidebar.Arrange(l.convs, l.sections, now, sidebar.QuietAfter)
	var out []listItem
	for i := range groups {
		g := &groups[i]
		out = append(out, listItem{kind: itemHeading, group: g})
		for _, c := range g.Rows {
			out = append(out, listItem{kind: itemConv, conv: c})
		}
		if len(g.Quiet) == 0 {
			continue
		}
		out = append(out, listItem{kind: itemFold, group: g})
		if l.expanded[g.Key] {
			for _, c := range g.Quiet {
				out = append(out, listItem{kind: itemConv, conv: c, quiet: true})
			}
		}
	}
	return out
}

func (l *listWindow) open(c backend.Conversation) { l.h.openChat(c) }

func (l *listWindow) layout(gtx C) {
	fillBackground(gtx)
	l.sizeDp = image.Pt(int(float32(gtx.Constraints.Max.X)/gtx.Metric.PxPerDp), int(float32(gtx.Constraints.Max.Y)/gtx.Metric.PxPerDp))
	submitted := false
	l.rl.keys(gtx, &l.filter)
	for {
		ev, ok := l.filter.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			submitted = true
		}
	}
	for id, btn := range l.rows {
		if btn.Clicked(gtx) {
			if c := l.conv(id); c != nil {
				l.open(*c)
			}
		}
	}
	for key, btn := range l.folds {
		if btn.Clicked(gtx) {
			l.expanded[key] = !l.expanded[key]
		}
	}
	items := l.items(gtx.Now)
	if submitted && strings.TrimSpace(l.filter.Text()) != "" {
		for _, it := range items {
			if it.kind == itemConv {
				l.open(it.conv)
				break
			}
		}
		l.filter.SetText("")
		items = l.items(gtx.Now)
	}
	if !l.focused {
		gtx.Execute(key.FocusCmd{Tag: &l.filter})
		l.focused = true
	}
	// Redraw now and then so conversations age into Quiet and the
	// times on the right stay true.
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Minute)})

	spec := l.headerSpec()
	l.hdr.update(gtx, l.w, l.cfg, spec)

	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return l.hdr.layout(gtx, l.th, spec, l.cfg) }),
		layout.Rigid(func(gtx C) D {
			return layout.UniformInset(unit.Dp(10)).Layout(gtx, framedEditor(l.th, &l.filter, "Find a conversation"))
		}),
		layout.Rigid(divider),
		layout.Flexed(1, func(gtx C) D {
			switch {
			case l.loading:
				return notice(gtx, l.th, "Loading conversations…", colMuted)
			case l.loadErr != "":
				return notice(gtx, l.th, l.loadErr, colError)
			case len(items) == 0 && strings.TrimSpace(l.filter.Text()) != "":
				return notice(gtx, l.th, "No conversation matches.", colMuted)
			}
			return material.List(l.th, &l.list).Layout(gtx, len(items), func(gtx C, i int) D {
				it := items[i]
				switch it.kind {
				case itemHeading:
					return l.heading(gtx, it.group)
				case itemFold:
					return l.fold(gtx, it.group)
				}
				return l.row(gtx, it.conv, it.quiet)
			})
		}),
	)
}

// headerSpec is the list's header: the workspace and how much is
// waiting to be read.
func (l *listWindow) headerSpec() headerSpec {
	name := l.h.be.Workspace()
	spec := headerSpec{
		Title:   name,
		Leading: func(gtx C) D { return tile(gtx, l.th, leadingSize, tint(colAccent), nil, initial(name)) },
		Actions: []headerAction{{icon: iconSearch, onClick: l.h.showSearch}},
	}
	n := 0
	for _, c := range l.convs {
		if sidebar.NeedsReading(c) {
			n++
		}
	}
	switch {
	case !l.connected:
		spec.Subtitle, spec.SubColor = "Offline · reconnecting…", colOffline
	case l.loading:
		spec.Subtitle = "Loading…"
	case n == 0:
		spec.Subtitle = "All caught up"
	case n == 1:
		spec.Subtitle = "1 unread conversation"
	default:
		spec.Subtitle = strconv.Itoa(n) + " unread conversations"
	}
	return spec
}

// heading is a group's title: "UNREAD", "★ STARRED", "🚀 PROJECTS".
// Slack names a section's emoji by shortcode ("rocket"); a standard one
// becomes its character, a custom one its picture, and an unknown one
// is left out rather than shown as a word.
func (l *listWindow) heading(gtx C, g *sidebar.Group) D {
	title := strings.ToUpper(g.Title)
	var picture string // a custom emoji's URL
	switch glyph, ok := rich.EmojiGlyph(g.Emoji); {
	case g.Emoji == "":
	case ok:
		title = glyph + " " + title
	case !isShortcode(g.Emoji):
		title = g.Emoji + " " + title // already a character, like Starred's ★
	default:
		if url, ok := emoji.BuildCustomEmojiURL(strings.Trim(g.Emoji, ":"), l.customs); ok {
			picture = url
		}
	}
	return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(2), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				if picture == "" {
					return D{}
				}
				op, state := l.view.images.get(emoji.EmojiCacheKey(picture), picture, image.Point{}, &l.box)
				if state != imgReady {
					return D{}
				}
				return layout.Inset{Right: unit.Dp(5)}.Layout(gtx, func(gtx C) D {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(14), gtx.Dp(14)))
					return widget.Image{Src: op, Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
				})
			}),
			layout.Flexed(1, func(gtx C) D {
				lbl := captionLabel(l.th, title)
				lbl.Color = colMuted
				lbl.Font.Weight = font.SemiBold
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
		)
	})
}

// isShortcode reports whether s is an emoji's name ("rocket",
// ":robot_face:", "wave::skin-tone-2") rather than the emoji itself.
func isShortcode(s string) bool {
	for _, r := range s {
		ok := r == '_' || r == '-' || r == '+' || r == ':' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if !ok {
			return false
		}
	}
	return s != ""
}

// fold is the "+N quiet" toggle under a group.
func (l *listWindow) fold(gtx C, g *sidebar.Group) D {
	btn := l.folds[g.Key]
	if btn == nil {
		btn = new(widget.Clickable)
		l.folds[g.Key] = btn
	}
	label := "+ " + strconv.Itoa(len(g.Quiet)) + " quiet"
	if l.expanded[g.Key] {
		label = "− hide " + strconv.Itoa(len(g.Quiet)) + " quiet"
	}
	return btn.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		return hoverFill(gtx, btn.Hovered(), func(gtx C) D {
			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(40), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				lbl := captionLabel(l.th, label)
				lbl.Color = colMuted
				return lbl.Layout(gtx)
			})
		})
	})
}

// row is one conversation: its glyph or avatar, its name (bold when it
// needs reading, dim when muted or quiet), and on the right its unread
// mentions or when it last had a message.
func (l *listWindow) row(gtx C, c backend.Conversation, quiet bool) D {
	btn := l.rows[c.ID]
	if btn == nil {
		btn = new(widget.Clickable)
		l.rows[c.ID] = btn
	}
	nameCol := convNameColor(c)
	if quiet && !c.Muted {
		nameCol = colMuted
	}
	return btn.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		return hoverFill(gtx, btn.Hovered(), func(gtx C) D {
			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(28))
						if c.Kind == backend.KindDM {
							return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
								return withPresence(gtx, func(gtx C) D {
									return l.view.avatar(gtx, c.ID, c.AvatarURL, c.Name, unit.Dp(20))
								}, l.presence[c.PeerID], colBg)
							})
						}
						g := bodyLabel(l.th, messages.ChannelGlyph(string(c.Kind)))
						g.Color = colMuted
						return g.Layout(gtx)
					}),
					layout.Flexed(1, func(gtx C) D {
						name := bodyLabel(l.th, c.Name)
						name.MaxLines = 1
						name.Color = nameCol
						return name.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D { return l.rowEnd(gtx, c) }),
				)
			})
		})
	})
}

// rowEnd is a mention badge, or the time of the newest message.
func (l *listWindow) rowEnd(gtx C, c backend.Conversation) D {
	if c.Mentions > 0 {
		return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
			return rounded(gtx, colBadge, unit.Dp(8), func(gtx C) D {
				return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
					lbl := captionLabel(l.th, strconv.Itoa(c.Mentions))
					lbl.Color = textOn(colBadge)
					lbl.Font.Weight = font.SemiBold
					return lbl.Layout(gtx)
				})
			})
		})
	}
	if c.Latest.IsZero() {
		return D{}
	}
	return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
		lbl := captionLabel(l.th, relTime(c.Latest, gtx.Now))
		lbl.Color = colMuted
		return lbl.Layout(gtx)
	})
}

// relTime is a short "when": the time today, the weekday this week, the
// date this year, else month and year.
func relTime(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	switch {
	case chat.SameDay(t, now):
		return t.Format("15:04")
	case now.Sub(t) < 6*24*time.Hour:
		return t.Format("Mon")
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2006")
	}
}

// hoverFill draws w with a hover background behind it when hovered.
func hoverFill(gtx C, hovered bool, w layout.Widget) D {
	m := op.Record(gtx.Ops)
	dims := w(gtx)
	call := m.Stop()
	if hovered {
		paint.FillShape(gtx.Ops, colHover, clip.Rect{Max: dims.Size}.Op())
	}
	call.Add(gtx.Ops)
	return dims
}

// notice is a centered line of text standing in for a pane's content.
func notice(gtx C, th *material.Theme, s string, col color.NRGBA) D {
	return layout.Center.Layout(gtx, func(gtx C) D {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx C) D {
			lbl := bodyLabel(th, s)
			lbl.Color = col
			return lbl.Layout(gtx)
		})
	})
}
