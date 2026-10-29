package notifications

import (
	"bufio"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// fakeServer is org.freedesktop.Notifications on a private bus,
// recording what it is asked.
type fakeServer struct {
	mu     sync.Mutex
	next   uint32
	calls  []notifyCall
	closed []uint32
}

type notifyCall struct {
	app, summary, body string
	replaces           uint32
	actions            []string
	hints              map[string]dbus.Variant
}

func (f *fakeServer) Notify(app string, replaces uint32, icon, summary, body string, actions []string, hints map[string]dbus.Variant, timeout int32) (uint32, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, notifyCall{app, summary, body, replaces, actions, hints})
	if replaces != 0 {
		return replaces, nil
	}
	f.next++
	return f.next, nil
}

func (f *fakeServer) CloseNotification(id uint32) *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, id)
	return nil
}

func (f *fakeServer) GetServerInformation() (string, string, string, string, *dbus.Error) {
	return "fake", "slk", "1", "1.2", nil
}

// privateBus starts a session bus of the test's own, so nothing reaches
// the desktop, and points DBUS_SESSION_BUS_ADDRESS at it.
func privateBus(t *testing.T) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon not installed")
	}
	cmd := exec.Command(daemon, "--session", "--nofork", "--print-address", "--address=unix:path="+t.TempDir()+"/bus")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addr, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	addr = strings.TrimSpace(addr)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	return addr
}

func TestDBusSink(t *testing.T) {
	addr := privateBus(t)
	srvConn, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer srvConn.Close()
	srv := &fakeServer{}
	if err := srvConn.Export(srv, "/org/freedesktop/Notifications", "org.freedesktop.Notifications"); err != nil {
		t.Fatal(err)
	}
	if reply, err := srvConn.RequestName("org.freedesktop.Notifications", dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("taking the name: %v, %v", reply, err)
	}

	opened := make(chan Note, 1)
	sink, err := newDBusSink(func(n Note) { opened <- n })
	if err != nil {
		t.Fatal(err)
	}

	first := Note{ChannelID: "D1", TS: "1.0", Title: "Ada", Body: "a < b & c"}
	if err := sink.Show(first); err != nil {
		t.Fatal(err)
	}
	second := Note{ChannelID: "D1", TS: "2.0", ThreadTS: "1.0", Title: "Ada", Body: "again"}
	if err := sink.Show(second); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	calls := srv.calls
	srv.mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("%d Notify calls", len(calls))
	}
	c := calls[0]
	if c.app != "slk" || c.summary != "Ada" || c.body != "a &lt; b &amp; c" || c.replaces != 0 ||
		strings.Join(c.actions, ",") != "default,Open" || c.hints["category"].Value() != "im.received" {
		t.Fatalf("first call %+v", c)
	}
	if calls[1].replaces != 1 {
		t.Fatalf("the second note for D1 replaces %d, want 1", calls[1].replaces)
	}

	// A click on it: the server says the default action was invoked.
	if err := srvConn.Emit("/org/freedesktop/Notifications", "org.freedesktop.Notifications.ActionInvoked", uint32(1), "default"); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-opened:
		if n.TS != "2.0" || n.ThreadTS != "1.0" {
			t.Fatalf("opened %+v, want the latest note", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a click did not open the note")
	}

	sink.Clear("D1")
	sink.Clear("D1") // nothing left to close
	srv.mu.Lock()
	closed := srv.closed
	srv.mu.Unlock()
	if len(closed) != 1 || closed[0] != 1 {
		t.Fatalf("closed %v, want [1]", closed)
	}
}
