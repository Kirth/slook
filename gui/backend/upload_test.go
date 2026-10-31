package backend

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/slack/slacktest"
)

// TestUpload_SendsEachFileWithTheCaptionOnTheLast uploads a pasted
// image and a file by path through Slack's three steps, the caption
// only on the second, reporting progress as each starts.
func TestUpload_SendsEachFileWithTheCaptionOnTheLast(t *testing.T) {
	bodies := bootBodies()
	srv := slacktest.NewServer(t, bodies)
	bodies["/api/files.getUploadURLExternal"] = `{"ok":true,"upload_url":"https://files.slack.com/upload/v1/abc","file_id":"F1"}`
	bodies["/upload/v1/abc"] = `OK` // every host dials srv
	bodies["/api/files.completeUploadExternal"] = `{"ok":true,"files":[{"id":"F1","title":"x"}]}`
	bodies["/api/conversations.history"] = `{"ok":true,"messages":[],"has_more":false}`
	s, _ := bootSlack(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	pdf := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.7 hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []core.PendingAttachment{
		{Filename: "slk-paste.png", Bytes: []byte("\x89PNG...."), Mime: "image/png", Size: 9},
		{Filename: "report.pdf", Path: pdf, Size: 3}, // grew since it was attached
	}
	var progress [][2]int
	if err := s.Upload(ctx, "C1", "", files, "photos <@U2>", func(d, n int) { progress = append(progress, [2]int{d, n}) }); err != nil {
		t.Fatal(err)
	}
	if want := [][2]int{{1, 2}, {2, 2}}; !slices.Equal(progress, want) {
		t.Fatalf("progress %v, want %v", progress, want)
	}
	gets := srv.Requests("/api/files.getUploadURLExternal")
	if len(gets) != 2 || gets[0].Form.Get("filename") != "slk-paste.png" || gets[1].Form.Get("length") != "14" {
		t.Fatalf("getUploadURLExternal requests %+v", gets)
	}
	done := srv.Requests("/api/files.completeUploadExternal")
	if len(done) != 2 {
		t.Fatalf("%d completeUploadExternal requests, want 2", len(done))
	}
	if c := done[0].Form.Get("initial_comment"); c != "" {
		t.Fatalf("first file carried the caption %q", c)
	}
	if c := done[1].Form.Get("initial_comment"); c != "photos <@U2>" || done[1].Form.Get("channel_id") != "C1" {
		t.Fatalf("last file: initial_comment %q, form %v", c, done[1].Form)
	}

	files[1].Path = filepath.Join(t.TempDir(), "gone.pdf")
	err := s.Upload(ctx, "C1", "", files, "", nil)
	if err == nil || !strings.HasPrefix(err.Error(), "uploading report.pdf (2/2): ") {
		t.Fatalf("err = %v, want it to name the file and its place", err)
	}
}
