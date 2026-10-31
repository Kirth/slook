package avatar

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestFetchKey(t *testing.T) {
	const u = "https://avatars.slack-edge.com/2024-01-02/1_ab_72.png"
	sum := sha256.Sum256([]byte(u))
	if got, want := FetchKey("U1", u), "avatar-U1-"+hex.EncodeToString(sum[:])[:12]; got != want {
		t.Errorf("FetchKey = %q; want %q", got, want)
	}
	if FetchKey("U1", u) == FetchKey("U1", u+"x") {
		t.Error("different URLs for one user share a key")
	}
	if FetchKey("U1", u) == FetchKey("U2", u) {
		t.Error("different users with one URL share a key")
	}
}
