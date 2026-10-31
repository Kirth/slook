package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"strconv"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/gesture"
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
	"github.com/gammons/slk/gui/internal/richtext"
	"github.com/gammons/slk/gui/internal/styledtext"
	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/attach"
	"github.com/gammons/slk/internal/avatar"
	"github.com/gammons/slk/internal/emoji"
	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/launch"
)

// imageMaxDp bounds an inline picture; thumbnails and context icons
// are smaller.
const (
	imageMaxDp = unit.Dp(360)
	thumbDp    = unit.Dp(80)
	iconDp     = unit.Dp(18)
)

// msgWidgets is the interaction state one message's blocks need across
// frames: text spans that can be clicked, pictures and files that can be
// clicked, code that can be selected. Each is keyed by the order it is
// laid out in, which is stable while the message does not change.
type msgWidgets struct {
	texts               map[int]*richtext.InteractiveText
	clicks              map[int]*openClick
	sels                map[int]*widget.Selectable
	nText, nClick, nSel int

	// gallery is the message's images and files in order, for the
	// previewer; nAtt counts them as they are drawn this frame.
	gallery []backend.Attachment
	nAtt    int
	// msg is the message drawn, whose Actions its buttons press.
	msg backend.Message

	// avatarHover and nameHover notice the pointer over the author.
	avatarHover, nameHover widget.Clickable

	// rowHover notices the pointer over the whole message, to offer
	// its actions: react (open the picker) and reply (open the thread);
	// replies is the reply count (or a broadcast's caption), which opens
	// the thread too.
	rowHover gesture.Hover
	hovered  bool             // the pointer is over the row (or its actions), as of this frame
	more     widget.Clickable // the actions menu
	react    widget.Clickable
	reply    widget.Clickable
	replies  widget.Clickable

	// pills are the reaction pills, by position; addPill is the "+"
	// after them. Pointers, so growing the slice keeps their state.
	pills   []*widget.Clickable
	addPill widget.Clickable

	// tsel is the message's text selection (textselect.go).
	tsel textSel
}

// openClick is a clickable that launches a URL, previews one of the
// message's attachments, or presses one of its actions (action, an
// index into the message's Actions; -1 for none).
type openClick struct {
	btn     widget.Clickable
	open    string
	preview *previewReq
	action  int
}

func newMsgWidgets() *msgWidgets {
	return &msgWidgets{texts: map[int]*richtext.InteractiveText{}, clicks: map[int]*openClick{}, sels: map[int]*widget.Selectable{}}
}

// begin resets the per-frame counters.
func (w *msgWidgets) begin() { w.nText, w.nClick, w.nSel, w.nAtt = 0, 0, 0, 0 }

// attachment is a clickable for the next of the message's attachments
// drawn: clicking previews it, with the others a step away.
func (w *msgWidgets) attachment() (*openClick, backend.Attachment) {
	c := w.click("")
	i := w.nAtt
	w.nAtt++
	if i >= len(w.gallery) {
		return c, backend.Attachment{}
	}
	c.preview = &previewReq{items: w.gallery, index: i}
	return c, w.gallery[i]
}

func (w *msgWidgets) text() *richtext.InteractiveText {
	st := w.texts[w.nText]
	if st == nil {
		st = new(richtext.InteractiveText)
		w.texts[w.nText] = st
	}
	w.nText++
	return st
}

func (w *msgWidgets) click(open string) *openClick {
	c := w.clicks[w.nClick]
	if c == nil {
		c = new(openClick)
		w.clicks[w.nClick] = c
	}
	c.open, c.preview, c.action = open, nil, -1
	w.nClick++
	return c
}

func (w *msgWidgets) sel() *widget.Selectable {
	s := w.sels[w.nSel]
	if s == nil {
		s = new(widget.Selectable)
		w.sels[w.nSel] = s
	}
	w.nSel++
	return s
}

// handle acts on events since the last frame: links open through open
// on a click (openURL, the browser, when nil), attachments in the
// previewer, buttons are pressed through press; hover reports the
// pointer entering or leaving a mention, with the user's ID.
func (w *msgWidgets) handle(gtx C, hover func(userID string, on bool), preview func(previewReq), press func(backend.Message, backend.Action), open func(url string)) {
	if open == nil {
		open = openURL
	}
	for _, st := range w.texts {
		for {
			span, ev, ok := st.Update(gtx)
			if !ok {
				break
			}
			switch ev.Type {
			case richtext.Click:
				if url, _ := span.Get("url").(string); url != "" {
					open(url)
				}
			case richtext.Hover, richtext.Unhover:
				if id, _ := span.Get("user").(string); id != "" && hover != nil {
					hover(id, ev.Type == richtext.Hover)
				}
			}
		}
	}
	for _, c := range w.clicks {
		if !c.btn.Clicked(gtx) {
			continue
		}
		switch {
		case c.action >= 0 && c.action < len(w.msg.Actions):
			if press != nil {
				press(w.msg, w.msg.Actions[c.action])
			}
		case c.preview != nil && preview != nil:
			preview(*c.preview)
		case c.open != "":
			open(c.open)
		}
	}
}

func openURL(url string) {
	if err := launch.Open(url); err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: open", url+":", err)
	}
}

// richView draws rich blocks for one window.
type richView struct {
	th     *material.Theme
	images *imageStore
	box    *inbox
	// busy reports an attachment being downloaded for the previewer.
	busy func(backend.Attachment) bool
	// customs is the workspace's custom emoji (name to picture URL or
	// "alias:<name>"); nil until loaded, and custom emoji show as
	// ":name:" until then.
	customs map[string]string
	// myGroup reports whether a usergroup is one of the user's; a
	// mention of it is drawn as a mention of them. Nil means none.
	myGroup func(id string) bool
	// open opens a clicked link: hub.openLink, which shows a link to a
	// message of this workspace in its window. Nil opens the browser.
	open func(url string)
}

// busyNote is "Downloading…" under an attachment being fetched.
func (v *richView) busyNote(gtx C, a backend.Attachment) D {
	if v.busy == nil || !v.busy(a) {
		return D{}
	}
	return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
		lbl := captionLabel(v.th, "Downloading…")
		lbl.Color = colMuted
		return lbl.Layout(gtx)
	})
}

// spanStyles turns spans into richtext styles at a given size and color.
func (v *richView) spanStyles(spans []rich.Span, size unit.Sp, col color.NRGBA) []richtext.SpanStyle {
	out := make([]richtext.SpanStyle, 0, len(spans))
	for _, s := range spans {
		f := font.Font{Typeface: v.th.Face}
		if s.Bold {
			f.Weight = font.SemiBold
		}
		if s.Italic {
			f.Style = font.Italic
		}
		st := richtext.SpanStyle{Font: f, Size: size, Color: col, Content: s.Text}
		switch {
		case s.Kind == rich.SpanEmoji:
			img, ok := v.customEmoji(s.Text)
			if !ok {
				st.Color = colMuted
				break
			}
			// A placeholder about an emoji wide, drawn over with the
			// picture.
			st.Content, st.Color = "\u2003", color.NRGBA{}
			st.Inline = func(gtx C, _ D) { emojiPicture(gtx, img) }
		case s.Code:
			st.Font.Typeface = fontMono.Typeface
			st.Font.Weight = font.Normal // code is never bold
			st.Color = colCodeFg
		case s.URL != "":
			st.Color = colAccent
			st.Interactive = true
			st.Set("url", s.URL)
		case s.Kind == rich.SpanMention:
			st.Color = colMention
			if s.ID != "" {
				st.Interactive = true
				st.Set("user", s.ID)
			}
		case s.Kind == rich.SpanChannel:
			st.Color = colMention
		case s.Kind == rich.SpanBroadcast:
			st.Color = colBroadcast
			if s.ID != "" { // a usergroup: its card on hover
				st.Interactive = true
				st.Set("user", s.ID)
				if v.myGroup != nil && v.myGroup(s.ID) {
					st.Color = colMention
				}
			}
		case s.Kind == rich.SpanMuted, s.Strike:
			st.Color = colMuted
		}
		out = append(out, st)
	}
	return out
}

// customEmoji is the picture of a workspace's custom emoji, by name
// (":partyparrot:", "partyparrot" or with a skin tone), once it has
// loaded; asking starts the download, and the window redraws when it
// lands.
func (v *richView) customEmoji(name string) (paint.ImageOp, bool) {
	name = strings.Trim(name, ":")
	if i := strings.Index(name, "::"); i >= 0 {
		name = name[:i]
	}
	url, ok := emoji.BuildCustomEmojiURL(name, v.customs)
	if !ok {
		return paint.ImageOp{}, false
	}
	op, state := v.images.get(emoji.EmojiCacheKey(url), url, image.Point{}, v.box)
	return op, state == imgReady
}

// emojiPicture draws an emoji's picture in the largest square that
// fits the constraints, centered.
func emojiPicture(gtx C, img paint.ImageOp) D {
	size := gtx.Constraints.Max
	side := min(size.X, size.Y)
	defer op.Offset(image.Pt((size.X-side)/2, (size.Y-side)/2)).Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Exact(image.Pt(side, side))
	widget.Image{Src: img, Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
	return D{Size: size}
}

func (v *richView) text(gtx C, w *msgWidgets, spans []rich.Span, size unit.Sp, col color.NRGBA) D {
	if len(spans) == 0 {
		return D{}
	}
	st := richtext.Text(w.text(), v.th.Shaper, v.spanStyles(spans, size, col)...)
	if !w.tsel.on {
		return st.Layout(gtx)
	}
	// Selectable: note where each piece of text went, and draw the
	// selection behind it.
	var segs []styledtext.Segment
	st.Segments = &segs
	m := op.Record(gtx.Ops)
	dims := st.Layout(gtx)
	call := m.Stop()
	w.tsel.drawn(gtx, segs)
	call.Add(gtx.Ops)
	return dims
}

// blocks lays out blocks one under another.
func (v *richView) blocks(gtx C, w *msgWidgets, bs []rich.Block) D {
	return stack(gtx, w, len(bs), func(gtx C, i int) D { return v.block(gtx, w, bs[i]) })
}

// stack lays out n children one under another, as a vertical Flex of
// Rigids does, keeping the selection's idea of where each child is
// drawn (w.tsel.origin) in step.
func stack(gtx C, w *msgWidgets, n int, child func(gtx C, i int) D) D {
	y, width := 0, 0
	for i := range n {
		cgtx := gtx
		cgtx.Constraints.Min = image.Point{}
		cgtx.Constraints.Max.Y = max(0, gtx.Constraints.Max.Y-y)
		off := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		var dims D
		w.tsel.at(image.Pt(0, y), func() { dims = child(cgtx, i) })
		off.Pop()
		y += dims.Size.Y
		width = max(width, dims.Size.X)
	}
	return D{Size: gtx.Constraints.Constrain(image.Pt(width, y))}
}

func (v *richView) block(gtx C, w *msgWidgets, b rich.Block) D {
	switch b := b.(type) {
	case rich.Paragraph:
		return v.text(gtx, w, b.Spans, textSize, colFg)
	case rich.Heading:
		return insetSel(gtx, w, layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(2)}, func(gtx C) D {
			spans := make([]rich.Span, len(b.Spans))
			for i, s := range b.Spans {
				s.Bold = true
				spans[i] = s
			}
			return v.text(gtx, w, spans, headingSize, colFg)
		})
	case rich.Quote:
		return insetSel(gtx, w, layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}, func(gtx C) D {
			return v.barredSel(gtx, w, colBorder, func(gtx C) D { return v.blocks(gtx, w, b.Blocks) })
		})
	case rich.CodeBlock:
		// Code in the monospace face, on its tint inside a 1 dp rule.
		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
			return widget.Border{Color: colBorder, CornerRadius: cardRadius, Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
				return rounded(gtx, colCodeBg, cardRadius, func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
						lbl := secondaryLabel(v.th, b.Text)
						lbl.Font = fontMono
						lbl.State = w.sel()
						return lbl.Layout(gtx)
					})
				})
			})
		})
	case rich.List:
		return v.list(gtx, w, b)
	case rich.Divider:
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, divider)
	case rich.Image:
		max := imageMaxDp
		if b.Small {
			max = thumbDp
		}
		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
			return v.image(gtx, w, b, max)
		})
	case rich.File:
		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
			return v.file(gtx, w, b)
		})
	case rich.Fields:
		// Fields and context sit side by side: not selectable (their
		// places are not tracked).
		return w.tsel.without(func() D { return v.fields(gtx, w, b) })
	case rich.Context:
		return w.tsel.without(func() D {
			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
				return v.context(gtx, w, b)
			})
		})
	case rich.Buttons:
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
			return v.buttons(gtx, w, b.Items)
		})
	case rich.Card:
		return insetSel(gtx, w, layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}, func(gtx C) D {
			return v.barredSel(gtx, w, cardColor(b.Color), func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return v.blocks(gtx, w, b.Blocks) }),
					layout.Rigid(func(gtx C) D {
						if b.Footer == "" {
							return D{}
						}
						return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
							lbl := captionLabel(v.th, b.Footer)
							lbl.Color = colMuted
							return lbl.Layout(gtx)
						})
					}),
				)
			})
		})
	}
	return D{}
}

// barredSel is barred keeping the selection's origin in step.
func (v *richView) barredSel(gtx C, w *msgWidgets, col color.NRGBA, content layout.Widget) D {
	shift := gtx.Dp(unit.Dp(4)) + gtx.Dp(unit.Dp(10))
	return v.barred(gtx, col, func(gtx C) D {
		var dims D
		w.tsel.at(image.Pt(shift, 0), func() { dims = content(gtx) })
		return dims
	})
}

// insetSel is in.Layout keeping the selection's origin in step.
func insetSel(gtx C, w *msgWidgets, in layout.Inset, content layout.Widget) D {
	return in.Layout(gtx, func(gtx C) D {
		var dims D
		w.tsel.at(image.Pt(gtx.Dp(in.Left), gtx.Dp(in.Top)), func() { dims = content(gtx) })
		return dims
	})
}

// barred draws content with a vertical stripe on its left, as tall as
// the content turns out to be.
func (v *richView) barred(gtx C, col color.NRGBA, content layout.Widget) D {
	barW := gtx.Dp(unit.Dp(4))
	gap := gtx.Dp(unit.Dp(10))
	inner := gtx
	inner.Constraints.Max.X -= barW + gap
	inner.Constraints.Min.X = 0
	m := op.Record(gtx.Ops)
	dims := content(inner)
	call := m.Stop()
	paint.FillShape(gtx.Ops, col, clip.UniformRRect(image.Rectangle{Max: image.Pt(barW, dims.Size.Y)}, gtx.Dp(unit.Dp(2))).Op(gtx.Ops))
	defer op.Offset(image.Pt(barW+gap, 0)).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
	dims.Size.X += barW + gap
	return dims
}

func (v *richView) list(gtx C, w *msgWidgets, l rich.List) D {
	return stack(gtx, w, len(l.Items), func(gtx C, i int) D {
		marker := "•"
		if l.Ordered {
			marker = strconv.Itoa(l.Start+i+1) + "."
		}
		indent := gtx.Dp(unit.Dp(16 * float32(l.Indent)))
		// The marker, at least 22 dp wide, then the item's text.
		mgtx := gtx
		mgtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(22)), 0)
		mm := op.Record(gtx.Ops)
		lbl := bodyLabel(v.th, marker)
		lbl.Color = colMuted
		md := lbl.Layout(mgtx)
		mcall := mm.Stop()
		tx := indent + md.Size.X
		tgtx := gtx
		tgtx.Constraints.Min = image.Point{}
		tgtx.Constraints.Max.X = max(0, gtx.Constraints.Max.X-tx)
		tm := op.Record(gtx.Ops)
		var td D
		w.tsel.at(image.Pt(tx, 0), func() { td = v.text(tgtx, w, l.Items[i], textSize, colFg) })
		tcall := tm.Stop()
		// Same font and size: aligned at the top, the marker sits on
		// the first line's baseline.
		my := 0
		off := op.Offset(image.Pt(indent, my)).Push(gtx.Ops)
		mcall.Add(gtx.Ops)
		off.Pop()
		off = op.Offset(image.Pt(tx, 0)).Push(gtx.Ops)
		tcall.Add(gtx.Ops)
		off.Pop()
		return D{Size: image.Pt(tx+td.Size.X, max(td.Size.Y, my+md.Size.Y))}
	})
}

// fields lays out short fields two to a row and the rest full width.
func (v *richView) fields(gtx C, w *msgWidgets, f rich.Fields) D {
	var rows []layout.FlexChild
	items := f.Items
	for len(items) > 0 {
		var row []rich.Field
		if items[0].Short && len(items) > 1 && items[1].Short {
			row, items = items[:2], items[2:]
		} else {
			row, items = items[:1], items[1:]
		}
		rows = append(rows, layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, func(gtx C) D {
				cells := make([]layout.FlexChild, 0, 2)
				for _, cell := range row {
					cells = append(cells, layout.Flexed(1, func(gtx C) D {
						return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
							return v.text(gtx, w, cell.Spans, textSize, colFg)
						})
					}))
				}
				return layout.Flex{}.Layout(gtx, cells...)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

func (v *richView) context(gtx C, w *msgWidgets, c rich.Context) D {
	children := make([]layout.FlexChild, 0, len(c.Images)+1)
	for _, im := range c.Images {
		children = append(children, layout.Rigid(func(gtx C) D {
			return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D { return v.image(gtx, w, im, iconDp) })
		}))
	}
	children = append(children, layout.Flexed(1, func(gtx C) D { return v.text(gtx, w, c.Items, captionSize, colMuted) }))
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

// buttons draws action labels as outlined chips; they do nothing.
func (v *richView) buttons(gtx C, w *msgWidgets, items []rich.Button) D {
	children := make([]layout.FlexChild, 0, len(items))
	for _, it := range items {
		children = append(children, layout.Rigid(func(gtx C) D {
			return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
				return v.button(gtx, w, it)
			})
		}))
	}
	return layout.Flex{}.Layout(gtx, children...)
}

// button is one button: a framed label. A link button opens its URL; a
// button of the message's Actions (an ephemeral's) is pressed, with a
// red frame when it is dangerous and an accent one when it is the
// primary choice; anything else (Block Kit, a menu) is only drawn.
func (v *richView) button(gtx C, w *msgWidgets, it rich.Button) D {
	frame := func(gtx C, border, bg color.NRGBA) D {
		b := widget.Border{Color: border, CornerRadius: unit.Dp(4), Width: unit.Dp(1)}
		return b.Layout(gtx, func(gtx C) D {
			return rounded(gtx, bg, unit.Dp(4), func(gtx C) D {
				return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx,
					secondaryLabel(v.th, it.Label).Layout)
			})
		})
	}
	var c *openClick
	switch {
	case it.URL != "":
		c = w.click(it.URL)
	case it.Action >= 0 && it.Action < len(w.msg.Actions) && w.msg.Actions[it.Action].Type == "button":
		c = w.click("")
		c.action = it.Action
	default:
		return frame(gtx, colBorder, color.NRGBA{})
	}
	border := colBorder
	switch it.Style {
	case "danger":
		border = colError
	case "primary":
		border = colAccent
	}
	return c.btn.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		bg := color.NRGBA{}
		if c.btn.Hovered() {
			bg = colControl
		}
		return frame(gtx, border, bg)
	})
}

// file is a chip naming a non-image upload; clicking opens it in Slack.
func (v *richView) file(gtx C, w *msgWidgets, f rich.File) D {
	c, att := w.attachment()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return v.fileChip(gtx, c, f) }),
		layout.Rigid(func(gtx C) D { return v.busyNote(gtx, att) }),
	)
}

// fileChip is a file's name and size in a frame.
func (v *richView) fileChip(gtx C, c *openClick, f rich.File) D {
	return c.btn.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		border := widget.Border{Color: colBorder, CornerRadius: cardRadius, Width: unit.Dp(1)}
		return border.Layout(gtx, func(gtx C) D {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						lbl := secondaryLabel(v.th, f.Name)
						lbl.Color = colAccent
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						lbl := captionLabel(v.th, strings.TrimSpace(attach.HumanSize(f.Size)+"  "+f.Mime))
						lbl.Color = colMuted
						return lbl.Layout(gtx)
					}),
				)
			})
		})
	})
}

// image draws a picture no larger than max on a side: a placeholder
// of the right shape while it loads, the picture once it has, a note
// if it failed. Clicking opens it.
func (v *richView) image(gtx C, w *msgWidgets, im rich.Image, max unit.Dp) D {
	c, att := w.attachment()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return v.picture(gtx, c, im, max) }),
		layout.Rigid(func(gtx C) D { return v.busyNote(gtx, att) }),
	)
}

// picture is the image itself, or its placeholder or name.
func (v *richView) picture(gtx C, c *openClick, im rich.Image, max unit.Dp) D {
	maxPx := gtx.Dp(max)
	if maxPx > gtx.Constraints.Max.X {
		maxPx = gtx.Constraints.Max.X
	}
	url, key := imageSource(im, image.Pt(maxPx, maxPx))
	op, state := v.images.get(key, url, image.Point{}, v.box)
	box := fitBox(im.W, im.H, maxPx)
	switch state {
	case imgReady:
		if sz := op.Size(); sz.X > 0 && sz.Y > 0 {
			box = fitBox(sz.X, sz.Y, maxPx)
		}
		return c.btn.Layout(gtx, func(gtx C) D {
			pointer.CursorPointer.Add(gtx.Ops)
			defer clip.UniformRRect(image.Rectangle{Max: box}, gtx.Dp(cardRadius)).Push(gtx.Ops).Pop()
			gtx.Constraints = layout.Exact(box)
			return widget.Image{Src: op, Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
		})
	case imgLoading:
		return c.btn.Layout(gtx, func(gtx C) D {
			pointer.CursorPointer.Add(gtx.Ops)
			paint.FillShape(gtx.Ops, colHover, clip.UniformRRect(image.Rectangle{Max: box}, gtx.Dp(cardRadius)).Op(gtx.Ops))
			return D{Size: box}
		})
	}
	// imgFailed or imgNone: name the picture and let a click open it.
	name := im.Title
	if name == "" {
		name = im.Alt
	}
	if name == "" {
		name = "image"
	}
	return c.btn.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		lbl := captionLabel(v.th, "🖼 "+name)
		lbl.Color = colAccent
		return lbl.Layout(gtx)
	})
}

// imageSource picks the URL to download for a picture shown at about
// target pixels, and its cache key, which matches the TUI's so the two
// share downloads: "<fileID>-<thumb suffix>" for uploads, "BK-<hash>"
// for anything else.
func imageSource(im rich.Image, target image.Point) (url, key string) {
	if im.FileID != "" && len(im.Thumbs) > 0 {
		thumbs := make([]imgpkg.ThumbSpec, len(im.Thumbs))
		for i, t := range im.Thumbs {
			thumbs[i] = imgpkg.ThumbSpec{URL: t.URL, W: t.W, H: t.H}
		}
		url, suffix := imgpkg.PickThumb(thumbs, target)
		return url, im.FileID + "-" + suffix
	}
	if im.FileID != "" {
		return im.URL, im.FileID + "-full"
	}
	return im.URL, urlKey(im.URL)
}

// fitBox scales w×h to fit in a max square, never enlarging; unknown
// sizes get a wide placeholder.
func fitBox(w, h, max int) image.Point {
	if w <= 0 || h <= 0 {
		return image.Pt(max*2/3, max*2/5)
	}
	if w <= max && h <= max {
		return image.Pt(w, h)
	}
	if w >= h {
		return image.Pt(max, h*max/w)
	}
	return image.Pt(w*max/h, max)
}

// avatar draws a user's picture, or a colored disc with their initial
// while it loads or when there is none.
func (v *richView) avatar(gtx C, id, url, name string, size unit.Dp) D {
	px := gtx.Dp(size)
	box := image.Pt(px, px)
	radius := gtx.Dp(unit.Dp(6))
	if url != "" {
		op, state := v.images.get(avatar.FetchKey(id, url), url, image.Point{}, v.box)
		if state == imgReady {
			defer clip.UniformRRect(image.Rectangle{Max: box}, radius).Push(gtx.Ops).Pop()
			gtx.Constraints = layout.Exact(box)
			return widget.Image{Src: op, Fit: widget.Cover, Position: layout.Center}.Layout(gtx)
		}
	}
	paint.FillShape(gtx.Ops, authorColor(name), clip.UniformRRect(image.Rectangle{Max: box}, radius).Op(gtx.Ops))
	gtx.Constraints = layout.Exact(box)
	return layout.Center.Layout(gtx, func(gtx C) D {
		lbl := material.Label(v.th, unit.Sp(float32(size)*0.5), initial(name))
		lbl.Color = textOn(authorColor(name))
		lbl.Font.Weight = font.SemiBold
		return lbl.Layout(gtx)
	})
}

func initial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// cardWidthDp is the hover card's width.
const cardWidthDp = unit.Dp(300)

// profileCard draws what is known about a person: picture, names,
// title, status and local time. While the profile loads only the name
// the message carried is shown.
func (v *richView) profileCard(gtx C, name string, p *backend.Profile, err error) D {
	gtx.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(cardWidthDp), gtx.Constraints.Max.Y)}
	id, avatarURL := "", ""
	if p != nil {
		id, avatarURL = p.ID, p.AvatarURL
		if p.Name != "" {
			name = p.Name
		}
	}
	return overlayFrame(gtx, func(gtx C) D {
		return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx C) D {
			return layout.Flex{Alignment: layout.Start}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
						return v.avatar(gtx, id, avatarURL, name, unit.Dp(56))
					})
				}),
				layout.Flexed(1, func(gtx C) D {
					var rows []layout.FlexChild
					line := func(s string, size unit.Sp, col color.NRGBA, bold bool) {
						if s == "" {
							return
						}
						rows = append(rows, layout.Rigid(func(gtx C) D {
							lbl := material.Label(v.th, size, s)
							lbl.Color = col
							if bold {
								lbl.Font.Weight = font.SemiBold
							}
							return lbl.Layout(gtx)
						}))
					}
					line(name, textSize, colFg, true)
					switch {
					case err != nil:
						line("Profile unavailable: "+err.Error(), captionSize, colError, false)
					case p == nil:
						line("Loading…", captionSize, colMuted, false)
					default:
						if p.RealName != "" && p.RealName != name {
							line(p.RealName, textSize, colMuted, false)
						}
						if p.Handle != "" {
							line("@"+p.Handle, captionSize, colMuted, false)
						}
						line(p.Title, textSize, colFg, false)
						if status := chat.StatusText(*p, time.Now()); status != "" || p.StatusEmoji != "" && p.StatusExpires.IsZero() {
							line(strings.TrimSpace(emoji.Sprint(p.StatusEmoji)+" "+status), textSize, colFg, false)
						}
						if p.DND && (p.DNDUntil.IsZero() || time.Now().Before(p.DNDUntil)) {
							line(chat.DNDText(p.DNDUntil, time.Now()), captionSize, colOffline, false)
						}
						if p.HasTZ {
							local := time.Now().UTC().Add(time.Duration(p.TZOffset) * time.Second)
							s := local.Format("15:04") + " local time"
							if p.TZLabel != "" {
								s += " (" + p.TZLabel + ")"
							}
							line(s, captionSize, colMuted, false)
						}
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
				}),
			)
		})
	})
}
