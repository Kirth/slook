package slackfmt

import (
	"regexp"
	"strings"

	"github.com/gammons/slk/internal/usergroups"
)

// Regexes for the entity forms FlattenMrkdwn handles beyond the shared
// ones in slackfmt.go: Slack normally emits bare <@UID> mentions, but
// search.messages snippets can carry the labeled form, and
// <!here>-style broadcasts appear with an optional |label.
var (
	userMentionWithLabelRe = regexp.MustCompile(`<@([A-Z0-9]+)\|([^>]+)>`)
	specialMentionRe       = regexp.MustCompile(`<!(here|channel|everyone)(?:\|[^>]*)?>`)
)

// FlattenMrkdwn converts Slack mrkdwn entity tokens into plain text for
// single-style contexts (e.g. search-result snippets, exports) where a
// styled renderer would be overkill:
//
//	<@U1|label>      -> @label
//	<@U1>            -> @name via resolveUser, or @U1 when unknown
//	<#C1|name>       -> #name
//	<#C1>            -> #name via resolveChannel, or #C1 when unknown
//	<url|label>      -> label
//	<url>            -> url (mailto: scheme dropped from display)
//	<!here> etc.     -> @here / @channel / @everyone
//	<!subteam^S1|@eng> -> @eng (unlabeled form -> @group)
//	<!date^..|fall>  -> local-formatted date (fallback on malformed)
//	&amp; &lt; &gt;  -> & < >
//
// Either resolver may be nil; unknown IDs fall back to the raw ID.
func FlattenMrkdwn(text string, resolveUser, resolveChannel func(id string) (string, bool)) string {
	return FlattenMrkdwnWithUserGroups(text, resolveUser, resolveChannel, nil)
}

// FlattenMrkdwnWithUserGroups is FlattenMrkdwn with a workspace-scoped
// Slack usergroup map for resolving bare <!subteam^SID> tokens.
func FlattenMrkdwnWithUserGroups(text string, resolveUser, resolveChannel func(id string) (string, bool), userGroups map[string]string) string {
	// Date tokens first: their payload never contains other entity
	// forms, and handling them up front keeps specialMentionRe from
	// having to exclude `<!date...>`.
	text = ExpandDateTokens(text)

	text = linkWithLabelRe.ReplaceAllStringFunc(text, func(match string) string {
		return linkWithLabelRe.FindStringSubmatch(match)[2]
	})
	text = linkBareRe.ReplaceAllStringFunc(text, func(match string) string {
		url := linkBareRe.FindStringSubmatch(match)[1]
		return strings.TrimPrefix(url, "mailto:")
	})

	// Labeled user mentions before bare ones: userMentionRe's
	// ([A-Z0-9]+) would otherwise stop at the `|` and not match.
	text = userMentionWithLabelRe.ReplaceAllStringFunc(text, func(match string) string {
		return "@" + userMentionWithLabelRe.FindStringSubmatch(match)[2]
	})
	text = userMentionRe.ReplaceAllStringFunc(text, func(match string) string {
		id := userMentionRe.FindStringSubmatch(match)[1]
		if resolveUser != nil {
			if name, ok := resolveUser(id); ok && name != "" {
				return "@" + name
			}
		}
		return "@" + id
	})

	text = channelRefRe.ReplaceAllStringFunc(text, func(match string) string {
		groups := channelRefRe.FindStringSubmatch(match)
		id, name := groups[1], groups[2]
		if name == "" && resolveChannel != nil {
			if resolved, ok := resolveChannel(id); ok && resolved != "" {
				name = resolved
			}
		}
		if name == "" {
			name = id
		}
		return "#" + name
	})

	text = specialMentionRe.ReplaceAllStringFunc(text, func(match string) string {
		return "@" + specialMentionRe.FindStringSubmatch(match)[1]
	})

	text = subteamMentionRe.ReplaceAllStringFunc(text, func(match string) string {
		groups := subteamMentionRe.FindStringSubmatch(match)
		return usergroups.Display(userGroups, groups[1], groups[2])
	})

	// Wire-format escapes, decoded last so they can't be reinterpreted
	// as entity delimiters (same ordering rule as the styled renderers).
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&amp;", "&")
	return text
}
