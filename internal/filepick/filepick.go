// Package filepick asks the user to choose files with the desktop's own
// file chooser, run as a separate program (zenity, else kdialog, else
// yad), so a frontend needs no toolkit dialog of its own.
package filepick

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

// ErrNoChooser is Choose's answer when none of the choosers is
// installed.
var ErrNoChooser = errors.New("no file chooser found (install zenity)")

// The process's environment, swapped by tests.
var (
	lookPath = exec.LookPath
	run      = runCmd
)

// runCmd runs name with args and returns what it printed and its exit
// code (-1 when it did not run to an exit).
func runCmd(ctx context.Context, name string, args ...string) ([]byte, int, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &out
	err := cmd.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return out.Bytes(), exit.ExitCode(), err
	}
	if err != nil {
		return nil, -1, err
	}
	return out.Bytes(), 0, nil
}

// choosers, in the order tried. Each prints the chosen paths one per
// line.
var choosers = []struct {
	name string
	args []string
}{
	{"zenity", []string{"--file-selection", "--multiple", "--separator=\n", "--title=Attach files"}},
	{"kdialog", []string{"--getopenfilename", "--multiple", "--separate-output", "--title", "Attach files"}},
	{"yad", []string{"--file", "--multiple", "--separator=\n", "--title=Attach files"}},
}

// Choose shows a file chooser and returns the files picked. Closing it
// without choosing is no files and no error. It blocks until the user
// answers, so call it off the UI goroutine.
func Choose(ctx context.Context) ([]string, error) {
	for _, c := range choosers {
		if _, err := lookPath(c.name); err != nil {
			continue
		}
		out, code, err := run(ctx, c.name, c.args...)
		// All three exit 1 on Cancel; yad 252 on Escape.
		if code == 1 || code == 252 {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var paths []string
		for p := range strings.SplitSeq(string(out), "\n") {
			if p = strings.TrimSpace(p); p != "" {
				paths = append(paths, p)
			}
		}
		return paths, nil
	}
	return nil, ErrNoChooser
}
