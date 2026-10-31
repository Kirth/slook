package slackfmt

import (
	"fmt"
	"regexp"
	"strings"

	emojiutil "github.com/gammons/slk/internal/emoji"
	"github.com/gammons/slk/internal/usergroups"
)

var (
	// Slack formatting patterns. Italic is not a regex because Go's RE2
	// has no lookaround, and CommonMark's _italic_ needs no rewriting.
	boldRe          = regexp.MustCompile(`\*([^*\n]+)\*`)
	strikethroughRe = regexp.MustCompile(`~([^~\n]+)~`)
	inlineCodeRe    = regexp.MustCompile("`([^`\n]+)`")
	codeBlockRe     = regexp.MustCompile("(?s)```(.+?)```")

	// channelRefRe matches both wire forms Slack accepts for a channel:
	//   <#CHANNELID>          — bare ID
	//   <#CHANNELID|name>     — ID with embedded display name
	// Group 1 is the ID; group 2 (optional) is the embedded name.
	// channelMentionRe (labelled form only) stays for StripMarkup.
	channelRefRe = regexp.MustCompile(`<#([A-Z0-9]+)(?:\|([^>]+))?>`)

	// Slack escapes &, <, > in user-typed text per
	// https://api.slack.com/reference/surfaces/formatting#escaping.
	// Decoded AFTER all markup regexes (which consume legitimate <...>
	// markers) so escaped user input doesn't get reinterpreted as Slack
	// markup. A NewReplacer rather than html.UnescapeString avoids
	// decoding entities Slack does not produce.
	slackEntityDecoder = strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&amp;", "&",
	)
)

// MrkdwnToCommonMark converts Slack-flavored mrkdwn into CommonMark
// markdown: the plain-text sibling of the frontends' styled renderers
// (same input format, CommonMark output). The Markdown export uses it.
func MrkdwnToCommonMark(text string, userNames map[string]string, channelNames map[string]string) string {
	return MrkdwnToCommonMarkWithUserGroups(text, userNames, channelNames, nil)
}

// MrkdwnToCommonMarkWithUserGroups is MrkdwnToCommonMark with
// a workspace-scoped Slack usergroup map for resolving bare subteam IDs.
func MrkdwnToCommonMarkWithUserGroups(text string, userNames map[string]string, channelNames map[string]string, userGroups map[string]string) string {
	// Protect code blocks: extract, convert, and replace with placeholders.
	var codeBlocks []string
	text = codeBlockRe.ReplaceAllStringFunc(text, func(match string) string {
		inner := codeBlockRe.FindStringSubmatch(match)[1]
		inner = strings.TrimSpace(inner)
		placeholder := fmt.Sprintf("\x00CB%d\x00", len(codeBlocks))
		codeBlocks = append(codeBlocks, "```\n"+inner+"\n```")
		return placeholder
	})

	// Protect inline code.
	var inlineCodes []string
	text = inlineCodeRe.ReplaceAllStringFunc(text, func(match string) string {
		inner := inlineCodeRe.FindStringSubmatch(match)[1]
		placeholder := fmt.Sprintf("\x00IC%d\x00", len(inlineCodes))
		inlineCodes = append(inlineCodes, "`"+inner+"`")
		return placeholder
	})

	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		if strings.HasPrefix(line, "&gt; ") || strings.HasPrefix(line, "> ") {
			quoted := strings.TrimPrefix(line, "&gt; ")
			quoted = strings.TrimPrefix(quoted, "> ")
			quoted = mrkdwnToCommonMarkInline(quoted, userNames, channelNames, userGroups)
			quoted = slackEntityDecoder.Replace(quoted)
			line = "> " + quoted
		} else {
			line = mrkdwnToCommonMarkInline(line, userNames, channelNames, userGroups)
			line = slackEntityDecoder.Replace(line)
		}
		result = append(result, line)
	}

	output := strings.Join(result, "\n")

	// Restore inline code placeholders.
	for i, code := range inlineCodes {
		output = strings.Replace(output, fmt.Sprintf("\x00IC%d\x00", i), code, 1)
	}
	// Restore code block placeholders.
	for i, block := range codeBlocks {
		output = strings.Replace(output, fmt.Sprintf("\x00CB%d\x00", i), block, 1)
	}

	return output
}

// mrkdwnToCommonMarkInline converts inline Slack formatting tokens
// to their CommonMark equivalents without any ANSI styling.
func mrkdwnToCommonMarkInline(text string, userNames map[string]string, channelNames map[string]string, userGroups map[string]string) string {
	text = boldRe.ReplaceAllString(text, "**$1**")

	text = strikethroughRe.ReplaceAllString(text, "~~$1~~")

	text = linkWithLabelRe.ReplaceAllStringFunc(text, func(match string) string {
		parts := linkWithLabelRe.FindStringSubmatch(match)
		url, label := parts[1], parts[2]
		return "[" + label + "](" + url + ")"
	})

	text = linkBareRe.ReplaceAllStringFunc(text, func(match string) string {
		url := linkBareRe.FindStringSubmatch(match)[1]
		return strings.TrimPrefix(url, "mailto:")
	})

	text = channelRefRe.ReplaceAllStringFunc(text, func(match string) string {
		groups := channelRefRe.FindStringSubmatch(match)
		channelID := groups[1]
		name := ""
		if len(groups) > 2 {
			name = groups[2]
		}
		if name == "" && channelNames != nil {
			if resolved, ok := channelNames[channelID]; ok {
				name = resolved
			}
		}
		if name == "" {
			name = "unknown"
		}
		return "#" + name
	})

	text = userMentionRe.ReplaceAllStringFunc(text, func(match string) string {
		userID := userMentionRe.FindStringSubmatch(match)[1]
		name := userID
		if userNames != nil {
			if resolved, ok := userNames[userID]; ok {
				name = resolved
			}
		}
		return "@" + name
	})

	text = subteamMentionRe.ReplaceAllStringFunc(text, func(match string) string {
		groups := subteamMentionRe.FindStringSubmatch(match)
		return usergroups.Display(userGroups, groups[1], groups[2])
	})

	text = specialMentionRe.ReplaceAllStringFunc(text, func(match string) string {
		return "@" + specialMentionRe.FindStringSubmatch(match)[1]
	})

	text = resolveShortcodesCommonMark(emojiutil.StripSkinToneFromText(text))

	return text
}

var commonMarkShortcodeRe = regexp.MustCompile(`:[A-Za-z0-9_+\-]+:`)

func resolveShortcodesCommonMark(s string) string {
	return commonMarkShortcodeRe.ReplaceAllStringFunc(s, func(match string) string {
		resolved := emojiutil.Sprint(match)
		if resolved == match {
			return match
		}
		return strings.TrimRight(resolved, " ")
	})
}
