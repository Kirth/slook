package main

import (
	"bytes"
	"errors"
	"image/png"
	"testing"
)

func TestTrayIconIsAPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(trayIcon()))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 32 || b.Dy() != 32 {
		t.Fatalf("size %v", b)
	}
	// Only opaque or transparent pixels survive fyne.io/systray's
	// conversion, so those are all the icon may have.
	opaque := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			switch _, _, _, a := img.At(x, y).RGBA(); a {
			case 0:
			case 0xffff:
				opaque++
			default:
				t.Fatalf("(%d,%d) has alpha %#x", x, y, a)
			}
		}
	}
	if opaque == 0 {
		t.Fatal("nothing left to see")
	}
}

func TestLastWindowClosed(t *testing.T) {
	for _, c := range []struct {
		name     string
		tray     bool
		err      error
		wantExit int // -1: keeps running
	}{
		{"no tray exits", false, nil, 0},
		{"tray keeps running", true, nil, -1},
		{"a failed window exits with 1", false, errors.New("x"), 1},
		{"even with the tray", true, errors.New("x"), 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := -1
			lastWindowClosed(func() bool { return c.tray }, func(code int) { got = code })(c.err)
			if got != c.wantExit {
				t.Fatalf("exit %d, want %d", got, c.wantExit)
			}
		})
	}
}
