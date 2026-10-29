package chat

import (
	"cmp"
	"slices"
	"strings"
	"sync"

	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/emoji"
)

// EmojiChoice is one emoji the reaction picker offers: its name as a
// reaction takes it, and its character; Glyph is "" for a workspace's
// custom emoji, which is a picture.
type EmojiChoice struct {
	Name  string
	Glyph string
}

// CommonReactions is what the picker offers before anything is typed.
var CommonReactions = []string{
	"+1", "heart", "joy", "tada", "eyes", "pray",
	"white_check_mark", "fire", "rocket", "raised_hands", "thinking_face", "100",
}

// standardNames is every standard emoji name, without colons, sorted;
// the skin-tone modifiers are left out, as they are not reactions.
var standardNames = sync.OnceValue(func() []string {
	var out []string
	for k := range emoji.CodeMap() {
		name := strings.Trim(k, ":")
		if name == "" || strings.HasPrefix(name, "skin-tone-") {
			continue
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
})

// MatchEmoji is what the reaction picker lists for query, at most limit
// choices: names starting with it first, then names containing it,
// each group shortest first. A character reached by several names
// (+1 and thumbsup) is listed once, under the best-ranked name. customs
// is the workspace's custom emoji, which are matched too. An empty
// query lists CommonReactions.
func MatchEmoji(query string, customs map[string]string, limit int) []EmojiChoice {
	query = strings.ToLower(strings.Trim(strings.TrimSpace(query), ":"))
	if query == "" {
		out := make([]EmojiChoice, 0, len(CommonReactions))
		for _, name := range CommonReactions {
			g, _ := rich.EmojiGlyph(name)
			out = append(out, EmojiChoice{Name: name, Glyph: g})
		}
		return out
	}
	type cand struct {
		EmojiChoice
		rank int // 0 prefix, 1 contains
	}
	var cands []cand
	add := func(name, glyph string) {
		switch {
		case strings.HasPrefix(name, query):
			cands = append(cands, cand{EmojiChoice{name, glyph}, 0})
		case strings.Contains(name, query):
			cands = append(cands, cand{EmojiChoice{name, glyph}, 1})
		}
	}
	codes := emoji.CodeMap()
	for _, name := range standardNames() {
		add(name, codes[":"+name+":"])
	}
	for name := range customs {
		if _, std := codes[":"+name+":"]; !std {
			add(name, "")
		}
	}
	slices.SortFunc(cands, func(a, b cand) int {
		return cmp.Or(cmp.Compare(a.rank, b.rank), cmp.Compare(len(a.Name), len(b.Name)), strings.Compare(a.Name, b.Name))
	})
	seen := map[string]bool{}
	var out []EmojiChoice
	for _, c := range cands {
		if c.Glyph != "" {
			if seen[c.Glyph] {
				continue
			}
			seen[c.Glyph] = true
		}
		out = append(out, c.EmojiChoice)
		if len(out) == limit {
			break
		}
	}
	return out
}
