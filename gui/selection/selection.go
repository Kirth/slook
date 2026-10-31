// Package selection is text selection over a message's text blocks as
// they were drawn: where a point falls in the text, the words and blocks
// around a position, the rectangles to highlight, and the text to copy.
// It is pure; slk-gio records each block's segments while drawing (see
// styledtext.Segment) and asks here.
package selection

import (
	"image"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Seg is a piece of text drawn on one line: its text, its rectangle
// (relative to its block's origin), and the x of each rune boundary
// relative to Rect.Min.X (len runes+1).
type Seg struct {
	Text string
	Rect image.Rectangle
	X    []int
}

// Block is one run of text (a paragraph, a list item): where it was
// drawn, in the coordinates of the whole selectable area, and its
// pieces in reading order.
type Block struct {
	Origin image.Point
	Segs   []Seg
}

// Pos is a place in the text between two runes: which block, and how
// many runes into it.
type Pos struct{ Block, Rune int }

// Less reports whether a comes before b.
func Less(a, b Pos) bool {
	if a.Block != b.Block {
		return a.Block < b.Block
	}
	return a.Rune < b.Rune
}

// Order returns a and b with the earlier first.
func Order(a, b Pos) (Pos, Pos) {
	if Less(b, a) {
		return b, a
	}
	return a, b
}

// runes is how many runes a block holds.
func (b Block) runes() int {
	n := 0
	for _, s := range b.Segs {
		n += utf8.RuneCountInString(s.Text)
	}
	return n
}

// text is the block's text, its pieces joined.
func (b Block) text() string {
	var sb strings.Builder
	for _, s := range b.Segs {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

// bounds is the block's rectangle in area coordinates.
func (b Block) bounds() image.Rectangle {
	var r image.Rectangle
	for i, s := range b.Segs {
		if i == 0 {
			r = s.Rect
		} else {
			r = r.Union(s.Rect)
		}
	}
	return r.Add(b.Origin)
}

// Hit is the position nearest p (area coordinates): in the block p is
// in or nearest above it, on the line nearest p, between the runes
// nearest p's x. Above the first block is its start; below the last,
// its end. ok is false when there are no blocks.
func Hit(blocks []Block, p image.Point) (Pos, bool) {
	bi := -1
	for i, b := range blocks {
		if len(b.Segs) == 0 {
			continue
		}
		if bi < 0 || p.Y >= b.bounds().Min.Y {
			bi = i
		}
	}
	if bi < 0 {
		return Pos{}, false
	}
	b := blocks[bi]
	if p.Y < b.bounds().Min.Y {
		return Pos{Block: bi}, true
	}
	if p.Y >= b.bounds().Max.Y && bi == lastWithText(blocks) {
		return Pos{Block: bi, Rune: b.runes()}, true
	}
	q := p.Sub(b.Origin)
	// The line: the segments whose rows hold q.Y, else the nearest row.
	line := nearestLine(b.Segs, q.Y)
	offset, best, bestD := 0, 0, -1
	for _, s := range b.Segs {
		n := utf8.RuneCountInString(s.Text)
		if s.Rect.Min.Y == line {
			for k := 0; k <= n && k < len(s.X); k++ {
				d := abs(s.Rect.Min.X + s.X[k] - q.X)
				if bestD < 0 || d < bestD {
					best, bestD = offset+k, d
				}
			}
		}
		offset += n
	}
	return Pos{Block: bi, Rune: best}, true
}

func lastWithText(blocks []Block) int {
	for i := len(blocks) - 1; i >= 0; i-- {
		if len(blocks[i].Segs) > 0 {
			return i
		}
	}
	return -1
}

// nearestLine is the top of the line (a segment row) nearest y.
func nearestLine(segs []Seg, y int) int {
	best, bestD := segs[0].Rect.Min.Y, -1
	for _, s := range segs {
		var d int
		switch {
		case y < s.Rect.Min.Y:
			d = s.Rect.Min.Y - y
		case y >= s.Rect.Max.Y:
			d = y - s.Rect.Max.Y + 1
		}
		if bestD < 0 || d < bestD {
			best, bestD = s.Rect.Min.Y, d
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Rects are the rectangles to highlight in block b (block coordinates)
// for the runes from..to of it.
func Rects(b Block, from, to int) []image.Rectangle {
	var out []image.Rectangle
	offset := 0
	for _, s := range b.Segs {
		n := utf8.RuneCountInString(s.Text)
		lo, hi := max(from-offset, 0), min(to-offset, n)
		if lo < hi && hi < len(s.X) {
			r := image.Rect(s.Rect.Min.X+s.X[lo], s.Rect.Min.Y, s.Rect.Min.X+s.X[hi], s.Rect.Max.Y)
			if r.Dx() > 0 {
				out = append(out, r)
			}
		}
		offset += n
	}
	return out
}

// Range is the rune range of block i that a selection from a to b
// covers, and whether it covers any of it.
func Range(blocks []Block, i int, a, b Pos) (from, to int, ok bool) {
	a, b = Order(a, b)
	if i < a.Block || i > b.Block || i >= len(blocks) {
		return 0, 0, false
	}
	from, to = 0, blocks[i].runes()
	if i == a.Block {
		from = a.Rune
	}
	if i == b.Block {
		to = b.Rune
	}
	return from, to, from < to
}

// Text is the text from a to b, blocks on lines of their own.
func Text(blocks []Block, a, b Pos) string {
	a, b = Order(a, b)
	var parts []string
	for i := a.Block; i <= b.Block && i < len(blocks); i++ {
		from, to, ok := Range(blocks, i, a, b)
		if !ok {
			if i > a.Block && i < b.Block {
				parts = append(parts, "")
			}
			continue
		}
		r := []rune(blocks[i].text())
		parts = append(parts, strings.TrimRight(string(r[from:min(to, len(r))]), "\n"))
	}
	return strings.Join(parts, "\n")
}

// Word is the word around p: the run of letters, digits and joiners
// (_ - . @ /) it is in, or the single rune after it when that is none.
func Word(blocks []Block, p Pos) (Pos, Pos) {
	if p.Block < 0 || p.Block >= len(blocks) {
		return p, p
	}
	r := []rune(blocks[p.Block].text())
	in := func(i int) bool {
		if i < 0 || i >= len(r) {
			return false
		}
		c := r[i]
		return unicode.IsLetter(c) || unicode.IsDigit(c) || strings.ContainsRune("_-.@/'", c)
	}
	from, to := p.Rune, p.Rune
	if !in(from) && in(from-1) {
		from--
		to--
	}
	if !in(from) {
		return p, Pos{p.Block, min(p.Rune+1, len(r))}
	}
	for in(from - 1) {
		from--
	}
	for in(to) {
		to++
	}
	return Pos{p.Block, from}, Pos{p.Block, to}
}

// Whole is the whole of p's block.
func Whole(blocks []Block, p Pos) (Pos, Pos) {
	if p.Block < 0 || p.Block >= len(blocks) {
		return p, p
	}
	return Pos{p.Block, 0}, Pos{p.Block, blocks[p.Block].runes()}
}
