package rich

import (
	"strings"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// FromFiles converts a message's uploads: images become Image blocks
// with their thumbnails, everything else a File chip.
func FromFiles(files []slack.File) []Block {
	var out []Block
	for _, f := range files {
		name := f.Title
		if name == "" {
			name = f.Name
		}
		open := f.Permalink
		if open == "" {
			open = f.URLPrivate
		}
		if strings.HasPrefix(f.Mimetype, "image/") {
			out = append(out, Image{
				URL:      f.URLPrivate,
				FileName: f.Name,
				Title:    name,
				Alt:      name,
				W:        f.OriginalW,
				H:        f.OriginalH,
				FileID:   f.ID,
				Thumbs:   blockkit.FileThumbs(f),
				Open:     open,
			})
			continue
		}
		url := f.URLPrivateDownload
		if url == "" {
			url = f.URLPrivate
		}
		out = append(out, File{Name: name, FileName: f.Name, Mime: f.Mimetype, Size: int64(f.Size), FileID: f.ID, URL: url, Open: open})
	}
	return out
}
