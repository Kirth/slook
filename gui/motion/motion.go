// Package motion is the GUI's small transitions as plain arithmetic
// over time: an easing curve, a value gliding toward a target, an
// entrance, a highlight's rise and fall. A window asks for its next
// frame while something it draws is still moving, and for nothing once
// it is still.
package motion

import "time"

// EaseOut is a cubic ease-out over t in [0,1]: a fast start that
// settles gently. t outside the range is clamped.
func EaseOut(t float64) float64 {
	t = clamp(t)
	u := 1 - t
	return 1 - u*u*u
}

// Tween is a value moving toward a target over a duration. The zero
// value has no value yet: its first Set takes the target at once.
type Tween struct {
	from, to float64
	start    time.Time
	dur      time.Duration
	set      bool
}

// Set retargets the tween to v over dur, starting from wherever it is
// at now, so a new target arriving mid-flight carries on from the
// current value rather than jumping. The first Set takes v at once:
// there is nothing to move from.
func (t *Tween) Set(v float64, now time.Time, dur time.Duration) {
	if !t.set {
		t.from, t.to, t.set = v, v, true
		return
	}
	if v == t.to {
		return
	}
	cur, _ := t.Value(now)
	t.from, t.to, t.start, t.dur = cur, v, now, dur
}

// Target is the value the tween is heading for.
func (t *Tween) Target() float64 { return t.to }

// Value is the tween's value at now and whether it is still moving.
func (t *Tween) Value(now time.Time) (float64, bool) {
	if t.dur <= 0 || !now.Before(t.start.Add(t.dur)) {
		return t.to, false
	}
	p := EaseOut(float64(now.Sub(t.start)) / float64(t.dur))
	return t.from + (t.to-t.from)*p, true
}

// Enter is how far an entrance that began at since and takes d has got
// at now, 0 to 1 and eased, and whether it is still under way.
func Enter(since, now time.Time, d time.Duration) (float64, bool) {
	if d <= 0 || !now.Before(since.Add(d)) {
		return 1, false
	}
	return EaseOut(float64(now.Sub(since)) / float64(d)), true
}

// Pulse is the strength, 0 to 1, of a highlight that began at start:
// rising over in (eased), then falling linearly to 0 at start+total.
func Pulse(start, now time.Time, in, total time.Duration) float64 {
	e := now.Sub(start)
	switch {
	case e <= 0 || e >= total:
		return 0
	case e < in:
		return EaseOut(float64(e) / float64(in))
	}
	return 1 - float64(e-in)/float64(total-in)
}

func clamp(t float64) float64 { return max(0, min(1, t)) }
