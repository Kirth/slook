package search

import (
	"strings"
	"unicode"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/gui/sidebar"
	"github.com/gammons/slk/internal/searchq"
	"github.com/gammons/slk/internal/text"
)

// Suggestion is what the search box offers where the caret is: choices,
// and the runes [Start, End) of the query that a choice's Insert
// replaces.
type Suggestion struct {
	Choices    []chat.Choice
	Start, End int
}

// keyDetail says what each filter is for, in the popup.
var keyDetail = map[string]string{
	"from": "who wrote it", "in": "a conversation, or @someone for your DM", "mentions": "a person, a usergroup or me",
	"is": "dm, channel, thread, reply, edited", "has": "link, file, reaction or an :emoji:",
	"on": "a day, week, month or year", "during": "a day, week, month or year",
	"after": "after a day, month…", "before": "before a day, month…", "since": "from a day, or 3d, 2w, 6m",
}

// Suggest is what to offer with the caret (a rune offset) in query: a
// filter's name while a word starting like one is typed (two letters
// or more), else values for the filter being typed: people, usergroups,
// conversations, is:/has: values, emoji, times. Only at the end of a
// clause; nothing once the value is exactly a choice.
func Suggest(query string, caret int, src Sources, limit int) Suggestion {
	c, ok := searchq.Parse(query).ClauseAt(caret)
	if !ok || caret != c.Span[1] {
		return Suggestion{}
	}
	start := c.ValueAt
	if c.Quoted {
		start-- // replace the opening quote with the choice's own
	}
	var out []chat.Choice
	v := c.Value
	switch c.Kind {
	case searchq.Word:
		if len([]rune(v)) < 2 || strings.Contains(v, ":") {
			return Suggestion{}
		}
		for _, k := range searchq.Keys {
			if strings.HasPrefix(k, strings.ToLower(v)) {
				out = append(out, chat.Choice{Label: k + ":", Detail: keyDetail[k], Insert: k + ":"})
			}
		}
	case searchq.From, searchq.Mentions:
		if strings.HasPrefix("me", strings.ToLower(strings.TrimPrefix(v, "@"))) {
			out = append(out, chat.Choice{Label: "me", Detail: "you", Insert: "me"})
		}
		var groups []backend.UserGroup
		if c.Kind == searchq.Mentions {
			groups = src.Groups
		}
		for _, ch := range chat.MatchPeople(strings.TrimPrefix(v, "@"), src.People, groups, limit) {
			switch {
			case ch.ID != "":
				ch.Insert = quote(ch.Label)
			case strings.HasPrefix(ch.Wire, "<!subteam^"):
				// A usergroup: inserted as its @handle.
			default:
				continue // @here and the like
			}
			out = append(out, ch)
		}
	case searchq.In:
		for _, cv := range sidebar.Search(src.Convs, v) {
			ch := chat.Choice{Label: cv.Name, Kind: cv.Kind}
			switch cv.Kind {
			case backend.KindChannel, backend.KindPrivate:
				ch.Insert = "#" + cv.Name
			case backend.KindDM:
				ch.Insert, ch.Detail, ch.ID, ch.Avatar = quote("@"+cv.Name), "direct message", cv.PeerID, cv.AvatarURL
			default:
				ch.Insert, ch.Detail = quote(cv.Name), "group message"
			}
			out = append(out, ch)
		}
	case searchq.Is:
		out = fixed(searchq.IsValues, v)
	case searchq.Has:
		if strings.HasPrefix(v, ":") {
			out = chat.EmojiChoices(strings.Trim(v, ":"), src.Emoji, limit)
		} else {
			out = fixed(searchq.HasValues, v)
		}
	case searchq.On, searchq.After, searchq.Before, searchq.Since:
		if v != "" && strings.IndexFunc(v, func(r rune) bool { return !unicode.IsDigit(r) }) < 0 {
			for _, u := range []struct{ suffix, detail string }{{"h", "hours"}, {"d", "days"}, {"w", "weeks"}, {"m", "months"}, {"y", "years"}} {
				out = append(out, chat.Choice{Label: v + u.suffix, Detail: v + " " + u.detail + " back", Insert: v + u.suffix})
			}
			break
		}
		out = fixed(append(append([]string{}, searchq.TimeWords...), "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"), v)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	for _, ch := range out {
		if ch.Insert == v || (c.Quoted && ch.Insert == quote(v)) {
			return Suggestion{} // already complete
		}
	}
	return Suggestion{Choices: out, Start: start, End: c.Span[1]}
}

// fixed is the values starting with v, folded.
func fixed(values []string, v string) []chat.Choice {
	var out []chat.Choice
	f := text.Fold(v)
	for _, s := range values {
		if strings.HasPrefix(s, f) {
			out = append(out, chat.Choice{Label: s, Insert: s})
		}
	}
	return out
}

// quote puts a value with spaces in quotes, so it stays one clause.
func quote(s string) string {
	if strings.ContainsFunc(s, unicode.IsSpace) {
		return `"` + s + `"`
	}
	return s
}
