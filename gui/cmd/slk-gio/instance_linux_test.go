package main

import "testing"

func TestBusSegment(t *testing.T) {
	for in, want := range map[string]string{
		"T01ABC": "T01ABC",
		"fake":   "fake",
		"E0-x.y": "E0_x_y",
		"9lives": "_9lives",
		"":       "_",
		"Ünï":    "_n_",
	} {
		if got := busSegment(in); got != want {
			t.Errorf("busSegment(%q) = %q, want %q", in, got, want)
		}
	}
}
