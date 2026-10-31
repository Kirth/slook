package chat

import (
	"slices"
	"testing"

	"github.com/gammons/slk/internal/core"
)

func TestAttachmentsRefuseLargeFiles(t *testing.T) {
	var as Attachments
	err := as.Add(core.PendingAttachment{Filename: "big.png", Bytes: make([]byte, 1), Size: 12_900_000})
	if err == nil || err.Error() != "Image too large (12.3 MB > 10 MB limit)" {
		t.Fatalf("Add(big image) = %v", err)
	}
	err = as.Add(core.PendingAttachment{Filename: "big.iso", Path: "/tmp/big.iso", Size: 12_900_000})
	if err == nil || err.Error() != "File too large (12.3 MB > 10 MB limit)" {
		t.Fatalf("Add(big file) = %v", err)
	}
	if !as.Empty() {
		t.Fatalf("refused files were attached: %+v", as.Items)
	}
	if err := as.Add(core.PendingAttachment{Filename: "ok.png", Bytes: []byte{1}, Size: 10 << 20}); err != nil {
		t.Fatalf("a file of exactly 10 MB was refused: %v", err)
	}
	if as.Total() != 10<<20 {
		t.Fatalf("Total = %d", as.Total())
	}
}

func TestAttachmentsRenameDuplicates(t *testing.T) {
	var as Attachments
	for _, n := range []string{"shot.png", "shot.png", "shot.png", "notes", "notes"} {
		if err := as.Add(core.PendingAttachment{Filename: n, Path: "/tmp/" + n, Size: 1}); err != nil {
			t.Fatal(err)
		}
	}
	var names []string
	for _, a := range as.Items {
		names = append(names, a.Filename)
	}
	if want := []string{"shot.png", "shot-2.png", "shot-3.png", "notes", "notes-2"}; !slices.Equal(names, want) {
		t.Fatalf("names %q, want %q", names, want)
	}
	as.Remove(1)
	as.Remove(9) // out of range: ignored
	if err := as.Add(core.PendingAttachment{Filename: "shot.png", Size: 1}); err != nil {
		t.Fatal(err)
	}
	if got := as.Items[len(as.Items)-1].Filename; got != "shot-2.png" || len(as.Items) != 5 {
		t.Fatalf("after removing shot-2.png, a new shot.png is %q (%d items)", got, len(as.Items))
	}
}

func TestUploadNote(t *testing.T) {
	for _, tc := range []struct {
		done, total int
		want        string
	}{
		{0, 2, "Uploading…"},
		{1, 2, "Uploading 1/2…"},
		{2, 2, "Uploading 2/2…"},
		{1, 1, "Uploading 1/1…"},
	} {
		if got := UploadNote(tc.done, tc.total); got != tc.want {
			t.Errorf("UploadNote(%d, %d) = %q, want %q", tc.done, tc.total, got, tc.want)
		}
	}
}
