package main

import (
	"context"
	"errors"
	"testing"

	"github.com/gammons/slk/internal/core"
	slackclient "github.com/gammons/slk/internal/slack"
)

type fakeCommandRunner struct {
	got  slackclient.CommandRequest
	err  error
	team string
}

func (f *fakeCommandRunner) RunCommand(_ context.Context, req slackclient.CommandRequest) error {
	f.got = req
	return f.err
}

func (f *fakeCommandRunner) TeamID() string { return f.team }

func TestRunCommandMapsFields(t *testing.T) {
	f := &fakeCommandRunner{team: "T0EXAMPLE01"}
	if err := runCommand(context.Background(), f, "C0EXAMPLE01", "10.0", "/giphy", "cats"); err != nil {
		t.Fatal(err)
	}
	want := slackclient.CommandRequest{ChannelID: "C0EXAMPLE01", ThreadTS: "10.0", Command: "/giphy", Args: "cats", TeamID: "T0EXAMPLE01"}
	if f.got != want {
		t.Errorf("request = %+v, want %+v", f.got, want)
	}
}

func TestRunCommandTranslatesRejection(t *testing.T) {
	f := &fakeCommandRunner{err: &slackclient.CommandRejectedError{Code: "unknown_command"}}
	err := runCommand(context.Background(), f, "C1", "", "/nope", "")
	var ce *core.CommandError
	if !errors.As(err, &ce) || ce.Code != "unknown_command" {
		t.Fatalf("err = %v, want *core.CommandError{unknown_command}", err)
	}
}

func TestRunCommandPassesOtherErrorsThrough(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeCommandRunner{err: boom}
	if err := runCommand(context.Background(), f, "C1", "", "/giphy", ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}
