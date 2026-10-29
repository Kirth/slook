package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/launch"
)

// previewer is the desktop previewer images and files open in. It
// shows FILE[N] (toggling it off if already shown), and its left and
// right keys step through the other FILEs. Without it, a downloaded
// file opens in the desktop's default application instead.
const previewer = "flux-preview"

// previewTimeout bounds downloading a message's attachments.
const previewTimeout = 2 * time.Minute

// previewReq is a click on one of a message's attachments: all of them,
// and which one.
type previewReq struct {
	items []backend.Attachment
	index int
}

// previewDone tells the window an attachment finished downloading, for
// better or worse.
type previewDone struct {
	path string
	name string
	err  error
}

// attachments is a message's images and files as downloads, in the
// order the message draws them (richView counts them the same way).
func attachments(blocks []rich.Block) []backend.Attachment {
	var out []backend.Attachment
	var walk func([]rich.Block)
	image := func(im rich.Image) {
		name := im.FileName
		if name == "" && im.FileID != "" {
			name = im.Title
		}
		out = append(out, backend.Attachment{Key: im.FileID, URL: im.URL, Name: name})
	}
	walk = func(bs []rich.Block) {
		for _, b := range bs {
			switch v := b.(type) {
			case rich.Image:
				image(v)
			case rich.File:
				name := v.FileName
				if name == "" {
					name = v.Name
				}
				out = append(out, backend.Attachment{Key: v.FileID, URL: v.URL, Name: name})
			case rich.Context:
				for _, im := range v.Images {
					image(im)
				}
			case rich.Quote:
				walk(v.Blocks)
			case rich.Card:
				walk(v.Blocks)
			}
		}
	}
	walk(blocks)
	return out
}

// showPreview opens paths[index] in the previewer, or the desktop's
// default application when there is none.
func showPreview(paths []string, index int) error {
	if len(paths) == 0 {
		return fmt.Errorf("nothing downloaded")
	}
	if _, err := exec.LookPath(previewer); err != nil {
		return launch.Open(paths[index])
	}
	args := append([]string{"--index", strconv.Itoa(index)}, paths...)
	out, err := exec.Command(previewer, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v %s", previewer, err, out)
	}
	return nil
}

// startPreviewer starts the previewer hidden, so the first preview
// opens at once. A previewer already running stays as it is.
var startPreviewer = sync.OnceFunc(func() {
	if _, err := exec.LookPath(previewer); err != nil {
		return
	}
	cmd := exec.Command(previewer, "--daemon")
	if err := cmd.Start(); err == nil {
		go cmd.Wait()
	}
})
