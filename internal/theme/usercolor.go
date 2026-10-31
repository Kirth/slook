package theme

import (
	"hash/fnv"
	"image/color"
)

// UserPalette is the fixed set of colors used to deterministically color
// usernames by hashing user IDs, mirroring how Slack clients assign
// stable per-user colors. Mid-brightness colors are chosen for adequate
// contrast on both dark and light theme backgrounds.
var UserPalette = []color.NRGBA{
	hex(0xE01E5A),
	hex(0x3B82F6),
	hex(0x2EB67D),
	hex(0x8B5CF6),
	hex(0xF43F5E),
	hex(0x06B6D4),
	hex(0xF97316),
	hex(0x10B981),
	hex(0x6366F1),
	hex(0xC026D3),
	hex(0x0EA5E9),
	hex(0xDB2777),
}

// hex is an opaque colour from 0xRRGGBB.
func hex(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}
}

// UserColor returns a deterministic color for the given user ID by
// hashing it into UserPalette via FNV-1a. An empty ID hashes like any
// other; a frontend that wants its theme default for it checks first.
func UserColor(id string) color.NRGBA {
	h := fnv.New32a()
	h.Write([]byte(id))
	return UserPalette[h.Sum32()%uint32(len(UserPalette))]
}
