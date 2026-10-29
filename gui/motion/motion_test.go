package motion

import (
	"testing"
	"time"
)

var t0 = time.Unix(1_700_000_000, 0)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestEaseOut(t *testing.T) {
	if EaseOut(0) != 0 || EaseOut(1) != 1 || EaseOut(-1) != 0 || EaseOut(2) != 1 {
		t.Fatal("endpoints")
	}
	if EaseOut(0.5) <= 0.5 {
		t.Fatal("an ease-out is past half way at the midpoint")
	}
	prev := 0.0
	for i := 1; i <= 100; i++ {
		v := EaseOut(float64(i) / 100)
		if v < prev {
			t.Fatalf("not monotonic at %d", i)
		}
		prev = v
	}
}

func TestTween(t *testing.T) {
	var tw Tween
	tw.Set(0.2, t0, ms(400))
	if v, moving := tw.Value(t0); v != 0.2 || moving {
		t.Fatalf("first Set: %v, %v; want 0.2 at once", v, moving)
	}
	tw.Set(0.6, t0, ms(400))
	prev := 0.2
	for i := 0; i <= 400; i += 20 {
		v, moving := tw.Value(t0.Add(ms(i)))
		if v < prev || v > 0.6 {
			t.Fatalf("at %dms: %v (prev %v)", i, v, prev)
		}
		if i < 400 && !moving {
			t.Fatalf("stopped early at %dms", i)
		}
		prev = v
	}
	if v, moving := tw.Value(t0.Add(ms(400))); v != 0.6 || moving {
		t.Fatalf("at the end: %v, %v", v, moving)
	}

	// Retarget mid-flight: no jump.
	tw.Set(1, t0.Add(ms(1000)), ms(400))
	mid, _ := tw.Value(t0.Add(ms(1100)))
	tw.Set(0.8, t0.Add(ms(1100)), ms(400))
	if v, _ := tw.Value(t0.Add(ms(1100))); v != mid {
		t.Fatalf("retarget jumped from %v to %v", mid, v)
	}
	if tw.Target() != 0.8 {
		t.Fatalf("target %v", tw.Target())
	}
}

func TestEnterAndPulse(t *testing.T) {
	if p, moving := Enter(t0, t0, ms(160)); p != 0 || !moving {
		t.Fatalf("Enter at start: %v, %v", p, moving)
	}
	if p, moving := Enter(t0, t0.Add(ms(160)), ms(160)); p != 1 || moving {
		t.Fatalf("Enter at end: %v, %v", p, moving)
	}
	in, total := ms(180), ms(2000)
	if Pulse(t0, t0, in, total) != 0 || Pulse(t0, t0.Add(in), in, total) != 1 || Pulse(t0, t0.Add(total), in, total) != 0 {
		t.Fatal("pulse endpoints")
	}
	if a, b := Pulse(t0, t0.Add(ms(500)), in, total), Pulse(t0, t0.Add(ms(1500)), in, total); !(a > b && b > 0) {
		t.Fatalf("pulse should fall: %v then %v", a, b)
	}
}
