package main

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/godbus/dbus/v5"
)

const (
	instanceName  = "io.github.gammons.slk.Gio"
	instancePath  = dbus.ObjectPath("/io/github/gammons/slk/Gio")
	activateLimit = 5 * time.Second
)

// claimInstance makes this process the one slk-gio for id (a workspace's
// team ID, or "fake"), by owning a name on the session bus. If another
// process owns it, that one is asked to bring its list forward and
// claimInstance reports false: this process should exit. activate is
// what a later launch asks of this one. Without a session bus every
// launch runs.
func claimInstance(id string, activate func()) bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return true
	}
	name := instanceName + "." + busSegment(id)
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil {
		conn.Close()
		return true
	}
	if reply == dbus.RequestNameReplyPrimaryOwner {
		// The connection stays open for the life of the process: closing
		// it gives the name up.
		_ = conn.Export(instance{activate}, instancePath, instanceName) // fails only on a bad path
		return true
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), activateLimit)
	defer cancel()
	_ = conn.Object(name, instancePath).CallWithContext(ctx, instanceName+".Activate", 0).Err
	return false
}

// instance is the object a later launch calls.
type instance struct{ activate func() }

func (i instance) Activate() *dbus.Error {
	go i.activate()
	return nil
}

// busSegment makes s one element of a bus name: letters, digits and
// underscores, not starting with a digit.
func busSegment(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			return r
		}
		return '_'
	}, s)
	if s == "" || unicode.IsDigit(rune(s[0])) {
		s = "_" + s
	}
	return s
}
