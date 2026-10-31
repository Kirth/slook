package backend

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/slack-go/slack"
)

func TestReactWithShowsThenTakesBackARefusal(t *testing.T) {
	for _, c := range []struct {
		name  string
		err   error
		want  []bool // the changes shown: true added
		fails bool
	}{
		{"accepted", nil, []bool{true}, false},
		{"refused", errors.New("adding reaction :x:: invalid_name"), []bool{true, false}, true},
		{"already so", fmt.Errorf("adding reaction :x:: %w", slack.SlackErrorResponse{Err: "already_reacted"}), []bool{true}, false},
		{"no answer", context.DeadlineExceeded, []bool{true, false}, true},
	} {
		var shown []bool
		err := reactWith(context.Background(), true, func(added bool) { shown = append(shown, added) },
			func(ctx context.Context) error {
				if _, ok := ctx.Deadline(); !ok {
					t.Errorf("%s: the call has no deadline", c.name)
				}
				return c.err
			})
		if (err != nil) != c.fails || !slices.Equal(shown, c.want) {
			t.Errorf("%s: err %v, shown %v; want fails %v, shown %v", c.name, err, shown, c.fails, c.want)
		}
	}
}

func TestIsSlackErrorThroughWrapping(t *testing.T) {
	if !isSlackError(errors.New("removing reaction :+1:: no_reaction"), "no_reaction") {
		t.Fatal("a wrapped code by text was not recognised")
	}
	if isSlackError(errors.New("not_reacted_anything"), "no_reaction") {
		t.Fatal("matched a different code")
	}
}
