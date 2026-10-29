package slackclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/gammons/slk/internal/slackhttp"
)

// SlashCommand is one command the workspace offers, as the web client's
// command picker lists it. AliasOf names the command this one is another
// spelling of ("/join" is an alias of "/open"); AppName is empty for
// Slack's own commands.
type SlashCommand struct {
	Name    string
	Usage   string
	Desc    string
	AppName string
	AliasOf string
}

// AppCommands lists the slash commands available in the workspace,
// Slack's own and those of installed apps, with client.appCommands (the
// call the web client makes for its command picker; captured in
// docs/superpowers/specs/2026-10-12-slash-commands-captures/client_app_commands.json).
func (c *Client) AppCommands(ctx context.Context) ([]SlashCommand, error) {
	ctx = slackhttp.WithReason(ctx, "dispatch-client-extras")
	raw, err := c.postForm(ctx, "client.appCommands", url.Values{})
	if err != nil {
		return nil, err
	}
	var resp struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		Commands []struct {
			Name    string `json:"name"`
			Usage   string `json:"usage"`
			Desc    string `json:"desc"`
			AppName string `json:"app_name"`
			AliasOf string `json:"alias_of"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parsing client.appCommands: %w (body=%s)", err, truncateForLog(raw))
	}
	if !resp.OK {
		return nil, fmt.Errorf("client.appCommands: %s", resp.Error)
	}
	out := make([]SlashCommand, 0, len(resp.Commands))
	for _, cmd := range resp.Commands {
		out = append(out, SlashCommand{Name: cmd.Name, Usage: cmd.Usage, Desc: cmd.Desc, AppName: cmd.AppName, AliasOf: cmd.AliasOf})
	}
	return out, nil
}
