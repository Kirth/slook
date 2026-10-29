package notifications

import (
	"fmt"
	"os"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/notify"
)

// NewSink is the sink cfg asks for: notify_command when set (the user
// routes notifications themselves), else the desktop's notification
// service over D-Bus, which can open the conversation on a click, else
// beeep's notification, which cannot. open is called (on its own
// goroutine) with the note whose notification was clicked.
func NewSink(cfg config.Notifications, open func(Note)) Sink {
	if cfg.NotifyCommand == "" {
		s, err := newDBusSink(open)
		if err == nil {
			return s
		}
		fmt.Fprintln(os.Stderr, "slk-gio: notifications: no desktop notification service (", err, "); using the basic kind, which cannot open a conversation")
	}
	return commandSink{notify.New(true, cfg.NotifyCommand)}
}

// commandSink shows notes through internal/notify: notify_command, or
// beeep. It cannot replace, take back or open what it shows.
type commandSink struct{ n *notify.Notifier }

func (c commandSink) Show(n Note) error { return c.n.Notify(n.Title, n.Body) }
func (commandSink) Clear(string)        {}
