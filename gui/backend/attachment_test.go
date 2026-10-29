package backend

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAttachmentPath(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	cases := []struct {
		a    Attachment
		want string
	}{
		{Attachment{Key: "F123", URL: "https://files.slack.com/x", Name: "cat.png"}, "/run/user/1000/slk-gio/files/F123/cat.png"},
		{Attachment{Key: "F123", Name: "../../etc/passwd"}, "/run/user/1000/slk-gio/files/F123/_.._etc_passwd"}, // never escapes its directory
		{Attachment{Key: "F9", Name: "Q3 report.pdf"}, "/run/user/1000/slk-gio/files/F9/Q3 report.pdf"},
	}
	for _, tc := range cases {
		if got := AttachmentPath(tc.a); got != tc.want {
			t.Errorf("AttachmentPath(%+v) = %q, want %q", tc.a, got, tc.want)
		}
	}
	// A plain URL: keyed by its hash, named after its last segment, and
	// the same path every time.
	a := Attachment{URL: "https://example.com/img/chart.svg?v=2"}
	got := AttachmentPath(a)
	if filepath.Base(got) != "chart.svg" || !strings.HasPrefix(filepath.Base(filepath.Dir(got)), "u-") || got != AttachmentPath(a) {
		t.Errorf("plain URL path = %q", got)
	}
	if AttachmentPath(Attachment{URL: "https://example.com/other.svg"}) == got {
		t.Error("different URLs share a path")
	}
}

func TestFakeDownloadOnce(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	f := NewFake(time.Unix(1_700_000_000, 0))
	a := Attachment{Key: "FAKE1", URL: "fake://sunset.png", Name: "sunset.png"}
	p, err := f.Download(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	if p != AttachmentPath(a) {
		t.Fatalf("path %q, want %q", p, AttachmentPath(a))
	}
}
