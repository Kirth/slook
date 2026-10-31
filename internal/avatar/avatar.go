// Package avatar names Slack user avatars: the sized CDN variant to
// fetch in place of an original (SizedURL) and the disk-cache key one
// user's avatar at one URL is stored under (FetchKey). The download and
// the cache are internal/image's Fetcher and Cache.
package avatar

import (
	"crypto/sha256"
	"encoding/hex"
)

// FetchKey is the disk-cache key for one user's avatar at one URL. The
// URL hash means a changed avatar (a new URL) is fetched fresh, and the
// older `avatar-<userID>` entries, which held full-size originals, are
// never read again; the image cache's LRU evicts them.
func FetchKey(userID, url string) string {
	sum := sha256.Sum256([]byte(url))
	return "avatar-" + userID + "-" + hex.EncodeToString(sum[:])[:12]
}
