package main

import (
	"context"
	"errors"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	slackclient "github.com/gammons/slk/internal/slack"
)

// commandRunner is the part of *slackclient.Client running a slash
// command needs.
type commandRunner interface {
	RunCommand(ctx context.Context, req slackclient.CommandRequest) error
	TeamID() string
}

// runCommand runs a slash command through client and translates Slack's
// refusal into core.CommandError, so the UI never sees slackclient types.
func runCommand(ctx context.Context, client commandRunner, channelID ids.ChannelID,
	threadTS ids.ThreadTS, command, args string) error {
	err := client.RunCommand(ctx, slackclient.CommandRequest{
		ChannelID: string(channelID),
		ThreadTS:  string(threadTS),
		Command:   command,
		Args:      args,
		TeamID:    client.TeamID(),
	})
	var rej *slackclient.CommandRejectedError
	if errors.As(err, &rej) {
		return &core.CommandError{Code: rej.Code}
	}
	return err
}
