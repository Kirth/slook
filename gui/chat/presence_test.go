package chat

import (
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
)

func current(items []PresenceItem) []string {
	var out []string
	for _, it := range items {
		if it.Current {
			out = append(out, it.Label)
		}
	}
	return out
}

func TestPresenceItemsMarkTheCurrentOne(t *testing.T) {
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC) // a Wednesday
	if got := current(PresenceItems(backend.SelfStatus{}, now)); len(got) != 1 || got[0] != "Active" {
		t.Errorf("active: current %v", got)
	}
	if got := current(PresenceItems(backend.SelfStatus{Away: true}, now)); len(got) != 1 || got[0] != "Away" {
		t.Errorf("away: current %v", got)
	}
	items := PresenceItems(backend.SelfStatus{}, now)
	var snoozes []int
	for _, it := range items {
		if it.Snooze {
			snoozes = append(snoozes, it.Minutes)
		}
	}
	want := []int{20, 60, 120, 240, 480, 1440, 18 * 60}
	if len(snoozes) != len(want) {
		t.Fatalf("snoozes %v, want %v", snoozes, want)
	}
	for i := range want {
		if snoozes[i] != want[i] {
			t.Fatalf("snoozes %v, want %v", snoozes, want)
		}
	}
	if !items[1].Away || items[0].Away {
		t.Errorf("Away/Active rows: %+v %+v", items[0], items[1])
	}
}

func TestPresenceItemsOfferEndOnlyWhileDND(t *testing.T) {
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC)
	hasEnd := func(items []PresenceItem) bool {
		for _, it := range items {
			if it.End {
				return true
			}
		}
		return false
	}
	if hasEnd(PresenceItems(backend.SelfStatus{Away: true}, now)) {
		t.Error("End offered without do-not-disturb")
	}
	items := PresenceItems(backend.SelfStatus{DND: true, DNDUntil: now.Add(time.Hour)}, now)
	if !hasEnd(items) {
		t.Fatal("End not offered during do-not-disturb")
	}
	if got := current(items); len(got) != 0 {
		t.Errorf("during do-not-disturb, current %v", got)
	}
}

func TestMinutesUntilTomorrowMorning(t *testing.T) {
	loc := time.FixedZone("X", 2*3600)
	cases := []struct {
		now  time.Time
		want int
	}{
		{time.Date(2026, 10, 28, 15, 0, 0, 0, loc), 18 * 60},     // Wednesday → Thursday 09:00
		{time.Date(2026, 10, 30, 17, 30, 0, 0, loc), 63*60 + 30}, // Friday → Monday 09:00
		{time.Date(2026, 10, 31, 9, 0, 0, 0, loc), 48 * 60},      // Saturday → Monday
		{time.Date(2026, 11, 1, 23, 0, 0, 0, loc), 10 * 60},      // Sunday night → Monday
		{time.Date(2026, 10, 29, 8, 59, 30, 0, loc), 24 * 60},    // Thursday before nine → Friday, not today
	}
	for i, c := range cases {
		if got := MinutesUntilTomorrowMorning(c.now); got != c.want {
			t.Errorf("case %d (%s): %d, want %d", i, c.now.Format(time.RFC1123), got, c.want)
		}
	}
}

func TestSelfText(t *testing.T) {
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC)
	until := time.Date(2026, 10, 28, 15, 20, 0, 0, time.UTC)
	cases := []struct {
		st   backend.SelfStatus
		want string
	}{
		{backend.SelfStatus{}, ""},
		{backend.SelfStatus{Away: true}, "Away"},
		{backend.SelfStatus{DND: true, DNDUntil: until}, "Do not disturb until 15:20"},
		{backend.SelfStatus{DND: true}, "Do not disturb"},
		{backend.SelfStatus{Away: true, DND: true, DNDUntil: until.Add(24 * time.Hour)}, "Away · do not disturb until Thu 15:20"},
	}
	for _, c := range cases {
		if got := SelfText(c.st, now); got != c.want {
			t.Errorf("SelfText(%+v) = %q, want %q", c.st, got, c.want)
		}
	}
}
