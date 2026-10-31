// Package blockkit parses Slack Block Kit blocks and the legacy
// `attachments` field into the typed values of internal/core/blocks,
// and reads them back: RichTextToMrkdwn reconstructs mrkdwn from a
// rich_text block, MessageText picks a message's body source,
// RendersBody says whether blocks carry the body, FileThumbs lists a
// file's thumbnails. It is parse-only: it draws nothing, so every
// frontend can use it.
package blockkit

import "github.com/gammons/slk/internal/core/blocks"

// The Block Kit data types live in internal/core/blocks so the engine
// can build them without importing a frontend.
type (
	Block            = blocks.Block
	SectionBlock     = blocks.SectionBlock
	HeaderBlock      = blocks.HeaderBlock
	ContextBlock     = blocks.ContextBlock
	ContextElement   = blocks.ContextElement
	DividerBlock     = blocks.DividerBlock
	ImageBlock       = blocks.ImageBlock
	ActionsBlock     = blocks.ActionsBlock
	UnknownBlock     = blocks.UnknownBlock
	RichTextBlock    = blocks.RichTextBlock
	AccessoryElement = blocks.AccessoryElement
	ImageAccessory   = blocks.ImageAccessory
	LabelAccessory   = blocks.LabelAccessory
	ActionElement    = blocks.ActionElement
	LegacyAttachment = blocks.LegacyAttachment
	LegacyField      = blocks.LegacyField
	LegacyAction     = blocks.LegacyAction
	ActionConfirm    = blocks.ActionConfirm
)

func blockType(b Block) string { return blocks.TypeName(b) }
