// Package backend is what the Gio prototype's windows talk to: a
// conversation list, a channel's history, sending, and a stream of live
// events. It has two implementations, Slack (a real workspace through
// internal/slack) and Fake (made-up conversations that post on a timer).
//
// This is a stopgap. The TUI's ports in internal/core return and push
// internal/ui message types, so a second frontend cannot use them yet;
// once the events move into core, this interface should be replaced by
// those ports.
package backend

import (
	"context"
	"time"

	"github.com/gammons/slk/gui/rich"
	imgpkg "github.com/gammons/slk/internal/image"
)

// Kind is a conversation's type, spelled as messages.ChannelGlyph
// expects it.
type Kind string

const (
	KindChannel Kind = "channel"
	KindPrivate Kind = "private"
	KindDM      Kind = "dm"
	KindGroupDM Kind = "group_dm"
)

// Conversation is one row of the conversation list. AvatarURL is the
// other person's picture for a direct message, when known. The rest is
// what the list arranges by: Slack's read state, when the conversation
// last had a message, when the user last opened it, and where Slack's
// sidebar puts it.
type Conversation struct {
	ID        string
	Name      string
	Kind      Kind
	AvatarURL string
	PeerID    string // the other person, for a direct message

	Unread   bool      // has messages the user has not read
	Mentions int       // unread mentions (for a DM, unread messages)
	Latest   time.Time // newest message; zero when unknown
	Visited  time.Time // the user last opened it here or in slk; zero if never
	Muted    bool
	Starred  bool
	Section  string // Slack sidebar section ID; empty for the default one
}

// Details is what a conversation's header shows beyond its name.
type Details struct {
	Topic   string
	Purpose string
	Members int // zero when unknown
}

// Section is one of the user's Slack sidebar sections. Type is Slack's:
// "standard" for the user's own, "channels" and "direct_messages" for
// the default homes of conversations not placed elsewhere, "stars".
type Section struct {
	ID    string
	Name  string
	Emoji string
	Type  string
}

// Message is one message, ready to display. Body is its content as
// blocks of styled spans; AuthorID keys the author's avatar and
// AvatarURL is a 72px picture, empty when none is known.
type Message struct {
	ChannelID string
	TS        string
	Author    string
	AuthorID  string
	AvatarURL string
	Body      []rich.Block
	// Text is the message as Slack holds it (mrkdwn, with mentions as
	// <@U…>): what an edit starts from.
	Text   string
	Time   time.Time
	Edited bool
	// ThreadTS is the parent's ts for a thread reply, empty otherwise.
	// A reply that was not also sent to the channel has InThreadOnly
	// set: a channel window counts it on its parent instead of showing it.
	ThreadTS     string
	InThreadOnly bool
	ReplyCount   int
	// Ephemeral marks a message only the user can see, such as a slash
	// command's reply. Slack does not keep it: it is gone on reload.
	Ephemeral bool
	// FromSelf marks the user's own messages, which never make a
	// conversation unread.
	FromSelf bool
	// Reactions are the emoji reactions on the message, in the order
	// they were first added.
	Reactions []Reaction
}

// Reaction is one emoji on a message: its name as Slack names it
// ("+1", "partyparrot", "wave::skin-tone-2"), how many reacted, who
// (Users may be shorter than Count: Slack truncates long lists), and
// whether the user is among them.
type Reaction struct {
	Name  string
	Count int
	Users []string
	Mine  bool
}

// Person is someone a message can mention: their display name, real
// name and @handle (any may be empty), and a 72px picture.
type Person struct {
	ID        string
	Name      string
	RealName  string
	Handle    string
	AvatarURL string
	Bot       bool
}

// Command is a slash command the workspace offers. App is the
// installed app providing it, empty for Slack's own.
type Command struct {
	Name  string
	Usage string
	Desc  string
	App   string
}

// Profile is what the hover card shows about a person (or an app).
type Profile struct {
	ID          string
	Name        string // display name
	RealName    string
	Handle      string // @-name
	Title       string
	StatusEmoji string // shortcode, e.g. ":palm_tree:"
	StatusText  string
	TZLabel     string
	TZOffset    int // seconds east of UTC
	HasTZ       bool
	AvatarURL   string
}

// Event is something that happened after startup. Run delivers them in
// order from a single goroutine.
type Event interface{ isEvent() }

// MessagePosted is a new message, or a new version of one already
// delivered (Edited is set).
type MessagePosted struct{ Message Message }

// MessageDeleted removes a message.
type MessageDeleted struct{ ChannelID, TS string }

// ReactionChanged is someone adding (Added) or removing a reaction on
// a message. Self marks the user's own.
type ReactionChanged struct {
	ChannelID, TS, Name, UserID string
	Added, Self                 bool
}

// ConnectionChanged reports the live connection going up or down.
type ConnectionChanged struct{ Connected bool }

// ReadChanged is a conversation marked read (or unread) by any client,
// including this one.
type ReadChanged struct {
	ChannelID string
	LastRead  string
	Unread    bool
	Mentions  int
}

// SectionsChanged says the user rearranged their Slack sidebar.
// Membership maps conversation ID to section ID; the order and names
// are Backend.Sections.
type SectionsChanged struct{ Membership map[string]string }

// PresenceChanged is a person going active or away.
type PresenceChanged struct {
	UserID string
	Active bool
}

// MutesChanged carries the complete set of muted conversations.
type MutesChanged struct{ Muted map[string]bool }

// SearchHit is one message the cache search matched. Message carries
// the conversation (ChannelID), the author, the body and, for a thread
// reply, ThreadTS and InThreadOnly.
type SearchHit struct {
	Message Message
}

// SearchOrder is how Search sorts its hits.
type SearchOrder int

const (
	// Relevance is best match first (how often and how prominently the
	// terms appear), newest first among equals.
	Relevance SearchOrder = iota
	// Newest is newest first.
	Newest
)

// Coverage is how much of one conversation the cache holds.
type Coverage struct {
	ID   string
	Name string
	Kind Kind
	// Messages is the cached rows, thread replies included.
	Messages int
	// Oldest is the oldest message a span covers; zero when nothing is
	// covered yet.
	Oldest time.Time
	// Complete means the oldest span reaches the conversation's first
	// message.
	Complete bool
	// Live means the newest span follows the socket: nothing posted
	// since is missing.
	Live bool
	// Holes is how many gaps lie between the spans.
	Holes int
	// Spans are the covered ranges, oldest first: what a progress bar
	// toward a horizon is computed from.
	Spans []TimeSpan
	// Ignored means the backfill skips this conversation (configured);
	// only what arrives live, or is opened, is cached.
	Ignored bool
}

// TimeSpan is a covered range of a conversation's history.
type TimeSpan struct {
	Oldest, Latest time.Time
}

// BackfillState is where the background backfill is.
type BackfillState int

const (
	BackfillOff     BackfillState = iota // not started, or stopped
	BackfillRunning                      // fetching
	BackfillPaused                       // holding while the user is busy
	BackfillDone                         // finished to the horizon
	BackfillFailed                       // ended with an error
)

// BackfillStatus is the backfill's progress.
type BackfillStatus struct {
	State   BackfillState
	Horizon time.Duration // 0 is everything
	// Phase is "conversations" or "threads"; Current names the item
	// being fetched (a conversation, with a thread ts when it is one).
	Phase   string
	Current string
	// Done and Total count the phase's items.
	Done, Total int
	// Requests is how many requests the run has made; Fetched how many
	// messages they brought in.
	Requests  int
	Fetched   int
	LastError string
	StartedAt time.Time
}

// SyncStatus is what the cache holds and what is being fetched.
type SyncStatus struct {
	// Messages is every cached message, replies included.
	Messages      int
	Conversations []Coverage
	Backfill      BackfillStatus
	// Active lists fetches in flight, worded for people: "catching up
	// #general", "older page in #random", "thread in #eng".
	Active []string
}

// SyncChanged says the sync status changed: a fetch started or ended,
// the backfill moved. The window asks SyncStatus for the new state.
type SyncChanged struct{}

// HistoryUpdated is the cache catching up with Slack for a conversation
// a window may be showing: Messages are rows written (new, edited, or
// re-fetched), Deleted the ts of messages Slack no longer has. It is
// not activity: nothing in it is new to the user in the way a
// MessagePosted is, so it never marks a conversation unread.
type HistoryUpdated struct {
	ChannelID string
	Messages  []Message
	Deleted   []string
	// ThreadTS is set when the update is one thread's fetch: Messages
	// are that thread's parent and replies, and a conversation window
	// refreshes the rows it shows rather than adding the parent, which
	// may be far older than anything it has loaded.
	ThreadTS string
}

func (MessagePosted) isEvent()     {}
func (MessageDeleted) isEvent()    {}
func (ConnectionChanged) isEvent() {}
func (ReadChanged) isEvent()       {}
func (SectionsChanged) isEvent()   {}
func (MutesChanged) isEvent()      {}
func (PresenceChanged) isEvent()   {}
func (HistoryUpdated) isEvent()    {}
func (ReactionChanged) isEvent()   {}
func (SyncChanged) isEvent()       {}

// Backend is one workspace.
type Backend interface {
	// Workspace is the workspace's name, for window titles.
	Workspace() string
	// Self is the user's own ID: whose messages are their own, whom a
	// mention names.
	Self() string
	// Conversations lists the conversations the user is in, sorted for
	// display.
	Conversations(ctx context.Context) ([]Conversation, error)
	// History returns a conversation's most recent messages, oldest
	// first. A backend with a cache answers from it and catches up with
	// Slack afterwards through a HistoryUpdated on Run.
	History(ctx context.Context, channelID string) ([]Message, error)
	// Older returns the messages before beforeTS, oldest first, for
	// scrolling back; empty when there are none.
	Older(ctx context.Context, channelID, beforeTS string) ([]Message, error)
	// Send posts text to a conversation. The message comes back through
	// Run as a MessagePosted, like anyone else's.
	Send(ctx context.Context, channelID, text string) error
	// Thread returns a thread's parent and replies, oldest first. Like
	// History, a backend with a cache answers from it and fetches the
	// thread from Slack afterwards, the changes arriving through Run as
	// a HistoryUpdated; a thread the cache holds nothing of is fetched
	// first.
	Thread(ctx context.Context, channelID, threadTS string) ([]Message, error)
	// SendReply posts text in the thread under threadTS, also to the
	// conversation when broadcast is set. It comes back through Run as
	// a MessagePosted.
	SendReply(ctx context.Context, channelID, threadTS, text string, broadcast bool) error
	// React adds the user's reaction name to a message, or removes it
	// (add false). The change comes back through Run as a
	// ReactionChanged.
	React(ctx context.Context, channelID, ts, name string, add bool) error
	// Edit replaces the text of one of the user's messages. The new
	// version comes back through Run as an edited MessagePosted.
	Edit(ctx context.Context, m Message, text string) error
	// Delete deletes one of the user's messages; Run reports it as a
	// MessageDeleted.
	Delete(ctx context.Context, channelID, ts string) error
	// Permalink is a link to the message in Slack.
	Permalink(ctx context.Context, channelID, ts string) (string, error)
	// MarkThreadRead marks a thread read up to ts in Slack.
	MarkThreadRead(ctx context.Context, channelID, threadTS, ts string) error
	// Run delivers events to emit until ctx is done.
	Run(ctx context.Context, emit func(Event)) error
	// Images downloads avatars and pictures with the workspace's
	// credentials. Nil means none can be shown.
	Images() *imgpkg.Fetcher
	// Profile describes a user or app by ID, for the hover card.
	Profile(ctx context.Context, id string) (Profile, error)
	// People lists the people slk knows in the workspace (everyone it
	// has seen), for completing @mentions. It makes no request: Slack's
	// full member list can be huge, and the people the user talks with
	// are the ones slk has seen.
	People(ctx context.Context) ([]Person, error)
	// Commands lists the slash commands available, without aliases,
	// sorted by name.
	Commands(ctx context.Context) ([]Command, error)
	// RunCommand runs a slash command (name with its slash) in a
	// conversation, or in the thread under threadTS when it is set. Its
	// result arrives through Run: a posted message or an ephemeral
	// reply. Slack refusing it is a *core.CommandError.
	RunCommand(ctx context.Context, channelID, threadTS, name, args string) error
	// Sections is the user's Slack sidebar sections in Slack's order;
	// empty when they are unknown.
	Sections() []Section
	// MarkRead marks a conversation read up to ts in Slack.
	MarkRead(ctx context.Context, channelID, ts string) error
	// Visited records that the user opened a conversation.
	Visited(channelID string)
	// Details fetches a conversation's topic, purpose and size.
	Details(ctx context.Context, channelID string) (Details, error)
	// WebURL is the conversation in Slack's web client, or "" if there
	// is none.
	WebURL(channelID string) string
	// Presence reports whether a person is active now. Changes for the
	// people the user has DMs with arrive through Run.
	Presence(ctx context.Context, userID string) (active bool, err error)
	// Download makes sure an image or file is on disk, at
	// AttachmentPath(a), and returns that path. Asking again is free.
	Download(ctx context.Context, a Attachment) (path string, err error)
	// CustomEmoji is the workspace's own emoji: name to picture URL, or
	// "alias:<name>" (see emoji.BuildCustomEmojiURL).
	CustomEmoji(ctx context.Context) (map[string]string, error)
	// Search finds cached messages matching query, newest first, at
	// most limit of them; replies included.
	Search(ctx context.Context, query string, limit int, order SearchOrder) ([]SearchHit, error)
	// SyncStatus reports what the cache holds and what is being fetched.
	SyncStatus(ctx context.Context) (SyncStatus, error)
	// StartBackfill fills the cache back to horizon (0 for everything)
	// in the background; an error when one is already running. Progress
	// arrives as SyncChanged events.
	StartBackfill(horizon time.Duration) error
	// StopBackfill ends a running backfill.
	StopBackfill()
}
