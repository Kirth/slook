package chat

import (
	"slices"
	"strconv"

	"github.com/gammons/slk/gui/backend"
)

// MaxRecipients is how many people a new message can go to: a DM with
// one, a group DM with up to eight (Slack's limit).
const MaxRecipients = backend.MaxRecipients

// Recipients is who a new message goes to, in the order chosen, at most
// MaxRecipients.
type Recipients struct {
	People []backend.Person
}

// Toggle adds p, or takes them out if chosen already. added is false
// when p was taken out or the list is full.
func (r *Recipients) Toggle(p backend.Person) (added bool) {
	if i := slices.IndexFunc(r.People, func(q backend.Person) bool { return q.ID == p.ID }); i >= 0 {
		r.People = slices.Delete(r.People, i, i+1)
		return false
	}
	if r.Full() {
		return false
	}
	r.People = append(r.People, p)
	return true
}

// Has reports whether the person with id is chosen.
func (r *Recipients) Has(id string) bool {
	return slices.ContainsFunc(r.People, func(p backend.Person) bool { return p.ID == id })
}

// Pop takes out the last one chosen (Backspace in an empty filter).
func (r *Recipients) Pop() {
	if n := len(r.People); n > 0 {
		r.People = r.People[:n-1]
	}
}

// Full is whether no one more can be added.
func (r *Recipients) Full() bool { return len(r.People) >= MaxRecipients }

// IDs is the chosen people's IDs.
func (r *Recipients) IDs() []string {
	ids := make([]string, len(r.People))
	for i, p := range r.People {
		ids[i] = p.ID
	}
	return ids
}

// Counter is how many are chosen of how many may be: "2 / 8".
func (r *Recipients) Counter() string {
	return strconv.Itoa(len(r.People)) + " / " + strconv.Itoa(MaxRecipients)
}

// PeopleChoices is who a new message can go to for query: MatchPeople's
// people, without the broadcasts (@here…), usergroups and apps, at most
// limit.
func PeopleChoices(query string, people []backend.Person, limit int) []Choice {
	humans := make([]backend.Person, 0, len(people))
	for _, p := range people {
		if !p.Bot {
			humans = append(humans, p)
		}
	}
	all := MatchPeople(query, humans, nil, limit+len(specials))
	out := make([]Choice, 0, limit)
	for _, c := range all {
		if c.ID != "" && len(out) < limit {
			out = append(out, c)
		}
	}
	return out
}
