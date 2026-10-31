package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/draw"
	"image/png"
)

//go:embed slook-32x32.png
var trayIconPNG []byte

// trayIcon is the tray's picture, a PNG as the tray wants it, with every
// pixel made fully opaque or fully transparent. fyne.io/systray (v1.12.2)
// sends each colour's low byte (byte(r), not byte(r>>8)), which is only
// right for opaque pixels: anything else comes out near white or as
// noise.
func trayIcon() []byte {
	src, err := png.Decode(bytes.NewReader(trayIconPNG))
	if err != nil {
		return trayIconPNG
	}
	img := image.NewNRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] >= 0x80 {
			img.Pix[i] = 0xff
		} else {
			img.Pix[i-3], img.Pix[i-2], img.Pix[i-1], img.Pix[i] = 0, 0, 0, 0
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return trayIconPNG
	}
	return b.Bytes()
}
