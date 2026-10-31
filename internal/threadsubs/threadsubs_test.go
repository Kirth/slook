package threadsubs

import (
	"testing"
	"time"
)

func TestGateAdmitsOncePerWindow(t *testing.T) {
	g := &Gate{Window: time.Hour}
	now := time.Unix(1_700_000_000, 0)
	first, ok := g.TryStart(now)
	if !first || !ok || !g.Running() {
		t.Fatalf("first: %v %v running %v", first, ok, g.Running())
	}
	if _, ok := g.TryStart(now.Add(2 * time.Hour)); ok {
		t.Fatal("admitted while running")
	}
	g.Done()
	if _, ok := g.TryStart(now.Add(30 * time.Minute)); ok {
		t.Fatal("admitted inside the window")
	}
	if first, ok := g.TryStart(now.Add(2 * time.Hour)); first || !ok {
		t.Fatalf("after the window: %v %v", first, ok)
	}
}
