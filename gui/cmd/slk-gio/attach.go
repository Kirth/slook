package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/gammons/slk/gui/chat"
	"github.com/gammons/slk/internal/attach"
	osclip "github.com/gammons/slk/internal/clipboard"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/filepick"
)

// iconAttach is the composer's paperclip: choose files to send.
var iconAttach = mustIcon(icons.EditorAttachFile)

type (
	// attachPicked is files to attach (from a paste or the chooser),
	// and a note when something offered could not be.
	attachPicked struct {
		files []core.PendingAttachment
		note  string
		chose bool // the chooser closed
	}
	// pasteText asks the composer to paste the clipboard's text: Ctrl+V
	// found no image and no file path on it.
	pasteText struct{}
	// uploadProgress is file done of total starting to upload.
	uploadProgress struct{ done, total int }
	// uploadDone ends an upload; a failure keeps the files and caption.
	uploadDone struct{ err error }
)

// post hands an item to the window that holds this composer now: a
// result that outlives a moveHere reaches the replacement.
func (b *composeBox) post(item any) {
	if b.deliver != nil {
		b.deliver(item)
		return
	}
	b.box.post(item)
}

// attachKeys takes Ctrl+V from the editor (before it pastes): the
// clipboard's image or the file its text names becomes an attachment,
// anything else is pasted as text after all (pasteText). Gio's own
// clipboard read is text-only, and with an image on the clipboard no
// text type is offered at all, so the editor alone would paste nothing.
// While editing a message the editor keeps Ctrl+V: an edit cannot add
// files.
func (b *composeBox) attachKeys(gtx C) {
	if b.pasteNext {
		b.pasteNext = false
		gtx.Execute(clipboard.ReadCmd{Tag: &b.composer})
	}
	if b.editing != nil {
		return
	}
	for {
		ev, ok := gtx.Event(key.Filter{Focus: &b.composer, Name: "V", Required: key.ModShortcut})
		if !ok {
			break
		}
		if k, ok := ev.(key.Event); ok && k.State == key.Press && !b.uploading {
			go b.paste()
		}
	}
	for i := range b.removeChips {
		if b.removeChips[i].Clicked(gtx) && !b.uploading {
			b.files.Remove(i)
			gtx.Execute(key.FocusCmd{Tag: &b.composer})
			break
		}
	}
	if b.attachBtn.Clicked(gtx) && !b.uploading && !b.choosing {
		b.choosing = true
		go b.choose()
	}
}

// paste reads the clipboard off the UI goroutine: an image, else a
// file path as text, else text to paste.
func (b *composeBox) paste() {
	if png, ok := osclip.ReadImage(); ok {
		b.post(attachPicked{files: []core.PendingAttachment{attach.FromImage(png, time.Now())}})
		return
	}
	if p, ok := attach.ResolvePath(osclip.ReadText()); ok {
		a, err := attach.FromPath(p)
		switch {
		case err == nil:
			b.post(attachPicked{files: []core.PendingAttachment{a}})
			return
		case errors.Is(err, attach.ErrEmpty):
			b.post(attachPicked{note: "Empty file: " + filepath.Base(p)})
			return
		}
		// No such file: the text is pasted as it is.
	}
	b.post(pasteText{})
}

// choose shows the desktop's file chooser and attaches what is picked.
func (b *composeBox) choose() {
	v := attachPicked{chose: true}
	paths, err := filepick.Choose(b.h.ctx)
	switch {
	case errors.Is(err, filepick.ErrNoChooser):
		v.note = "No file chooser found (install zenity)"
	case err != nil:
		v.note = "Could not choose files: " + err.Error()
	}
	for _, p := range paths {
		a, err := attach.FromPath(p)
		switch {
		case errors.Is(err, attach.ErrEmpty):
			v.note = "Empty file: " + filepath.Base(p)
		case err != nil:
			v.note = "Not a file: " + p
		default:
			v.files = append(v.files, a)
		}
	}
	b.post(v)
}

// attachApply takes the composer's own items: attachments picked, the
// text paste, an upload's progress and end. ok is false for others.
func (b *composeBox) attachApply(item any) bool {
	switch v := item.(type) {
	case attachPicked:
		if v.chose {
			b.choosing = false
		}
		if v.note != "" {
			b.note(v.note, true)
		} else if len(v.files) > 0 {
			b.sendErr = "" // an earlier refusal no longer applies
		}
		for _, a := range v.files {
			if err := b.files.Add(a); err != nil {
				b.note(err.Error(), true)
			}
		}
		if len(v.files) > 0 {
			b.focusNext = true
		}
	case pasteText:
		b.pasteNext = true
	case uploadProgress:
		b.uploadNote = chat.UploadNote(v.done, v.total)
	case uploadDone:
		b.setUploading(false)
		if v.err != nil {
			b.note("Not sent: "+v.err.Error(), true)
			break
		}
		b.files = chat.Attachments{}
		b.composer.SetText("")
		b.sendErr = ""
	default:
		return false
	}
	return true
}

// uploadState is the attachments a window hands to its replacement on
// moveHere, an upload in flight with them: its uploadDone is posted
// through hub.toChat / toThread, so it lands in the new window.
type uploadState struct {
	files     chat.Attachments
	uploading bool
	note      string
}

func (b *composeBox) uploadState() uploadState {
	return uploadState{files: b.files, uploading: b.uploading, note: b.uploadNote}
}

func (b *composeBox) adoptUpload(st uploadState) {
	b.files = st.files
	b.uploading, b.composer.ReadOnly, b.uploadNote = st.uploading, st.uploading, st.note
}

// setUploading marks an upload in flight: the caption cannot change
// and Enter does nothing until it ends.
func (b *composeBox) setUploading(on bool) {
	b.uploading, b.composer.ReadOnly = on, on
	b.uploadNote = ""
	if on {
		b.uploadNote = chat.UploadNote(0, len(b.files.Items))
	}
}

// submitFiles is Enter with files attached: they go with the text as
// their caption (a slash command is not run; it is the caption too).
func (b *composeBox) submitFiles(raw string) {
	caption := strings.TrimSpace(raw)
	files := slices.Clone(b.files.Items)
	b.sendErr = ""
	b.setUploading(true)
	go b.upload(files, b.mentions.Wire(caption))
}

// upload sends the files, reporting to whichever window holds the
// composer by then.
func (b *composeBox) upload(files []core.PendingAttachment, caption string) {
	err := b.h.be.Upload(b.h.ctx, b.channelID, b.threadTS, files, caption, func(done, total int) {
		b.post(uploadProgress{done, total})
	})
	b.post(uploadDone{err})
}

// chips draws the attached files above the editor: name, size and a ×
// to take one off; muted while they upload.
func (b *composeBox) chips(gtx C) D {
	if b.files.Empty() {
		return D{}
	}
	for len(b.removeChips) < len(b.files.Items) {
		b.removeChips = append(b.removeChips, widget.Clickable{})
	}
	items := make([]layout.Widget, len(b.files.Items))
	for i, a := range b.files.Items {
		cl := &b.removeChips[i]
		items[i] = func(gtx C) D {
			return widget.Border{Color: colBorder, CornerRadius: unit.Dp(10), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
				return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(8), Right: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							lbl := secondaryLabel(b.th, a.Filename)
							lbl.MaxLines = 1
							if b.uploading {
								lbl.Color = colMuted
							}
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx C) D {
							lbl := captionLabel(b.th, attach.HumanSize(a.Size))
							lbl.Color = colMuted
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx C) D {
							if b.uploading {
								return layout.Spacer{Width: unit.Dp(4)}.Layout(gtx)
							}
							return iconButton(gtx, cl, iconClose, unit.Dp(20), unit.Dp(12), colMuted, colBg, colHover)
						}),
					)
				})
			})
		}
	}
	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
		return wrapRow(gtx, unit.Dp(6), items)
	})
}

// attachButton is the paperclip beside the editor; hidden while
// editing a message.
func (b *composeBox) attachButton(gtx C) D {
	if b.editing != nil {
		return D{}
	}
	fg := colMuted
	if b.uploading || b.choosing {
		fg = colBorder
	}
	return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
		return iconButton(gtx, &b.attachBtn, iconAttach, unit.Dp(32), unit.Dp(20), fg, colBg, colHover)
	})
}
