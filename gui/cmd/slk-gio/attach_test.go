package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gammons/slk/gui/backend"
	"github.com/gammons/slk/internal/core"
)

func pngAttachment(name string) core.PendingAttachment {
	return core.PendingAttachment{Filename: name, Bytes: []byte("png"), Mime: "image/png", Size: 3}
}

func TestComposerUploadsAttachedFilesWithTheCaption(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := backend.NewFake(time.Unix(1_700_000_000, 0))
	f.Interval = 0
	posted := make(chan backend.MessagePosted, 4)
	go f.Run(ctx, func(ev backend.Event) {
		if m, ok := ev.(backend.MessagePosted); ok {
			posted <- m
		}
	})
	b := newComposeBox(&hub{ctx: ctx, be: f}, nil, nil, "C1", "")
	delivered := make(chan any, 8)
	b.deliver = func(item any) { delivered <- item }

	b.attachApply(attachPicked{files: []core.PendingAttachment{pngAttachment("a.png"), pngAttachment("a.png")}})
	if len(b.files.Items) != 2 || b.files.Items[1].Filename != "a-2.png" {
		t.Fatalf("attached %+v", b.files.Items)
	}
	b.setText("photos")
	b.submit("photos")
	if !b.uploading || !b.composer.ReadOnly || b.composer.Text() != "photos" || b.uploadNote != "Uploading…" {
		t.Fatalf("after Enter: uploading %v, read-only %v, text %q, note %q", b.uploading, b.composer.ReadOnly, b.composer.Text(), b.uploadNote)
	}
	b.submit("photos") // Enter again while it uploads: nothing

	var notes []string
	for done := false; !done; {
		select {
		case item := <-delivered:
			b.attachApply(item)
			if _, ok := item.(uploadProgress); ok {
				notes = append(notes, b.uploadNote)
			}
			_, done = item.(uploadDone)
		case <-time.After(5 * time.Second):
			t.Fatal("the upload never ended")
		}
	}
	if len(notes) != 2 || notes[0] != "Uploading 1/2…" || notes[1] != "Uploading 2/2…" {
		t.Fatalf("progress notes %q", notes)
	}
	if b.uploading || b.composer.ReadOnly || !b.files.Empty() || b.composer.Text() != "" {
		t.Fatalf("after the upload: uploading %v, files %+v, text %q", b.uploading, b.files.Items, b.composer.Text())
	}
	m := (<-posted).Message
	if m.Text != "photos" || len(m.Body) < 2 {
		t.Fatalf("posted %#v", m)
	}
	select {
	case m := <-posted:
		t.Fatalf("a second upload was posted: %#v", m)
	default:
	}
}

func TestComposerKeepsFilesAndCaptionWhenTheUploadFails(t *testing.T) {
	b := newComposeBox(nil, nil, nil, "C1", "")
	b.attachApply(attachPicked{files: []core.PendingAttachment{pngAttachment("a.png")}})
	b.setText("caption")
	b.setUploading(true)
	b.attachApply(uploadDone{err: errors.New("uploading a.png (1/1): file_upload_failed")})
	if b.uploading || b.composer.ReadOnly || len(b.files.Items) != 1 || b.composer.Text() != "caption" {
		t.Fatalf("after a failure: uploading %v, files %+v, text %q", b.uploading, b.files.Items, b.composer.Text())
	}
	if b.sendErr != "Not sent: uploading a.png (1/1): file_upload_failed" {
		t.Fatalf("note %q", b.sendErr)
	}
}

func TestComposerRefusesLargeFilesAndCarriesFilesOnMove(t *testing.T) {
	b := newComposeBox(nil, nil, nil, "C1", "")
	big := core.PendingAttachment{Filename: "big.png", Bytes: []byte{1}, Size: 11 << 20}
	b.attachApply(attachPicked{files: []core.PendingAttachment{big, pngAttachment("ok.png")}})
	if len(b.files.Items) != 1 || b.sendErr != "Image too large (11.0 MB > 10 MB limit)" {
		t.Fatalf("files %+v, note %q", b.files.Items, b.sendErr)
	}
	b.setUploading(true)

	// moveHere: the new window's composer is uploading the same files.
	moved := newComposeBox(nil, nil, nil, "C1", "")
	moved.adoptUpload(b.uploadState())
	if !moved.uploading || !moved.composer.ReadOnly || len(moved.files.Items) != 1 || moved.uploadNote != "Uploading…" {
		t.Fatalf("moved composer: uploading %v, files %+v, note %q", moved.uploading, moved.files.Items, moved.uploadNote)
	}
	moved.attachApply(pasteText{})
	if !moved.pasteNext {
		t.Fatal("pasteText did not ask for the text paste")
	}
}
