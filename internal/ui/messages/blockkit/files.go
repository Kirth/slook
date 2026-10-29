package blockkit

import (
	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/core"
)

// FileThumbs lists a slack.File's thumbnails from its thumb_* fields,
// smallest first (the order Slack returns them). Tiers with an empty
// URL or non-positive dimensions are skipped. Both frontends build
// their image attachments from it.
func FileThumbs(f slack.File) []core.ThumbSpec {
	var out []core.ThumbSpec
	add := func(url string, w, h int) {
		if url != "" && w > 0 && h > 0 {
			out = append(out, core.ThumbSpec{URL: url, W: w, H: h})
		}
	}
	add(f.Thumb360, f.Thumb360W, f.Thumb360H)
	add(f.Thumb480, f.Thumb480W, f.Thumb480H)
	add(f.Thumb720, f.Thumb720W, f.Thumb720H)
	add(f.Thumb960, f.Thumb960W, f.Thumb960H)
	add(f.Thumb1024, f.Thumb1024W, f.Thumb1024H)
	return out
}
