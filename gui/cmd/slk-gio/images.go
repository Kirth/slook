package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"os"
	"sync"

	"gioui.org/op/paint"

	imgpkg "github.com/gammons/slk/internal/image"
)

// imageState is where an image is between being asked for and drawn.
type imageState int

const (
	imgNone    imageState = iota // no fetcher: nothing will ever load
	imgLoading                   // a download is under way
	imgReady
	imgFailed
)

// imageReady tells a window an image it asked for has arrived (or
// failed); the window just redraws.
type imageReady struct{ key string }

// imageStore hands decoded images to every window. Downloads go through
// the backend's fetcher (its disk cache, auth and dedup); the store adds
// the GPU-ready op and wakes the windows that are waiting.
type imageStore struct {
	ctx     context.Context
	fetcher *imgpkg.Fetcher

	mu      sync.Mutex
	ops     map[string]paint.ImageOp
	failed  map[string]bool
	pending map[string][]*inbox
}

func newImageStore(ctx context.Context, f *imgpkg.Fetcher) *imageStore {
	return &imageStore{
		ctx:     ctx,
		fetcher: f,
		ops:     map[string]paint.ImageOp{},
		failed:  map[string]bool{},
		pending: map[string][]*inbox{},
	}
}

// get returns the image for key, or its state when it is not ready. A
// miss starts the download; who is told when it ends. target, when not
// zero, is the pixel size to downscale to.
func (s *imageStore) get(key, url string, target image.Point, who *inbox) (paint.ImageOp, imageState) {
	if s.fetcher == nil || url == "" {
		return paint.ImageOp{}, imgNone
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if op, ok := s.ops[key]; ok {
		return op, imgReady
	}
	if s.failed[key] {
		return paint.ImageOp{}, imgFailed
	}
	waiting, started := s.pending[key]
	if who != nil {
		for _, w := range waiting {
			if w == who {
				who = nil
				break
			}
		}
		if who != nil {
			waiting = append(waiting, who)
		}
	}
	s.pending[key] = waiting
	if !started {
		go s.fetch(key, url, target)
	}
	return paint.ImageOp{}, imgLoading
}

func (s *imageStore) fetch(key, url string, target image.Point) {
	res, err := s.fetcher.Fetch(s.ctx, imgpkg.FetchRequest{Key: key, URL: url, Target: target})
	s.mu.Lock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "slk-gio: image %s: %v\n", key, err)
		s.failed[key] = true
	} else {
		s.ops[key] = paint.NewImageOp(res.Img)
	}
	waiting := s.pending[key]
	delete(s.pending, key)
	s.mu.Unlock()
	for _, w := range waiting {
		w.post(imageReady{key})
	}
}

// urlKey is the cache key for an image known only by URL (Block Kit
// images, unfurls), in the TUI's "BK-<sha1>" form so the two share
// cache entries.
func urlKey(url string) string {
	sum := sha1.Sum([]byte(url))
	return "BK-" + hex.EncodeToString(sum[:])[:8]
}
