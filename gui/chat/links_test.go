package chat

import "testing"

func TestLinkTargetAcceptsOwnWorkspaceOnly(t *testing.T) {
	ch, ts, thread, ok := LinkTarget("https://acme.slack.com/archives/C1/p1700000000000100", "acme")
	if !ok || ch != "C1" || ts != "1700000000.000100" || thread != "" {
		t.Fatalf("own workspace: %q %q %q %v", ch, ts, thread, ok)
	}
	if _, _, _, ok := LinkTarget("https://ACME.slack.com/archives/C1/p1700000000000100", "acme"); !ok {
		t.Fatal("the subdomain's case mattered")
	}
	for _, domain := range []string{"other", ""} {
		if _, _, _, ok := LinkTarget("https://acme.slack.com/archives/C1/p1700000000000100", domain); ok {
			t.Fatalf("a link to acme opened in-app for %q", domain)
		}
	}
}

func TestLinkTargetReadsThreadReplies(t *testing.T) {
	ch, ts, thread, ok := LinkTarget("https://acme.slack.com/archives/C1/p1700000005000200?thread_ts=1700000000.000100&cid=C1", "acme")
	if !ok || ch != "C1" || ts != "1700000005.000200" || thread != "1700000000.000100" {
		t.Fatalf("reply: %q %q %q %v", ch, ts, thread, ok)
	}
}

func TestLinkTargetRejectsNonArchiveURLs(t *testing.T) {
	for _, url := range []string{
		"https://acme.slack.com/archives/C1",
		"https://acme.slack.com/client/T1/C1",
		"https://app.slack.com/client/T1/C1",
		"http://acme.slack.com/archives/C1/p1700000000000100",
		"https://acme.slack.com.evil.example/archives/C1/p1700000000000100",
		"https://example.com/archives/C1/p1700000000000100",
		"not a url",
	} {
		if _, _, _, ok := LinkTarget(url, "acme"); ok {
			t.Errorf("LinkTarget(%q) opened in-app", url)
		}
	}
}
