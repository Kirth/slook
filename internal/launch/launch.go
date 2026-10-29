// Package launch opens a URL or file with the desktop's default handler.
package launch

import (
	"os/exec"
	"runtime"
)

// Open starts the platform's default handler for target (a URL or file
// path): open (macOS), rundll32 (Windows), xdg-open (elsewhere). It
// returns once the handler has started, not when it finishes.
func Open(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}
