package clipboard

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// fakeSession swaps the environment for one with the given variables
// and installed tools, whose runs answer from out by "name args" (a
// missing key fails, as the tools do for a type not offered). It
// returns the runs made.
func fakeSession(t *testing.T, env map[string]string, installed []string, out map[string]string) *[]string {
	t.Helper()
	oldEnv, oldLook, oldRun := getenv, lookPath, run
	t.Cleanup(func() { getenv, lookPath, run = oldEnv, oldLook, oldRun })
	getenv = func(k string) string { return env[k] }
	lookPath = func(name string) (string, error) {
		if slices.Contains(installed, name) {
			return "/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
	var runs []string
	run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		k := name + " " + strings.Join(args, " ")
		runs = append(runs, k)
		if v, ok := out[k]; ok {
			return []byte(v), nil
		}
		return nil, errors.New("exit status 1")
	}
	return &runs
}

const png = "\x89PNG\r\n\x1a\nrest"

func TestReadImageOnWayland(t *testing.T) {
	runs := fakeSession(t, map[string]string{"WAYLAND_DISPLAY": "wayland-1", "DISPLAY": ":0"},
		[]string{"wl-paste", "xclip"},
		map[string]string{"wl-paste --no-newline --type image/png": png})
	b, ok := ReadImage()
	if !ok || string(b) != png {
		t.Fatalf("ReadImage = %q, %v; want the PNG", b, ok)
	}
	if want := []string{"wl-paste --no-newline --type image/png"}; !slices.Equal(*runs, want) {
		t.Fatalf("runs = %q, want %q (wl-paste wins over XWayland's DISPLAY)", *runs, want)
	}
}

func TestReadImageOnX11(t *testing.T) {
	runs := fakeSession(t, map[string]string{"DISPLAY": ":0"}, []string{"xclip"},
		map[string]string{"xclip -selection clipboard -t image/png -o": png})
	if _, ok := ReadImage(); !ok {
		t.Fatal("ReadImage found nothing under X11")
	}
	if want := []string{"xclip -selection clipboard -t image/png -o"}; !slices.Equal(*runs, want) {
		t.Fatalf("runs = %q, want %q", *runs, want)
	}
}

func TestReadImageWithTextOnly(t *testing.T) {
	fakeSession(t, map[string]string{"WAYLAND_DISPLAY": "wayland-1"}, []string{"wl-paste"},
		map[string]string{"wl-paste --no-newline --type text": "/tmp/a.pdf"})
	if b, ok := ReadImage(); ok {
		t.Fatalf("ReadImage = %q with only text on the clipboard", b)
	}
}

func TestReadImageRefusesWhatIsNotAPNG(t *testing.T) {
	fakeSession(t, map[string]string{"WAYLAND_DISPLAY": "wayland-1"}, []string{"wl-paste"},
		map[string]string{"wl-paste --no-newline --type image/png": "GIF89a"})
	if _, ok := ReadImage(); ok {
		t.Fatal("ReadImage took bytes without a PNG header")
	}
}

func TestReadWithoutATool(t *testing.T) {
	runs := fakeSession(t, map[string]string{"WAYLAND_DISPLAY": "wayland-1", "DISPLAY": ":0"}, nil, nil)
	if _, ok := ReadImage(); ok {
		t.Fatal("ReadImage found an image without wl-paste")
	}
	if got := ReadText(); got != "" {
		t.Fatalf("ReadText = %q without a tool", got)
	}
	if len(*runs) != 0 {
		t.Fatalf("ran %q without a tool installed", *runs)
	}
	runs = fakeSession(t, nil, []string{"wl-paste", "xclip"}, nil)
	ReadText()
	if len(*runs) != 0 {
		t.Fatalf("ran %q with no display", *runs)
	}
}

func TestReadText(t *testing.T) {
	fakeSession(t, map[string]string{"WAYLAND_DISPLAY": "wayland-1"}, []string{"wl-paste"},
		map[string]string{"wl-paste --no-newline --type text": "~/report.pdf"})
	if got := ReadText(); got != "~/report.pdf" {
		t.Fatalf("ReadText = %q", got)
	}
	fakeSession(t, map[string]string{"DISPLAY": ":0"}, []string{"xclip"},
		map[string]string{"xclip -selection clipboard -o": "hello"})
	if got := ReadText(); got != "hello" {
		t.Fatalf("ReadText under X11 = %q", got)
	}
}
