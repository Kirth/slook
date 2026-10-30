package backend

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/slack-go/slack"
	"golang.org/x/sync/errgroup"

	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/avatar"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/core/blocks"
	"github.com/gammons/slk/internal/export"
	"github.com/gammons/slk/internal/filedl"
	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/msgsync"
	"github.com/gammons/slk/internal/service"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/slack/mrkdwn"
	"github.com/gammons/slk/internal/slackfmt"
	"github.com/gammons/slk/internal/slackhttp"
	"github.com/gammons/slk/internal/text"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// historyLimit is how many messages a newly opened window starts with,
// and how many a scroll back adds.
const historyLimit = 100

// avatarPx is the avatar size asked of Slack's CDN.
const avatarPx = 72

// imageCacheMB bounds the on-disk image cache, as slk's default does.
const imageCacheMB = 200

// Slack is a real workspace, signed in with a token slk stored.
type Slack struct {
	client  *slackclient.Client
	team    string
	fetcher *imgpkg.Fetcher
	files   *filedl.Downloader // attachments, for the previewer
	// db is slk's cache: messages, user names, avatars and the user's
	// channel visits. Nil when it could not be opened; then nothing is
	// cached and every window open goes to Slack.
	db *cache.DB
	// sync writes Slack's messages into db and reads them back for
	// windows; nil with db.
	sync *msgsync.Syncer
	// sections and mutes are the user's Slack sidebar and mute prefs,
	// kept current from the socket.
	sections *service.SectionStore
	mutes    *service.MuteStore

	// queue carries socket callbacks and finished fetches to Run's
	// goroutine, which resolves names (possibly over the network)
	// without stalling the socket and emits in order.
	queue chan any
	// connected is whether the socket is up; History reads it off its
	// own goroutine to mark a span live.
	connected atomic.Bool
	// runCtx is Run's context, the parent of a backfill's; nil before
	// Run. paused is the user-is-busy hook the backfill holds on;
	// autoStart is the -backfill flag's horizon. bf is the backfill;
	// active counts fetches in flight by label; syncPending says a
	// SyncChanged is already queued. All under mu.
	runCtx    context.Context
	paused    func() bool
	ignore    func(name string) bool // conversations the backfill skips, by name
	autoStart *time.Duration
	// catchUpOn and catchUpMax shape the catch-up a connect runs (see
	// catchUpAfterConnect); on, and DefaultCatchUpLimit, unless set.
	catchUpOn   bool
	catchUpMax  int
	bf          backfillRun
	active      map[string]int
	syncPending bool

	mu        sync.Mutex
	users     map[string]string // user ID -> display name
	avatars   map[string]string // user or bot ID -> picture URL
	handles   map[string]string // @-handle -> display name, for group DM names
	bots      map[string]string // bot ID -> name
	channels  map[string]string // channel ID -> name
	kinds     map[string]Kind   // channel ID -> kind
	profiles  map[string]Profile
	peers     []string          // DM partners, whose presence the socket reports
	commands  []Command         // nil until fetched
	customs   map[string]string // custom emoji; nil until fetched
	convOrder []string          // conversations, most recently active first
}

var _ rich.Names = (*Slack)(nil)

// ConnectSlack signs in to tok's workspace. db, when not nil, is slk's
// cache: messages are served from it and kept current, and it seeds
// user names and avatars and holds the user's channel visits. The
// backend keeps it. cacheDir is slk's cache directory; downloaded
// images go in its images/ subdirectory.
func ConnectSlack(ctx context.Context, tok slackclient.Token, db *cache.DB, cacheDir string) (*Slack, error) {
	client := slackclient.NewClient(tok.AccessToken, tok.Cookie)
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connecting %s: %w", tok.TeamName, err)
	}
	s := &Slack{
		client:   client,
		team:     tok.TeamName,
		db:       db,
		sections: service.NewSectionStore(),
		mutes:    service.NewMuteStore(),
		queue:    make(chan any, 256),
		users:    map[string]string{},
		avatars:  map[string]string{},
		handles:  map[string]string{},
		bots:     map[string]string{},
		channels: map[string]string{},
		kinds:    map[string]Kind{},
		profiles: map[string]Profile{},
		active:   map[string]int{},

		catchUpOn:  true,
		catchUpMax: config.DefaultCatchUpLimit,
	}
	if db != nil {
		// The workspace row first: users and channels reference it.
		if err := db.UpsertWorkspace(cache.Workspace{ID: client.TeamID(), Name: tok.TeamName}); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: cache:", err)
		}
		s.sync = &msgsync.Syncer{DB: db, Source: client, WorkspaceID: client.TeamID()}
		if cached, err := db.ListUsers(client.TeamID()); err == nil {
			for _, u := range cached {
				s.rememberUser(u.ID, u.Name, u.DisplayName, "", u.AvatarURL)
			}
		}
	}
	auths := []slackhttp.TeamAuth{{TeamID: client.TeamID(), Token: tok.AccessToken, DCookie: tok.Cookie}}
	s.files = filedl.New(slackhttp.NewAuthResolver(auths), "")
	if store, err := imgpkg.NewCache(filepath.Join(cacheDir, "images"), imageCacheMB); err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: image cache:", err, "(images disabled)")
	} else {
		s.fetcher = imgpkg.NewFetcher(store, slackhttp.NewImageHTTPClient(nil))
		s.fetcher.SetAuths(auths)
	}
	return s, nil
}

func (s *Slack) Workspace() string       { return s.team }
func (s *Slack) Self() string            { return s.client.UserID() }
func (s *Slack) Images() *imgpkg.Fetcher { return s.fetcher }

// rememberUser records a user's name (display name, then real name,
// then handle) and picture.
func (s *Slack) rememberUser(id, handle, display, real, avatarURL string) string {
	name := display
	if name == "" {
		name = real
	}
	if name == "" {
		name = handle
	}
	if name == "" {
		name = id
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[id] = name
	if handle != "" {
		s.handles[handle] = name
	}
	if avatarURL != "" {
		s.avatars[id] = avatar.SizedURL(avatarURL, avatarPx)
	}
	return name
}

// persistUser writes a profile learned from Slack to the cache, so the
// next start (and a later search) has the name.
func (s *Slack) persistUser(id, handle, display, avatarURL string, isBot bool) {
	if s.db == nil {
		return
	}
	err := s.db.UpsertUser(cache.User{
		ID: id, WorkspaceID: s.client.TeamID(), Name: handle, DisplayName: display,
		AvatarURL: avatarURL, IsBot: isBot, UpdatedAt: time.Now().Unix(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: cache user:", err)
	}
}

func (s *Slack) knownUser(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, ok := s.users[id]
	return name, ok
}

func (s *Slack) avatarURL(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.avatars[id]
}

// User, Channel and UserGroup make Slack a rich.Names.
func (s *Slack) User(id string) (string, bool) { return s.knownUser(id) }
func (s *Slack) Channel(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, ok := s.channels[id]
	return name, ok
}
func (s *Slack) UserGroup(string) (string, bool) { return "", false }

// userName returns a user's name, asking Slack for one it does not know.
func (s *Slack) userName(ctx context.Context, id string) string {
	if name, ok := s.knownUser(id); ok {
		return name
	}
	p, err := s.client.GetUserProfileContext(ctx, id)
	if err != nil || p.User == nil {
		return id
	}
	s.persistUser(id, p.Name, p.Profile.DisplayName, p.Profile.Image72, p.IsBot)
	return s.rememberUser(id, p.Name, p.Profile.DisplayName, p.RealName, p.Profile.Image72)
}

func (s *Slack) botName(ctx context.Context, id string) string {
	s.mu.Lock()
	name, ok := s.bots[id]
	s.mu.Unlock()
	if ok {
		return name
	}
	b, err := s.client.GetBotInfo(ctx, id)
	if err != nil || b == nil || b.Name == "" {
		return "app"
	}
	s.mu.Lock()
	s.bots[id] = b.Name
	if b.Icons.Image72 != "" {
		s.avatars[id] = b.Icons.Image72
	}
	s.mu.Unlock()
	return b.Name
}

// resolveAll learns the names of users not yet known, a few at a time.
func (s *Slack) resolveAll(ctx context.Context, ids []string) {
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := s.knownUser(id); ok {
			continue
		}
		g.Go(func() error {
			s.userName(gctx, id)
			return nil
		})
	}
	_ = g.Wait()
}

func (s *Slack) Conversations(ctx context.Context) ([]Conversation, error) {
	// The list's activity data comes from one request (or query) each,
	// alongside the conversations themselves. Only the conversations
	// are essential; without the rest the list still works, plainer.
	var (
		chans   []slack.Channel
		unreads []slackclient.UnreadInfo
		visits  map[string]int64
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		chans, err = s.client.GetChannels(gctx)
		return err
	})
	g.Go(func() error {
		var err error
		if unreads, _, err = s.client.GetUnreadCountsContext(gctx); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: unread counts:", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := s.sections.Bootstrap(gctx, s.client); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: sidebar sections:", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := s.mutes.Bootstrap(gctx, s.client); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: muted channels:", err)
		}
		return nil
	})
	if s.db != nil {
		g.Go(func() error {
			var err error
			if visits, err = s.db.GetChannelVisits(s.client.TeamID()); err != nil {
				fmt.Fprintln(os.Stderr, "slk-gio: channel visits:", err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	reads := make(map[string]slackclient.UnreadInfo, len(unreads))
	for _, u := range unreads {
		reads[u.ChannelID] = u
	}
	starred := map[string]bool{}
	for _, id := range s.sections.StarredConversationIDs() {
		starred[id] = true
	}
	var peers []string
	for _, c := range chans {
		if c.IsIM {
			peers = append(peers, c.User)
		}
	}
	s.resolveAll(ctx, peers)

	convs := make([]Conversation, 0, len(chans))
	s.mu.Lock()
	for _, c := range chans {
		conv := Conversation{ID: c.ID, Name: c.Name, Kind: KindChannel}
		switch {
		case c.IsIM:
			conv.Kind = KindDM
			conv.Name = s.users[c.User]
			if conv.Name == "" {
				conv.Name = c.User
			}
			conv.AvatarURL = s.avatars[c.User]
			conv.PeerID = c.User
		case c.IsMpIM:
			conv.Kind = KindGroupDM
			conv.Name = slackfmt.FormatMPDMName(c.Name, func(h string) string { return s.handles[h] })
		case c.IsPrivate:
			conv.Kind = KindPrivate
		}
		s.channels[c.ID] = conv.Name
		s.kinds[c.ID] = conv.Kind
		if r, ok := reads[c.ID]; ok {
			conv.Unread, conv.Mentions = r.HasUnread, r.MentionCount
			conv.Latest, _ = export.TimeFromTS(r.Latest)
		}
		if v := visits[c.ID]; v > 0 {
			conv.Visited = time.Unix(v, 0)
		}
		conv.Muted = s.mutes.IsMuted(c.ID)
		conv.Starred = starred[c.ID]
		conv.Section, _ = s.sections.SectionForChannel(c.ID)
		convs = append(convs, conv)
	}
	s.convOrder = activityOrder(convs)
	s.mu.Unlock()
	s.persistChannels(convs)
	sortConversations(convs)
	s.watchPresence(peers)
	return convs, nil
}

// persistChannels writes the conversations to the cache: the rows a
// later search names channels by.
func (s *Slack) persistChannels(convs []Conversation) {
	if s.db == nil {
		return
	}
	now := time.Now().Unix()
	for _, c := range convs {
		err := s.db.UpsertChannel(cache.Channel{
			ID: c.ID, WorkspaceID: s.client.TeamID(), Name: c.Name, Type: string(c.Kind),
			IsMember: true, IsStarred: c.Starred, UpdatedAt: now,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: cache channel:", err)
			return
		}
	}
}

// activityOrder lists conversation IDs most recently visited first,
// then most recently active: the order the backfill works in.
func activityOrder(convs []Conversation) []string {
	sorted := slices.Clone(convs)
	slices.SortStableFunc(sorted, func(a, b Conversation) int {
		if c := b.Visited.Compare(a.Visited); c != 0 {
			return c
		}
		return b.Latest.Compare(a.Latest)
	})
	out := make([]string, len(sorted))
	for i, c := range sorted {
		out[i] = c.ID
	}
	return out
}

// watchPresence subscribes to the presence of the user's DM partners.
// Before the socket is up the subscription fails; Run repeats it on
// connect.
func (s *Slack) watchPresence(peers []string) {
	s.mu.Lock()
	s.peers = peers
	s.mu.Unlock()
	if len(peers) > 0 {
		_ = s.client.SubscribePresence(peers)
	}
}

// Download fetches an attachment with the workspace's credentials, once.
func (s *Slack) Download(ctx context.Context, a Attachment) (string, error) {
	p := AttachmentPath(a)
	return p, s.files.Fetch(ctx, a.URL, p)
}

// Presence asks Slack whether a person is active now.
func (s *Slack) Presence(ctx context.Context, userID string) (bool, error) {
	p, err := s.client.GetUserPresence(ctx, userID)
	if err != nil {
		return false, err
	}
	return p.Presence == "active", nil
}

// Sections is the user's sidebar sections in Slack's order.
func (s *Slack) Sections() []Section {
	var out []Section
	for _, sec := range s.sections.OrderedSections() {
		out = append(out, Section{ID: sec.ID, Name: sec.Name, Emoji: sec.Emoji, Type: sec.Type})
	}
	return out
}

// membership maps every known conversation to its section.
func (s *Slack) membership() map[string]string {
	s.mu.Lock()
	ids := make([]string, 0, len(s.channels))
	for id := range s.channels {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		if sec, ok := s.sections.SectionForChannel(id); ok {
			out[id] = sec
		}
	}
	return out
}

// Details is the conversation's topic, purpose and member count, with
// mentions and links in the topic resolved to plain text.
func (s *Slack) Details(ctx context.Context, channelID string) (Details, error) {
	ch, err := s.client.GetConversationInfo(ctx, channelID)
	if err != nil {
		return Details{}, err
	}
	return Details{
		Topic:   plainMrkdwn(ch.Topic.Value, s),
		Purpose: plainMrkdwn(ch.Purpose.Value, s),
		Members: ch.NumMembers,
	}, nil
}

// plainMrkdwn is mrkdwn as one line of plain text.
func plainMrkdwn(text string, n rich.Names) string {
	return strings.Join(strings.Fields(rich.Plain(rich.Spans(text, n))), " ")
}

// WebURL opens the conversation in Slack's web client.
func (s *Slack) WebURL(channelID string) string {
	return "https://app.slack.com/client/" + s.client.TeamID() + "/" + channelID
}

// MarkRead marks a conversation read up to ts, as viewing it in Slack
// does. Slack echoes it back as a ReadChanged.
func (s *Slack) MarkRead(ctx context.Context, channelID, ts string) error {
	return s.client.MarkChannel(ctx, channelID, ts)
}

// Visited records that the user opened a conversation, in the cache the
// list orders by.
func (s *Slack) Visited(channelID string) {
	if s.db == nil {
		return
	}
	if err := s.db.RecordChannelVisit(s.client.TeamID(), channelID); err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio:", err)
	}
}

// kindOrder puts channels before direct messages in the list.
var kindOrder = map[Kind]int{KindChannel: 0, KindPrivate: 0, KindDM: 1, KindGroupDM: 1}

func sortConversations(convs []Conversation) {
	slices.SortStableFunc(convs, func(a, b Conversation) int {
		if d := kindOrder[a.Kind] - kindOrder[b.Kind]; d != 0 {
			return d
		}
		return strings.Compare(text.Fold(a.Name), text.Fold(b.Name))
	})
}

// History answers from the cache when it covers the conversation, and
// catches up with Slack in the background: the catch-up's changes
// arrive through Run as a HistoryUpdated. A conversation the cache has
// never seen is fetched first (one page), so the first open still shows
// Slack's current state. Without a cache every open goes to Slack.
func (s *Slack) History(ctx context.Context, channelID string) ([]Message, error) {
	if s.sync == nil {
		raw, err := s.client.GetHistory(ctx, channelID, historyLimit, "")
		if err != nil {
			return nil, err
		}
		slices.Reverse(raw) // newest first on the wire
		return s.fromSlack(ctx, channelID, raw), nil
	}
	spans, err := s.db.Spans(channelID)
	if err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		done := s.trackIn("opening", channelID)
		_, err := s.sync.Open(ctx, channelID, s.connected.Load())
		done()
		if err != nil {
			return nil, err
		}
	} else {
		go s.catchUp(ctx, channelID)
	}
	rows, err := s.db.GetMessages(channelID, historyLimit, "")
	if err != nil {
		return nil, err
	}
	return s.fromCache(ctx, rows), nil
}

// catchUp brings a conversation's cache to Slack's head and hands the
// changes to Run, which emits them in order with the socket's events.
func (s *Slack) catchUp(ctx context.Context, channelID string) {
	done := s.trackIn("catching up", channelID)
	changed, err := s.sync.CatchUp(ctx, channelID, s.connected.Load())
	done()
	if err != nil {
		if ctx.Err() == nil {
			fmt.Fprintln(os.Stderr, "slk-gio: catching up", channelID+":", err)
		}
		return
	}
	if changed.Empty() {
		return
	}
	select {
	case s.queue <- rawChanged{changed: changed}:
	case <-ctx.Done():
	}
}

// Older returns the page before beforeTS: from the cache when a span
// covers it, else one page from Slack, cached on the way.
func (s *Slack) Older(ctx context.Context, channelID, beforeTS string) ([]Message, error) {
	if s.sync == nil {
		raw, err := s.client.GetOlderHistory(ctx, channelID, historyLimit, beforeTS)
		if err != nil {
			return nil, err
		}
		slices.Reverse(raw)
		return s.fromSlack(ctx, channelID, raw), nil
	}
	rows, err := s.db.GetMessages(channelID, historyLimit, beforeTS)
	if err != nil {
		return nil, err
	}
	if !s.cacheHasOlder(channelID, beforeTS, rows) {
		done := s.trackIn("older page in", channelID)
		_, err := s.sync.Older(ctx, channelID, beforeTS, s.connected.Load())
		done()
		if err != nil {
			return nil, err
		}
		if rows, err = s.db.GetMessages(channelID, historyLimit, beforeTS); err != nil {
			return nil, err
		}
	}
	return s.fromCache(ctx, rows), nil
}

// cacheHasOlder reports whether rows, the cached page before beforeTS,
// is all Slack would return: a full page inside one span, or the
// channel's beginning.
func (s *Slack) cacheHasOlder(channelID, beforeTS string, rows []cache.Message) bool {
	spans, err := s.db.Spans(channelID)
	if err != nil || len(spans) == 0 {
		return false
	}
	if spans[0].ReachesStart && strings.Compare(spans[0].LatestTS, beforeTS) >= 0 {
		return true
	}
	if len(rows) < historyLimit {
		return false
	}
	covered, err := s.db.Covers(channelID, rows[0].TS, beforeTS)
	return err == nil && covered
}

// fromSlack converts messages as the API returned them (oldest first).
func (s *Slack) fromSlack(ctx context.Context, channelID string, raw []slack.Message) []Message {
	msgs := make([]rawMessage, 0, len(raw))
	var ids []string
	for _, m := range raw {
		r := rawFromSlack(channelID, m)
		msgs = append(msgs, r)
		ids = append(ids, r.mentioned()...)
	}
	return s.finish(ctx, msgs, ids)
}

// fromCache converts cached rows (oldest first). Each row's raw_json is
// the message as Slack sent it; the row's own columns win where a
// socket frame may have been partial (a message_changed's inner object
// carries no subtype) or since overtaken (reply_count).
func (s *Slack) fromCache(ctx context.Context, rows []cache.Message) []Message {
	msgs := make([]rawMessage, 0, len(rows))
	var ids []string
	for _, row := range rows {
		var m slack.Message
		if err := json.Unmarshal([]byte(row.RawJSON), &m); err != nil || m.Timestamp == "" {
			m = slack.Message{Msg: slack.Msg{Timestamp: row.TS, User: row.UserID, Text: row.Text}}
		}
		r := rawFromSlack(row.ChannelID, m)
		r.ts, r.threadTS, r.replyCount = row.TS, row.ThreadTS, row.ReplyCount
		r.edited = r.edited || row.EditedAt != ""
		if row.Subtype != "" {
			r.subtype = row.Subtype
		}
		msgs = append(msgs, r)
		ids = append(ids, r.mentioned()...)
	}
	s.cachedReactions(msgs)
	return s.finish(ctx, msgs, ids)
}

// cachedReactions replaces the reactions each message's raw_json had
// when it was fetched with the reactions table's, which the socket
// keeps current: one query per conversation among msgs.
func (s *Slack) cachedReactions(msgs []rawMessage) {
	byChannel := map[string][]string{}
	for _, r := range msgs {
		byChannel[r.channelID] = append(byChannel[r.channelID], r.ts)
	}
	got := map[[2]string][]cache.ReactionRow{}
	for ch, ts := range byChannel {
		rows, err := s.db.ReactionsFor(ch, ts)
		if err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: cached reactions:", err)
			return // keep raw_json's
		}
		for t, rs := range rows {
			got[[2]string{ch, t}] = rs
		}
	}
	for i := range msgs {
		rows := got[[2]string{msgs[i].channelID, msgs[i].ts}]
		msgs[i].reactions = msgs[i].reactions[:0:0]
		for _, row := range rows {
			msgs[i].reactions = append(msgs[i].reactions, slack.ItemReaction{Name: row.Emoji, Count: row.Count, Users: row.UserIDs})
		}
	}
}

// finish resolves the names msgs need and builds them.
func (s *Slack) finish(ctx context.Context, msgs []rawMessage, ids []string) []Message {
	s.resolveAll(ctx, ids)
	out := make([]Message, 0, len(msgs))
	for _, r := range msgs {
		out = append(out, s.message(ctx, r))
	}
	return out
}

// rawMessage is a message as the API or the socket delivered it, before
// names are resolved. Socket callbacks travel to Run as these.
type rawMessage struct {
	channelID, userID, ts, text, threadTS, subtype, botID, username string
	edited, ephemeral                                               bool
	replyCount                                                      int
	files                                                           []slack.File
	blocks                                                          []blocks.Block
	attachments                                                     []slack.Attachment
	reactions                                                       []slack.ItemReaction
}

// rawFromSlack is the message as slack-go decoded it.
func rawFromSlack(channelID string, m slack.Message) rawMessage {
	return rawMessage{
		channelID: channelID, userID: m.User, ts: m.Timestamp, text: m.Text,
		threadTS: m.ThreadTimestamp, subtype: m.SubType, botID: m.BotID, username: m.Username,
		edited: m.Edited != nil, replyCount: m.ReplyCount,
		files: m.Files, blocks: blockkit.Parse(m.Blocks), attachments: m.Attachments,
		reactions: m.Reactions,
	}
}

// mentioned lists the users whose names the message needs: its author
// and everyone mentioned in its text or rich_text.
func (r rawMessage) mentioned() []string {
	ids := []string{r.userID}
	ids = append(ids, slackfmt.MentionedUserIDs(r.text)...)
	for _, b := range r.blocks {
		if rt, ok := b.(blocks.RichTextBlock); ok {
			ids = append(ids, rich.UserIDs(rt.Elements)...)
		}
	}
	return ids
}

// message builds a displayable Message from a raw one.
func (s *Slack) message(ctx context.Context, r rawMessage) Message {
	author := r.username
	authorID := r.userID
	switch {
	case r.userID == slackbotID:
		if author == "" {
			author = "Slackbot"
		}
	case r.userID != "":
		if author == "" {
			author = s.userName(ctx, r.userID)
		}
	case r.botID != "":
		authorID = r.botID
		name := s.botName(ctx, r.botID)
		if author == "" {
			author = name
		}
	default:
		if author == "" {
			author = "unknown"
		}
	}
	when, _ := export.TimeFromTS(r.ts)
	isReply := r.threadTS != "" && r.threadTS != r.ts
	body := rich.Body(r.text, r.blocks, s)
	body = append(body, rich.FromAttachments(blockkit.ParseAttachments(r.attachments), s)...)
	body = append(body, rich.FromFiles(r.files)...)
	if r.subtype == "me_message" {
		rich.Italicize(body) // a /me line, italic as in Slack's clients
	}
	return Message{
		ChannelID:    r.channelID,
		TS:           r.ts,
		Author:       author,
		AuthorID:     authorID,
		AvatarURL:    s.avatarURL(authorID),
		Body:         body,
		Text:         r.text,
		Time:         when,
		Edited:       r.edited,
		ThreadTS:     r.threadTS,
		InThreadOnly: isReply && r.subtype != "thread_broadcast",
		ReplyCount:   r.replyCount,
		Ephemeral:    r.ephemeral,
		FromSelf:     r.userID != "" && r.userID == s.client.UserID(),
		Reactions:    reactions(r.reactions, s.client.UserID()),
	}
}

// reactions converts Slack's reactions, marking those self is in.
func reactions(rs []slack.ItemReaction, self string) []Reaction {
	if len(rs) == 0 {
		return nil
	}
	out := make([]Reaction, 0, len(rs))
	for _, r := range rs {
		out = append(out, Reaction{Name: r.Name, Count: r.Count, Users: r.Users, Mine: slices.Contains(r.Users, self)})
	}
	return out
}

// React adds or removes the user's reaction and queues the change for
// Run, as Send queues a message: the socket's echo, when it comes, is
// a no-op by then. Slack saying the reaction is already there (or
// already gone) is what the user asked for, so it is no error.
func (s *Slack) React(ctx context.Context, channelID, ts, name string, add bool) error {
	var err error
	if add {
		err = s.client.AddReaction(ctx, channelID, ts, name)
	} else {
		err = s.client.RemoveReaction(ctx, channelID, ts, name)
	}
	if err != nil && !isSlackError(err, "already_reacted", "no_reaction") {
		return err
	}
	select {
	case s.queue <- rawReaction{channelID, ts, s.client.UserID(), name, add}:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// isSlackError reports whether err is a Slack API error with one of
// codes.
func isSlackError(err error, codes ...string) bool {
	var se slack.SlackErrorResponse
	if errors.As(err, &se) {
		return slices.Contains(codes, se.Err)
	}
	return slices.Contains(codes, err.Error())
}

// slackbotID is Slackbot's user ID; it needs no lookup.
const slackbotID = "USLACKBOT"

// Commands lists the workspace's slash commands, fetched once.
func (s *Slack) Commands(ctx context.Context) ([]Command, error) {
	s.mu.Lock()
	cached := s.commands
	s.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	all, err := s.client.AppCommands(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Command, 0, len(all))
	for _, c := range all {
		if c.AliasOf != "" {
			continue
		}
		out = append(out, Command{Name: c.Name, Usage: c.Usage, Desc: c.Desc, App: c.AppName})
	}
	slices.SortFunc(out, func(a, b Command) int { return strings.Compare(a.Name, b.Name) })
	s.mu.Lock()
	s.commands = out
	s.mu.Unlock()
	return out, nil
}

// CustomEmoji lists the workspace's own emoji, fetched once.
func (s *Slack) CustomEmoji(ctx context.Context) (map[string]string, error) {
	s.mu.Lock()
	cached := s.customs
	s.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	customs, err := s.client.ListCustomEmoji(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.customs = customs
	s.mu.Unlock()
	return customs, nil
}

// RunCommand runs a slash command as Slack's web client does, as
// cmd/slk's runCommand does for the TUI.
func (s *Slack) RunCommand(ctx context.Context, channelID, threadTS, name, args string) error {
	err := s.client.RunCommand(ctx, slackclient.CommandRequest{
		ChannelID: channelID,
		ThreadTS:  threadTS,
		Command:   name,
		Args:      args,
		TeamID:    s.client.TeamID(),
	})
	var rej *slackclient.CommandRejectedError
	if errors.As(err, &rej) {
		return &core.CommandError{Code: rej.Code}
	}
	return err
}

// Profile fetches a person's profile once and remembers it.
func (s *Slack) Profile(ctx context.Context, id string) (Profile, error) {
	s.mu.Lock()
	p, ok := s.profiles[id]
	s.mu.Unlock()
	if ok {
		return p, nil
	}
	if strings.HasPrefix(id, "B") {
		b, err := s.client.GetBotInfo(ctx, id)
		if err != nil {
			return Profile{}, err
		}
		if b == nil {
			return Profile{}, fmt.Errorf("no app %s", id)
		}
		p = Profile{ID: id, Name: b.Name, Title: "App", AvatarURL: b.Icons.Image72}
	} else {
		u, err := s.client.GetUserProfileContext(ctx, id)
		if err != nil {
			return Profile{}, err
		}
		if u.User == nil {
			return Profile{}, fmt.Errorf("no user %s", id)
		}
		s.persistUser(id, u.Name, u.Profile.DisplayName, u.Profile.Image72, u.IsBot)
		name := s.rememberUser(id, u.Name, u.Profile.DisplayName, u.RealName, u.Profile.Image72)
		p = Profile{
			ID:          id,
			Name:        name,
			RealName:    u.RealName,
			Handle:      u.Name,
			Title:       u.Profile.Title,
			StatusEmoji: u.Profile.StatusEmoji,
			StatusText:  u.Profile.StatusText,
			TZLabel:     u.TZLabel,
			TZOffset:    u.TZOffset,
			HasTZ:       u.TZ != "",
			AvatarURL:   u.Profile.Image192,
		}
		if p.AvatarURL == "" {
			p.AvatarURL = u.Profile.Image72
		}
		if u.IsBot {
			p.Title = "App"
		}
	}
	s.mu.Lock()
	s.profiles[id] = p
	s.mu.Unlock()
	return p, nil
}

// Send posts text. The posted message is queued for Run right away,
// built from Slack's reply, because the socket does not reliably echo
// one's own messages; if it does, the echo replaces this copy (same ts)
// and is what the cache keeps.
func (s *Slack) Send(ctx context.Context, channelID, text string) error {
	ts, mr, err := s.client.SendMessage(ctx, channelID, text)
	if err != nil {
		return err
	}
	_, block := mrkdwn.Convert(text)
	var bs []blocks.Block
	if block != nil {
		bs = []blocks.Block{blocks.RichTextBlock{Elements: block.Elements}}
	}
	s.queue <- rawMessage{channelID: channelID, userID: s.client.UserID(), ts: ts, text: mr, blocks: bs}
	return nil
}

// SendReply posts in a thread and queues the reply for Run, as Send
// does. A broadcast reply is a thread_broadcast, in the channel too.
func (s *Slack) SendReply(ctx context.Context, channelID, threadTS, text string, broadcast bool) error {
	ts, mr, err := s.client.SendReply(ctx, channelID, threadTS, text, broadcast)
	if err != nil {
		return err
	}
	_, block := mrkdwn.Convert(text)
	var bs []blocks.Block
	if block != nil {
		bs = []blocks.Block{blocks.RichTextBlock{Elements: block.Elements}}
	}
	r := rawMessage{channelID: channelID, userID: s.client.UserID(), ts: ts, text: mr, threadTS: threadTS, blocks: bs}
	if broadcast {
		r.subtype = "thread_broadcast"
	}
	s.queue <- r
	return nil
}

// Edit replaces one of the user's messages and queues the new version
// for Run, as Send does; Slack's message_changed echo replaces it. A
// broadcast reply stays one, so the channel keeps showing it.
func (s *Slack) Edit(ctx context.Context, m Message, text string) error {
	mr, err := s.client.EditMessage(ctx, m.ChannelID, m.TS, text)
	if err != nil {
		return err
	}
	_, block := mrkdwn.Convert(text)
	var bs []blocks.Block
	if block != nil {
		bs = []blocks.Block{blocks.RichTextBlock{Elements: block.Elements}}
	}
	r := rawMessage{channelID: m.ChannelID, userID: s.client.UserID(), ts: m.TS, text: mr,
		threadTS: m.ThreadTS, blocks: bs, edited: true}
	if m.ThreadTS != "" && m.ThreadTS != m.TS && !m.InThreadOnly {
		r.subtype = "thread_broadcast"
	}
	if cached, ok := s.cachedRaw(m.ChannelID, m.TS); ok {
		// The cached message keeps its files and attachments; only the
		// text changes.
		cached.text, cached.blocks, cached.edited = mr, bs, true
		r = cached
	}
	select {
	case s.queue <- r:
	case <-ctx.Done():
	}
	return nil
}

// cachedRaw is the cached message at ts as a rawMessage, with the
// row's own thread, reply count and subtype.
func (s *Slack) cachedRaw(channelID, ts string) (rawMessage, bool) {
	if s.db == nil {
		return rawMessage{}, false
	}
	row, err := s.db.GetMessage(channelID, ts)
	if err != nil {
		return rawMessage{}, false
	}
	var m slack.Message
	if err := json.Unmarshal([]byte(row.RawJSON), &m); err != nil || m.Timestamp == "" {
		return rawMessage{}, false
	}
	r := rawFromSlack(channelID, m)
	r.threadTS, r.replyCount = row.ThreadTS, row.ReplyCount
	if row.Subtype != "" {
		r.subtype = row.Subtype
	}
	return r, true
}

// Delete deletes one of the user's messages and queues the deletion for
// Run as the socket reports one: the cache drops it and the windows
// hear a MessageDeleted. The socket's own report then changes nothing.
func (s *Slack) Delete(ctx context.Context, channelID, ts string) error {
	if err := s.client.RemoveMessage(ctx, channelID, ts); err != nil {
		return err
	}
	ev := slackclient.MessageEvent{
		ChannelID: channelID, Kind: slackclient.MessageDeleted,
		Message: slack.Message{Msg: slack.Msg{Channel: channelID, Timestamp: ts}},
	}
	select {
	case s.queue <- rawEvent{ev}:
	case <-ctx.Done():
	}
	return nil
}

// Permalink asks Slack for a link to the message.
func (s *Slack) Permalink(ctx context.Context, channelID, ts string) (string, error) {
	return s.client.GetPermalink(ctx, channelID, ts)
}

// MarkThreadRead marks a thread read up to ts, as viewing it in Slack
// does.
func (s *Slack) MarkThreadRead(ctx context.Context, channelID, threadTS, ts string) error {
	return s.client.MarkThread(ctx, channelID, threadTS, ts)
}

// Thread answers from the cache when it holds the thread and fetches
// the whole thread in the background (one conversations.replies call),
// its changes arriving through Run as a HistoryUpdated. A thread the
// cache holds nothing of is fetched first. Without a cache every open
// goes to Slack.
func (s *Slack) Thread(ctx context.Context, channelID, threadTS string) ([]Message, error) {
	if s.sync == nil {
		raw, err := s.client.GetReplies(ctx, channelID, threadTS)
		if err != nil {
			return nil, err
		}
		return s.fromSlack(ctx, channelID, raw), nil
	}
	rows, err := s.cachedThread(channelID, threadTS)
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		// Nothing, or only the parent: what there is to read is in
		// Slack. (A parent with no replies yet costs one request too,
		// which is what opening a thread is.)
		done := s.trackIn("opening thread in", channelID)
		_, err := s.sync.Thread(ctx, channelID, threadTS)
		done()
		if err != nil {
			return nil, err
		}
		if rows, err = s.cachedThread(channelID, threadTS); err != nil {
			return nil, err
		}
	} else {
		go s.fetchThread(ctx, channelID, threadTS)
	}
	return s.fromCache(ctx, rows), nil
}

// cachedThread is the thread's cached rows, oldest first: the replies
// and the parent, which a parent cached before it had replies is not
// among (its thread_ts is empty).
func (s *Slack) cachedThread(channelID, threadTS string) ([]cache.Message, error) {
	rows, err := s.db.GetThreadReplies(channelID, threadTS)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 && rows[0].TS == threadTS {
		return rows, nil
	}
	parent, err := s.db.GetMessage(channelID, threadTS)
	if errors.Is(err, sql.ErrNoRows) || err == nil && parent.IsDeleted {
		return rows, nil
	}
	if err != nil {
		return nil, err
	}
	return append([]cache.Message{parent}, rows...), nil
}

// fetchThread brings a cached thread up to Slack's and hands the
// changes to Run, as catchUp does for a conversation.
func (s *Slack) fetchThread(ctx context.Context, channelID, threadTS string) {
	done := s.trackIn("thread in", channelID)
	changed, err := s.sync.Thread(ctx, channelID, threadTS)
	done()
	if err != nil {
		if ctx.Err() == nil {
			fmt.Fprintln(os.Stderr, "slk-gio: thread in", channelID+":", err)
		}
		return
	}
	if changed.Empty() {
		return
	}
	select {
	case s.queue <- rawChanged{changed: changed, threadTS: threadTS}:
	case <-ctx.Done():
	}
}

type (
	// rawEvent is a message frame, whole, for the cache and the windows.
	rawEvent     struct{ ev slackclient.MessageEvent }
	rawConnected bool
	rawMarked    struct {
		channelID, ts    string
		unread, mentions int
	}
	// rawSections is a section change, applied to the store on Run's
	// goroutine.
	rawSections func(*service.SectionStore)
	rawPref     struct{ name, value string }
	rawPresence struct{ user, presence string }
	rawReaction struct {
		channelID, ts, user, emoji string
		added                      bool
	}
	// rawChanged is a finished fetch's changes, emitted as
	// HistoryUpdated; threadTS is set for a thread's fetch.
	rawChanged struct {
		changed  msgsync.Changed
		threadTS string
	}
	// rawSync says the sync status changed, emitted as SyncChanged.
	rawSync struct{}
	// rawCounts is client.counts after a reconnect, emitted as one
	// ReadChanged per conversation.
	rawCounts struct{ unreads []slackclient.UnreadInfo }
)

// lookupTimeout bounds the name lookups Run makes inline: a hung
// request must not stall event delivery for every window.
const lookupTimeout = 10 * time.Second

// reconnectGate is the least time between two reconnect catch-ups, so a
// flapping socket does not ask client.counts over and over.
const reconnectGate = 30 * time.Second

func (s *Slack) Run(ctx context.Context, emit func(Event)) error {
	// The connection manager redials with backoff after every drop
	// (the 60 s read deadline, a Slack-side close, sleep, a network
	// change); the handler hears OnDisconnect and OnConnect each time.
	cm := slackclient.NewConnectionManager(s.client, socketHandler{queue: s.queue})
	go cm.Run(ctx)
	defer cm.Stop()
	s.mu.Lock()
	s.runCtx = ctx
	auto := s.autoStart
	s.mu.Unlock()
	if auto != nil {
		if err := s.StartBackfill(*auto); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: backfill:", err)
		}
	}
	var (
		up          bool      // what the windows were last told
		wasUp       bool      // the socket has been up at least once
		lastCatchUp time.Time // the last connect catch-up
		// conn is the current connection's background work (its
		// catch-up), ended when the socket drops.
		conn connScope
	)
	defer conn.end()
	for {
		select {
		case <-ctx.Done():
			return nil
		case item := <-s.queue:
			// Names are resolved inline, possibly over the network;
			// a deadline keeps one slow lookup from holding up the
			// queue (a lookup that times out shows the ID).
			lctx, cancel := context.WithTimeout(ctx, lookupTimeout)
			switch v := item.(type) {
			case rawMessage:
				// Names the message needs are resolved first, so they
				// render as names, not IDs.
				s.resolveAll(lctx, v.mentioned())
				emit(MessagePosted{Message: s.message(lctx, v)})
			case rawEvent:
				s.applyEvent(lctx, v.ev, emit)
			case rawReaction:
				if s.sync != nil {
					if err := s.sync.ApplyReaction(v.channelID, v.ts, v.user, v.emoji, v.added); err != nil {
						fmt.Fprintln(os.Stderr, "slk-gio: cache reaction:", err)
					}
				}
				emit(ReactionChanged{ChannelID: v.channelID, TS: v.ts, Name: v.emoji, UserID: v.user,
					Added: v.added, Self: v.user == s.client.UserID()})
			case rawSync:
				s.mu.Lock()
				s.syncPending = false
				s.mu.Unlock()
				emit(SyncChanged{})
			case rawChanged:
				emit(HistoryUpdated{
					ChannelID: v.changed.ChannelID,
					Messages:  s.fromCache(lctx, v.changed.Upserted),
					Deleted:   v.changed.Deleted,
					ThreadTS:  v.threadTS,
				})
			case rawCounts:
				for _, u := range v.unreads {
					emit(ReadChanged{ChannelID: u.ChannelID, LastRead: u.LastRead, Unread: u.HasUnread, Mentions: u.MentionCount})
				}
			case rawConnected:
				now := bool(v)
				s.connected.Store(now)
				if now == up {
					break // a failed redial reports down again; the windows know
				}
				up = now
				switch {
				case now:
					fmt.Fprintf(os.Stderr, "slk-gio: %s websocket connected\n", time.Now().Format(time.TimeOnly))
					s.mu.Lock()
					peers := s.peers
					s.mu.Unlock()
					if len(peers) > 0 {
						_ = s.client.SubscribePresence(peers)
					}
					// The socket replays nothing that happened while it
					// was down (or before slk-gio started): one
					// client.counts brings every conversation's read
					// state and newest message, and the conversations
					// the cache follows that got new messages are caught
					// up a page each. Open windows catch up themselves
					// when they hear the connection is back.
					if !wasUp || time.Since(lastCatchUp) > reconnectGate {
						lastCatchUp = time.Now()
						go s.afterConnect(conn.begin(ctx), !wasUp)
					}
					wasUp = true
				default:
					conn.end() // a catch-up in flight stops with the socket
					fmt.Fprintf(os.Stderr, "slk-gio: %s websocket disconnected; reconnecting\n", time.Now().Format(time.TimeOnly))
					if s.db != nil {
						// No span reaches the head any more.
						if err := s.db.ClearLiveSpans(s.client.TeamID()); err != nil {
							fmt.Fprintln(os.Stderr, "slk-gio: cache:", err)
						}
					}
				}
				emit(ConnectionChanged{Connected: now})
			case rawPresence:
				emit(PresenceChanged{UserID: v.user, Active: v.presence == "active"})
			case rawMarked:
				emit(ReadChanged{ChannelID: v.channelID, LastRead: v.ts, Unread: v.unread > 0, Mentions: v.mentions})
			case rawSections:
				v(s.sections)
				emit(SectionsChanged{Membership: s.membership()})
			case rawPref:
				if s.mutes.ApplyPrefChange(v.name, v.value) {
					muted := map[string]bool{}
					for _, id := range s.mutes.MutedChannels() {
						muted[id] = true
					}
					emit(MutesChanged{Muted: muted})
				}
			}
			cancel()
		}
	}
}

// connScope is one socket connection's background work: begin starts
// it (ending the previous one's), end stops it.
type connScope struct{ cancel context.CancelFunc }

func (c *connScope) begin(parent context.Context) context.Context {
	c.end()
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	return ctx
}

func (c *connScope) end() {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
}

// afterConnect runs once per connect (gated by reconnectGate): one
// client.counts for every conversation's read state and newest message.
// After a reconnect the read state goes to Run for the list (the first
// connect's is what Conversations just loaded); then the conversations
// the socket missed are caught up (catchUpAfterConnect).
func (s *Slack) afterConnect(ctx context.Context, first bool) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	unreads, _, err := s.client.GetUnreadCountsContext(cctx)
	cancel()
	if err != nil {
		if ctx.Err() == nil {
			fmt.Fprintln(os.Stderr, "slk-gio: unread counts after connect:", err)
		}
		return
	}
	if !first {
		select {
		case s.queue <- rawCounts{unreads}:
		case <-ctx.Done():
			return
		}
	}
	s.catchUpAfterConnect(ctx, unreads)
}

// applyEvent writes a message frame to the cache and tells the windows.
func (s *Slack) applyEvent(ctx context.Context, ev slackclient.MessageEvent, emit func(Event)) {
	if s.sync != nil {
		if _, err := s.sync.ApplyEvent(ev, true); err != nil {
			fmt.Fprintln(os.Stderr, "slk-gio: cache message:", err)
		}
	}
	if ev.Kind == slackclient.MessageDeleted {
		emit(MessageDeleted{ChannelID: ev.ChannelID, TS: ev.Message.Timestamp})
		return
	}
	r := rawFromSlack(ev.ChannelID, ev.Message)
	// A changed message is a new version of one already delivered,
	// whatever changed (the text, or a parent's reply count).
	r.edited = r.edited || ev.Kind == slackclient.MessageChanged
	s.resolveAll(ctx, r.mentioned())
	emit(MessagePosted{Message: s.message(ctx, r)})
}

// socketHandler forwards the socket events the GUI uses to Run and
// ignores the rest. The connection manager calls OnDisconnect on every
// drop and failed redial, and Slack's hello brings OnConnect. Message frames come whole through OnMessageEvent;
// OnMessage and OnMessageDeleted are kept for the interface and are
// not reached.
type socketHandler struct{ queue chan<- any }

var (
	_ slackclient.EventHandler        = socketHandler{}
	_ slackclient.MessageEventHandler = socketHandler{}
)

// OnMessageEvent is a new, changed or deleted message, whole.
func (h socketHandler) OnMessageEvent(ev slackclient.MessageEvent) {
	h.queue <- rawEvent{ev}
}

func (h socketHandler) OnMessage(channelID, userID, ts, text, threadTS, subtype string, edited bool, files []slack.File, bs slack.Blocks, attachments []slack.Attachment, botID, username string) {
	h.queue <- rawEvent{slackclient.MessageEvent{
		ChannelID: channelID,
		Kind:      slackclient.MessageNew,
		Message: slack.Message{Msg: slack.Msg{
			Channel: channelID, User: userID, Timestamp: ts, Text: text, ThreadTimestamp: threadTS,
			SubType: subtype, Files: files, Blocks: bs, Attachments: attachments, BotID: botID, Username: username,
		}},
	}}
}

// OnEphemeralMessage forwards a reply only the user can see, such as
// Slackbot answering /remind. It is never cached.
func (h socketHandler) OnEphemeralMessage(m slackclient.EphemeralMessage) {
	h.queue <- rawMessage{
		channelID: m.ChannelID, userID: m.UserID, ts: m.TS, text: m.Text, threadTS: m.ThreadTS, subtype: m.Subtype,
		botID: m.BotID, username: m.Username, ephemeral: true,
		blocks: blockkit.Parse(m.Blocks), attachments: m.Attachments,
	}
}

// OnChannelMarked is any client marking a conversation read or unread.
func (h socketHandler) OnChannelMarked(channelID, ts string, unread, mentions int) {
	h.queue <- rawMarked{channelID, ts, unread, mentions}
}

// Sidebar section changes, as cmd/slk/sections.go applies them.
func (h socketHandler) OnChannelSectionUpserted(ev slackclient.ChannelSectionUpserted) {
	h.queue <- rawSections(func(st *service.SectionStore) { st.ApplyUpsert(ev) })
}
func (h socketHandler) OnChannelSectionDeleted(id string) {
	h.queue <- rawSections(func(st *service.SectionStore) { st.ApplyDelete(id) })
}
func (h socketHandler) OnChannelSectionChannelsUpserted(id string, ids []string) {
	h.queue <- rawSections(func(st *service.SectionStore) { st.ApplyChannelsAdded(id, ids) })
}
func (h socketHandler) OnChannelSectionChannelsRemoved(id string, ids []string) {
	h.queue <- rawSections(func(st *service.SectionStore) { st.ApplyChannelsRemoved(id, ids) })
}

// OnPresenceChange is a subscribed person going active or away.
func (h socketHandler) OnPresenceChange(user, presence string) {
	h.queue <- rawPresence{user, presence}
}

// OnPrefChange carries mute changes (muted_channels,
// all_notifications_prefs); MuteStore ignores the rest.
func (h socketHandler) OnPrefChange(name, value string) {
	h.queue <- rawPref{name, value}
}

// Reactions are cached and drawn.
func (h socketHandler) OnReactionAdded(channelID, ts, userID, emoji string) {
	h.queue <- rawReaction{channelID, ts, userID, emoji, true}
}
func (h socketHandler) OnReactionRemoved(channelID, ts, userID, emoji string) {
	h.queue <- rawReaction{channelID, ts, userID, emoji, false}
}

func (h socketHandler) OnMessageDeleted(channelID, ts string) {
	h.queue <- rawEvent{slackclient.MessageEvent{
		ChannelID: channelID, Kind: slackclient.MessageDeleted,
		Message: slack.Message{Msg: slack.Msg{Channel: channelID, Timestamp: ts}},
	}}
}
func (h socketHandler) OnConnect()    { h.queue <- rawConnected(true) }
func (h socketHandler) OnDisconnect() { h.queue <- rawConnected(false) }

func (socketHandler) OnViewOpened(string)                                           {}
func (socketHandler) OnUserTyping(string, string)                                   {}
func (socketHandler) OnAssistantStatus(slackclient.AssistantStatus)                 {}
func (socketHandler) OnSelfPresenceChange(string)                                   {}
func (socketHandler) OnDNDChange(bool, int64)                                       {}
func (socketHandler) OnThreadMarked(string, string, string, slackclient.Subscribed) {}
func (socketHandler) OnThreadSubscriptionChanged(string, string, string, bool)      {}
func (socketHandler) OnConversationOpened(slack.Channel)                            {}
func (socketHandler) OnMemberJoined(string, string)                                 {}
func (socketHandler) OnMemberLeft(string, string)                                   {}
func (socketHandler) OnUserStatusChange(string, slackclient.UserStatus)             {}
func (socketHandler) OnUserInvalidated(string)                                      {}
func (socketHandler) OnDNDInvalidated(string)                                       {}
func (socketHandler) OnUserDNDChange(string, bool, int64)                           {}
