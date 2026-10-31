package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gammons/slk/internal/core"
)

// uploadTimeout bounds one file's upload (Slack's three steps:
// getUploadURLExternal, the PUT, completeUploadExternal).
const uploadTimeout = 60 * time.Second

// Upload posts the files one at a time, the caption on the last, and
// then brings the conversation (or thread) up to Slack's so the
// messages show even if the socket's echo is late. The first failure
// stops it; files before it are already posted.
func (s *Slack) Upload(ctx context.Context, channelID, threadTS string, files []core.PendingAttachment, caption string, progress func(done, total int)) error {
	for i, f := range files {
		if progress != nil {
			progress(i+1, len(files))
		}
		c := ""
		if i == len(files)-1 {
			c = caption
		}
		if err := s.uploadOne(ctx, channelID, threadTS, f, c); err != nil {
			return fmt.Errorf("uploading %s (%d/%d): %w", f.Filename, i+1, len(files), err)
		}
	}
	if s.sync != nil {
		if threadTS != "" {
			go s.fetchThread(ctx, channelID, threadTS)
		} else {
			go s.catchUp(ctx, channelID)
		}
	}
	return nil
}

// uploadOne sends one file: a pasted image's bytes, or the file at its
// path, read now (it may have changed since it was attached).
func (s *Slack) uploadOne(ctx context.Context, channelID, threadTS string, f core.PendingAttachment, caption string) error {
	var r io.Reader
	size := f.Size
	if f.Bytes != nil {
		r, size = bytes.NewReader(f.Bytes), int64(len(f.Bytes))
	} else {
		file, err := os.Open(f.Path)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		r, size = file, info.Size()
	}
	ctx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()
	_, err := s.client.UploadFile(ctx, channelID, threadTS, f.Filename, r, size, caption)
	return err
}
