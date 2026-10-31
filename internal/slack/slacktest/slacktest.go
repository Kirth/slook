// Package slacktest is a fake Slack for tests: a TLS server that answers
// a fixed body per path and records what it was asked, and a real
// *slackclient.Client whose every request lands on it.
//
// The redirection happens at the transport, not by pointing the client
// at another URL: the workspace API defaults to https://slack.com/api/
// and edge.Client is hardcoded to https://edgeapi.slack.com, neither
// settable from outside internal/slack. NewClient replaces the
// BrowserTransport's Inner with one that dials the server whatever host
// it was asked for, so the BrowserTransport itself — its Chrome header
// set, its telemetry envelope and slackhttp.DefaultCounter — stays in
// the path, which is what tests of the wiring are checking survives.
//
// Paths are the request paths as the server sees them: "/api/<method>"
// for the Web API, "/cache/<team>/<endpoint>" for edgeapi.
package slacktest

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/slackhttp"
)

// UserBootBody is a minimal client.userBoot answer: self U1 in team T1
// (Acme), channel C1 "general" (topic "talk"), an open DM D1 with U2,
// C1 muted in all_notifications_prefs.
const UserBootBody = `{
  "ok": true,
  "self": {"id":"U1","name":"grant","team_id":"T1"},
  "team": {"id":"T1","name":"Acme","domain":"acme"},
  "channels": [
    {"id":"C1","name":"general","is_channel":true,"updated":1783337533019,"topic":{"value":"talk"}}
  ],
  "ims": [{"id":"D1","user":"U2","is_im":true,"is_open":true,"updated":1783337533020}],
  "is_open": ["C1","D1"],
  "prefs": {"all_notifications_prefs":"{\"channels\":{\"C1\":{\"muted\":true}}}"},
  "emoji_cache_ts": "17833375330191740"
}`

// Request is one request the fake Slack received.
type Request struct {
	Path   string
	Form   url.Values // a form-encoded POST body (the Web API)
	Body   []byte     // the raw body (edgeapi posts JSON as text/plain)
	Query  url.Values
	Header http.Header
}

// Server is a TLS httptest server that answers a fixed body per path
// and records every request.
type Server struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []Request
}

// NewServer starts a fake Slack answering bodies[path] (as JSON) and
// closes it when the test ends. A request to a path not in bodies fails
// the test and gets a 404 with ok:false.
func NewServer(t testing.TB, bodies map[string]string) *Server {
	t.Helper()
	s := &Server{}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		_ = r.ParseForm()
		rec := Request{
			Path:   r.URL.Path,
			Form:   r.PostForm,
			Body:   raw,
			Query:  r.URL.Query(),
			Header: r.Header.Clone(),
		}
		s.mu.Lock()
		s.reqs = append(s.reqs, rec)
		s.mu.Unlock()

		body, ok := bodies[r.URL.Path]
		if !ok {
			t.Errorf("fake Slack got an unexpected request to %s", r.URL.Path)
			http.Error(w, `{"ok":false,"error":"unexpected_path"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

// RequestTo returns the first request to path, failing the test when
// there was none.
func (s *Server) RequestTo(t testing.TB, path string) Request {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.reqs {
		if r.Path == path {
			return r
		}
	}
	paths := make([]string, 0, len(s.reqs))
	for _, r := range s.reqs {
		paths = append(paths, r.Path)
	}
	t.Fatalf("no request to %s; got %v", path, paths)
	return Request{}
}

// Paths lists every request's path, in arrival order.
func (s *Server) Paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.reqs))
	for _, r := range s.reqs {
		out = append(out, r.Path)
	}
	return out
}

// Count is how many requests went to path.
func (s *Server) Count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.reqs {
		if r.Path == path {
			n++
		}
	}
	return n
}

// NewClient returns a real *slackclient.Client (token "xoxc-test",
// cookie "d-cookie") whose every request lands on srv, which must be a
// TLS server (NewServer's, or any httptest.NewTLSServer).
func NewClient(t testing.TB, srv *httptest.Server) *slackclient.Client {
	t.Helper()
	c := slackclient.NewClient("xoxc-test", "d-cookie")
	bt, ok := c.HTTPClient().Transport.(*slackhttp.BrowserTransport)
	if !ok {
		t.Fatalf("client transport is %T; want *slackhttp.BrowserTransport", c.HTTPClient().Transport)
	}
	addr := srv.Listener.Addr().String()
	bt.Inner = &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		//nolint:gosec // dialing a local httptest server with a self-signed cert
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return c
}
