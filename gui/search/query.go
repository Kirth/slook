package search

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/internal/emoji"
	"github.com/gammons/slk/internal/searchq"
)

// Sources are what a query's names resolve against: all already held,
// none asked for.
type Sources struct {
	People []backend.Person
	Groups []backend.UserGroup
	Convs  []backend.Conversation
	Self   string            // the user's own ID, for from:me and mentions:me
	Emoji  map[string]string // the workspace's custom emoji, for has::name:
}

// Chip is how one clause was read, for the line under the search box:
// "from Ada Lovelace", or what is wrong with it (Err).
type Chip struct {
	Text string
	Err  bool
	Span [2]int // the clause's runes in the query
}

// Resolve turns a parsed query into what the backend searches for,
// matching names fuzzily (the best match wins and its chip names it),
// and says how it read each clause. Plain words and phrases get no chip;
// a filter still being typed (from: with nothing after it) is left out.
// A name that matches nothing makes the query find nothing.
func Resolve(q searchq.Query, src Sources, now time.Time) (backend.SearchQuery, []Chip) {
	var out backend.SearchQuery
	var chips []Chip
	var inSet, kindSet, notKind []string // in: (any of), is:dm/channel, !is:dm/channel
	haveIn, haveKind := false, false
	chip := func(c searchq.Clause, text string) { chips = append(chips, Chip{Text: text, Span: c.Span}) }
	fail := func(c searchq.Clause, text string) {
		chips = append(chips, Chip{Text: text, Err: true, Span: c.Span})
		if !c.Neg {
			out.Nothing = true
		}
	}
	not := func(c searchq.Clause, s string) string {
		if c.Neg {
			return "not " + s
		}
		return s
	}
	for _, c := range q.Clauses {
		v := strings.TrimSpace(c.Value)
		if v == "" && c.Kind != searchq.Or {
			continue
		}
		switch c.Kind {
		case searchq.Word, searchq.Phrase:
			if c.Neg {
				out.None = append(out.None, v)
				chip(c, "without “"+v+"”")
			} else {
				out.All = append(out.All, []string{v})
			}
		case searchq.Or:
			var alts, quoted []string
			for _, a := range c.Alts {
				if a := strings.TrimSpace(a.Value); a != "" {
					alts = append(alts, a)
					quoted = append(quoted, "“"+a+"”")
				}
			}
			if len(alts) > 0 {
				out.All = append(out.All, alts)
				chip(c, strings.Join(quoted, " or "))
			}
		case searchq.From:
			p, ok := person(v, src)
			if !ok {
				fail(c, "no one matches “"+v+"”")
				continue
			}
			if c.Neg {
				out.NotUsers = append(out.NotUsers, p.ID)
			} else {
				out.Users = append(out.Users, p.ID)
			}
			chip(c, not(c, "from "+p.Name))
		case searchq.In:
			conv, label, ok := conversation(v, src)
			if !ok {
				fail(c, "no conversation matches “"+v+"”")
				continue
			}
			if c.Neg {
				out.NotChannels = append(out.NotChannels, conv.ID)
			} else {
				inSet, haveIn = append(inSet, conv.ID), true
			}
			chip(c, not(c, "in "+label))
		case searchq.Mentions:
			wire, name, ok := mention(v, src)
			if !ok {
				fail(c, "no one matches “"+v+"”")
				continue
			}
			if c.Neg {
				out.NotMentions = append(out.NotMentions, wire)
				chip(c, "not mentioning "+name)
			} else {
				out.Mentions = append(out.Mentions, wire)
				chip(c, "mentions "+name)
			}
		case searchq.Is:
			tri := backend.Yes
			if c.Neg {
				tri = backend.No
			}
			switch strings.ToLower(v) {
			case "dm", "dms":
				ids := convsOf(src.Convs, backend.KindDM, backend.KindGroupDM)
				if c.Neg {
					notKind = append(notKind, ids...)
				} else {
					kindSet, haveKind = intersectOr(kindSet, ids, haveKind), true
				}
				chip(c, not(c, "in direct messages"))
			case "channel", "channels":
				ids := convsOf(src.Convs, backend.KindChannel, backend.KindPrivate)
				if c.Neg {
					notKind = append(notKind, ids...)
				} else {
					kindSet, haveKind = intersectOr(kindSet, ids, haveKind), true
				}
				chip(c, not(c, "in channels"))
			case "thread":
				out.Thread = tri
				chip(c, not(c, "a thread"))
			case "reply":
				out.Reply = tri
				chip(c, not(c, "a reply"))
			case "edited":
				out.Edited = tri
				chip(c, not(c, "edited"))
			default:
				fail(c, "is: takes "+strings.Join(searchq.IsValues, ", "))
			}
		case searchq.Has:
			tri := backend.Yes
			if c.Neg {
				tri = backend.No
			}
			lv := strings.ToLower(v)
			switch {
			case lv == "link" || lv == "links":
				out.Link = tri
				chip(c, hasChip(c, "a link"))
			case lv == "file" || lv == "files":
				out.File = tri
				chip(c, hasChip(c, "a file"))
			case lv == "reaction" || lv == "reactions":
				out.Reaction = tri
				chip(c, hasChip(c, "a reaction"))
			case len(lv) > 2 && strings.HasPrefix(lv, ":") && strings.HasSuffix(lv, ":"):
				name := emoji.CanonicalSlackName(strings.Trim(lv, ":"), src.Emoji)
				if c.Neg {
					out.NotEmoji = append(out.NotEmoji, name)
				} else {
					out.Emoji = append(out.Emoji, name)
				}
				chip(c, hasChip(c, ":"+name+":"))
			default:
				fail(c, "has: takes "+strings.Join(searchq.HasValues, ", ")+" or an :emoji:")
			}
		case searchq.On, searchq.After, searchq.Before, searchq.Since:
			sp, err := searchq.ParseTime(v, now)
			if err != nil {
				fail(c, "“"+v+"” is not a time")
				continue
			}
			if c.Neg {
				fail(c, "a time cannot be negated: use before: or after:")
				continue
			}
			from, until := time.Time{}, time.Time{}
			switch c.Kind {
			case searchq.On:
				from, until = sp.Start, sp.End
			case searchq.After:
				from = sp.End
				if sp.Relative {
					from = sp.Start
				}
			case searchq.Before:
				until = sp.Start
			case searchq.Since:
				from = sp.Start
			}
			if !from.IsZero() && from.After(out.Since) {
				out.Since = from
			}
			if !until.IsZero() && (out.Until.IsZero() || until.Before(out.Until)) {
				out.Until = until
			}
			chip(c, timeChip(c.Kind, sp, now))
		}
	}
	switch {
	case haveIn && haveKind:
		out.Channels = intersectOr(inSet, kindSet, true)
		if len(out.Channels) == 0 {
			out.Nothing = true
		}
	case haveIn:
		out.Channels = inSet
	case haveKind:
		out.Channels = kindSet
		if len(kindSet) == 0 {
			out.Nothing = true
		}
	}
	out.NotChannels = append(out.NotChannels, notKind...)
	if !out.Since.IsZero() && !out.Until.IsZero() && !out.Since.Before(out.Until) {
		out.Nothing = true
	}
	return out, chips
}

// person is who name means: me, else the best fuzzy match.
func person(name string, src Sources) (backend.Person, bool) {
	if strings.EqualFold(strings.TrimPrefix(name, "@"), "me") && src.Self != "" {
		if i := slices.IndexFunc(src.People, func(p backend.Person) bool { return p.ID == src.Self }); i >= 0 {
			return src.People[i], true
		}
		return backend.Person{ID: src.Self, Name: "you"}, true
	}
	return chat.BestPerson(name, src.People)
}

// conversation is the conversation in:name means, and how to name it:
// @someone is the direct message with them, anything else the best
// fuzzy match among the conversations.
func conversation(name string, src Sources) (backend.Conversation, string, bool) {
	if strings.HasPrefix(name, "@") {
		p, ok := person(name, src)
		if !ok {
			return backend.Conversation{}, "", false
		}
		i := slices.IndexFunc(src.Convs, func(c backend.Conversation) bool { return c.Kind == backend.KindDM && c.PeerID == p.ID })
		if i < 0 {
			return backend.Conversation{}, "", false
		}
		return src.Convs[i], "your DM with " + p.Name, true
	}
	c, ok := chat.BestConversation(name, src.Convs)
	if !ok {
		return backend.Conversation{}, "", false
	}
	switch c.Kind {
	case backend.KindChannel, backend.KindPrivate:
		return c, "#" + c.Name, true
	case backend.KindDM:
		return c, "your DM with " + c.Name, true
	}
	return c, c.Name, true
}

// mention is the markup a mention of name carries and how to name it:
// me, a usergroup by its handle, else a person.
func mention(name string, src Sources) (wire, label string, ok bool) {
	if p, ok := person(name, src); ok && strings.EqualFold(strings.TrimPrefix(name, "@"), "me") {
		return "<@" + p.ID + ">", "you", true
	}
	q := strings.ToLower(strings.TrimPrefix(name, "@"))
	if i := slices.IndexFunc(src.Groups, func(g backend.UserGroup) bool { return strings.ToLower(g.Handle) == q }); i >= 0 {
		return "<!subteam^" + src.Groups[i].ID, "@" + src.Groups[i].Handle, true
	}
	if p, ok := chat.BestPerson(name, src.People); ok {
		return "<@" + p.ID + ">", p.Name, true
	}
	if g, ok := chat.BestGroup(name, src.Groups); ok {
		return "<!subteam^" + g.ID, "@" + g.Handle, true
	}
	return "", "", false
}

// convsOf is the IDs of the conversations of the given kinds.
func convsOf(convs []backend.Conversation, kinds ...backend.Kind) []string {
	var ids []string
	for _, c := range convs {
		if slices.Contains(kinds, c.Kind) {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// intersectOr is set ∩ ids when have, else ids: successive "any of"
// lists that must all hold.
func intersectOr(set, ids []string, have bool) []string {
	if !have {
		return slices.Clone(ids)
	}
	var out []string
	for _, id := range set {
		if slices.Contains(ids, id) {
			out = append(out, id)
		}
	}
	return out
}

func hasChip(c searchq.Clause, what string) string {
	if c.Neg {
		return "without " + what
	}
	return "with " + what
}

// timeChip words a time clause: "on Mon 26 Oct", "during October 2026",
// "after 2025", "in the last 3 days".
func timeChip(k searchq.Kind, sp searchq.Span, now time.Time) string {
	if sp.Relative {
		ago := relativeWords(sp.Start, now)
		if k == searchq.Before {
			return "before " + ago + " ago"
		}
		return "in the last " + ago
	}
	what := spanWords(sp, now)
	switch k {
	case searchq.After:
		return "after " + what
	case searchq.Before:
		return "before " + what
	case searchq.Since:
		return "since " + what
	}
	if sp.End.Sub(sp.Start) <= 25*time.Hour {
		return "on " + what
	}
	return "during " + what
}

// spanWords names a span: a day, a week, a month, a year, or its ends.
func spanWords(sp searchq.Span, now time.Time) string {
	day := func(t time.Time) string {
		if t.Year() == now.Year() {
			return t.Format("Mon 2 Jan")
		}
		return t.Format("Mon 2 Jan 2006")
	}
	switch {
	case sp.End.Equal(sp.Start.AddDate(0, 0, 1)):
		return day(sp.Start)
	case sp.End.Equal(sp.Start.AddDate(0, 0, 7)) && sp.Start.Weekday() == time.Monday:
		return "the week of " + day(sp.Start)
	case sp.Start.Day() == 1 && sp.End.Equal(sp.Start.AddDate(0, 1, 0)):
		return sp.Start.Format("January 2006")
	case sp.Start.YearDay() == 1 && sp.End.Equal(sp.Start.AddDate(1, 0, 0)):
		return sp.Start.Format("2006")
	}
	return day(sp.Start) + " – " + day(sp.End.AddDate(0, 0, -1))
}

// relativeWords is how long ago from was: "4 hours", "3 days".
func relativeWords(from, now time.Time) string {
	d := now.Sub(from)
	plural := func(n int, unit string) string {
		if n == 1 {
			return "1 " + unit
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}
	switch {
	case d < 48*time.Hour && d%(24*time.Hour) != 0:
		return plural(int(d.Round(time.Hour)/time.Hour), "hour")
	case d < 14*24*time.Hour:
		return plural(int(d.Round(24*time.Hour)/(24*time.Hour)), "day")
	case d < 60*24*time.Hour:
		return plural(int(d.Round(24*time.Hour)/(7*24*time.Hour)), "week")
	case d < 365*24*time.Hour:
		return plural(int(d.Round(24*time.Hour)/(30*24*time.Hour)), "month")
	}
	return plural(int(d.Round(24*time.Hour)/(365*24*time.Hour)), "year")
}
