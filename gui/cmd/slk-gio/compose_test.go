package main

import (
	"errors"
	"testing"

	"github.com/gammons/slk/gui/backend"
)

func TestEditSetsTheDraftAsideAndBringsItBack(t *testing.T) {
	b := newComposeBox(nil, nil, nil, "C1", "")
	b.setText("half a thought")
	mine := backend.Message{ChannelID: "C1", TS: "1.0", Text: "a &lt; b, see <@U1>", FromSelf: true}

	b.beginEdit(mine)
	if b.editing == nil || b.composer.Text() != "a < b, see <@U1>" {
		t.Fatalf("editing %v with %q", b.editing, b.composer.Text())
	}
	// Starting another edit keeps the first draft, not the first edit.
	b.beginEdit(backend.Message{TS: "2.0", Text: "other"})
	b.endEdit()
	if b.editing != nil || b.composer.Text() != "half a thought" {
		t.Fatalf("after the edit: editing %v, text %q", b.editing, b.composer.Text())
	}
}

func TestEditEndsWhenItsMessageIsDeleted(t *testing.T) {
	b := newComposeBox(nil, nil, nil, "C1", "")
	b.beginEdit(backend.Message{TS: "1.0", Text: "x"})
	b.deleted("2.0")
	if b.editing == nil {
		t.Fatal("another message's deletion ended the edit")
	}
	b.deleted("1.0")
	if b.editing != nil || b.sendErr == "" {
		t.Fatalf("editing %v, note %q", b.editing, b.sendErr)
	}
}

func TestFailedEditGoesBackToEditing(t *testing.T) {
	b := newComposeBox(nil, nil, nil, "C1", "")
	b.setText("draft")
	m := backend.Message{TS: "1.0", Text: "old"}
	b.failedEdit(editFailed{m: m, text: "new", err: errors.New("cant_update_message")})
	if b.editing == nil || b.editing.TS != "1.0" || b.composer.Text() != "new" || b.sendErr == "" {
		t.Fatalf("editing %v, text %q, note %q", b.editing, b.composer.Text(), b.sendErr)
	}
	b.endEdit()
	if b.composer.Text() != "draft" {
		t.Fatalf("draft %q", b.composer.Text())
	}
}

func TestUnchangedOrEmptyEditSendsNothing(t *testing.T) {
	b := newComposeBox(nil, nil, nil, "C1", "")
	b.beginEdit(backend.Message{TS: "1.0", Text: "same"})
	b.submit("") // emptied: refused, still editing
	if b.editing == nil || b.sendErr == "" {
		t.Fatalf("emptied edit: editing %v, note %q", b.editing, b.sendErr)
	}
	b.submit("same \n") // unchanged: ends without a request (h is nil)
	if b.editing != nil {
		t.Fatal("unchanged edit did not end")
	}
}
