package search

import (
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
)

func TestSummaryAndCommas(t *testing.T) {
	st := backend.SyncStatus{Messages: 12304, Conversations: []backend.Coverage{
		{Name: "a", Complete: true}, {Name: "b"}, {Name: "c", Complete: true},
	}}
	if got := Summary(st); got != "12,304 messages · 2 of 3 conversations complete" {
		t.Fatalf("Summary = %q", got)
	}
	if got := Summary(backend.SyncStatus{Messages: 1}); got != "1 message" {
		t.Fatalf("Summary of one = %q", got)
	}
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567"} {
		if got := Commas(n); got != want {
			t.Errorf("Commas(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestBackfillLine(t *testing.T) {
	now := time.Date(2026, 10, 29, 14, 0, 0, 0, time.Local)
	cases := []struct {
		b    backend.BackfillStatus
		want string
	}{
		{backend.BackfillStatus{}, "Off"},
		{backend.BackfillStatus{State: backend.BackfillRunning, Phase: "conversations", Current: "#general", Done: 12, Total: 57, Requests: 340},
			"Backfilling #general · 12 of 57 · 340 requests"},
		{backend.BackfillStatus{State: backend.BackfillPaused, Phase: "conversations", Current: "#general", Done: 12, Total: 57, Requests: 340},
			"Backfilling #general · 12 of 57 · 340 requests · paused while you read"},
		{backend.BackfillStatus{State: backend.BackfillRunning, Phase: "threads", Done: 8, Requests: 1200},
			"Threads · 8 fetched · 1,200 requests"},
		{backend.BackfillStatus{State: backend.BackfillDone, Requests: 2000, StartedAt: now.Add(-time.Hour)}, "Done today · 2,000 requests"},
		{backend.BackfillStatus{State: backend.BackfillDone, Requests: 5, StartedAt: now.Add(-48 * time.Hour)}, "Done Oct 27 · 5 requests"},
		{backend.BackfillStatus{State: backend.BackfillFailed, LastError: "rate limited"}, "Failed: rate limited"},
	}
	for _, c := range cases {
		if got := BackfillLine(c.b, now); got != c.want {
			t.Errorf("BackfillLine(%+v) = %q, want %q", c.b, got, c.want)
		}
	}
}

func TestHorizons(t *testing.T) {
	if HorizonFor("90d") != 90*24*time.Hour || HorizonFor("1y") != 365*24*time.Hour || HorizonFor("all") != 0 {
		t.Fatal("HorizonFor")
	}
	for d, want := range map[time.Duration]string{0: "everything", 90 * 24 * time.Hour: "90 days", 365 * 24 * time.Hour: "1 year", 2 * 365 * 24 * time.Hour: "2 years"} {
		if got := Horizon(d); got != want {
			t.Errorf("Horizon(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestOrderCoverageAndLine(t *testing.T) {
	jan := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	cs := []backend.Coverage{
		{Name: "zeta", Complete: true},
		{Name: "Beta", Oldest: jan, Holes: 2},
		{Name: "alpha", Complete: true},
		{Name: "gamma"},
	}
	got := OrderCoverage(cs)
	var names []string
	for _, c := range got {
		names = append(names, c.Name)
	}
	if want := []string{"Beta", "gamma", "alpha", "zeta"}; len(names) != 4 || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] || names[3] != want[3] {
		t.Fatalf("order = %v, want %v", names, want)
	}
	if cs[0].Name != "zeta" {
		t.Fatal("OrderCoverage changed its input")
	}
	if l := CoverageLine(cs[1]); l != "since "+jan.Local().Format("Jan 2006")+" · 2 gaps" {
		t.Errorf("CoverageLine = %q", l)
	}
	if l := CoverageLine(cs[0]); l != "complete" {
		t.Errorf("CoverageLine complete = %q", l)
	}
	if l := CoverageLine(backend.Coverage{Holes: 1, Oldest: jan}); l != "since "+jan.Local().Format("Jan 2006")+" · 1 gap" {
		t.Errorf("CoverageLine one gap = %q", l)
	}
	if l := CoverageLine(cs[3]); l != "nothing yet" {
		t.Errorf("CoverageLine empty = %q", l)
	}
}

func TestTarget(t *testing.T) {
	plain := backend.SearchHit{Message: backend.Message{ChannelID: "C1", TS: "2.0"}}
	if ch, ts, thread := Target(plain); ch != "C1" || ts != "2.0" || thread != "" {
		t.Fatal("plain target")
	}
	reply := backend.SearchHit{Message: backend.Message{ChannelID: "C1", TS: "3.0", ThreadTS: "2.0", InThreadOnly: true}}
	if _, ts, thread := Target(reply); ts != "2.0" || thread != "2.0" {
		t.Fatalf("reply target = %s in thread %q, want the parent and its thread", ts, thread)
	}
	broadcast := backend.SearchHit{Message: backend.Message{ChannelID: "C1", TS: "3.0", ThreadTS: "2.0"}}
	if _, ts, thread := Target(broadcast); ts != "3.0" || thread != "" {
		t.Fatalf("broadcast target = %s in thread %q, want itself in the channel", ts, thread)
	}
}

func TestFractionAndOverall(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	horizon := 90 * day
	cases := []struct {
		name string
		c    backend.Coverage
		want float64
	}{
		{"nothing", backend.Coverage{}, 0},
		{"complete", backend.Coverage{Complete: true}, 1},
		{"half, live", backend.Coverage{Live: true, Spans: []backend.TimeSpan{{Oldest: now.Add(-45 * day), Latest: now.Add(-time.Hour)}}}, 0.5},
		{"past the horizon", backend.Coverage{Live: true, Spans: []backend.TimeSpan{{Oldest: now.Add(-400 * day), Latest: now}}}, 1},
		{"two spans with a hole", backend.Coverage{Live: true, Spans: []backend.TimeSpan{
			{Oldest: now.Add(-90 * day), Latest: now.Add(-60 * day)}, // 30 days
			{Oldest: now.Add(-30 * day), Latest: now},                // 30 days
		}}, 2.0 / 3},
		{"not live: the gap to now counts as missing", backend.Coverage{Spans: []backend.TimeSpan{{Oldest: now.Add(-90 * day), Latest: now.Add(-45 * day)}}}, 0.5},
	}
	for _, c := range cases {
		got := Fraction(c.c, horizon, now)
		if got < c.want-0.001 || got > c.want+0.001 {
			t.Errorf("%s: Fraction = %.3f, want %.3f", c.name, got, c.want)
		}
	}
	if Fraction(cases[2].c, 0, now) != 0 || Fraction(cases[1].c, 0, now) != 1 {
		t.Error("horizon 0: want 0 unless complete")
	}
	all := []backend.Coverage{cases[1].c, cases[2].c, cases[0].c}
	if got := Overall(all, horizon, now); got < 0.499 || got > 0.501 {
		t.Errorf("Overall = %.3f, want 0.5", got)
	}
	if Overall(nil, horizon, now) != 0 {
		t.Error("Overall of nothing")
	}
	if Percent(0.4249) != "42%" || Percent(1) != "100%" {
		t.Error("Percent")
	}
}

func TestRateEstimatorLeft(t *testing.T) {
	if Rate(0, time.Minute) != "" || Rate(100, 0) != "" {
		t.Error("Rate with nothing to say")
	}
	if got := Rate(2460, 2*time.Minute); got != "1,230 msgs/min" {
		t.Errorf("Rate = %q", got)
	}
	if got := Rate(3, 2*time.Minute); got != "1.5 msgs/min" {
		t.Errorf("Rate slow = %q", got)
	}
	if got := Rate(1, 2*time.Hour); got != "0.5 msgs/hour" {
		t.Errorf("Rate slower = %q", got)
	}

	var e Estimator
	t0 := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	if _, ok := e.Remaining(); ok {
		t.Error("estimate from nothing")
	}
	e.Add(t0, 0.2)
	if _, ok := e.Remaining(); ok {
		t.Error("estimate from one sample")
	}
	e.Add(t0.Add(time.Minute), 0.2)
	if _, ok := e.Remaining(); ok {
		t.Error("estimate without movement")
	}
	e.Add(t0.Add(2*time.Minute), 0.4) // 0.2 in 2 min → 0.6 left takes 6 min
	d, ok := e.Remaining()
	if !ok || d < 5*time.Minute-time.Second || d > 6*time.Minute+time.Second {
		t.Errorf("Remaining = %v, %v; want ~6 min", d, ok)
	}
	// Old samples fall out of the window.
	e.Add(t0.Add(10*time.Minute), 0.5)
	if len(e.samples) != 1 {
		t.Errorf("window kept %d samples, want 1", len(e.samples))
	}
	e.Reset()
	if len(e.samples) != 0 {
		t.Error("Reset")
	}

	for d, want := range map[time.Duration]string{
		30 * time.Second: "under a minute left",
		18 * time.Minute: "~18 min left",
		3 * time.Hour:    "~3.0 h left",
		72 * time.Hour:   "~3 days left",
	} {
		if got := Left(d); got != want {
			t.Errorf("Left(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestCountedExcludesIgnored(t *testing.T) {
	now := time.Now()
	cs := []backend.Coverage{{Name: "a", Complete: true}, {Name: "alerts", Ignored: true}, {Name: "b"}}
	if got := Counted(cs); len(got) != 2 || got[1].Name != "b" {
		t.Fatalf("Counted = %+v", got)
	}
	if got := Overall(cs, 24*time.Hour, now); got < 0.499 || got > 0.501 {
		t.Fatalf("Overall = %.3f, want 0.5 (the ignored row does not count)", got)
	}
	if Overall([]backend.Coverage{{Ignored: true}}, 24*time.Hour, now) != 0 {
		t.Fatal("Overall of only ignored rows")
	}
}
