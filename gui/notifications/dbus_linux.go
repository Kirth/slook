package notifications

import (
	"strings"
	"sync"
	"time"

	dnotify "github.com/esiqveland/notify"
	"github.com/godbus/dbus/v5"
)

// dbusSink shows notes through the desktop's notification service
// (org.freedesktop.Notifications on the session bus): one notification
// per conversation, replaced as new messages arrive, taken back once
// the conversation is read, and opening it when clicked.
type dbusSink struct {
	n    dnotify.Notifier
	open func(Note)

	mu        sync.Mutex
	byChannel map[string]uint32 // a conversation's notification on screen
	notes     map[uint32]Note
}

// newDBusSink connects to the session bus. It fails when there is no
// bus or no notification service on it.
func newDBusSink(open func(Note)) (Sink, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	if _, err := dnotify.GetServerInformation(conn); err != nil {
		conn.Close()
		return nil, err
	}
	s := &dbusSink{open: open, byChannel: map[string]uint32{}, notes: map[uint32]Note{}}
	s.n, err = dnotify.New(conn, dnotify.WithOnAction(s.action), dnotify.WithOnClosed(s.closed), dnotify.WithLogger(quiet{}))
	if err != nil {
		conn.Close()
		return nil, err
	}
	return s, nil
}

// markup escapes text for a notification body, which servers may read
// as markup.
var markup = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func (s *dbusSink) Show(n Note) error {
	s.mu.Lock()
	replaces := s.byChannel[n.ChannelID]
	s.mu.Unlock()
	id, err := s.n.SendNotification(dnotify.Notification{
		AppName:       "slk",
		ReplacesID:    replaces,
		Summary:       n.Title,
		Body:          markup.Replace(n.Body),
		Actions:       []dnotify.Action{{Key: "default", Label: "Open"}},
		Hints:         map[string]dbus.Variant{"category": dbus.MakeVariant("im.received")},
		ExpireTimeout: -time.Millisecond, // the server's default
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.notes, replaces)
	s.byChannel[n.ChannelID] = id
	s.notes[id] = n
	s.mu.Unlock()
	return nil
}

func (s *dbusSink) Clear(channelID string) {
	s.mu.Lock()
	id, ok := s.byChannel[channelID]
	delete(s.byChannel, channelID)
	delete(s.notes, id)
	s.mu.Unlock()
	if ok {
		_, _ = s.n.CloseNotification(id)
	}
}

// action is a notification's action invoked: "default" is a click on it.
func (s *dbusSink) action(sig *dnotify.ActionInvokedSignal) {
	if sig.ActionKey != "default" {
		return
	}
	s.mu.Lock()
	n, ok := s.notes[sig.ID]
	s.mu.Unlock()
	if ok && s.open != nil {
		go s.open(n)
	}
}

// closed forgets a notification that went away (dismissed, expired).
func (s *dbusSink) closed(sig *dnotify.NotificationClosedSignal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n, ok := s.notes[sig.ID]; ok {
		delete(s.notes, sig.ID)
		if s.byChannel[n.ChannelID] == sig.ID {
			delete(s.byChannel, n.ChannelID)
		}
	}
}

// quiet drops the D-Bus library's log lines.
type quiet struct{}

func (quiet) Printf(string, ...any) {}
