//go:build !linux

package notifications

import "errors"

// newDBusSink: the desktop notification service over D-Bus is Linux's.
func newDBusSink(func(Note)) (Sink, error) {
	return nil, errors.New("not on Linux")
}
