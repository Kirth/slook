// Package clipboard reads the desktop clipboard by shelling out to the
// session's clipboard tool: wl-paste on Wayland, xclip on X11.
//
// It exists because a GUI toolkit's own clipboard is not enough: Gio's
// clipboard.ReadCmd is text-only on every platform, so a pasted
// screenshot never reaches the app through it. Elsewhere than Linux, and
// without either tool, every read finds nothing.
package clipboard

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"
)

// readTimeout bounds one read: a clipboard owner that never answers
// must not hang a paste.
const readTimeout = 3 * time.Second

// The process's environment, swapped by tests.
var (
	getenv   = os.Getenv
	lookPath = exec.LookPath
	run      = runCmd
)

// runCmd runs name with args and returns what it printed.
func runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// pngMagic starts every PNG.
var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// tool is the command that reads the clipboard here and the arguments
// for one MIME type ("" for the clipboard's text).
type tool struct {
	name string
	args func(mime string) []string
}

var (
	wlPaste = tool{"wl-paste", func(mime string) []string {
		if mime == "" {
			// "text" is wl-paste's own pick of whichever textual
			// type the clipboard offers.
			return []string{"--no-newline", "--type", "text"}
		}
		return []string{"--no-newline", "--type", mime}
	}}
	xclip = tool{"xclip", func(mime string) []string {
		if mime == "" {
			return []string{"-selection", "clipboard", "-o"}
		}
		return []string{"-selection", "clipboard", "-t", mime, "-o"}
	}}
)

// session picks the tool for this session: wl-paste when
// WAYLAND_DISPLAY is set (DISPLAY may be set too, for XWayland, so it
// alone does not mean X11), else xclip when DISPLAY is. ok is false
// when neither applies or the tool is not installed.
func session() (tool, bool) {
	switch {
	case getenv("WAYLAND_DISPLAY") != "":
		if _, err := lookPath(wlPaste.name); err == nil {
			return wlPaste, true
		}
	case getenv("DISPLAY") != "":
		if _, err := lookPath(xclip.name); err == nil {
			return xclip, true
		}
	}
	return tool{}, false
}

// read runs the session's tool for mime. Nothing on the clipboard of
// that type is a failed command, which is nil here.
func read(mime string) []byte {
	t, ok := session()
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	out, err := run(ctx, t.name, t.args(mime)...)
	if err != nil {
		return nil
	}
	return out
}

// ReadImage returns the PNG on the clipboard (a screenshot, a copied
// image). ok is false when there is none, including when the clipboard
// holds only text: then no image type is offered and the tool fails.
// It runs a process, so call it off the UI goroutine.
func ReadImage() ([]byte, bool) {
	b := read("image/png")
	if !bytes.HasPrefix(b, pngMagic) {
		return nil, false
	}
	return b, true
}

// ReadText returns the clipboard's text, "" when there is none or no
// tool to read it. It runs a process, so call it off the UI goroutine.
func ReadText() string {
	return string(read(""))
}
