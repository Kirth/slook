package chat

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gammons/slk/internal/attach"
	"github.com/gammons/slk/internal/core"
)

// Attachments are the files waiting to go with a composer's next
// message, in the order added.
type Attachments struct {
	Items []core.PendingAttachment
}

// Add attaches a, refusing one over attach.MaxSize ("Image too large
// (12.3 MB > 10 MB limit)"). A name already attached gets "-2" (then
// "-3", …) before its extension, so Slack shows them apart.
func (as *Attachments) Add(a core.PendingAttachment) error {
	if a.Size > attach.MaxSize {
		what := "File"
		if a.Bytes != nil {
			what = "Image"
		}
		return fmt.Errorf("%s too large (%s > %d MB limit)", what, attach.HumanSize(a.Size), attach.MaxSize>>20)
	}
	a.Filename = as.freeName(a.Filename)
	as.Items = append(as.Items, a)
	return nil
}

// freeName is name, or name with the first free "-n" suffix.
func (as *Attachments) freeName(name string) string {
	taken := func(n string) bool {
		for _, it := range as.Items {
			if it.Filename == n {
				return true
			}
		}
		return false
	}
	if !taken(name) {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		if n := base + "-" + strconv.Itoa(i) + ext; !taken(n) {
			return n
		}
	}
}

// Remove takes the ith attachment off; an index out of range is
// ignored (a chip clicked as the list changed).
func (as *Attachments) Remove(i int) {
	if i < 0 || i >= len(as.Items) {
		return
	}
	as.Items = append(as.Items[:i:i], as.Items[i+1:]...)
}

// Empty reports whether nothing is attached.
func (as Attachments) Empty() bool { return len(as.Items) == 0 }

// Total is the attachments' size together.
func (as Attachments) Total() int64 {
	var n int64
	for _, a := range as.Items {
		n += a.Size
	}
	return n
}

// UploadNote is the line under the messages while files upload:
// "Uploading 1/2…" as the first of two starts, "Uploading…" before any
// has.
func UploadNote(done, total int) string {
	if done <= 0 || total <= 0 {
		return "Uploading…"
	}
	return fmt.Sprintf("Uploading %d/%d…", done, total)
}
