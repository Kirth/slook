package main

import (
	"image"
	"math"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/internal/typing"
)

// typingStripDp is the height kept under a composer for the typing
// line, whether or not anyone is typing, so nothing moves when it
// shows.
const typingStripDp = unit.Dp(18)

// typingDotsCycle is how long the three dots take to pulse in turn.
const typingDotsCycle = 1200 * time.Millisecond

// typingState is a composer's view of who is typing in its
// conversation: people (a chat window only; Slack's user_typing names
// no thread) and apps working (in a thread window, that thread's).
// It lives on the window's goroutine.
type typingState struct {
	tr    *typing.Tracker
	names map[string]string // by user ID, from the events
	sent  time.Time         // the user's own last typing ping
}

// typingApply takes a window's typing events. It reports whether item
// was one only for Typing and AppStatus: a MessagePosted ends its
// author's typing here but is the window's to show too.
func (b *composeBox) typingApply(item any) bool {
	if b.h == nil || !b.h.typingOn {
		switch item.(type) {
		case backend.Typing, backend.AppStatus:
			return true
		}
		return false
	}
	t := &b.typing
	if t.tr == nil {
		t.tr, t.names = typing.New(), map[string]string{}
	}
	switch v := item.(type) {
	case backend.Typing:
		if b.threadTS == "" && v.ChannelID == b.channelID {
			t.names[v.UserID] = v.Name
			t.tr.Add(v.ChannelID, v.UserID)
		}
		return true
	case backend.AppStatus:
		if v.ChannelID == b.channelID && (b.threadTS == "" || v.ThreadTS == b.threadTS) {
			t.names[v.BotUserID] = v.Name
			t.tr.SetBotStatus(v.ChannelID, v.ThreadTS, v.BotUserID, v.Status)
		}
		return true
	case backend.MessagePosted:
		if id := v.Message.AuthorID; id != "" {
			t.tr.Done(v.Message.ChannelID, id)
		}
	}
	return false
}

// typingShown reports whether the composer keeps a typing strip.
func (b *composeBox) typingShown() bool { return b.h != nil && b.h.typingOn }

// typed sends the user's own typing, at most every typing.Throttle,
// while they write a message (not a command, not an edit).
func (b *composeBox) typed() {
	if b.h == nil || !b.h.typingOn || b.editing != nil {
		return
	}
	text := b.composer.Text()
	if strings.TrimSpace(text) == "" || strings.HasPrefix(text, "/") {
		return
	}
	if time.Since(b.typing.sent) < typing.Throttle {
		return
	}
	b.typing.sent = time.Now()
	b.h.be.Typing(b.channelID)
}

// typingText is what the line says: in a chat window who is typing and
// which apps are working, in a thread window its apps' own statuses
// ("Claude is thinking"). "" when there is nothing to say.
func (b *composeBox) typingText() string {
	t := &b.typing
	if t.tr == nil {
		return ""
	}
	t.tr.Expire()
	name := func(id string) string {
		if n := t.names[id]; n != "" {
			return n
		}
		return id
	}
	if b.threadTS != "" {
		var parts []string
		for _, s := range t.tr.ThreadBotStatuses(b.channelID, b.threadTS) {
			parts = append(parts, statusPhrase(name(s.BotUserID), s.Text))
		}
		return strings.Join(parts, " · ")
	}
	var people, apps []string
	for _, id := range t.tr.Users(b.channelID) {
		people = append(people, name(id))
	}
	for _, id := range t.tr.WorkingBots(b.channelID) {
		apps = append(apps, name(id))
	}
	return typing.Row(people, apps, "")
}

// statusPhrase is an app's status after its name: Slack's status text
// reads on from it ("is thinking..."); the dots are drawn, not written.
func statusPhrase(name, status string) string {
	status = strings.TrimRight(strings.Join(strings.Fields(status), " "), ".… ")
	return strings.TrimSpace(name + " " + status)
}

// typingStrip is the line under the composer: what typingText says,
// then three dots pulsing in turn. It keeps its height when empty.
func (b *composeBox) typingStrip(gtx C) D {
	height := gtx.Dp(typingStripDp)
	size := image.Pt(gtx.Constraints.Max.X, height)
	text := b.typingText()
	if text == "" {
		return D{Size: size}
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(50 * time.Millisecond)})
	gtx.Constraints = layout.Exact(size)
	layout.Inset{Left: unit.Dp(20), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						lbl := captionLabel(b.th, text)
						lbl.Color = colMuted
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D { return typingDots(gtx) }),
				)
			}),
		)
	})
	return D{Size: size}
}

// typingDots draws three small dots, each brightening in turn.
func typingDots(gtx C) D {
	d, gap := gtx.Dp(3), gtx.Dp(3)
	phase := float64(gtx.Now.UnixMilli()%typingDotsCycle.Milliseconds()) / float64(typingDotsCycle.Milliseconds())
	x := gtx.Dp(4)
	for i := range 3 {
		// A raised cosine per dot, a little after the one before.
		p := phase - float64(i)*0.18
		strength := 0.5 + 0.5*math.Cos(2*math.Pi*p)
		col := colMuted
		col.A = uint8(float64(col.A) * (0.25 + 0.75*strength))
		y := gtx.Dp(2) - int(strength*float64(gtx.Dp(2)))
		r := image.Rect(x, y, x+d, y+d)
		paint.FillShape(gtx.Ops, col, clip.Ellipse(r).Op(gtx.Ops))
		x += d + gap
	}
	return D{Size: image.Pt(x, gtx.Dp(4)+d)}
}
