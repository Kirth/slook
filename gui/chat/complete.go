package chat

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/sidebar"
	itext "github.com/gammons/slk/internal/text"
)

// Trigger is the completion the text before the caret asks for: its
// sigil ('@' a person, '#' a channel, ':' an emoji), the query typed
// after it, and the rune offset where the token (sigil included)
// starts. The sigil must open a word: at the start, or after whitespace
// or an opening bracket. A channel or emoji query is one word; a
// person's may hold one space ("@ada lov"), names having them. An emoji
// waits for two characters, so "re: x" or a smiley stays quiet.
func Trigger(s string, caret int) (sigil rune, query string, start int, ok bool) {
	rs := []rune(s)
	if caret < 0 || caret > len(rs) {
		return 0, "", 0, false
	}
	spaces := 0
	for i := caret - 1; i >= 0 && caret-i <= 64; i-- {
		r := rs[i]
		switch {
		case r == '\n':
			return 0, "", 0, false
		case unicode.IsSpace(r):
			spaces++
			if spaces > 1 {
				return 0, "", 0, false
			}
			continue
		case r != '@' && r != '#' && r != ':':
			continue
		}
		if i > 0 && !opensWord(rs[i-1]) {
			continue // inside a word: "foo@bar", "a:b"
		}
		q := string(rs[i+1 : caret])
		switch r {
		case '#', ':':
			if spaces > 0 {
				return 0, "", 0, false
			}
		case '@':
			if strings.HasPrefix(q, " ") {
				return 0, "", 0, false
			}
		}
		if r == ':' && !emojiQuery(q) {
			return 0, "", 0, false
		}
		return r, q, i, true
	}
	return 0, "", 0, false
}

func opensWord(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune("([{\"'", r) }

// emojiQuery is two or more characters an emoji name can hold.
func emojiQuery(q string) bool {
	if len([]rune(q)) < 2 {
		return false
	}
	for _, r := range q {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_+-", r) {
			return false
		}
	}
	return true
}

// Choice is one completion: what the popup shows, what the composer
// inserts, and what that becomes when the message is sent.
type Choice struct {
	Label  string // "Ada Lovelace", "general", ":tada:"
	Detail string // "@ada.lovelace", "private"; may be empty
	Insert string // "@Ada Lovelace", "#general", ":tada:"
	Wire   string // "<@U1>", "<#C1|general>", "<!here>"; "" when Insert is sent as is
	ID     string // a person's ID, for their avatar
	Avatar string // a person's picture URL
	Glyph  string // an emoji's character; "" for a custom emoji
	Emoji  string // an emoji's name, for a custom one's picture
	Kind   backend.Kind
}

// specials are the @-mentions that notify a group, not a person.
var specials = []struct{ name, detail string }{
	{"here", "Notify everyone active in this conversation"},
	{"channel", "Notify everyone in this conversation"},
	{"everyone", "Notify everyone in the workspace"},
}

// MatchPeople is what "@query" completes to: the broadcasts (@here…)
// whose name starts with it, the usergroups whose handle or name
// starts with it, then people by how well a name matches (a display
// name, real name or handle starting with it, then any word of them,
// then anywhere), apps last, then usergroups matching further in;
// case- and accent-blind. An empty query lists everyone, apps last,
// then the groups.
func MatchPeople(query string, people []backend.Person, groups []backend.UserGroup, limit int) []Choice {
	q := itext.Fold(strings.TrimSpace(query))
	var out []Choice
	for _, s := range specials {
		if q != "" && strings.HasPrefix(s.name, q) {
			out = append(out, Choice{Label: "@" + s.name, Detail: s.detail, Insert: "@" + s.name, Wire: "<!" + s.name + ">"})
		}
	}
	var laterGroups []Choice
	for _, g := range groups {
		if g.Handle == "" {
			continue
		}
		c := groupChoice(g)
		switch rank := groupRank(q, g); {
		case rank == 0 && q != "":
			out = append(out, c)
		case rank >= 0:
			laterGroups = append(laterGroups, c)
		}
	}
	type ranked struct {
		p    backend.Person
		rank int
	}
	var rs []ranked
	seen := map[string]bool{}
	for _, p := range people {
		if seen[p.ID] || p.Name == "" {
			continue
		}
		seen[p.ID] = true
		rank := personRank(q, p)
		if rank < 0 {
			continue
		}
		if p.Bot {
			rank += 3
		}
		rs = append(rs, ranked{p, rank})
	}
	slices.SortStableFunc(rs, func(a, b ranked) int {
		return cmp.Or(cmp.Compare(a.rank, b.rank), strings.Compare(itext.Fold(a.p.Name), itext.Fold(b.p.Name)))
	})
	for _, r := range rs {
		p := r.p
		detail := ""
		if p.Handle != "" && p.Handle != p.Name {
			detail = "@" + p.Handle
		}
		if p.RealName != "" && p.RealName != p.Name {
			detail = strings.TrimSpace(detail + "  " + p.RealName)
		}
		out = append(out, Choice{Label: p.Name, Detail: detail, Insert: "@" + p.Name, Wire: "<@" + p.ID + ">", ID: p.ID, Avatar: p.AvatarURL})
	}
	out = append(out, laterGroups...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// groupChoice is a usergroup as a completion: it reads "@handle" and
// is sent as Slack's <!subteam^ID|@handle>.
func groupChoice(g backend.UserGroup) Choice {
	detail := g.Name
	n := strconv.Itoa(len(g.Members)) + " members"
	if len(g.Members) == 1 {
		n = "1 member"
	}
	if detail != "" {
		detail += " · " + n
	} else {
		detail = n
	}
	return Choice{Label: "@" + g.Handle, Detail: detail, Insert: "@" + g.Handle,
		Wire: "<!subteam^" + g.ID + "|@" + g.Handle + ">", ID: g.ID}
}

// groupRank is how well q matches a usergroup's handle or name, as
// personRank does a person's.
func groupRank(q string, g backend.UserGroup) int {
	return personRank(q, backend.Person{Name: g.Handle, RealName: g.Name})
}

// personRank is how well q matches p's names: 0 a name starts with it,
// 1 a word of one does, 2 one contains it, -1 no match.
func personRank(q string, p backend.Person) int {
	if q == "" {
		return 0
	}
	names := []string{itext.Fold(p.Name), itext.Fold(p.RealName), itext.Fold(p.Handle)}
	best := -1
	for _, n := range names {
		if n == "" {
			continue
		}
		r := -1
		switch {
		case strings.HasPrefix(n, q):
			r = 0
		case wordPrefix(n, q):
			r = 1
		case strings.Contains(n, q):
			r = 2
		}
		if r >= 0 && (best < 0 || r < best) {
			best = r
		}
	}
	return best
}

// wordPrefix reports whether a word of s (after a space, dot, dash or
// underscore) starts with q.
func wordPrefix(s, q string) bool {
	for i, r := range s {
		if i > 0 && strings.ContainsRune(" ._-", r) && strings.HasPrefix(s[i+1:], q) {
			return true
		}
	}
	return false
}

// MatchChannels is what "#query" completes to: the user's channels
// (public and private, not direct messages) whose name holds it,
// starting with it first.
func MatchChannels(query string, convs []backend.Conversation, limit int) []Choice {
	var chans []backend.Conversation
	for _, c := range convs {
		if c.Kind == backend.KindChannel || c.Kind == backend.KindPrivate {
			chans = append(chans, c)
		}
	}
	var out []Choice
	for _, c := range sidebar.Search(chans, query) {
		ch := Choice{Label: c.Name, Insert: "#" + c.Name, Wire: "<#" + c.ID + "|" + c.Name + ">", Kind: c.Kind}
		if c.Kind == backend.KindPrivate {
			ch.Detail = "private"
		}
		out = append(out, ch)
		if len(out) == limit {
			break
		}
	}
	return out
}

// EmojiChoices is what ":query" completes to, as the reaction picker
// matches (MatchEmoji). An emoji is sent as typed.
func EmojiChoices(query string, customs map[string]string, limit int) []Choice {
	var out []Choice
	for _, e := range MatchEmoji(query, customs, limit) {
		out = append(out, Choice{Label: ":" + e.Name + ":", Insert: ":" + e.Name + ":", Glyph: e.Glyph, Emoji: e.Name})
	}
	return out
}

// Mentions is what the composer inserted and how it is sent: readable
// text ("@Ada Lovelace") to Slack's form ("<@U1>").
type Mentions map[string]string

// Wire is text as sent: each mention the composer inserted becomes its
// wire form where it stands as a whole token (not inside a longer
// word), the longest first, so "@Ada Lovelace" wins over "@Ada". Text
// the user typed that merely looks like a mention is left alone.
func (m Mentions) Wire(s string) string {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if k != "" && v != "" {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	for _, k := range keys {
		var b strings.Builder
		rest := s
		for {
			i := strings.Index(rest, k)
			if i < 0 {
				b.WriteString(rest)
				break
			}
			before := b.String() + rest[:i]
			after := rest[i+len(k):]
			if startsToken(before) && endsToken(after) {
				b.WriteString(rest[:i])
				b.WriteString(m[k])
			} else {
				b.WriteString(rest[:i+len(k)])
			}
			rest = after
		}
		s = b.String()
	}
	return s
}

func startsToken(before string) bool {
	if before == "" {
		return true
	}
	r := []rune(before)
	return opensWord(r[len(r)-1])
}

func endsToken(after string) bool {
	for _, r := range after {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}
	return true
}

var (
	wireUser    = regexp.MustCompile(`<@([UW][A-Z0-9]+)(?:\|[^>]*)?>`)
	wireChannel = regexp.MustCompile(`<#([CG][A-Z0-9]+)(?:\|([^>]*))?>`)
	wireSpecial = regexp.MustCompile(`<!(here|channel|everyone)(?:\|[^>]*)?>`)
	wireGroup   = regexp.MustCompile(`<!subteam\^([A-Z0-9]+)(?:\|@?([^>]*))?>`)
)

// Readable is text with Slack's wire forms shown as people read them,
// for an edit: <@U1> becomes "@Ada Lovelace" when names knows U1,
// <#C1|general> "#general", <!here> "@here", <!subteam^S1|@eng> "@eng"
// (a bare one when names knows S1's handle). Each replacement is
// recorded in m, so sending turns it back. A form it cannot name stays
// as it is (and is sent as it is).
func Readable(s string, names func(id string) (string, bool), m Mentions) string {
	s = wireUser.ReplaceAllStringFunc(s, func(tok string) string {
		id := wireUser.FindStringSubmatch(tok)[1]
		name, ok := names(id)
		if !ok || name == "" {
			return tok
		}
		m["@"+name] = "<@" + id + ">"
		return "@" + name
	})
	s = wireChannel.ReplaceAllStringFunc(s, func(tok string) string {
		sub := wireChannel.FindStringSubmatch(tok)
		label := sub[2]
		if label == "" {
			label, _ = names(sub[1])
		}
		if label == "" {
			return tok
		}
		m["#"+label] = "<#" + sub[1] + "|" + label + ">"
		return "#" + label
	})
	s = wireGroup.ReplaceAllStringFunc(s, func(tok string) string {
		sub := wireGroup.FindStringSubmatch(tok)
		handle := sub[2]
		if handle == "" {
			handle, _ = names(sub[1])
		}
		if handle == "" {
			return tok
		}
		m["@"+handle] = tok
		return "@" + handle
	})
	return wireSpecial.ReplaceAllStringFunc(s, func(tok string) string {
		name := wireSpecial.FindStringSubmatch(tok)[1]
		m["@"+name] = "<!" + name + ">"
		return "@" + name
	})
}
