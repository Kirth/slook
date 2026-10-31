package selection

import (
	"image"
	"slices"
	"testing"
)

// line is a segment whose runes are each 10 px wide, at row y.
func line(text string, x, y int) Seg {
	n := len([]rune(text))
	xs := make([]int, n+1)
	for i := range xs {
		xs[i] = i * 10
	}
	return Seg{Text: text, Rect: image.Rect(x, y, x+n*10, y+20), X: xs}
}

// two paragraphs: "hello world" wrapped as "hello " / "world" (two lines),
// then "second para" 50 px lower.
var blocks = []Block{
	{Origin: image.Pt(0, 0), Segs: []Seg{line("hello ", 0, 0), line("world", 0, 20)}},
	{Origin: image.Pt(0, 50), Segs: []Seg{line("second ", 0, 0), line("para", 70, 0)}},
}

func TestHit(t *testing.T) {
	for _, c := range []struct {
		p    image.Point
		want Pos
	}{
		{image.Pt(0, 5), Pos{0, 0}},
		{image.Pt(24, 5), Pos{0, 2}},   // nearer 20 than 30
		{image.Pt(26, 5), Pos{0, 3}},   // nearer 30
		{image.Pt(500, 5), Pos{0, 6}},  // past the line's end: its end
		{image.Pt(12, 30), Pos{0, 7}},  // second line: "w|orld"
		{image.Pt(85, 55), Pos{1, 8}},  // the second segment of block 1
		{image.Pt(0, -10), Pos{0, 0}},  // above everything
		{image.Pt(0, 500), Pos{1, 11}}, // below everything
		{image.Pt(30, 45), Pos{0, 9}},  // in the gap: the block above, "wor|ld"
	} {
		got, ok := Hit(blocks, c.p)
		if !ok || got != c.want {
			t.Errorf("Hit(%v) = %v %v, want %v", c.p, got, ok, c.want)
		}
	}
	if _, ok := Hit(nil, image.Pt(0, 0)); ok {
		t.Fatal("a hit with no blocks")
	}
}

func TestTextAndRange(t *testing.T) {
	if got := Text(blocks, Pos{0, 6}, Pos{0, 11}); got != "world" {
		t.Errorf("one block: %q", got)
	}
	if got := Text(blocks, Pos{1, 3}, Pos{0, 8}); got != "rld\nsec" {
		t.Errorf("across blocks, backwards: %q", got)
	}
	if from, to, ok := Range(blocks, 1, Pos{0, 2}, Pos{1, 4}); !ok || from != 0 || to != 4 {
		t.Errorf("range of the last block: %d %d %v", from, to, ok)
	}
	if _, _, ok := Range(blocks, 1, Pos{0, 2}, Pos{0, 5}); ok {
		t.Error("a block outside the selection is in range")
	}
}

func TestRects(t *testing.T) {
	got := Rects(blocks[0], 3, 8)
	want := []image.Rectangle{image.Rect(30, 0, 60, 20), image.Rect(0, 20, 20, 40)}
	if !slices.Equal(got, want) {
		t.Fatalf("rects %v, want %v", got, want)
	}
}

func TestWordAndWhole(t *testing.T) {
	a, b := Word(blocks, Pos{0, 8})
	if a != (Pos{0, 6}) || b != (Pos{0, 11}) {
		t.Errorf("word: %v %v", a, b)
	}
	a, b = Word(blocks, Pos{0, 5}) // between "hello" and the space: hello
	if a != (Pos{0, 0}) || b != (Pos{0, 5}) {
		t.Errorf("word at its end: %v %v", a, b)
	}
	a, b = Whole(blocks, Pos{1, 3})
	if a != (Pos{1, 0}) || b != (Pos{1, 11}) {
		t.Errorf("whole: %v %v", a, b)
	}
}
