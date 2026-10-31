package blockkit

import (
	"github.com/slack-go/slack"

	bk "github.com/gammons/slk/internal/blockkit"
	"github.com/gammons/slk/internal/core"
)

// The parse half of this package moved to internal/blockkit; these
// delegate to it so the renderer's callers are unchanged.

func Parse(in slack.Blocks) []Block                             { return bk.Parse(in) }
func ParseAttachments(in []slack.Attachment) []LegacyAttachment { return bk.ParseAttachments(in) }
func RichTextToMrkdwn(rt RichTextBlock) string                  { return bk.RichTextToMrkdwn(rt) }
func FileThumbs(f slack.File) []core.ThumbSpec                  { return bk.FileThumbs(f) }
func RendersBody(blocks []Block) bool                           { return bk.RendersBody(blocks) }
