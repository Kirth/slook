// Package attach turns what the user offers to send — a file path, a
// pasted image — into a core.PendingAttachment, with the rules for
// which pasted text is a path and the size limit every frontend shows.
package attach

import (
	"errors"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gammons/slk/internal/core"
)

// MaxSize is the largest file slk uploads, 10 MB.
const MaxSize = 10 << 20

// ErrNotAFile is FromPath's answer for a path that names nothing, or
// something that is not a regular file (a directory): such pasted text
// is text, not an attachment.
var ErrNotAFile = errors.New("not a file")

// ErrEmpty is FromPath's answer for an empty file, which Slack refuses.
var ErrEmpty = errors.New("empty file")

// ResolvePath reads pasted text as a file path: one line, absolute,
// ~/-relative or ./-relative, cleaned (~ expanded). ok is false for
// anything else; whether the file exists is FromPath's question.
func ResolvePath(text string) (string, bool) {
	s := strings.TrimSpace(text)
	if s == "" || strings.ContainsAny(s, "\n\r") || len(s) > 4096 {
		return "", false
	}
	// A file manager's "Copy" is a file:// URL.
	if rest, ok := strings.CutPrefix(s, "file://"); ok {
		s = rest
		if p, err := url.PathUnescape(s); err == nil {
			s = p
		}
	}
	if strings.HasPrefix(s, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		s = filepath.Join(home, s[2:])
	}
	if !filepath.IsAbs(s) && !strings.HasPrefix(s, "./") {
		return "", false
	}
	return filepath.Clean(s), true
}

// HumanSize is a file size for people: "<1 KB", "243 KB", "12.3 MB".
func HumanSize(size int64) string {
	const kb = 1024
	const mb = 1024 * kb
	switch {
	case size >= mb:
		return fmt.Sprintf("%.1f MB", float64(size)/float64(mb))
	case size >= kb:
		return fmt.Sprintf("%d KB", size/kb)
	default:
		return "<1 KB"
	}
}

// FromPath is the regular file at path as an attachment, read when it
// is uploaded, not now. A path that names no regular file is
// ErrNotAFile, an empty file ErrEmpty; the size limit is the caller's
// to apply, with the other attachments in view.
func FromPath(path string) (core.PendingAttachment, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return core.PendingAttachment{}, fmt.Errorf("%s: %w", path, ErrNotAFile)
	}
	if info.Size() == 0 {
		return core.PendingAttachment{}, ErrEmpty
	}
	return core.PendingAttachment{
		Filename: filepath.Base(path),
		Path:     path,
		Mime:     mime.TypeByExtension(filepath.Ext(path)),
		Size:     info.Size(),
	}, nil
}

// FromImage is a pasted PNG as an attachment named for when it was
// pasted: slk-paste-2006-01-02-15-04-05.png.
func FromImage(png []byte, now time.Time) core.PendingAttachment {
	return core.PendingAttachment{
		Filename: "slk-paste-" + now.Format("2006-01-02-15-04-05") + ".png",
		Bytes:    png,
		Mime:     "image/png",
		Size:     int64(len(png)),
	}
}
