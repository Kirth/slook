// Package resolve names users slk has not seen: from the cache when it
// holds them, else in batches through edgeapi's users/info, else one
// Web API users.info each.
//
// It is the batching core of the TUI's userResolver (cmd/slk, retired
// 2026-10-31), without its UI messages, name store, avatar cache and
// bot path. The design is
// docs/superpowers/specs/2026-08-05-edge-health-and-batched-resolver-design.md:
// on the first working Enterprise Grid session one users.info call per
// cache miss came to 282 calls in one session; edge users/info takes 80
// ids a request and returns full records inline, so the same misses are
// ~4 requests.
package resolve

import (
	"context"
	"sync"
	"time"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/slack/edge"
)

// userResolverConcurrency caps how many users.info round trips the
// resolver has open at once.
//
// The number is a rate bound, not a throughput target. Before it,
// Request spawned one goroutine per unresolved user with nothing
// between it and the transport, and a cold cache turned that into a
// 40,000-request burst -- one per distinct channel member -- all
// entering RoundTrip within moments of each other. The membership
// fan-out that produced that particular burst is gone, but Request is
// still reachable from the render path and inbound messages, so the
// bound stays. Eight is well under what a browser opens to one host and
// far more than a person generates.
const userResolverConcurrency = 8

// userResolverBatchWindow is how long Request waits for more misses
// to coalesce before flushing them as one edge users/info call. The
// render path resolves a channel's unknown authors in a single
// burst, so a short window turns a channel open from N requests into
// one; 200 ms is below what a person perceives as resolution lag.
const userResolverBatchWindow = 200 * time.Millisecond

// Profile is a resolved user as the cache now holds it.
//
// DisplayName is the name to show: the profile's display name, then its
// real name, then the handle (bootstrap's userDisplayName chain, so the
// boot and the resolver cannot disagree). RealName is the real name
// when the source carried one (empty for a cache hit). AvatarURL is the
// image the source had: image_original from edge, the 72px image from
// users.info. Version is edge's `updated` stamp, 0 from users.info and
// the cache.
type Profile struct {
	ID, Handle, DisplayName, RealName, AvatarURL, TeamID string
	IsBot, IsExternal                                    bool
	StatusEmoji, StatusText                              string
	StatusExpiration                                     int64
	HuddleState                                          string
	HuddleExpiration                                     int64
	Version                                              int64
}

// Batcher is the edge.UsersInfo subset the resolver batches misses
// through. *edge.Client satisfies it; nil means no batching and every
// miss takes the per-user users.info path.
type Batcher interface {
	UsersInfo(ctx context.Context, updatedIDs map[string]int64) ([]edge.User, error)
}

// Single is the per-user users.info call; *slackclient.Client
// satisfies it. Nil means no per-user fallback: what edge does not
// resolve stays unresolved.
type Single interface {
	GetUserProfileContext(ctx context.Context, userID string) (*slackclient.UserProfile, error)
}

// Resolver resolves unknown users for one workspace. Misses coalesce
// into batched edge users/info calls (see flush); ids edge cannot
// resolve, a failed batch, and degraded workspaces fall back to the
// per-user Web API users.info path (see resolveOne). Deduplicates
// concurrent requests for the same userID; failures are silent (the
// caller keeps showing the user ID). Bound to a single workspace because
// external classification is scoped to it; cached profiles can be
// shared across workspaces by user ID.
type Resolver struct {
	// Logf, when set, receives the resolver's diagnostics (failed
	// lookups, fallbacks). Set it before the first call.
	Logf func(format string, args ...any)

	teamID     string
	db         *cache.DB
	single     Single
	onResolved func(Profile)
	inflight   sync.Map // userID -> struct{}
	// sem bounds concurrent round trips on the per-user users.info
	// path (resolveOne). Buffered, so acquiring it happens inside the
	// request goroutine and Request itself never blocks -- it is
	// called from the render path and from WS event handlers, neither
	// of which may wait on the network. The batch path needs no
	// semaphore: it is bounded by the window itself, at one edge
	// users/info call per userResolverBatchWindow (plus that call's
	// internal 80-id splitting, sequential within the call).
	sem chan struct{}

	batcher  Batcher
	degraded func() bool // nil: never degraded

	pendingMu  sync.Mutex
	pending    map[string]struct{}
	flushTimer *time.Timer
}

// New returns a Resolver for workspaceID that writes what it learns to
// db. batch (edge) and single (users.info) may each be nil; degraded
// (nil: never) reports that boot found the workspace's edge broken, in
// which case no edge call is made. onResolved (may be nil) hears every
// user resolved over the network, from whichever goroutine resolved it,
// after the cache row is written.
func New(workspaceID string, db *cache.DB, batch Batcher, single Single, degraded func() bool, onResolved func(Profile)) *Resolver {
	return &Resolver{
		teamID:     workspaceID,
		db:         db,
		single:     single,
		onResolved: onResolved,
		sem:        make(chan struct{}, userResolverConcurrency),
		batcher:    batch,
		degraded:   degraded,
		pending:    map[string]struct{}{},
	}
}

func (r *Resolver) logf(format string, args ...any) {
	if r.Logf != nil {
		r.Logf(format, args...)
	}
}

func (r *Resolver) isDegraded() bool { return r.degraded != nil && r.degraded() }

// cached returns the cache's row for userID when it names the user.
// Unnamed placeholder rows still need a first successful lookup.
func (r *Resolver) cached(userID string) (Profile, bool) {
	if r.db == nil {
		return Profile{}, false
	}
	u, err := r.db.GetUser(userID)
	if err != nil || (u.DisplayName == "" && u.Name == "") {
		return Profile{}, false
	}
	name := u.DisplayName
	if name == "" {
		name = u.Name
	}
	return Profile{
		ID: u.ID, Handle: u.Name, DisplayName: name, AvatarURL: u.AvatarURL, TeamID: u.HomeTeamID,
		IsBot: u.IsBot, IsExternal: u.IsExternal,
		StatusEmoji: u.StatusEmoji, StatusText: u.StatusText, StatusExpiration: u.StatusExpiration,
		HuddleState: u.HuddleState, HuddleExpiration: u.HuddleExpiration,
	}, true
}

// Request enqueues a lookup for userID and returns immediately. On
// success onResolved hears it.
func (r *Resolver) Request(userID string) {
	if r == nil || userID == "" {
		return
	}
	if _, exists := r.inflight.LoadOrStore(userID, struct{}{}); exists {
		return
	}
	// Skip if already named (in cache.User); unnamed placeholder rows
	// still need a first successful profile lookup. Stale-data refresh
	// is boot's edge revalidation's job; this gate is for "first time
	// we see this user". Without it every render of a channel would
	// refetch each author.
	//
	// Order matters: inflight.LoadOrStore first (claim the slot),
	// THEN the cache check (bail if cached), with inflight.Delete on
	// the bail path. This avoids a race where two concurrent Requests
	// both miss the cache, both then LoadOrStore (one wins, the loser
	// silently returns). Store-first + check-second means at most one
	// goroutine ever passes the cache check.
	if _, ok := r.cached(userID); ok {
		r.inflight.Delete(userID)
		return
	}
	if r.batcher == nil || r.isDegraded() {
		go r.resolveOne(userID)
		return
	}
	r.pendingMu.Lock()
	r.pending[userID] = struct{}{}
	if r.flushTimer == nil {
		r.flushTimer = time.AfterFunc(userResolverBatchWindow, r.flush)
	}
	r.pendingMu.Unlock()
}

// resolveOne resolves a single user through the Web API users.info
// path: the pre-batch behaviour, now the fallback for ids edge did
// not return, for a failed edge call, and for workspaces whose edge
// is degraded. Callers run it on its own goroutine.
func (r *Resolver) resolveOne(userID string) {
	defer r.inflight.Delete(userID)
	r.resolveOneContext(context.Background(), userID)
}

// resolveOneContext is the synchronous, context-bounded variant of
// resolveOne. Cancellation also bounds the semaphore wait. It does not
// own a Request inflight slot; background callers release their own
// claim in resolveOne rather than clearing a racing request.
func (r *Resolver) resolveOneContext(ctx context.Context, userID string) (Profile, bool) {
	if r.single == nil {
		return Profile{}, false
	}
	if r.sem != nil {
		select {
		case r.sem <- struct{}{}:
			defer func() { <-r.sem }()
		case <-ctx.Done():
			return Profile{}, false
		}
	}
	u, err := r.single.GetUserProfileContext(ctx, userID)
	if err != nil || u == nil || u.User == nil {
		r.logf("resolve: users.info team=%s user=%s err=%v", r.teamID, userID, err)
		return Profile{}, false
	}
	name := u.Profile.DisplayName
	if name == "" {
		name = u.RealName
	}
	if name == "" {
		name = u.Name
	}
	avatarURL := u.Profile.Image72
	if avatarURL == "" {
		avatarURL = u.Profile.Image48
	}
	if avatarURL == "" {
		avatarURL = u.Profile.Image32
	}
	// r.teamID is the workspace's home TeamID; u.TeamID is the
	// user's home TeamID. If they differ (and u.TeamID is set),
	// the user is a Slack Connect / shared-channel guest. Treat
	// an empty u.TeamID as internal — better to under-detect than
	// to falsely flag.
	p := Profile{
		ID:               userID,
		Handle:           u.Name,
		DisplayName:      name,
		RealName:         u.RealName,
		AvatarURL:        avatarURL,
		TeamID:           u.TeamID,
		IsBot:            u.IsBot || u.IsAppUser,
		IsExternal:       u.TeamID != "" && u.TeamID != r.teamID,
		StatusEmoji:      u.Profile.StatusEmoji,
		StatusText:       u.Profile.StatusText,
		StatusExpiration: int64(u.Profile.StatusExpiration),
		HuddleState:      u.Profile.HuddleState,
		HuddleExpiration: int64(u.Profile.HuddleStateExpirationTS),
	}
	if r.db != nil {
		_ = r.db.UpsertUser(cache.User{
			ID:               userID,
			WorkspaceID:      r.teamID,
			HomeTeamID:       p.TeamID,
			Name:             p.Handle,
			DisplayName:      name,
			AvatarURL:        avatarURL,
			Presence:         "away",
			IsBot:            p.IsBot,
			IsExternal:       p.IsExternal,
			StatusEmoji:      p.StatusEmoji,
			StatusText:       p.StatusText,
			StatusExpiration: p.StatusExpiration,
			HuddleState:      p.HuddleState,
			HuddleExpiration: p.HuddleExpiration,
		})
		// UpsertUser intentionally preserves existing status for
		// placeholder callers. This is a full profile: persist changes
		// and clears too.
		_ = r.db.UpdateUserStatus(userID, p.StatusEmoji, p.StatusText, p.StatusExpiration, p.HuddleState, p.HuddleExpiration)
	}
	if r.onResolved != nil {
		r.onResolved(p)
	}
	return p, true
}

// flush resolves everything queued since the window opened, as one
// edge users/info batch. Anything the batch does not return falls
// back to the per-user path: absence from the batch means "could not
// resolve", and an errored batch resolves nothing at all.
func (r *Resolver) flush() {
	r.pendingMu.Lock()
	ids := make([]string, 0, len(r.pending))
	for id := range r.pending {
		ids = append(ids, id)
	}
	clear(r.pending)
	r.flushTimer = nil
	r.pendingMu.Unlock()
	if len(ids) == 0 {
		return
	}
	// Re-check at flush time: boot may have marked the workspace
	// degraded while the window was open.
	if r.isDegraded() {
		for _, id := range ids {
			go r.resolveOne(id)
		}
		return
	}
	got, err := r.batch(context.Background(), ids)
	if err != nil {
		r.logf("resolve: edge users/info for %d users team=%s: %v (falling back to per-user users.info)", len(ids), r.teamID, err)
	}
	for _, id := range ids {
		if _, ok := got[id]; ok {
			r.inflight.Delete(id)
			continue
		}
		go r.resolveOne(id)
	}
}

// batch makes one edge users/info call for ids (each at version 0, the
// conditional protocol's "never seen, send the full record") and
// applies every record that names its user. An empty record is left
// out: it would cache an empty name and blank a shown one.
func (r *Resolver) batch(ctx context.Context, ids []string) (map[string]Profile, error) {
	updated := make(map[string]int64, len(ids))
	for _, id := range ids {
		if id != "" {
			updated[id] = 0
		}
	}
	if len(updated) == 0 {
		return nil, nil
	}
	users, err := r.batcher.UsersInfo(ctx, updated)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Profile, len(users))
	for _, u := range users {
		if p, ok := r.applyEdgeUser(u); ok {
			out[p.ID] = p
		}
	}
	return out, nil
}

// Resolve resolves ids now and returns every one it could, keyed by
// ID: those the cache names without a request, the rest through one
// edge users/info batch, and what that leaves through users.info (at
// most userResolverConcurrency at a time), all bounded by ctx. Empty
// ids are skipped. With the workspace degraded (or no Batcher) the
// edge step is skipped; with no Single there is no per-user step, so
// what edge does not resolve is absent. onResolved hears each one
// resolved over the network.
//
// A Request racing a Resolve for the same id can resolve it twice; the
// writes are idempotent, so no guard is taken.
func (r *Resolver) Resolve(ctx context.Context, ids []string) map[string]Profile {
	if r == nil {
		return nil
	}
	out := map[string]Profile{}
	var misses []string
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if p, ok := r.cached(id); ok {
			out[id] = p
			continue
		}
		misses = append(misses, id)
	}
	if len(misses) == 0 {
		return out
	}
	if r.batcher != nil && !r.isDegraded() {
		got, err := r.batch(ctx, misses)
		if err != nil {
			r.logf("resolve: edge users/info for %d users team=%s: %v (falling back to per-user users.info)", len(misses), r.teamID, err)
		}
		rest := misses[:0:0]
		for _, id := range misses {
			if p, ok := got[id]; ok {
				out[id] = p
			} else {
				rest = append(rest, id)
			}
		}
		misses = rest
	}
	if r.single == nil || len(misses) == 0 {
		return out
	}
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for _, id := range misses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if p, ok := r.resolveOneContext(ctx, id); ok {
				mu.Lock()
				out[id] = p
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return out
}

// applyEdgeUser records one user the edge batch returned (the cache
// row, created — these are misses — with its version) and reports it,
// unless the record names nobody.
func (r *Resolver) applyEdgeUser(u edge.User) (Profile, bool) {
	name := u.Profile.DisplayName
	if name == "" {
		name = u.Profile.RealName
	}
	if name == "" {
		name = u.Name
	}
	if name == "" {
		// Unobserved, but an empty record would otherwise cache
		// an empty name and blank a rendered one.
		return Profile{}, false
	}
	p := Profile{
		ID:               u.ID,
		Handle:           u.Name,
		DisplayName:      name,
		RealName:         u.Profile.RealName,
		AvatarURL:        u.Profile.ImageOriginal,
		TeamID:           u.TeamID,
		IsBot:            u.IsBot,
		IsExternal:       u.TeamID != "" && u.TeamID != r.teamID,
		StatusEmoji:      u.Profile.StatusEmoji,
		StatusText:       u.Profile.StatusText,
		StatusExpiration: u.Profile.StatusExpiration,
		HuddleState:      u.Profile.HuddleState,
		HuddleExpiration: u.Profile.HuddleStateExpirationTS,
		Version:          u.Version,
	}
	if r.db != nil {
		_ = r.db.UpsertUserFromEdge(r.teamID, cache.EdgeUserUpdate{
			ID:               u.ID,
			HomeTeamID:       u.TeamID,
			Name:             u.Name,
			DisplayName:      name,
			AvatarURL:        u.Profile.ImageOriginal,
			IsBot:            u.IsBot,
			IsExternal:       p.IsExternal,
			StatusEmoji:      p.StatusEmoji,
			StatusText:       p.StatusText,
			StatusExpiration: p.StatusExpiration,
			HuddleState:      p.HuddleState,
			HuddleExpiration: p.HuddleExpiration,
			Version:          u.Version,
		})
	}
	if r.onResolved != nil {
		r.onResolved(p)
	}
	return p, true
}
