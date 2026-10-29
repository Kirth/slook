package main

import (
	"fmt"
	"os"
	"time"
)

// tracing is on with SLK_GIO_TRACE set: timing lines on stderr for how
// long windows take to appear.
var tracing = os.Getenv("SLK_GIO_TRACE") != ""

// tracef prints a timing line: what happened, and how long after since.
func tracef(since time.Time, format string, args ...any) {
	if !tracing {
		return
	}
	fmt.Fprintf(os.Stderr, "slk-gio: %7.1fms  %s\n", float64(time.Since(since).Microseconds())/1000, fmt.Sprintf(format, args...))
}
