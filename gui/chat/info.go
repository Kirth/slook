package chat

import (
	"slices"
	"strings"
	"time"

	"github.com/gammons/slk/gui/backend"
	itext "github.com/gammons/slk/internal/text"
)

// InfoTab is one tab of a conversation's info panel.
type InfoTab int

const (
	TabAbout InfoTab = iota
	TabMembers
	TabNotify
)

// InfoTabs is which tabs a conversation's info panel has: a channel
// shows what it is about and who is in it; a group DM only who is in
// it; a DM the person. Every conversation can set how it notifies.
func InfoTabs(kind backend.Kind) []InfoTab {
	switch kind {
	case backend.KindDM:
		return []InfoTab{TabAbout, TabNotify}
	case backend.KindGroupDM:
		return []InfoTab{TabMembers, TabNotify}
	default:
		return []InfoTab{TabAbout, TabMembers, TabNotify}
	}
}

// FilterMembers is the members whose name, real name or handle
// contains query (case- and accent-blind; all of them for an empty
// query): people with a name sorted by it, then the ones not yet
// resolved, by ID. An unresolved member matches only an empty query.
func FilterMembers(people []backend.Person, query string) []backend.Person {
	q := itext.Fold(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(query), "@")))
	var named, unnamed []backend.Person
	for _, p := range people {
		if p.Name == "" {
			if q == "" {
				unnamed = append(unnamed, p)
			}
			continue
		}
		if q == "" || strings.Contains(itext.Fold(p.Name), q) || strings.Contains(itext.Fold(p.RealName), q) ||
			strings.Contains(itext.Fold(p.Handle), q) {
			named = append(named, p)
		}
	}
	slices.SortStableFunc(named, func(a, b backend.Person) int {
		return strings.Compare(itext.Fold(a.Name), itext.Fold(b.Name))
	})
	slices.SortStableFunc(unnamed, func(a, b backend.Person) int { return strings.Compare(a.ID, b.ID) })
	return append(named, unnamed...)
}

// CreatedLine says who made a conversation and when, from what is
// known: "Created by Ada on March 14, 2019", or less.
func CreatedLine(name string, at time.Time) string {
	switch {
	case name != "" && !at.IsZero():
		return "Created by " + name + " on " + at.Format("January 2, 2006")
	case name != "":
		return "Created by " + name
	case !at.IsZero():
		return "Created on " + at.Format("January 2, 2006")
	}
	return ""
}
