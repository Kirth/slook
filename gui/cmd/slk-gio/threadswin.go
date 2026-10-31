package main

import (
	"image"
	"strconv"

	"gioui.org/app"
	"gioui.org/font"
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
	"github.com/gammons/slk/internal/slackfmt"
)

// threadsWindow lists the threads the user follows, newest activity
// first, the ones with unread replies bold. Choosing one opens it in a
// thread window (which marks it read). There is one; it reads the
// cache, and asks Slack for the list at most every half hour.
type threadsWindow struct {
	h   *hub
	w   *app.Window
	box inbox
	th  *material.Theme

	threads []backend.ThreadSummary
	loaded  bool
	loadErr string
	rows    map[string]*widget.Clickable // by channel/thread

	list   widget.List
	cfg    app.Config
	hdr    headerBar
	sizeDp image.Point
	moved  bool
}

// threadsLoaded carries the followed threads, read from the cache.
type threadsLoaded struct {
	threads []backend.ThreadSummary
	err     error
}

func newThreadsWindow(h *hub) *threadsWindow {
	t := &threadsWindow{h: h, w: new(app.Window), th: newTheme(), rows: map[string]*widget.Clickable{}}
	t.box.w = t.w
	t.list.Axis = layout.Vertical
	t.w.Option(app.Title("Threads — "+h.be.Workspace()), app.Size(unit.Dp(480), unit.Dp(640)),
		app.Decorated(false)) // the header bar is ours
	return t
}

// load reads the followed threads from the cache.
func (t *threadsWindow) load() {
	ts, err := t.h.be.Threads(t.h.ctx)
	t.box.post(threadsLoaded{ts, err})
}

func (t *threadsWindow) run() {
	var ops op.Ops
	for {
		switch e := t.w.Event().(type) {
		case app.DestroyEvent:
			t.h.closed(t, e.Err)
			return
		case app.ConfigEvent:
			t.cfg = e.Config
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			t.apply()
			t.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (t *threadsWindow) apply() {
	for _, item := range t.box.drain() {
		switch v := item.(type) {
		case threadsLoaded:
			t.loaded = true
			if v.err != nil {
				t.loadErr = v.err.Error()
				break
			}
			t.loadErr, t.threads = "", v.threads
		case backend.ThreadsChanged, backend.ThreadState:
			go t.load()
		case raiseWindow:
			t.w.Perform(system.ActionRaise)
		case moveHere:
			if !t.moved {
				// Wayland lets no app raise its own window: a new one
				// opens where the user is, and this one closes.
				t.moved = true
				t.h.adoptThreads(t.sizeDp)
				t.w.Perform(system.ActionClose)
			}
		}
	}
}

// open opens a followed thread in its window.
func (t *threadsWindow) open(s backend.ThreadSummary) {
	t.h.mu.Lock()
	conv := t.h.conversationLocked(s.ChannelID)
	t.h.mu.Unlock()
	t.h.openThread(conv, s.ThreadTS, "")
}

func (t *threadsWindow) layout(gtx C) {
	fillBackground(gtx)
	t.sizeDp = image.Pt(int(float32(gtx.Constraints.Max.X)/gtx.Metric.PxPerDp), int(float32(gtx.Constraints.Max.Y)/gtx.Metric.PxPerDp))
	for _, s := range t.threads {
		if cl := t.rows[s.ChannelID+"/"+s.ThreadTS]; cl != nil && cl.Clicked(gtx) {
			t.open(s)
		}
	}
	spec := headerSpec{Title: "Threads", Leading: func(gtx C) D {
		return tile(gtx, t.th, leadingSize, tint(colAccent), iconReplies, "")
	}}
	switch n := chat.UnreadThreads(t.threads); {
	case !t.loaded:
		spec.Subtitle = "Loading…"
	case n == 0:
		spec.Subtitle = "All caught up"
	case n == 1:
		spec.Subtitle = "1 thread with new replies"
	default:
		spec.Subtitle = strconv.Itoa(n) + " threads with new replies"
	}
	t.hdr.update(gtx, t.w, t.cfg, spec)
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return t.hdr.layout(gtx, t.th, spec, t.cfg) }),
		layout.Flexed(1, func(gtx C) D {
			switch {
			case t.loadErr != "":
				return notice(gtx, t.th, t.loadErr, colError)
			case t.loaded && len(t.threads) == 0:
				return notice(gtx, t.th, "No followed threads yet. Threads you reply in or are mentioned in show here.", colMuted)
			}
			return material.List(t.th, &t.list).Layout(gtx, len(t.threads), func(gtx C, i int) D {
				return t.row(gtx, t.threads[i])
			})
		}),
	)
}

// row is one followed thread: where it is, its parent's author and
// first line, and its replies; unread ones bold behind an accent dot.
func (t *threadsWindow) row(gtx C, s backend.ThreadSummary) D {
	key := s.ChannelID + "/" + s.ThreadTS
	cl := t.rows[key]
	if cl == nil {
		cl = new(widget.Clickable)
		t.rows[key] = cl
	}
	return cl.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		return hoverFill(gtx, cl.Hovered(), func(gtx C) D {
			return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						d := gtx.Dp(8)
						size := image.Pt(gtx.Dp(16), d)
						if s.Unread {
							at := image.Pt(0, gtx.Dp(6))
							paint.FillShape(gtx.Ops, colAccent, clip.Ellipse{Min: at, Max: at.Add(image.Pt(d, d))}.Op(gtx.Ops))
						}
						return D{Size: size}
					}),
					layout.Flexed(1, func(gtx C) D {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								lbl := captionLabel(t.th, slackfmt.ChannelGlyph(string(s.Kind))+" "+s.Channel)
								lbl.Color = colMuted
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx C) D {
								text := s.Preview
								if s.Author != "" {
									text = s.Author + ": " + text
								}
								lbl := bodyLabel(t.th, text)
								lbl.MaxLines = 2
								if s.Unread {
									lbl.Font.Weight = font.SemiBold
								} else {
									lbl.Color = colMuted
								}
								return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, lbl.Layout)
							}),
							layout.Rigid(func(gtx C) D {
								lbl := captionLabel(t.th, chat.ThreadMeta(s, gtx.Now))
								lbl.Color = colMuted
								if s.Unread {
									lbl.Color = colAccent
								}
								return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, lbl.Layout)
							}),
						)
					}),
				)
			})
		})
	})
}

// showThreads opens the Threads window, or brings it here; opening it
// asks Slack for the followed threads (at most every half hour).
func (h *hub) showThreads() {
	h.mu.Lock()
	if h.threadsWin != nil {
		h.threadsWin.box.post(moveHere{})
		h.mu.Unlock()
		return
	}
	t := newThreadsWindow(h)
	h.threadsWin = t
	h.windows++
	h.mu.Unlock()
	go t.run()
	go t.load()
	h.be.RefreshThreads()
}

// adoptThreads is adoptChat for the Threads window.
func (h *hub) adoptThreads(sizeDp image.Point) {
	t := newThreadsWindow(h)
	if sizeDp.X > 0 && sizeDp.Y > 0 {
		t.w.Option(app.Size(unit.Dp(sizeDp.X), unit.Dp(sizeDp.Y)))
	}
	h.mu.Lock()
	h.threadsWin = t
	h.windows++
	h.mu.Unlock()
	go t.run()
	go t.load()
}
