package blocks

import "testing"

// Ids are positional and best-effort: a frame with fewer or more ids
// than parsed actions must not panic or shift ids onto the wrong button.
func TestApplyActionIDs_Misaligned(t *testing.T) {
	atts := []LegacyAttachment{
		{Actions: []LegacyAction{{Name: "a"}, {Name: "b"}}},
		{Actions: []LegacyAction{{Name: "c"}}},
	}
	ApplyActionIDs(atts, [][]string{{"1"}})
	if atts[0].Actions[0].ID != "1" || atts[0].Actions[1].ID != "" || atts[1].Actions[0].ID != "" {
		t.Errorf("short ids: %+v", atts)
	}
	ApplyActionIDs(atts, [][]string{{"1", "2", "3"}, {"4", "5"}, {"6"}})
	if atts[0].Actions[1].ID != "2" || atts[1].Actions[0].ID != "4" {
		t.Errorf("long ids: %+v", atts)
	}
	ApplyActionIDs(nil, [][]string{{"1"}}) // must not panic
}
