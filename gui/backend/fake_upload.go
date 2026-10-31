package backend

import (
	"context"
	"strings"

	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/core"
)

// Upload posts one message from the user with the caption and the
// files: images as pictures (the fake's made-up ones), the rest as
// file chips. It reports each file as it starts, as Slack's does.
func (f *Fake) Upload(ctx context.Context, channelID, threadTS string, files []core.PendingAttachment, caption string, progress func(done, total int)) error {
	for i := range files {
		if progress != nil {
			progress(i+1, len(files))
		}
	}
	f.mu.Lock()
	var m Message
	if threadTS != "" {
		m = f.replyLocked(channelID, threadTS, "You", caption, false)
	} else {
		m = f.appendLocked(channelID, "You", caption)
	}
	for i, a := range files {
		id := "FAKEUP" + m.TS + "-" + string(rune('a'+i))
		if strings.HasPrefix(a.Mime, "image/") {
			m.Body = append(m.Body, rich.Image{FileID: id, URL: "fake://" + a.Filename, FileName: a.Filename,
				Title: a.Filename, Alt: a.Filename, W: 640, H: 400})
			continue
		}
		m.Body = append(m.Body, rich.File{FileID: id, URL: "fake://" + a.Filename, Name: a.Filename,
			FileName: a.Filename, Mime: a.Mime, Size: a.Size})
	}
	// The log holds the message the files were added to.
	log := f.logs[channelID]
	log[len(log)-1] = m
	f.mu.Unlock()
	select {
	case f.events <- MessagePosted{Message: m}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
