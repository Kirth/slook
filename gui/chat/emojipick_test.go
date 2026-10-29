package chat

import (
	"strings"
	"testing"
)

func TestCommonReactionsAreGlyphs(t *testing.T) {
	got := MatchEmoji("", nil, 40)
	if len(got) != len(CommonReactions) {
		t.Fatalf("got %d, want the %d common reactions", len(got), len(CommonReactions))
	}
	for _, c := range got {
		if c.Glyph == "" {
			t.Errorf("%s has no glyph", c.Name)
		}
	}
}

func TestMatchEmojiRanksPrefixesFirstAndListsAGlyphOnce(t *testing.T) {
	got := MatchEmoji("tad", nil, 40)
	if len(got) == 0 || got[0].Name != "tada" {
		t.Fatalf("got %v, want tada first", got)
	}
	thumbs := MatchEmoji("thumbsup", nil, 40)
	if len(thumbs) != 1 || thumbs[0].Glyph != "👍" {
		t.Fatalf("thumbsup: %v", thumbs)
	}
	seen := map[string]string{}
	for _, c := range MatchEmoji("heart", nil, 200) {
		if prev, ok := seen[c.Glyph]; ok && c.Glyph != "" {
			t.Fatalf("%s listed as %s and %s", c.Glyph, prev, c.Name)
		}
		seen[c.Glyph] = c.Name
	}
	for _, c := range MatchEmoji("skin", nil, 200) {
		if strings.HasPrefix(c.Name, "skin-tone-") {
			t.Fatalf("offered the modifier %s", c.Name)
		}
	}
	if got := MatchEmoji("e", nil, 5); len(got) != 5 {
		t.Fatalf("limit: got %d", len(got))
	}
}

func TestMatchEmojiIncludesCustoms(t *testing.T) {
	customs := map[string]string{"partyparrot": "https://emoji/pp.gif", "shipit": "alias:partyparrot"}
	got := MatchEmoji("party", customs, 40)
	if len(got) == 0 || got[0].Name != "partyparrot" || got[0].Glyph != "" {
		t.Fatalf("got %v, want partyparrot first, as a picture", got)
	}
	if got := MatchEmoji(":shipit:", customs, 40); len(got) != 1 || got[0].Name != "shipit" {
		t.Fatalf("got %v", got)
	}
}
