package messages

import (
	"regexp"
	"time"

	"github.com/gammons/slk/internal/datefmt"
	"github.com/gammons/slk/internal/slackfmt"
	"github.com/gammons/slk/internal/usergroups"
)

// The plain-text mrkdwn helpers moved to internal/slackfmt and the day
// label to internal/datefmt; these delegate to them so the TUI's
// callers are unchanged.

// FlattenMrkdwn delegates to slackfmt.FlattenMrkdwn.
func FlattenMrkdwn(text string, resolveUser, resolveChannel func(id string) (string, bool)) string {
	return slackfmt.FlattenMrkdwn(text, resolveUser, resolveChannel)
}

// FlattenMrkdwnWithUserGroups delegates to slackfmt.FlattenMrkdwnWithUserGroups.
func FlattenMrkdwnWithUserGroups(text string, resolveUser, resolveChannel func(id string) (string, bool), userGroups map[string]string) string {
	return slackfmt.FlattenMrkdwnWithUserGroups(text, resolveUser, resolveChannel, userGroups)
}

// SlackMrkdwnToCommonMark delegates to slackfmt.MrkdwnToCommonMark.
func SlackMrkdwnToCommonMark(text string, userNames, channelNames map[string]string) string {
	return slackfmt.MrkdwnToCommonMark(text, userNames, channelNames)
}

// SlackMrkdwnToCommonMarkWithUserGroups delegates to slackfmt.MrkdwnToCommonMarkWithUserGroups.
func SlackMrkdwnToCommonMarkWithUserGroups(text string, userNames, channelNames, userGroups map[string]string) string {
	return slackfmt.MrkdwnToCommonMarkWithUserGroups(text, userNames, channelNames, userGroups)
}

// SetDateTokenTimeLayout delegates to slackfmt.SetDateTokenTimeLayout.
func SetDateTokenTimeLayout(layout string) { slackfmt.SetDateTokenTimeLayout(layout) }

// ChannelGlyph delegates to slackfmt.ChannelGlyph.
func ChannelGlyph(chType string) string { return slackfmt.ChannelGlyph(chType) }

// FormatDateSeparator delegates to datefmt.DateSeparator.
func FormatDateSeparator(dateStr string) string { return datefmt.DateSeparator(dateStr) }

// SetNowFunc injects a clock for tests, both this package's (nowFunc)
// and datefmt's. Pass nil to revert to time.Now.
func SetNowFunc(fn func() time.Time) {
	datefmt.SetNow(fn)
	if fn == nil {
		nowFunc = time.Now
		return
	}
	nowFunc = fn
}

// renderDateToken renders one whole <!date^…> token for render.go.
func renderDateToken(match string) string { return slackfmt.ExpandDateTokens(match) }

// The entity forms render.go shares with slackfmt's flatten.go.
var (
	specialMentionRe = regexp.MustCompile(`<!(here|channel|everyone)(?:\|[^>]*)?>`)
	// Usergroup mentions: <!subteam^SID|@label> or bare <!subteam^SID>.
	// Group 1 is the ID; group 2 is the optional label.
	usergroupMentionRe = regexp.MustCompile(`<!subteam\^([A-Z0-9]+)(?:\|([^>]+))?>`)
)

func usergroupDisplay(groups map[string]string, id, label string) string {
	return usergroups.Display(groups, id, label)
}
