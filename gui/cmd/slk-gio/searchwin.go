package main

import (
	"context"
	"errors"
	"image"
	"image/color"
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
	"github.com/gammons/slk/gui/internal/styledtext"
	"github.com/gammons/slk/gui/motion"
	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/gui/search"
	"github.com/gammons/slk/internal/ui/messages"
)

// searchDebounce is how long typing pauses before a search runs.
const searchDebounce = 150 * time.Millisecond

// searchLimit is how many results a search returns.
const searchLimit = 50

// statusRefresh bounds how often the status is re-read while the
// backfill is reporting.
const statusRefresh = 400 * time.Millisecond

// searchWindow searches the cached messages and shows what the cache
// holds: a search box over either the results or, while the box is
// empty, each conversation's coverage, and a panel with the backfill's
// progress and controls and the fetches in flight.
type searchWindow struct {
	h    *hub
	w    *app.Window
	box  inbox
	th   *material.Theme
	view richView

	ed  widget.Editor
	rl  readline
	due time.Time // when the typed text is searched; zero when nothing is pending
	gen int       // the current search; results of older ones are dropped
	// cancel ends the search in flight.
	cancel    context.CancelFunc
	query     string // the query the results answer
	hits      []backend.SearchHit
	searching bool
	searchErr string
	rows      map[string]*widget.Clickable // by channel+ts
	// bars glide the coverage bars to new values, by conversation ID
	// (and overallBar).
	bars map[string]*motion.Tween
	// order is how results are sorted; sortBest and sortNewest switch it.
	order                backend.SearchOrder
	sortBest, sortNewest widget.Clickable

	status      backend.SyncStatus
	statusErr   string
	loaded      bool
	refreshing  bool      // a SyncStatus call is in flight
	dirty       bool      // a change arrived while refreshing
	nextRefresh time.Time // not before this
	horizon     widget.Enum
	startStop   widget.Clickable
	ctlErr      string
	// eta guesses how long a running backfill has left from how the
	// overall progress moves between status reads.
	eta     search.Estimator
	etaRun  time.Time // StartedAt of the run the samples belong to
	running bool      // the backfill was running at the last status read

	list      widget.List
	connected bool
	focused   bool
	cfg       app.Config
	hdr       headerBar
	sizeDp    image.Point
	moved     bool
	shown     bool
}

// searchState is what the window hands to its replacement when it
// moves to the user's workspace.
type searchState struct {
	query     string
	hits      []backend.SearchHit
	horizon   string
	status    backend.SyncStatus
	loaded    bool
	connected bool
	position  layout.Position
	sizeDp    image.Point
	order     backend.SearchOrder
}

type (
	// statusLoaded is a SyncStatus answer.
	statusLoaded struct {
		st  backend.SyncStatus
		err error
	}
	// searchResults is a search's answer; gen says which.
	searchResults struct {
		gen   int
		query string
		hits  []backend.SearchHit
		err   error
	}
)

func newSearchWindow(h *hub, connected bool) *searchWindow {
	s := &searchWindow{
		h:         h,
		w:         new(app.Window),
		th:        newTheme(),
		connected: connected,
		rows:      map[string]*widget.Clickable{},
		bars:      map[string]*motion.Tween{},
	}
	s.box.w = s.w
	s.view = richView{th: s.th, images: h.images, box: &s.box}
	s.ed.SingleLine = true
	s.ed.Submit = true
	s.horizon.Value = "90d"
	s.list.Axis = layout.Vertical
	s.w.Option(app.Title("Search — "+h.be.Workspace()), app.Size(unit.Dp(520), unit.Dp(720)),
		app.Decorated(false)) // the header bar is ours
	return s
}

// newSearchWindowFrom opens a window that carries on where another
// left off.
func newSearchWindowFrom(h *hub, st searchState) *searchWindow {
	s := newSearchWindow(h, st.connected)
	s.ed.SetText(st.query)
	s.query, s.hits = st.query, st.hits
	if st.horizon != "" {
		s.horizon.Value = st.horizon
	}
	s.status, s.loaded = st.status, st.loaded
	s.order = st.order
	s.list.Position = st.position
	if st.sizeDp.X > 0 && st.sizeDp.Y > 0 {
		s.w.Option(app.Size(unit.Dp(st.sizeDp.X), unit.Dp(st.sizeDp.Y)))
	}
	return s
}

// moveHere hands the window's state to a new window on the user's
// current workspace and closes this one.
func (s *searchWindow) moveHere() {
	if s.moved {
		return
	}
	s.moved = true
	if s.cancel != nil {
		s.cancel()
	}
	s.h.adoptSearch(searchState{
		query: s.ed.Text(), hits: s.hits, horizon: s.horizon.Value, status: s.status, loaded: s.loaded,
		connected: s.connected, position: s.list.Position, sizeDp: s.sizeDp, order: s.order,
	})
	s.w.Perform(system.ActionClose)
}

// load reads the status once, on open.
func (s *searchWindow) load() {
	st, err := s.h.be.SyncStatus(s.h.ctx)
	s.box.post(statusLoaded{st, err})
}

func (s *searchWindow) run() {
	var ops op.Ops
	for {
		switch e := s.w.Event().(type) {
		case app.DestroyEvent:
			if s.cancel != nil {
				s.cancel()
			}
			s.h.closed(s, e.Err)
			return
		case app.ConfigEvent:
			s.cfg = e.Config
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			s.apply(gtx.Now)
			s.layout(gtx)
			e.Frame(gtx.Ops)
			if !s.shown {
				s.shown = true
				refillThemes()
			}
		}
	}
}

func (s *searchWindow) apply(now time.Time) {
	for _, item := range s.box.drain() {
		switch v := item.(type) {
		case statusLoaded:
			s.refreshing = false
			s.loaded = true
			if v.err != nil {
				s.statusErr = v.err.Error()
			} else {
				s.statusErr = ""
				s.status = v.st
				s.sample(now)
				s.pruneBars()
			}
			if s.dirty {
				s.dirty = false
				s.refresh(now)
			}
		case searchResults:
			if v.gen != s.gen {
				break // a search typed over
			}
			s.searching = false
			switch {
			case errors.Is(v.err, context.Canceled):
			case v.err != nil:
				s.searchErr = v.err.Error()
			default:
				s.searchErr = ""
				s.query, s.hits = v.query, v.hits
				s.pruneRows()
			}
		case backend.SyncChanged:
			s.refresh(now)
		case backend.ConnectionChanged:
			s.connected = v.Connected
		case raiseWindow:
			s.w.Perform(system.ActionRaise)
		case moveHere:
			s.moveHere()
		case imageReady:
			// The post already woke the window; it redraws.
		}
	}
}

// sample feeds the estimator while a backfill runs, starting over for
// a new run.
func (s *searchWindow) sample(now time.Time) {
	b := s.status.Backfill
	running := b.State == backend.BackfillRunning || b.State == backend.BackfillPaused
	if running && (!s.running || !b.StartedAt.Equal(s.etaRun)) {
		s.eta.Reset()
		s.etaRun = b.StartedAt
	}
	s.running = running
	if running {
		s.eta.Add(now, search.Overall(s.status.Conversations, b.Horizon, now))
	}
}

// shownHorizon is the horizon progress is measured against: the running
// backfill's, else the one chosen for the next.
func (s *searchWindow) shownHorizon() time.Duration {
	if s.backfillRunning() {
		return s.status.Backfill.Horizon
	}
	return search.HorizonFor(s.horizon.Value)
}

// bar draws a progress bar of the given width filled to f, in the
// colour of its state: accent while filling, green when complete, amber
// when there are gaps. label (a percentage), when set, sits inside the
// bar, the bar growing to hold it; one too wide for the bar goes after
// it instead.
//
// The fill glides to f (see barMotion) under key, which names the bar
// across frames; the label shows f itself, not the moving fill.
func (s *searchWindow) bar(gtx C, key string, f float64, width unit.Dp, state search.BarState, label string) D {
	fillCol := colAccent
	switch state {
	case search.Complete:
		fillCol = colPresent
	case search.Gaps:
		fillCol = colOffline
	}
	w := gtx.Dp(width)
	h := gtx.Dp(unit.Dp(6))
	var lw int
	if label != "" {
		lw = measure(gtx, captionLabel(s.th, label))
		if lw+gtx.Dp(unit.Dp(8)) > w {
			// No room inside: the bar, then the label after it.
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return s.bar(gtx, key, f, width, state, "") }),
				layout.Rigid(func(gtx C) D {
					lbl := captionLabel(s.th, label)
					lbl.Color = colMuted
					return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, lbl.Layout)
				}),
			)
		}
		h = gtx.Dp(unit.Dp(16))
	}
	r := h / 2
	paint.FillShape(gtx.Ops, colControl, clip.UniformRRect(image.Rectangle{Max: image.Pt(w, h)}, r).Op(gtx.Ops))
	shown := s.glide(gtx, key, f)
	fill := int(float64(w)*max(0, min(1, shown)) + 0.5)
	if fill > 0 {
		paint.FillShape(gtx.Ops, fillCol, clip.UniformRRect(image.Rectangle{Max: image.Pt(fill, h)}, min(r, fill/2)).Op(gtx.Ops))
	}
	if label != "" {
		// Over the fill the label takes the colour that reads on it;
		// over the track, muted.
		x := (w - lw) / 2
		col := colMuted
		if fill >= x+lw/2 {
			col = textOn(fillCol)
		}
		lbl := captionLabel(s.th, label)
		lbl.Color = col
		lbl.MaxLines = 1
		cgtx := gtx
		cgtx.Constraints = layout.Exact(image.Pt(w, h))
		layout.Center.Layout(cgtx, lbl.Layout)
	}
	return D{Size: image.Pt(w, h)}
}

// barMotion is how long a bar takes to glide to a new value.
const barMotion = 240 * time.Millisecond

// overallBar keys the panel's bar among the conversations' (IDs never
// start with a NUL).
const overallBar = "\x00overall"

// glide is the value a bar draws at: f the first time key is seen, then
// moving toward each new f. It asks for the next frame while moving.
func (s *searchWindow) glide(gtx C, key string, f float64) float64 {
	tw := s.bars[key]
	if tw == nil {
		tw = new(motion.Tween)
		s.bars[key] = tw
	}
	tw.Set(f, gtx.Now, barMotion) // no-op unless f is new; the first takes f at once
	v, moving := tw.Value(gtx.Now)
	if moving {
		gtx.Execute(op.InvalidateCmd{})
	}
	return v
}

// pruneBars drops the motion of bars whose conversation the status no
// longer lists.
func (s *searchWindow) pruneBars() {
	keep := map[string]bool{overallBar: true}
	for _, c := range s.status.Conversations {
		keep[c.ID] = true
	}
	for k := range s.bars {
		if !keep[k] {
			delete(s.bars, k)
		}
	}
}

// measure is the width l would take, drawn nowhere.
func measure(gtx C, l material.LabelStyle) int {
	rec := op.Record(gtx.Ops)
	gtx.Constraints.Min = image.Point{}
	d := l.Layout(gtx)
	rec.Stop()
	return d.Size.X
}

// snippetWidth is how many runes of a result's text show around its
// match: about three lines at the window's default width.
const snippetWidth = 160

// snippet is a result's text cut around its first match, with the
// query's words picked out in the accent colour.
func (s *searchWindow) snippet(gtx C, text string) D {
	if strings.TrimSpace(text) == "" {
		lbl := secondaryLabel(s.th, "(no text)")
		lbl.Color = colMuted
		return lbl.Layout(gtx)
	}
	pieces := search.Highlight(search.Snippet(text, s.query, snippetWidth), s.query)
	spans := make([]styledtext.SpanStyle, 0, len(pieces))
	for _, p := range pieces {
		st := styledtext.SpanStyle{Font: font.Font{Typeface: s.th.Face}, Size: secondarySize, Color: colFg, Content: p.Text}
		if p.Hit {
			st.Font.Weight = font.SemiBold
			st.Color = colAccent
		}
		spans = append(spans, st)
	}
	return styledtext.Text(s.th.Shaper, spans...).Layout(gtx, nil)
}

// refresh re-reads the status, one call at a time and not more often
// than statusRefresh: the backfill reports every step.
func (s *searchWindow) refresh(now time.Time) {
	if s.refreshing {
		s.dirty = true
		return
	}
	if now.Before(s.nextRefresh) {
		s.dirty = true
		s.w.Invalidate() // come back at the next frame to check the time
		return
	}
	s.refreshing = true
	s.nextRefresh = now.Add(statusRefresh)
	go s.load()
}

// pruneRows keeps a clickable per result shown and drops the rest.
func (s *searchWindow) pruneRows() {
	keep := make(map[string]bool, len(s.hits))
	for _, h := range s.hits {
		keep[hitKey(h)] = true
	}
	for k := range s.rows {
		if !keep[k] {
			delete(s.rows, k)
		}
	}
}

func hitKey(h backend.SearchHit) string { return h.Message.ChannelID + "\x00" + h.Message.TS }

// fire starts a search for q, dropping the one in flight.
func (s *searchWindow) fire(q string) {
	if s.cancel != nil {
		s.cancel()
	}
	s.gen++
	ctx, cancel := context.WithCancel(s.h.ctx)
	s.cancel = cancel
	s.searching = true
	go func(gen int, order backend.SearchOrder) {
		hits, err := s.h.be.Search(ctx, q, searchLimit, order)
		s.box.post(searchResults{gen: gen, query: q, hits: hits, err: err})
	}(s.gen, s.order)
}

// clearSearch drops the query, its results and any search in flight.
func (s *searchWindow) clearSearch() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.gen++
	s.due = time.Time{}
	s.query, s.hits, s.searching, s.searchErr = "", nil, false, ""
	s.pruneRows()
}

func (s *searchWindow) layout(gtx C) {
	fillBackground(gtx)
	s.sizeDp = image.Pt(int(float32(gtx.Constraints.Max.X)/gtx.Metric.PxPerDp), int(float32(gtx.Constraints.Max.Y)/gtx.Metric.PxPerDp))
	if s.dirty && !s.refreshing && !gtx.Now.Before(s.nextRefresh) {
		s.dirty = false
		s.refresh(gtx.Now)
	}

	// Escape clears, before the editor sees the key.
	for {
		ev, ok := gtx.Event(key.Filter{Focus: &s.ed, Name: key.NameEscape})
		if !ok {
			break
		}
		if k, ok := ev.(key.Event); ok && k.State == key.Press {
			s.ed.SetText("")
			s.clearSearch()
		}
	}
	s.rl.keys(gtx, &s.ed)
	for {
		ev, ok := s.ed.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			if strings.TrimSpace(s.ed.Text()) == "" {
				s.clearSearch()
			} else {
				s.due = gtx.Now.Add(searchDebounce)
				gtx.Execute(op.InvalidateCmd{At: s.due})
			}
		case widget.SubmitEvent:
			if q := strings.TrimSpace(s.ed.Text()); q != "" {
				s.due = time.Time{}
				s.fire(q)
			}
		}
	}
	if !s.due.IsZero() && !gtx.Now.Before(s.due) {
		s.due = time.Time{}
		if q := strings.TrimSpace(s.ed.Text()); q != "" {
			s.fire(q)
		}
	}
	for _, sw := range []struct {
		btn   *widget.Clickable
		order backend.SearchOrder
	}{{&s.sortBest, backend.Relevance}, {&s.sortNewest, backend.Newest}} {
		if sw.btn.Clicked(gtx) && s.order != sw.order {
			s.order = sw.order
			if q := strings.TrimSpace(s.ed.Text()); q != "" {
				s.due = time.Time{}
				s.fire(q)
			}
		}
	}
	for k, btn := range s.rows {
		if btn.Clicked(gtx) {
			for _, h := range s.hits {
				if hitKey(h) == k {
					s.open(h)
					break
				}
			}
		}
	}
	if s.startStop.Clicked(gtx) {
		s.ctlErr = ""
		if s.backfillRunning() {
			s.h.be.StopBackfill()
		} else if err := s.h.be.StartBackfill(search.HorizonFor(s.horizon.Value)); err != nil {
			s.ctlErr = err.Error()
		}
		s.refresh(gtx.Now)
	}
	if !s.focused {
		gtx.Execute(key.FocusCmd{Tag: &s.ed})
		s.focused = true
	}

	spec := s.headerSpec()
	s.hdr.update(gtx, s.w, s.cfg, spec)

	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return s.hdr.layout(gtx, s.th, spec, s.cfg) }),
		layout.Rigid(func(gtx C) D {
			return layout.UniformInset(unit.Dp(10)).Layout(gtx, framedEditor(s.th, &s.ed, "Search messages"))
		}),
		layout.Rigid(divider),
		layout.Flexed(1, s.main),
		layout.Rigid(divider),
		layout.Rigid(s.panel),
	)
}

// open shows a result: its conversation, scrolled to the message (or
// to the parent of a reply that lives in its thread).
func (s *searchWindow) open(h backend.SearchHit) {
	channelID, ts, threadTS := search.Target(h)
	conv := backend.Conversation{ID: channelID, Name: channelID, Kind: backend.KindChannel}
	for _, c := range s.h.cachedConversations() {
		if c.ID == channelID {
			conv = c
			break
		}
	}
	if conv.Name == channelID {
		// The list does not know it (left, or archived); the cache does.
		for _, c := range s.status.Conversations {
			if c.ID == channelID {
				conv.Name, conv.Kind = strings.TrimPrefix(c.Name, "#"), c.Kind
				break
			}
		}
	}
	s.h.openChatAt(conv, ts)
	if threadTS != "" {
		// The reply itself is in its thread: open that on top.
		s.h.openThread(conv, threadTS, h.Message.TS)
	}
}

func (s *searchWindow) backfillRunning() bool {
	st := s.status.Backfill.State
	return st == backend.BackfillRunning || st == backend.BackfillPaused
}

func (s *searchWindow) headerSpec() headerSpec {
	spec := headerSpec{
		Title:   "Search",
		Leading: func(gtx C) D { return tile(gtx, s.th, leadingSize, tint(colAccent), iconSearch, "") },
	}
	switch {
	case !s.connected:
		spec.Subtitle, spec.SubColor = "Offline · reconnecting…", colOffline
	case !s.loaded:
		spec.Subtitle = "Reading the cache…"
	case s.statusErr != "":
		spec.Subtitle, spec.SubColor = s.statusErr, colError
	default:
		spec.Subtitle = search.Summary(s.status)
	}
	return spec
}

// main is the results, or the coverage while nothing is searched for.
func (s *searchWindow) main(gtx C) D {
	if strings.TrimSpace(s.ed.Text()) == "" {
		return s.coverage(gtx)
	}
	switch {
	case s.searchErr != "":
		return notice(gtx, s.th, s.searchErr, colError)
	case len(s.hits) == 0 && s.searching:
		return notice(gtx, s.th, "Searching…", colMuted)
	case len(s.hits) == 0 && s.query != "":
		return notice(gtx, s.th, "No cached message matches.", colMuted)
	case len(s.hits) == 0:
		return notice(gtx, s.th, "Searching…", colMuted)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(s.sortBar),
		layout.Flexed(1, func(gtx C) D {
			return material.List(s.th, &s.list).Layout(gtx, len(s.hits), func(gtx C, i int) D {
				return s.result(gtx, s.hits[i])
			})
		}),
	)
}

// sortBar is the results' count and how they are sorted: the order in
// use, and the other as a link that switches to it.
func (s *searchWindow) sortBar(gtx C) D {
	count := search.Commas(len(s.hits)) + " results"
	if len(s.hits) == 1 {
		count = "1 result"
	}
	option := func(cl *widget.Clickable, label string, current bool) layout.FlexChild {
		return layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
				if current {
					lbl := captionLabel(s.th, label)
					lbl.Color = colFg
					lbl.Font.Weight = font.SemiBold
					return lbl.Layout(gtx)
				}
				return linkCaption(gtx, s.th, cl, label)
			})
		})
	}
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(2), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				lbl := captionLabel(s.th, count)
				lbl.Color = colMuted
				return lbl.Layout(gtx)
			}),
			option(&s.sortBest, "Best match", s.order == backend.Relevance),
			option(&s.sortNewest, "Newest", s.order == backend.Newest),
		)
	})
}

// coverage lists each conversation with what the cache holds of it.
func (s *searchWindow) coverage(gtx C) D {
	if !s.loaded {
		return notice(gtx, s.th, "Reading the cache…", colMuted)
	}
	rows := search.OrderCoverage(s.status.Conversations)
	if len(rows) == 0 {
		return notice(gtx, s.th, "Nothing cached yet. Open a conversation, or start the backfill below.", colMuted)
	}
	return material.List(s.th, &s.list).Layout(gtx, len(rows), func(gtx C, i int) D {
		return s.coverageRow(gtx, rows[i])
	})
}

// The coverage rows' fixed columns: the message count and the status
// ("complete", "since Sep 3 · 1 gap"), so the bars line up.
const (
	coverageCountDp  = unit.Dp(76)
	coverageStatusDp = unit.Dp(140)
)

func (s *searchWindow) coverageRow(gtx C, c backend.Coverage) D {
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				col := colBorder
				if c.Live {
					col = colPresent
				}
				d := gtx.Dp(unit.Dp(8))
				paint.FillShape(gtx.Ops, col, clip.Ellipse{Max: image.Pt(d, d)}.Op(gtx.Ops))
				return D{Size: image.Pt(d+gtx.Dp(unit.Dp(8)), d)}
			}),
			layout.Flexed(1, func(gtx C) D {
				name := bodyLabel(s.th, strings.TrimPrefix(c.Name, "#"))
				if c.Kind == backend.KindChannel || c.Kind == backend.KindPrivate {
					name.Text = messages.ChannelGlyph(string(c.Kind)) + " " + name.Text
				}
				name.MaxLines = 1
				if c.Messages == 0 {
					name.Color = colMuted
				}
				return name.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				if c.Ignored {
					lbl := captionLabel(s.th, "skipped (config)")
					lbl.Color = colMuted
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, lbl.Layout)
				}
				horizon := s.shownHorizon()
				if horizon <= 0 && !c.Complete {
					return D{} // nothing to measure "everything" against
				}
				f := search.Fraction(c, horizon, gtx.Now)
				return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
					return s.bar(gtx, c.ID, f, unit.Dp(64), search.StateOf(c, f), search.Percent(f))
				})
			}),
			layout.Rigid(func(gtx C) D {
				// Fixed widths for the count and the status keep the
				// bars in one column.
				lbl := captionLabel(s.th, search.Commas(c.Messages)+" msgs")
				lbl.Color = colMuted
				lbl.MaxLines = 1
				gtx.Constraints.Min.X = gtx.Dp(coverageCountDp)
				return layout.E.Layout(gtx, lbl.Layout)
			}),
			layout.Rigid(func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Dp(coverageStatusDp)
				lbl := captionLabel(s.th, search.CoverageLine(c))
				lbl.MaxLines = 1
				// The bar's state, in the bar's colours.
				switch search.StateOf(c, 0) {
				case search.Gaps:
					lbl.Color = colOffline
				case search.Complete:
					lbl.Color = colPresent
				default:
					lbl.Color = colMuted
				}
				return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, lbl.Layout)
			}),
		)
	})
}

// result is one hit: where and who, when, and the text.
func (s *searchWindow) result(gtx C, h backend.SearchHit) D {
	m := h.Message
	key := hitKey(h)
	btn := s.rows[key]
	if btn == nil {
		btn = new(widget.Clickable)
		s.rows[key] = btn
	}
	where := m.ChannelID
	for _, c := range s.status.Conversations {
		if c.ID == m.ChannelID {
			where = c.Name
			break
		}
	}
	return btn.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		return hoverFill(gtx, btn.Hovered(), func(gtx C) D {
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Flex{Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Right: unit.Dp(10), Top: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
							return s.view.avatar(gtx, m.AuthorID, m.AvatarURL, m.Author, unit.Dp(28))
						})
					}),
					layout.Flexed(1, func(gtx C) D {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
									layout.Rigid(func(gtx C) D {
										lbl := secondaryLabel(s.th, m.Author)
										lbl.Font.Weight = font.SemiBold
										lbl.Color = authorColor(m.Author)
										lbl.MaxLines = 1
										return lbl.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Flexed(1, func(gtx C) D {
										lbl := captionLabel(s.th, where)
										lbl.Color = colMuted
										lbl.MaxLines = 1
										return lbl.Layout(gtx)
									}),
									layout.Rigid(func(gtx C) D {
										when := relTime(m.Time, gtx.Now)
										if m.InThreadOnly {
											when = "in thread · " + when
										}
										lbl := captionLabel(s.th, when)
										lbl.Color = colMuted
										return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, lbl.Layout)
									}),
								)
							}),
							layout.Rigid(func(gtx C) D { return s.snippet(gtx, rich.PlainBlocks(m.Body)) }),
						)
					}),
				)
			})
		})
	})
}

// panelSplit is the window width from which the panel puts the fetches
// in flight beside the backfill instead of under it.
const panelSplit = unit.Dp(720)

// panel is the cache panel: the backfill's state and controls, and the
// fetches in flight, side by side when the window is wide.
func (s *searchWindow) panel(gtx C) D {
	return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		var rows []layout.FlexChild
		caption := func(text string, col color.NRGBA) layout.FlexChild {
			return layout.Rigid(func(gtx C) D {
				lbl := captionLabel(s.th, text)
				lbl.Color = col
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			})
		}
		b := s.status.Backfill
		line := "Backfill: " + search.BackfillLine(b, gtx.Now)
		col := colMuted
		switch b.State {
		case backend.BackfillPaused:
			col = colOffline
		case backend.BackfillFailed:
			col = colError
		case backend.BackfillRunning:
			col = colFg
		}
		rows = append(rows, caption(line, col))
		if horizon, counted := s.shownHorizon(), search.Counted(s.status.Conversations); horizon > 0 && len(counted) > 0 {
			f := search.Overall(counted, horizon, gtx.Now)
			progress := search.Percent(f) + " of " + search.Horizon(horizon) + " across " + search.Commas(len(counted)) + " conversations"
			if skipped := len(s.status.Conversations) - len(counted); skipped > 0 {
				progress += " (" + search.Commas(skipped) + " skipped)"
			}
			if s.backfillRunning() {
				if rate := search.Rate(b.Fetched, gtx.Now.Sub(b.StartedAt)); rate != "" {
					progress += " · " + rate
				}
				if left, ok := s.eta.Remaining(); ok {
					progress += " · " + search.Left(left)
				}
			}
			rows = append(rows, layout.Rigid(func(gtx C) D {
				return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
					return s.bar(gtx, overallBar, f, unit.Dp(float32(gtx.Constraints.Max.X)/gtx.Metric.PxPerDp), search.OverallState(counted, f), search.Percent(f))
				})
			}))
			rows = append(rows, caption(progress, colMuted))
		}
		if b.LastError != "" && b.State != backend.BackfillFailed {
			rows = append(rows, caption("Last error: "+b.LastError, colError))
		}
		if s.ctlErr != "" {
			rows = append(rows, caption(s.ctlErr, colError))
		}
		rows = append(rows, layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
				running := s.backfillRunning()
				radio := func(key, label string) layout.FlexChild {
					return layout.Rigid(func(gtx C) D {
						if running {
							gtx = gtx.Disabled()
						}
						r := material.RadioButton(s.th, &s.horizon, key, label)
						r.Size = unit.Dp(18)
						r.TextSize = secondarySize
						return r.Layout(gtx)
					})
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					radio("90d", "90 days"),
					radio("1y", "1 year"),
					radio("all", "everything"),
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(gtx C) D {
						label := "Start"
						if running {
							label = "Stop"
						}
						btn := material.Button(s.th, &s.startStop, label)
						btn.TextSize = secondarySize
						btn.Inset = layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(14), Right: unit.Dp(14)}
						return btn.Layout(gtx)
					}),
				)
			})
		}))
		var running []layout.FlexChild
		active := s.status.Active
		switch {
		case len(active) == 0:
			running = append(running, caption("Running now: idle", colMuted))
		default:
			running = append(running, caption("Running now:", colMuted))
			for i, a := range active {
				if i == 3 {
					running = append(running, caption("  +"+search.Commas(len(active)-3)+" more", colMuted))
					break
				}
				running = append(running, caption("  "+a, colFg))
			}
		}
		if gtx.Constraints.Max.X < gtx.Dp(panelSplit) {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, append(rows, running...)...)
		}
		column := func(children []layout.FlexChild) layout.Widget {
			return func(gtx C) D { return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...) }
		}
		return layout.Flex{}.Layout(gtx,
			layout.Flexed(1, column(rows)),
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			layout.Flexed(1, column(running)),
		)
	})
}
