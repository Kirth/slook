package blockkit

// RendersBody reports whether blocks render content in place of the
// message text fallback. RichTextBlock renders through the host body
// row, UnknownBlock renders only a compatibility marker, and a lone
// DividerBlock does not carry the author's body; none suppress text.
// Empty blocks likewise preserve the fallback rather than blanking the
// message.
//
// Keep this switch aligned with the TUI renderer's appendBlock
// (internal/ui/messages/blockkit/render.go).
func RendersBody(blocks []Block) bool {
	for _, b := range blocks {
		switch v := b.(type) {
		case SectionBlock:
			if v.Text != "" || len(v.Fields) > 0 || v.Accessory != nil {
				return true
			}
		case HeaderBlock:
			if v.Text != "" {
				return true
			}
		case ContextBlock:
			if len(v.Elements) > 0 {
				return true
			}
		case ImageBlock:
			if v.URL != "" {
				return true
			}
		case ActionsBlock:
			if len(v.Elements) > 0 {
				return true
			}
		}
	}
	return false
}
