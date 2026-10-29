package filedl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gammons/slk/internal/slackhttp"
)

func TestDownloadWritesFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("a,b,c"))
	}))
	defer srv.Close()
	d := New(slackhttp.NewAuthResolver(nil), t.TempDir())
	path, err := d.Download(context.Background(), srv.URL+"/report.csv", "report.csv")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "a,b,c" {
		t.Errorf("body = %q", body)
	}
	if filepath.Base(path) != "report.csv" {
		t.Errorf("path = %q", path)
	}
}

func TestGetSendsAuthHeaders(t *testing.T) {
	var gotAuth, gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		w.Write([]byte("x"))
	}))
	defer srv.Close()
	d := New(slackhttp.NewAuthResolver(nil), t.TempDir())
	_, _, _, err := d.get(context.Background(), srv.URL, slackhttp.TeamAuth{TeamID: "T1", Token: "xoxc-test", DCookie: "cookie-test"})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer xoxc-test" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotCookie != "d=cookie-test" {
		t.Errorf("Cookie = %q", gotCookie)
	}
}

func TestDownloadCollisionSuffixes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	d := New(slackhttp.NewAuthResolver(nil), dir)
	p1, err := d.Download(context.Background(), srv.URL, "a.csv")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := d.Download(context.Background(), srv.URL, "a.csv")
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatalf("second download overwrote first: %q", p1)
	}
	if filepath.Base(p2) != "a-2.csv" {
		t.Errorf("p2 = %q", p2)
	}
}

func TestDownloadHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	d := New(slackhttp.NewAuthResolver(nil), t.TempDir())
	if _, err := d.Download(context.Background(), srv.URL, "x.csv"); err == nil {
		t.Fatal("expected error on 403")
	}
}

func TestDownloadHTMLLoginPageIsAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html><body>Slack login</body></html>"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	d := New(slackhttp.NewAuthResolver(nil), dir)
	if _, err := d.Download(context.Background(), srv.URL+"/report.csv", "report.csv"); err == nil {
		t.Fatal("expected error on 200 text/html login page")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no file written, found %d entries", len(entries))
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"report.csv":     "report.csv",
		"../evil.csv":    "_evil.csv",
		"a/b\\c.csv":     "a_b_c.csv",
		"  spaced.csv  ": "spaced.csv",
		"":               "download",
		"...":            "download",
		"bell\aname.csv": "bell_name.csv",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFetchIsIdempotent(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("PNGDATA"))
	}))
	defer srv.Close()
	d := New(slackhttp.NewAuthResolver(nil), t.TempDir())
	path := filepath.Join(t.TempDir(), "F1", "cat.png") // the directory does not exist yet
	for range 2 {
		if err := d.Fetch(context.Background(), srv.URL+"/cat.png", path); err != nil {
			t.Fatal(err)
		}
	}
	if hits != 1 {
		t.Errorf("downloaded %d times, want once", hits)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "PNGDATA" {
		t.Fatalf("file = %q, %v", body, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("left %d files beside it, want only the file", len(entries))
	}
}

func TestFetchLoginPageLeavesNoFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html>sign in</html>"))
	}))
	defer srv.Close()
	d := New(slackhttp.NewAuthResolver(nil), t.TempDir())
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := d.Fetch(context.Background(), srv.URL+"/doc.pdf", path); err == nil {
		t.Fatal("want an error for the login page")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a file was left at %s", path)
	}
}
