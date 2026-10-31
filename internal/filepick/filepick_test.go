package filepick

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

type answer struct {
	out  string
	code int
}

// fakeChoosers installs the named choosers, answering each run with
// answers[name], and returns the runs made.
func fakeChoosers(t *testing.T, installed []string, answers map[string]answer) *[]string {
	t.Helper()
	oldLook, oldRun := lookPath, run
	t.Cleanup(func() { lookPath, run = oldLook, oldRun })
	lookPath = func(name string) (string, error) {
		if slices.Contains(installed, name) {
			return "/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
	var runs []string
	run = func(_ context.Context, name string, args ...string) ([]byte, int, error) {
		runs = append(runs, name+" "+strings.Join(args, " "))
		a := answers[name]
		if a.code != 0 {
			return []byte(a.out), a.code, errors.New("exit status")
		}
		return []byte(a.out), 0, nil
	}
	return &runs
}

func TestChooseReadsOnePathPerLine(t *testing.T) {
	runs := fakeChoosers(t, []string{"zenity", "kdialog"},
		map[string]answer{"zenity": {out: "/tmp/a.png\n/tmp/My File.pdf\n"}})
	got, err := Choose(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/tmp/a.png", "/tmp/My File.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("Choose = %q, want %q", got, want)
	}
	if len(*runs) != 1 || !strings.HasPrefix((*runs)[0], "zenity --file-selection --multiple") {
		t.Fatalf("runs = %q, want zenity alone", *runs)
	}
}

func TestChooseFallsBackToKdialogThenYad(t *testing.T) {
	runs := fakeChoosers(t, []string{"kdialog", "yad"},
		map[string]answer{"kdialog": {out: "/tmp/a.png\n"}})
	if got, err := Choose(context.Background()); err != nil || !slices.Equal(got, []string{"/tmp/a.png"}) {
		t.Fatalf("Choose = %q, %v", got, err)
	}
	if len(*runs) != 1 || !strings.HasPrefix((*runs)[0], "kdialog --getopenfilename --multiple --separate-output") {
		t.Fatalf("runs = %q, want kdialog", *runs)
	}
	runs = fakeChoosers(t, []string{"yad"}, map[string]answer{"yad": {out: "/tmp/b\n"}})
	if got, _ := Choose(context.Background()); !slices.Equal(got, []string{"/tmp/b"}) || !strings.HasPrefix((*runs)[0], "yad --file --multiple") {
		t.Fatalf("Choose = %q, runs %q", got, *runs)
	}
}

func TestChooseCancelledIsNothing(t *testing.T) {
	for _, code := range []int{1, 252} {
		fakeChoosers(t, []string{"yad"}, map[string]answer{"yad": {code: code}})
		got, err := Choose(context.Background())
		if got != nil || err != nil {
			t.Errorf("exit %d: Choose = %q, %v; want nothing", code, got, err)
		}
	}
}

func TestChooseFailure(t *testing.T) {
	fakeChoosers(t, []string{"zenity"}, map[string]answer{"zenity": {code: 5}})
	if _, err := Choose(context.Background()); err == nil {
		t.Fatal("a chooser that failed was not reported")
	}
}

func TestChooseWithoutAChooser(t *testing.T) {
	runs := fakeChoosers(t, nil, nil)
	if _, err := Choose(context.Background()); !errors.Is(err, ErrNoChooser) {
		t.Fatalf("err = %v, want ErrNoChooser", err)
	}
	if len(*runs) != 0 {
		t.Fatalf("ran %q", *runs)
	}
}
