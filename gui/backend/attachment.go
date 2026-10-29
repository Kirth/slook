package backend

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/gammons/slk/internal/filedl"
)

// Attachment is an image or file to download: a Slack upload (Key is
// its file ID) or any image URL (Key empty). Name is what the file is
// called, its extension included.
type Attachment struct {
	Key  string
	URL  string
	Name string
}

// AttachmentPath is where an attachment is kept on disk: the same path
// every time for the same file, so it is downloaded once.
//
//	$XDG_RUNTIME_DIR/slk-gio/files/<key>/<name>
//
// The runtime directory is private to the user and cleared at logout;
// without one, a per-user directory in the system's temp dir is used.
// An attachment known only by URL is keyed by a hash of it. The real
// name and extension are kept so a viewer can tell the file's type.
func AttachmentPath(a Attachment) string {
	key := a.Key
	if key == "" {
		sum := sha1.Sum([]byte(a.URL))
		key = "u-" + hex.EncodeToString(sum[:])[:12]
	}
	name := a.Name
	if name == "" {
		name = "download"
		if u, err := url.Parse(a.URL); err == nil && path.Base(u.Path) != "/" && path.Base(u.Path) != "." {
			name = path.Base(u.Path)
		}
	}
	return filepath.Join(filesDir(), filedl.SafeName(key), filedl.SafeName(name))
}

func filesDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "slk-gio", "files")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("slk-gio-%d", os.Getuid()), "files")
}
