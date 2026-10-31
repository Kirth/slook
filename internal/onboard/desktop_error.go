package onboard

import (
	"errors"

	"github.com/gammons/slk/internal/slackdesktop"
)

// DesktopErrorMessage maps a slackdesktop error to an actionable message.
func DesktopErrorMessage(err error) string {
	switch {
	case errors.Is(err, slackdesktop.ErrDesktopNotFound):
		return "Slack desktop app not found. Install it and sign in, then retry."
	case errors.Is(err, slackdesktop.ErrNotSignedIn):
		return "No Slack workspaces are signed in. Open Slack, sign in, then retry."
	case errors.Is(err, slackdesktop.ErrCookieDBMissing):
		return "Slack is installed but has never signed in on this machine."
	case errors.Is(err, slackdesktop.ErrKeyringLocked):
		return "Your system keyring is locked. Unlock it (log in to your desktop session) and retry."
	case errors.Is(err, slackdesktop.ErrNoSecretService):
		return "No system keyring/secret service found. slk needs it to read the Slack session."
	case errors.Is(err, slackdesktop.ErrSecretNotFound):
		return "No Slack entry found in your keyring or KWallet. Sign in to the Slack desktop app, then retry. " +
			"If Slack was launched with --password-store=basic it never stored a key at all, and slk cannot read that session."
	case errors.Is(err, slackdesktop.ErrDecryptFailed):
		// Keep the wrapped detail: it names which step failed (padding, length,
		// non-printable result), which is the difference between a diagnosable
		// report and a round-trip asking what actually broke.
		return "Could not decrypt the Slack session cookie: " + err.Error() +
			". If you have had more than one Slack build installed (App Store and standalone), " +
			"sign out of the one you no longer use. Otherwise please file an issue with your OS + Slack version."
	case errors.Is(err, slackdesktop.ErrCookieLocked):
		return "Your Slack cookie is locked by a running process of Slack. Close Slack and try again."
	default:
		return "Could not read Slack desktop session: " + err.Error()
	}
}
