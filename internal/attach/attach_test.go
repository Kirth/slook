package attach

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolvePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"/tmp/a.pdf", "/tmp/a.pdf", true},
		{"  /tmp/../tmp/a.pdf \n", "/tmp/a.pdf", true},
		{"~/shot.png", filepath.Join(home, "shot.png"), true},
		{"./notes.txt", "notes.txt", true},
		{"file:///tmp/My%20File.pdf", "/tmp/My File.pdf", true},
		{"notes.txt", "", false},
		{"hello world", "", false},
		{"/tmp/a\n/tmp/b", "", false},
		{"", "", false},
		{"/" + string(make([]byte, 5000)), "", false},
	} {
		got, ok := ResolvePath(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ResolvePath(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{
		0:                 "<1 KB",
		1023:              "<1 KB",
		1024:              "1 KB",
		243 * 1024:        "243 KB",
		MaxSize:           "10.0 MB",
		12_900_000:        "12.3 MB",
		1024*1024 - 1:     "1023 KB",
		3*1024*1024 + 512: "3.0 MB",
	} {
		if got := HumanSize(n); got != want {
			t.Errorf("HumanSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFromPath(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.7"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := FromPath(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if a.Filename != "report.pdf" || a.Path != pdf || a.Size != 8 || a.Mime != "application/pdf" || a.Bytes != nil {
		t.Fatalf("FromPath = %+v", a)
	}

	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := FromPath(empty); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty file: err = %v, want ErrEmpty", err)
	}
	for _, p := range []string{dir, filepath.Join(dir, "missing.png")} {
		if _, err := FromPath(p); !errors.Is(err, ErrNotAFile) {
			t.Errorf("FromPath(%s): err = %v, want ErrNotAFile", p, err)
		}
	}
}

func TestFromImage(t *testing.T) {
	at := time.Date(2026, 4, 29, 15, 30, 45, 0, time.UTC)
	a := FromImage([]byte("png!"), at)
	if a.Filename != "slk-paste-2026-04-29-15-30-45.png" || string(a.Bytes) != "png!" || a.Size != 4 || a.Mime != "image/png" || a.Path != "" {
		t.Fatalf("FromImage = %+v", a)
	}
}
