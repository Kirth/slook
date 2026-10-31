package chat

import (
	"strings"

	"github.com/gammons/slk/internal/slackurl"
)

// LinkTarget reads a link to a message in the workspace whose subdomain
// is domain (https://<domain>.slack.com/archives/<channel>/p<ts>): the
// conversation, the message, and for a thread reply its parent
// (threadTS, from the link's thread_ts; empty otherwise). ok is false
// for any other link, another workspace's included, which open in the
// browser.
func LinkTarget(url, domain string) (channelID, ts, threadTS string, ok bool) {
	p, ok := slackurl.Parse(url)
	if !ok || domain == "" || !strings.EqualFold(p.Subdomain, domain) || p.MessageTS == "" {
		return "", "", "", false
	}
	return string(p.ChannelID), string(p.MessageTS), string(p.ThreadTS), true
}
