package chat

import (
	"reflect"
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
)

func TestChips(t *testing.T) {
	noon := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	channel := backend.Conversation{Name: "general", Kind: backend.KindChannel}
	muted := backend.Conversation{Name: "alerts", Kind: backend.KindChannel, Muted: true}
	dm := backend.Conversation{Name: "Ada", Kind: backend.KindDM}
	ada := &backend.Profile{StatusEmoji: ":palm_tree:", StatusText: "Out until Monday", Title: "Analyst", HasTZ: true, TZOffset: 3600}
	cases := []struct {
		name    string
		conv    backend.Conversation
		details *backend.Details
		profile *backend.Profile
		now     time.Time
		want    []Chip
	}{
		{"channel topic and size", channel, &backend.Details{Topic: "Announcements", Purpose: "ignored", Members: 48}, nil, noon,
			[]Chip{{Text: "Announcements", Grow: true}, {Icon: ChipPeople, Text: "48"}}},
		{"purpose when no topic", channel, &backend.Details{Purpose: "Banter"}, nil, noon,
			[]Chip{{Text: "Banter", Grow: true}}},
		{"channel not loaded", channel, nil, nil, noon, nil},
		{"muted channel", muted, &backend.Details{Members: 3}, nil, noon,
			[]Chip{{Icon: ChipPeople, Text: "3"}, {Icon: ChipMuted, Text: "muted"}}},
		{"dm by day", dm, nil, ada, noon,
			[]Chip{{Text: "Out until Monday", Grow: true}, {Text: "Analyst"}, {Icon: ChipClock, Text: "13:00"}}},
		{"dm at night there", dm, nil, ada, time.Date(2026, 10, 29, 21, 30, 0, 0, time.UTC),
			[]Chip{{Text: "Out until Monday", Grow: true}, {Text: "Analyst"}, {Icon: ChipNight, Text: "22:30", Tone: ToneWarn}}},
		{"dm early morning there", dm, nil, &backend.Profile{HasTZ: true, TZOffset: -5 * 3600}, noon,
			[]Chip{{Icon: ChipNight, Text: "07:00", Tone: ToneWarn}}},
		{"dm title grows without a status", dm, nil, &backend.Profile{Title: "Admiral"}, noon,
			[]Chip{{Text: "Admiral", Grow: true}}},
		{"dm emoji only: it goes by the name", dm, nil, &backend.Profile{StatusEmoji: ":coffee:"}, noon, nil},
		{"dm status expired", dm, nil, &backend.Profile{StatusText: "Lunch", StatusExpires: noon.Add(-time.Minute), Title: "Analyst"}, noon,
			[]Chip{{Text: "Analyst", Grow: true}}},
		{"dm status not yet expired", dm, nil, &backend.Profile{StatusText: "Lunch", StatusExpires: noon.Add(time.Hour)}, noon,
			[]Chip{{Text: "Lunch", Grow: true}}},
		{"dm do not disturb", dm, nil, &backend.Profile{DND: true, DNDUntil: noon.Add(5 * time.Hour)}, noon,
			[]Chip{{Icon: ChipDND, Text: "do not disturb until 17:00", Tone: ToneWarn}}},
		{"dm do not disturb over", dm, nil, &backend.Profile{DND: true, DNDUntil: noon.Add(-time.Hour)}, noon, nil},
		{"dm not loaded", dm, nil, nil, noon, nil},
		{"group dm", backend.Conversation{Name: "Grace Hopper, Linus Torvalds", Kind: backend.KindGroupDM}, nil, nil, noon,
			[]Chip{{Icon: ChipPeople, Text: "3"}}},
	}
	for _, tc := range cases {
		if got := Chips(tc.conv, tc.details, tc.profile, tc.now); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", tc.name, got, tc.want)
		}
	}
}
