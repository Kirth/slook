package styledtext

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"golang.org/x/exp/constraints"
	"golang.org/x/image/math/fixed"
)

// textIterator computes the bounding box of and paints text. This iterator is
// specialized to laying out single lines of text.
type textIterator struct {
	// viewport is the rectangle of document coordinates that the iterator is
	// trying to fill with text.
	viewport image.Rectangle
	// maxLines tracks the maximum allowed number of glyphs with FlagLineBreak.
	maxLines int

	// linesSeen tracks the number of FlagLineBreak glyphs we have seen.
	linesSeen int
	// init tracks whether the iterator has processed any glyphs.
	init bool
	// firstX tracks the x offset of the first processed glyph. This is subtracted
	// from all glyph x offsets in order to ensure that the text is rendered at
	// x=0.
	firstX fixed.Int26_6
	// hasNewline tracks whether the processed glyphs contained a synthetic newline
	// character.
	hasNewline bool
	// lineOff tracks the origin for the glyphs in the current line.
	lineOff image.Point
	// padding is the space needed outside of the bounds of the text to ensure no
	// part of a glyph is clipped.
	padding image.Rectangle
	// bounds is the logical bounding box of the text.
	bounds image.Rectangle
	// runes is the count of runes represented by the processed glyphs.
	runes int
	// visible tracks whether the most recently iterated glyph is visible within
	// the viewport.
	visible bool
	// first tracks whether the iterator has processed a glyph yet.
	first bool
	// baseline tracks the location of the first line of text's baseline.
	baseline int
	// clusters are where each shaped cluster starts (x from the first
	// glyph) and how many runes it covers; end is where the last glyph
	// ends. Together they give every rune's x, for text selection.
	clusters []cluster
	end      int
}

// cluster is a run of runes shaped together: where it starts, and how
// many runes.
type cluster struct {
	x, runes int
}

// processGlyph checks whether the glyph is visible within the iterator's configured
// viewport and (if so) updates the iterator's text dimensions to include the glyph.
func (it *textIterator) processGlyph(g text.Glyph, ok bool) (_ text.Glyph, visibleOrBefore bool) {
	logicalBounds := image.Rectangle{
		Min: image.Pt(g.X.Floor(), int(g.Y)-g.Ascent.Ceil()),
		Max: image.Pt((g.X + g.Advance).Ceil(), int(g.Y)+g.Descent.Ceil()),
	}
	if g.Flags&text.FlagTruncator != 0 {
		// If the truncator is the first glyph, force a newline.
		if it.runes == 0 {
			it.hasNewline = true
		}
		// We always need to update the vertical bounds for the truncator glyph in case it's the only
		// glyph on its line. Otherwise the line will seem to have zero size.
		it.bounds.Min.Y = min(it.bounds.Min.Y, logicalBounds.Min.Y)
		it.bounds.Max.Y = max(it.bounds.Max.Y, logicalBounds.Max.Y)
		return g, false
	}
	it.runes += int(g.Runes)
	it.hasNewline = it.hasNewline || (g.Flags&text.FlagLineBreak > 0 && g.Flags&text.FlagParagraphBreak > 0)
	if it.maxLines > 0 {
		if g.Flags&text.FlagLineBreak != 0 {
			it.linesSeen++
		}
		if it.linesSeen == it.maxLines && g.Flags&text.FlagParagraphBreak != 0 {
			return g, false
		}
	}
	// Compute the maximum extent to which glyphs overhang on the horizontal
	// axis.
	if d := g.Bounds.Min.X.Floor(); d < it.padding.Min.X {
		it.padding.Min.X = d
	}
	if d := (g.Bounds.Max.X - g.Advance).Ceil(); d > it.padding.Max.X {
		it.padding.Max.X = d
	}
	if !it.first {
		it.first = true
		it.baseline = int(g.Y)
		it.bounds = logicalBounds
	}

	above := logicalBounds.Max.Y < it.viewport.Min.Y
	below := logicalBounds.Min.Y > it.viewport.Max.Y
	left := logicalBounds.Max.X < it.viewport.Min.X
	right := logicalBounds.Min.X > it.viewport.Max.X
	it.visible = !above && !below && !left && !right
	if it.visible {
		it.bounds.Min.X = min(it.bounds.Min.X, logicalBounds.Min.X)
		it.bounds.Min.Y = min(it.bounds.Min.Y, logicalBounds.Min.Y)
		it.bounds.Max.X = max(it.bounds.Max.X, logicalBounds.Max.X)
		it.bounds.Max.Y = max(it.bounds.Max.Y, logicalBounds.Max.Y)
	}
	return g, ok && !below

}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func max[T constraints.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

// paintGlyph buffers up and paints text glyphs. It should be invoked iteratively upon each glyph
// until it returns false. The line parameter should be a slice with
// a backing array of sufficient size to buffer multiple glyphs.
// A modified slice will be returned with each invocation, and is
// expected to be passed back in on the following invocation.
// This design is awkward, but prevents the line slice from escaping
// to the heap.
func (it *textIterator) paintGlyph(gtx layout.Context, shaper *text.Shaper, glyph text.Glyph, line []text.Glyph) ([]text.Glyph, bool) {
	_, visibleOrBefore := it.processGlyph(glyph, true)
	if it.visible {
		if !it.init {
			it.firstX = glyph.X
			it.init = true
		}
		if glyph.Flags&text.FlagTruncator == 0 {
			if glyph.Runes > 0 {
				it.clusters = append(it.clusters, cluster{x: (glyph.X - it.firstX).Round(), runes: int(glyph.Runes)})
			}
			it.end = max(it.end, (glyph.X + glyph.Advance - it.firstX).Round())
		}
		if len(line) == 0 {
			it.lineOff = image.Point{X: (glyph.X - it.firstX).Floor(), Y: int(glyph.Y)}.Sub(it.viewport.Min)
		}
		line = append(line, glyph)
	}
	if glyph.Flags&text.FlagLineBreak > 0 || cap(line)-len(line) == 0 || !visibleOrBefore {
		t := op.Offset(it.lineOff).Push(gtx.Ops)
		outline := clip.Outline{Path: shaper.Shape(line)}.Op().Push(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		outline.Pop()
		// Bitmap glyphs (color emoji) are not part of the outline path.
		if call := shaper.Bitmaps(line); call != (op.CallOp{}) {
			call.Add(gtx.Ops)
		}
		t.Pop()
		line = line[:0]
	}
	return line, visibleOrBefore
}

// runeXs is the x of each rune boundary of the processed glyphs: len
// runes+1, from the first rune's start to the last's end. A cluster's
// runes share its width evenly.
func (it *textIterator) runeXs(runes int) []int {
	xs := make([]int, 0, runes+1)
	for i, c := range it.clusters {
		next := it.end
		if i+1 < len(it.clusters) {
			next = it.clusters[i+1].x
		}
		for k := 0; k < c.runes; k++ {
			xs = append(xs, c.x+(next-c.x)*k/c.runes)
		}
	}
	for len(xs) < runes {
		xs = append(xs, it.end) // runes not shaped (a line's newline)
	}
	xs = xs[:runes]
	return append(xs, it.end)
}
