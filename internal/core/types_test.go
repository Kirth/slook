package core

import "testing"

func TestCommandErrorMessageIsTheCode(t *testing.T) {
	if got := (&CommandError{Code: "unknown_command"}).Error(); got != "unknown_command" {
		t.Errorf("Error() = %q, want unknown_command", got)
	}
}
