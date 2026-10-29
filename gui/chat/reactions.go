package chat

import (
	"slices"

	"github.com/gammons/slk/gui/backend"
)

// ApplyReaction returns rs with user's reaction name added (or removed),
// as a ReactionChanged reports it. It is idempotent for a user already
// counted (or already gone): Slack echoing a change the window applied
// already changes nothing. A reaction nobody holds any more is dropped.
// self marks the user's own change, which sets Mine. These are the
// rules msgsync.ApplyReaction keeps the cache by.
func ApplyReaction(rs []backend.Reaction, name, user string, added, self bool) []backend.Reaction {
	i := slices.IndexFunc(rs, func(r backend.Reaction) bool { return r.Name == name })
	if added {
		if i < 0 {
			return append(slices.Clone(rs), backend.Reaction{Name: name, Count: 1, Users: []string{user}, Mine: self})
		}
		r := rs[i]
		if slices.Contains(r.Users, user) {
			return rs
		}
		r.Users = append(slices.Clone(r.Users), user)
		r.Count++
		r.Mine = r.Mine || self
		out := slices.Clone(rs)
		out[i] = r
		return out
	}
	if i < 0 {
		return rs
	}
	r := rs[i]
	users := slices.DeleteFunc(slices.Clone(r.Users), func(u string) bool { return u == user })
	if len(users) == len(r.Users) && r.Count <= len(r.Users) {
		return rs // not among them, and the list is whole: nothing to remove
	}
	r.Users = users
	r.Count--
	if self {
		r.Mine = false
	}
	out := slices.Clone(rs)
	if r.Count <= 0 {
		return slices.Delete(out, i, i+1)
	}
	out[i] = r
	return out
}
