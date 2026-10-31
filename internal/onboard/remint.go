package onboard

import (
	"context"
	"log"

	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/slackdesktop"
)

// RemintTokens refreshes every token's xoxc from the live desktop cookie. On
// any failure for a given token it keeps the cached token (offline-friendly).
// cookieFn is read once up front; mintFn/saveFn are injected for testing.
// Tokens signed in from a browser session are left alone. Remint is the
// form a caller with a token store uses.
func RemintTokens(
	ctx context.Context,
	tokens []slackclient.Token,
	cookieFn func() (string, error),
	tokensFn func() (map[string]string, error),
	mintFn func(ctx context.Context, domain, cookie string) (string, error),
	saveFn func(slackclient.Token) error,
) []slackclient.Token {
	cookie, err := cookieFn()
	if err != nil {
		log.Printf("remint: could not read desktop cookie, using cached tokens: %v", err)
		return tokens
	}
	// Primary source: the desktop app's stored tokens (client-v2). A read
	// failure is non-fatal — we fall back to page-scrape minting per token.
	desktopTokens, err := tokensFn()
	if err != nil {
		log.Printf("remint: could not read desktop tokens, falling back to mint: %v", err)
	}
	out := make([]slackclient.Token, len(tokens))
	copy(out, tokens)
	for i := range out {
		if out[i].Source == slackclient.TokenSourceBrowser {
			// Signed in from a browser session on purpose. The desktop
			// app can hold the same workspace under another account, and
			// its credentials would silently replace the chosen ones.
			continue
		}
		newTok := desktopTokens[out[i].TeamID]
		if newTok == "" {
			if out[i].Domain == "" {
				continue // legacy token without a domain; cannot re-mint
			}
			minted, err := mintFn(ctx, out[i].Domain, cookie)
			if err != nil {
				log.Printf("remint: %s: %v (keeping cached token)", out[i].TeamName, err)
				continue
			}
			newTok = minted
		}
		out[i].AccessToken = newTok
		out[i].Cookie = cookie
		if err := saveFn(out[i]); err != nil {
			log.Printf("remint: %s: save failed: %v", out[i].TeamName, err)
		}
	}
	return out
}

// Remint is RemintTokens over a token store: it lists the stored tokens,
// re-mints each from the Slack desktop app's live session (its stored
// tokens first, mint as the fallback) and saves every refreshed token back
// to store. The result is the full list, refreshed where that worked and
// as stored where it did not; the only error is failing to list the store.
// Callers run it once at startup, before connecting.
func Remint(ctx context.Context, store *slackclient.TokenStore, mint Minter) ([]slackclient.Token, error) {
	tokens, err := store.List()
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return tokens, nil
	}
	return RemintTokens(ctx, tokens, slackdesktop.Cookie, slackdesktop.Tokens, mint, store.Save), nil
}
