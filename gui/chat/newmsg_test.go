package chat

import (
	"slices"
	"strconv"
	"testing"

	"github.com/gammons/slk/gui/backend"
)

func TestRecipientsCapAtEight(t *testing.T) {
	var r Recipients
	for i := range MaxRecipients {
		if !r.Toggle(backend.Person{ID: "U" + strconv.Itoa(i)}) {
			t.Fatalf("person %d not added", i)
		}
	}
	if !r.Full() || r.Counter() != "8 / 8" {
		t.Fatalf("full %v, counter %q", r.Full(), r.Counter())
	}
	if r.Toggle(backend.Person{ID: "U99"}) || r.Has("U99") {
		t.Fatal("a ninth person was added")
	}
	// Toggling one chosen takes them out, making room.
	if r.Toggle(backend.Person{ID: "U3"}) || r.Has("U3") || r.Full() {
		t.Fatal("toggling a chosen person did not take them out")
	}
	if r.Counter() != "7 / 8" {
		t.Fatalf("counter %q", r.Counter())
	}
}

func TestRecipientsPop(t *testing.T) {
	var r Recipients
	r.Pop() // nothing to take out
	r.Toggle(backend.Person{ID: "U1"})
	r.Toggle(backend.Person{ID: "U2"})
	r.Pop()
	if got := r.IDs(); !slices.Equal(got, []string{"U1"}) {
		t.Fatalf("after Pop: %v", got)
	}
}

func TestPeopleChoicesLeaveOutSpecialsAndBots(t *testing.T) {
	people := []backend.Person{
		{ID: "U1", Name: "Ada Lovelace"},
		{ID: "U2", Name: "Charlie Chaplin"},
		{ID: "B1", Name: "Channel Bot", Bot: true},
	}
	var got []string
	for _, c := range PeopleChoices("ch", people, 8) {
		got = append(got, c.ID)
	}
	if !slices.Equal(got, []string{"U2"}) {
		t.Fatalf("PeopleChoices(ch) = %v, want only Charlie (no @channel, no app)", got)
	}
	if n := len(PeopleChoices("", people, 1)); n != 1 {
		t.Fatalf("limit 1 gave %d", n)
	}
}
