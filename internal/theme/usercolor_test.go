package theme

import (
	"fmt"
	"image/color"
	"testing"
)

func colorEqual(a, b color.Color) bool {
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

func TestUserColorDeterministic(t *testing.T) {
	c1 := UserColor("U12345")
	c2 := UserColor("U12345")
	if !colorEqual(c1, c2) {
		t.Fatal("same userID must return same color")
	}
}

func TestUserColorPaletteSpread(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range []string{"U1", "U2", "U3", "U4", "U5", "U6", "U7", "U8", "U9", "U10", "U11", "U12"} {
		r, g, b, _ := UserColor(id).RGBA()
		key := fmt.Sprintf("%02x%02x%02x", r>>8, g>>8, b>>8)
		seen[key] = true
	}
	if len(seen) < 8 {
		t.Fatalf("poor distribution: only %d distinct colors for 12 users", len(seen))
	}
}

func TestUserPaletteValues(t *testing.T) {
	if len(UserPalette) != 12 {
		t.Fatalf("UserPalette has %d colours, want 12", len(UserPalette))
	}
	if got, want := UserPalette[0], (color.NRGBA{0xE0, 0x1E, 0x5A, 0xFF}); got != want {
		t.Errorf("UserPalette[0] = %v, want %v", got, want)
	}
}
