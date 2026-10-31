package slackfmt

import "testing"

func TestCommonMark_Bold(t *testing.T) {
	got := MrkdwnToCommonMark("hello *world*", nil, nil)
	if got != "hello **world**" {
		t.Errorf("got %q", got)
	}
}

func TestCommonMark_Strikethrough(t *testing.T) {
	got := MrkdwnToCommonMark("~removed~", nil, nil)
	if got != "~~removed~~" {
		t.Errorf("got %q", got)
	}
}

func TestCommonMark_LabeledLink(t *testing.T) {
	got := MrkdwnToCommonMark("<https://example.com|docs>", nil, nil)
	if got != "[docs](https://example.com)" {
		t.Errorf("got %q", got)
	}
}

func TestCommonMark_BareLink(t *testing.T) {
	got := MrkdwnToCommonMark("<https://example.com>", nil, nil)
	if got != "https://example.com" {
		t.Errorf("got %q", got)
	}
}

func TestCommonMark_MailtoStripsScheme(t *testing.T) {
	got := MrkdwnToCommonMark("<mailto:a@b.com>", nil, nil)
	if got != "a@b.com" {
		t.Errorf("got %q", got)
	}
}

func TestCommonMark_UserMention(t *testing.T) {
	names := map[string]string{"U123": "alice"}
	got := MrkdwnToCommonMark("hi <@U123>", names, nil)
	if got != "hi @alice" {
		t.Errorf("got %q", got)
	}
}

func TestCommonMark_ChannelMention(t *testing.T) {
	got := MrkdwnToCommonMark("<#C456|general>", nil, nil)
	if got != "#general" {
		t.Errorf("got %q", got)
	}
}

func TestCommonMark_EntityDecode(t *testing.T) {
	got := MrkdwnToCommonMark("a &amp; b &lt; c &gt; d", nil, nil)
	if got != "a & b < c > d" {
		t.Errorf("got %q", got)
	}
}

func TestCommonMark_InlineCodePreserved(t *testing.T) {
	got := MrkdwnToCommonMark("use `*foo*`", nil, nil)
	if got != "use `*foo*`" {
		t.Errorf("got %q, want bold NOT applied inside backticks", got)
	}
}

func TestCommonMark_CodeBlockPreserved(t *testing.T) {
	got := MrkdwnToCommonMark("```*bold* ~strike~```", nil, nil)
	want := "```\n*bold* ~strike~\n```"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommonMark_IntraWordUnderscoreUntouched(t *testing.T) {
	got := MrkdwnToCommonMark("is_unpaid_yes", nil, nil)
	if got != "is_unpaid_yes" {
		t.Errorf("got %q", got)
	}
}

func TestCommonMark_EmojiInInlineCodePreserved(t *testing.T) {
	got := MrkdwnToCommonMark("see `:smile:` here", nil, nil)
	if got != "see `:smile:` here" {
		t.Errorf("got %q, want emoji shortcode preserved inside backticks", got)
	}
}

func TestCommonMark_EmojiInCodeBlockPreserved(t *testing.T) {
	got := MrkdwnToCommonMark("```\n:smile:\n```", nil, nil)
	want := "```\n:smile:\n```"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommonMark_EmojiNoTrailingSpace(t *testing.T) {
	got := MrkdwnToCommonMark(":smile:end", nil, nil)
	want := "😄end"
	if got != want {
		t.Errorf("got %q, want %q (no trailing space between emoji and adjacent text)", got, want)
	}
}

func TestCommonMark_Blockquote(t *testing.T) {
	got := MrkdwnToCommonMark("&gt; quoted text", nil, nil)
	if got != "> quoted text" {
		t.Errorf("got %q", got)
	}
}

// TestCommonMark_BlockquoteInlineFormatting: inline markup inside a
// quote converts as it does outside one, instead of exporting raw mrkdwn.
func TestCommonMark_BlockquoteInlineFormatting(t *testing.T) {
	in := "&gt; *bold* ~gone~ <https://example.com|docs> <@U1> in <#C2|general> :smile: `*code*`"
	got := MrkdwnToCommonMark(in, map[string]string{"U1": "alice"}, nil)
	want := "> **bold** ~~gone~~ [docs](https://example.com) @alice in #general 😄 `*code*`"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// TestCommonMark_BlockquoteEscapedMarkupStaysLiteral: entities decode
// after the markup is converted, so text the author typed as "<@U1>"
// inside a quote is not exported as a mention.
func TestCommonMark_BlockquoteEscapedMarkupStaysLiteral(t *testing.T) {
	got := MrkdwnToCommonMark("&gt; typed &lt;@U1&gt; literally", map[string]string{"U1": "alice"}, nil)
	if want := "> typed <@U1> literally"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommonMarkUsergroupMention(t *testing.T) {
	out := MrkdwnToCommonMarkWithUserGroups(
		"ping <!subteam^S0AB12CD3> and <!subteam^S9|@qa>",
		nil,
		nil,
		map[string]string{"S0AB12CD3": "backend"},
	)
	if out != "ping @backend and @qa" {
		t.Errorf("MrkdwnToCommonMarkWithUserGroups = %q; want %q", out, "ping @backend and @qa")
	}
}

// Usergroup mentions arrive as <!subteam^SID|@label> (labeled) or
// <!subteam^SID> (bare). The conversion must never leak the raw wire
// token: labeled forms use the embedded label, bare forms resolve
// through the per-workspace usergroup map, and unresolvable bare forms
// fall back to a generic "@group". (The CommonMark twins of the TUI
// renderer's TestRenderUsergroupMention* cases.)

func TestCommonMarkUsergroupMentionLabeled(t *testing.T) {
	if got, want := MrkdwnToCommonMark("ping <!subteam^S123|@team> please", nil, nil), "ping @team please"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommonMarkUsergroupMentionBareResolved(t *testing.T) {
	got := MrkdwnToCommonMarkWithUserGroups("dear <!subteam^S0TESTGRP01> hello", nil, nil,
		map[string]string{"S0TESTGRP01": "platform-team"})
	if want := "dear @platform-team hello"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCommonMarkUsergroupMentionBareUnresolved(t *testing.T) {
	if got, want := MrkdwnToCommonMark("dear <!subteam^S0MISSING> hello", nil, nil), "dear @group hello"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Labels arrive conventionally "@"-prefixed on the wire but that is
// not guaranteed; the conversion must not double the "@".
func TestCommonMarkUsergroupMentionLabelWithoutAt(t *testing.T) {
	if got, want := MrkdwnToCommonMark("ping <!subteam^S123|eng>", nil, nil), "ping @eng"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
