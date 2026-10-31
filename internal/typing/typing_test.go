package typing

import (
	"slices"
	"testing"
	"time"
)

func clockAt(t *Tracker, at *time.Time) { t.Now = func() time.Time { return *at } }

func TestTypingExpiresAndDone(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tr := New()
	clockAt(tr, &now)
	tr.Add("C1", "U2")
	tr.Add("C1", "U1")
	tr.Add("C2", "U3")
	if got := tr.Users("C1"); !slices.Equal(got, []string{"U1", "U2"}) {
		t.Fatalf("users %v", got)
	}
	if got := tr.UsersExcluding("C1", "U1"); !slices.Equal(got, []string{"U2"}) {
		t.Fatalf("excluding %v", got)
	}
	tr.Done("C1", "U2")
	if got := tr.Users("C1"); !slices.Equal(got, []string{"U1"}) {
		t.Fatalf("after done %v", got)
	}
	now = now.Add(Expiry - time.Second)
	tr.Add("C2", "U3") // typing again keeps them
	now = now.Add(2 * time.Second)
	if got := tr.Users("C1"); len(got) != 0 {
		t.Fatalf("lapsed still shown: %v", got)
	}
	if !tr.Expire() {
		t.Fatal("Expire says nothing is left; C2's typer is")
	}
	now = now.Add(Expiry)
	if tr.Expire() {
		t.Fatal("Expire says something is left")
	}
}

func TestBotStatuses(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tr := New()
	clockAt(tr, &now)
	tr.SetBotStatus("C1", "1.0", "B2", "is searching")
	tr.SetBotStatus("C1", "1.0", "B1", "is thinking")
	tr.SetBotStatus("C1", "2.0", "B1", "is reading")
	if got := tr.WorkingBots("C1"); !slices.Equal(got, []string{"B1", "B2"}) {
		t.Fatalf("working %v", got)
	}
	want := []BotStatus{{"B1", "is thinking"}, {"B2", "is searching"}}
	if got := tr.ThreadBotStatuses("C1", "1.0"); !slices.Equal(got, want) {
		t.Fatalf("thread %v", got)
	}
	tr.SetBotStatus("C1", "1.0", "B2", "") // cleared
	tr.Done("C1", "B1")                    // replied: all its statuses go
	if got := tr.WorkingBots("C1"); len(got) != 0 {
		t.Fatalf("after clear and reply %v", got)
	}
	tr.SetBotStatus("C1", "1.0", "B1", "is thinking")
	now = now.Add(BotExpiry + time.Second)
	if tr.Expire() {
		t.Fatal("a lapsed status is left")
	}
}

func TestWords(t *testing.T) {
	for _, c := range []struct {
		typers, apps []string
		end, want    string
	}{
		{nil, nil, "...", ""},
		{[]string{"Ada"}, nil, "...", "Ada is typing..."},
		{[]string{"Ada", "Grace"}, nil, "", "Ada and Grace are typing"},
		{[]string{"A", "B", "C"}, nil, "", "Several people are typing"},
		{[]string{"Ada"}, []string{"Claude"}, "...", "Ada is typing... · Claude is working..."},
		{nil, []string{"A", "B", "C"}, "", "Several apps are working"},
	} {
		if got := Row(c.typers, c.apps, c.end); got != c.want {
			t.Errorf("Row(%v, %v, %q) = %q, want %q", c.typers, c.apps, c.end, got, c.want)
		}
	}
}
