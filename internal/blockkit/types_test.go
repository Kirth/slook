package blockkit

import "testing"

func TestBlockTypesImplementInterface(t *testing.T) {
	var blocks []Block = []Block{
		SectionBlock{},
		HeaderBlock{},
		ContextBlock{},
		DividerBlock{},
		ImageBlock{},
		ActionsBlock{},
		UnknownBlock{Type: "video"},
	}
	if got := blockType(blocks[6]); got != "video" {
		t.Errorf("UnknownBlock.blockType() = %q, want %q", got, "video")
	}
}
