package chat

import (
	"testing"

	"github.com/gammons/slk/gui/backend"
)

func TestApplyReactionIsIdempotent(t *testing.T) {
	var rs []backend.Reaction
	rs = ApplyReaction(rs, "tada", "U1", true, false)
	rs = ApplyReaction(rs, "tada", "ME", true, true)
	rs = ApplyReaction(rs, "tada", "ME", true, true) // Slack's echo
	if len(rs) != 1 || rs[0].Count != 2 || !rs[0].Mine || len(rs[0].Users) != 2 {
		t.Fatalf("got %+v, want tada ×2 with the user in it", rs)
	}
	rs = ApplyReaction(rs, "tada", "ME", false, true)
	rs = ApplyReaction(rs, "tada", "ME", false, true) // the echo again
	if len(rs) != 1 || rs[0].Count != 1 || rs[0].Mine {
		t.Fatalf("got %+v, want tada ×1 without the user", rs)
	}
	rs = ApplyReaction(rs, "tada", "U1", false, false)
	if len(rs) != 0 {
		t.Fatalf("got %+v, want the pill dropped at zero", rs)
	}
	if got := ApplyReaction(nil, "eyes", "U1", false, false); len(got) != 0 {
		t.Fatalf("removing what is not there: %+v", got)
	}
}

func TestApplyReactionKeepsOrderAndInputs(t *testing.T) {
	rs := []backend.Reaction{{Name: "+1", Count: 1, Users: []string{"U1"}}, {Name: "eyes", Count: 1, Users: []string{"U2"}}}
	got := ApplyReaction(rs, "+1", "U3", true, false)
	if got[0].Name != "+1" || got[0].Count != 2 || got[1].Name != "eyes" {
		t.Fatalf("got %+v", got)
	}
	if rs[0].Count != 1 || len(rs[0].Users) != 1 {
		t.Fatalf("the input changed: %+v", rs)
	}
}

func TestApplyReactionOnATruncatedUserList(t *testing.T) {
	// Slack lists only some users of a popular reaction.
	rs := []backend.Reaction{{Name: "+1", Count: 30, Users: []string{"U1", "U2"}}}
	got := ApplyReaction(rs, "+1", "U9", false, false)
	if got[0].Count != 29 {
		t.Fatalf("count %d, want 29", got[0].Count)
	}
}

func TestTimelineAndThreadApplyReactions(t *testing.T) {
	var tl Timeline
	tl.Merge([]backend.Message{msg(root, "parent")}, nil)
	ev := backend.ReactionChanged{ChannelID: "C1", TS: root, Name: "+1", UserID: "ME", Added: true, Self: true}
	if !tl.Apply("C1", ev) || tl.Apply("C1", ev) {
		t.Fatal("timeline: want a change, then none for the echo")
	}
	if r := tl.Messages()[0].Reactions; len(r) != 1 || !r[0].Mine {
		t.Fatalf("timeline reactions %+v", r)
	}
	if tl.Apply("C1", backend.ReactionChanged{ChannelID: "C2", TS: root, Name: "+1", UserID: "U1", Added: true}) {
		t.Fatal("a reaction in C2 changed C1")
	}

	th := newThread()
	th.Merge([]backend.Message{msg(root, "parent"), reply("1700000200.000100", "r1")}, nil)
	if !th.Apply(backend.ReactionChanged{ChannelID: "C1", TS: "1700000200.000100", Name: "eyes", UserID: "U1", Added: true}) {
		t.Fatal("thread: no change")
	}
	if r := th.Replies()[0].Reactions; len(r) != 1 || r[0].Name != "eyes" || r[0].Mine {
		t.Fatalf("thread reactions %+v", r)
	}
}

func TestEditKeepsReactions(t *testing.T) {
	var tl Timeline
	m := msg(root, "parent")
	m.Reactions = []backend.Reaction{{Name: "+1", Count: 1, Users: []string{"U1"}}}
	tl.Merge([]backend.Message{m}, nil)
	th := newThread()
	th.Merge([]backend.Message{m}, nil)
	edit := msg(root, "parent, edited")
	edit.Edited = true
	tl.Apply("C1", backend.MessagePosted{Message: edit})
	th.Apply(backend.MessagePosted{Message: edit})
	if len(tl.Messages()[0].Reactions) != 1 || len(th.Messages()[0].Reactions) != 1 {
		t.Fatal("an edit dropped the reactions")
	}
}
