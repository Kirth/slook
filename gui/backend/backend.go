// Package backend is what the Gio prototype's windows talk to: a
// conversation list, a channel's history, sending, and a stream of live
// events. It has two implementations, Slack (a real workspace through
// internal/slack) and Fake (made-up conversations that post on a timer).
//
// This is a stopgap. The TUI's ports in internal/core return and push
// the TUI's own message types, so a second frontend cannot use them yet;
// once the events move into core, this interface should be replaced by
// those ports.
package backend

import (
	"context"
	"strings"
	"time"

	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/core/blocks"
	imgpkg "github.com/gammons/slk/internal/image"
)

// Kind is a conversation's type, spelled as slackfmt.ChannelGlyph
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
	Notify   NotifyLevel
}

// NotifyLevel is how a conversation notifies on the desktop. It is
// slk's own setting, kept in its cache; Slack's notification settings
// are not changed.
type NotifyLevel int

const (
	// NotifyDefault follows [notifications]: mentions, DMs and
	// keywords, and nothing while the conversation is muted in Slack.
	NotifyDefault NotifyLevel = iota
	// NotifyAll notifies for every new message, muted in Slack or not.
	NotifyAll
	// NotifyNone never notifies.
	NotifyNone
)

// Details is what a conversation's header shows beyond its name.
type Details struct {
	Topic   string // one line, plain, for the header
	Purpose string
	// TopicBody and PurposeBody are the same as rich text, mentions
	// and links kept, for the info panel.
	TopicBody   []rich.Block
	PurposeBody []rich.Block
	Members     int       // zero when unknown
	Created     time.Time // zero when unknown
	CreatorID   string
	Creator     string // the creator's name
	Archived    bool
}

// Members is who is in a conversation. A Person slk has no name for
// yet carries only its ID. Partial means the list was cut short (a
// huge channel), Total being the conversation's size when known.
type Members struct {
	People  []Person
	Total   int
	Partial bool
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

// ChannelResult is a channel a search found: Member says the user is
// in it. Kind is KindChannel or KindPrivate; a search finds no DMs.
type ChannelResult struct {
	ID, Name string
	Kind     Kind
	Member   bool
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
	// ThreadUnread marks a parent whose thread the user follows and has
	// replies they have not read.
	ThreadUnread bool
	// Ephemeral marks a message only the user can see, such as a slash
	// command's reply. Slack does not keep it: it is gone on reload.
	Ephemeral bool
	// FromSelf marks the user's own messages, which never make a
	// conversation unread.
	FromSelf bool
	// Reactions are the emoji reactions on the message, in the order
	// they were first added.
	Reactions []Reaction
	// Actions are the legacy attachment actions of a message only the
	// user can see (an app's ephemeral reply), every action of every
	// attachment in order: rich.Button.Action indexes them. Nil for
	// other messages: Slack's web client presses buttons only on
	// ephemerals, which carry the action ids a press echoes.
	Actions []Action
}

// Action is one legacy attachment action: a button (Type "button") or
// a menu ("select"). Index is its place in Message.Actions. URL, when
// set, makes the button a link to open, not a press. Confirm, when set,
// is the question to ask before pressing. The rest is what a press
// echoes back to Slack.
type Action struct {
	Index   int
	Label   string
	Type    string
	Style   string // "", "default", "primary" or "danger"
	URL     string
	Confirm *blocks.ActionConfirm

	legacy       blocks.LegacyAction
	attachmentID int
	callbackID   string
	serviceID    string
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
	// StatusExpires is when the status lapses; zero when it does not.
	StatusExpires time.Time
	// DND is whether they have do-not-disturb on, until DNDUntil (zero
	// when unknown or open-ended). Known for people whose DND slk has
	// asked about (a DM's peer) or heard change.
	DND       bool
	DNDUntil  time.Time
	TZLabel   string
	TZOffset  int // seconds east of UTC
	HasTZ     bool
	AvatarURL string
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

// SelfStatus is the user's own presence and do-not-disturb: Away when
// they set themselves away (else Slack decides, "auto"), DND while a
// snooze or a scheduled do-not-disturb is on, until DNDUntil (zero when
// unknown).
type SelfStatus struct {
	Away     bool
	DND      bool
	DNDUntil time.Time
}

// SelfPresenceChanged is the user's own presence or do-not-disturb
// changing: set here (at once, and back if Slack refuses) or elsewhere.
type SelfPresenceChanged struct{ Status SelfStatus }

// UserGroup is one of the workspace's usergroups: the @team behind a
// <!subteam^S…> mention. Mine says the user is a member.
type UserGroup struct {
	ID          string
	Handle      string // what a mention reads, without the "@"
	Name        string
	Description string
	Members     []string // user IDs
	Mine        bool
}

// UserGroupsChanged is the workspace's usergroups changing (loaded at
// connect, or changed in Slack). Mine is the IDs of the user's own.
type UserGroupsChanged struct{ Mine []string }

// ThreadSummary is one thread the user follows, for the Threads view:
// where it is, its parent (author and a one-line preview), how many
// replies, when the last came, and whether some are unread.
type ThreadSummary struct {
	ChannelID string
	Channel   string // the conversation's name
	Kind      Kind
	ThreadTS  string
	AuthorID  string
	Author    string
	Preview   string
	Replies   int
	LastReply time.Time
	Unread    bool
}

// ThreadState is a followed thread becoming read or unread.
type ThreadState struct {
	ChannelID string
	ThreadTS  string
	Unread    bool
}

// ThreadsUnread is how many followed threads have unread replies, and
// how many of those mention the user (when known).
type ThreadsUnread struct{ Count, Mentions int }

// ThreadsChanged is the list of followed threads changing (a sync with
// Slack, a thread followed or left): a Threads view reads it again.
type ThreadsChanged struct{}

// ProfileChanged is someone's status, DND or profile changing: what
// was shown of them (a header, a hover card) should be asked again.
type ProfileChanged struct{ UserID string }

// ConversationOpened is a conversation the user newly has: a DM
// someone started, a channel they joined or were added to, or one a
// message arrived from that the list did not know. Read state comes
// with the conversation's messages.
type ConversationOpened struct{ Conv Conversation }

// Typing is someone typing in a conversation (Slack's user_typing,
// which names no thread). It lasts typing.Expiry unless repeated, and
// ends with their message.
type Typing struct {
	ChannelID string
	UserID    string
	Name      string
}

// AppStatus is an app's "working" status on a thread (Slack's
// ai_assistant_status): Status reads after its name ("is thinking…");
// empty clears it.
type AppStatus struct {
	ChannelID string
	ThreadTS  string
	BotUserID string
	Name      string
	Status    string
}

// NotifyChanged is a conversation's notify level changing.
type NotifyChanged struct {
	ChannelID string
	Level     NotifyLevel
}

// MutesChanged carries the complete set of muted conversations.
type MutesChanged struct{ Muted map[string]bool }

// SearchHit is one message the cache search matched. Message carries
// the conversation (ChannelID), the author, the body and, for a thread
// reply, ThreadTS and InThreadOnly.
type SearchHit struct {
	Message Message
}

// SearchQuery is what Search looks for, with names already resolved to
// IDs (gui/search.Resolve builds it from what the user typed). Every
// part that is set must hold; a query with nothing set finds nothing.
type SearchQuery struct {
	// All are groups of alternatives, one of each group the text must
	// match. An alternative is whole words in a row; a trailing * makes
	// the last one a prefix ("ig*" matches "ignore", "ig" does not).
	All [][]string
	// None are alternatives the text must not match.
	None []string
	// Users and Channels are authors and conversations a message must
	// be one of; NotUsers and NotChannels ones it must not.
	Users, NotUsers, Channels, NotChannels []string
	// Mentions are pieces of Slack markup (<@U1>, <!subteam^S1) the
	// text must contain one of; NotMentions none of.
	Mentions, NotMentions []string
	// Since and Until bound the message's time, [Since, Until); zero is
	// unbounded.
	Since, Until time.Time
	// Thread is a parent with replies, Reply a reply that was not also
	// sent to the channel, Edited an edited message, Link one linking
	// somewhere, File one with files, Reaction one with any reaction.
	Thread, Reply, Edited, Link, File, Reaction Tri
	// Emoji are reactions a message must have each of, NotEmoji ones it
	// must have none of: names as Slack keeps them, without colons.
	Emoji, NotEmoji []string
	// Nothing is set when part of the query matched no one (from:
	// someone unknown): it finds nothing.
	Nothing bool
}

// Tri is a yes/no filter that may be left out.
type Tri int8

const (
	Any Tri = iota // not filtered
	Yes
	No
)

// Words is a query for messages with every one of s's words (whole
// words; end one with * for a prefix).
func Words(s string) SearchQuery {
	var q SearchQuery
	for _, w := range strings.Fields(s) {
		q.All = append(q.All, []string{w})
	}
	return q
}

// Empty is whether q asks for nothing at all.
func (q SearchQuery) Empty() bool {
	return len(q.All)+len(q.None)+len(q.Users)+len(q.NotUsers)+len(q.Channels)+len(q.NotChannels)+
		len(q.Mentions)+len(q.NotMentions)+len(q.Emoji)+len(q.NotEmoji) == 0 &&
		q.Since.IsZero() && q.Until.IsZero() && !q.Nothing &&
		q.Thread == Any && q.Reply == Any && q.Edited == Any && q.Link == Any && q.File == Any && q.Reaction == Any
}

// Ranked is whether q has text to rank its hits by.
func (q SearchQuery) Ranked() bool { return len(q.All) > 0 }

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

func (MessagePosted) isEvent()       {}
func (MessageDeleted) isEvent()      {}
func (ConnectionChanged) isEvent()   {}
func (ReadChanged) isEvent()         {}
func (SectionsChanged) isEvent()     {}
func (MutesChanged) isEvent()        {}
func (NotifyChanged) isEvent()       {}
func (UserGroupsChanged) isEvent()   {}
func (Typing) isEvent()              {}
func (ConversationOpened) isEvent()  {}
func (ProfileChanged) isEvent()      {}
func (ThreadState) isEvent()         {}
func (ThreadsUnread) isEvent()       {}
func (ThreadsChanged) isEvent()      {}
func (AppStatus) isEvent()           {}
func (PresenceChanged) isEvent()     {}
func (SelfPresenceChanged) isEvent() {}
func (HistoryUpdated) isEvent()      {}
func (ReactionChanged) isEvent()     {}
func (SyncChanged) isEvent()         {}

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
	// Upload posts files to a conversation, or in the thread under
	// threadTS, with caption as the message's text (on the last file:
	// Slack shows each file it is sent alone as its own message). It
	// calls progress(n, total) as the nth file starts. It is not
	// optimistic: the files come back through Run once Slack has them.
	Upload(ctx context.Context, channelID, threadTS string, files []core.PendingAttachment, caption string, progress func(done, total int)) error
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
	// Forward shares m in another conversation as its permalink, which
	// Slack shows as a preview of m, after comment when there is one.
	// The posted message comes back through Run as a MessagePosted.
	Forward(ctx context.Context, m Message, toChannelID, comment string) error
	// Threads lists the threads the user follows, newest activity
	// first, from the cache; it makes no request.
	Threads(ctx context.Context) ([]ThreadSummary, error)
	// RefreshThreads brings the followed threads up to date with Slack,
	// at most every half hour (a sweep of up to ~60 requests); when it
	// is done Run reports ThreadsChanged.
	RefreshThreads()
	// MarkThreadRead marks a thread read up to ts in Slack.
	MarkThreadRead(ctx context.Context, channelID, threadTS, ts string) error
	// MarkUnread moves the read mark of a conversation (threadTS empty)
	// or of the thread under threadTS back to boundaryTS: the message
	// before the first one to read again; "0" for a conversation's
	// start, the parent's ts for a thread's first reply. The new state
	// comes back through Run as a ReadChanged (Unread, with the mentions
	// after the boundary counted) or a ThreadState (Unread).
	MarkUnread(ctx context.Context, channelID, threadTS, boundaryTS string) error
	// Typing tells Slack the user is typing in a conversation; callers
	// throttle it (typing.Throttle). Best-effort: a failure is dropped.
	Typing(channelID string)
	// Run delivers events to emit until ctx is done.
	Run(ctx context.Context, emit func(Event)) error
	// Images downloads avatars and pictures with the workspace's
	// credentials. Nil means none can be shown.
	Images() *imgpkg.Fetcher
	// Profile describes a user or app by ID, for the hover card.
	// Changes arrive through Run as ProfileChanged.
	Profile(ctx context.Context, id string) (Profile, error)
	// WatchDND asks Slack for someone's do-not-disturb state once (one
	// request), for a DM's header; it lands in their Profile and comes
	// back as a ProfileChanged. Later changes arrive by themselves.
	WatchDND(userID string)
	// People lists the people slk knows in the workspace (everyone it
	// has seen), for completing @mentions. It makes no request: Slack's
	// full member list can be huge, and the people the user talks with
	// are the ones slk has seen.
	People(ctx context.Context) ([]Person, error)
	// UserGroups is the workspace's usergroups as last loaded, by
	// handle; it makes no request. Changes arrive as UserGroupsChanged.
	UserGroups() []UserGroup
	// Commands lists the slash commands available, without aliases,
	// sorted by name.
	Commands(ctx context.Context) ([]Command, error)
	// RunCommand runs a slash command (name with its slash) in a
	// conversation, or in the thread under threadTS when it is set. Its
	// result arrives through Run: a posted message or an ephemeral
	// reply. Slack refusing it is a *core.CommandError.
	RunCommand(ctx context.Context, channelID, threadTS, name, args string) error
	// PressAction presses a button of a message only the user can see
	// (chat.attachmentAction, as Slack's web client does). Slack answers
	// through Run: an ephemeral replaced (a MessagePosted) or gone (a
	// MessageDeleted), or a new message.
	PressAction(ctx context.Context, m Message, a Action) error
	// Sections is the user's Slack sidebar sections in Slack's order;
	// empty when they are unknown.
	Sections() []Section
	// MarkRead marks a conversation read up to ts in Slack.
	MarkRead(ctx context.Context, channelID, ts string) error
	// Visited records that the user opened a conversation.
	Visited(channelID string)
	// Conversation describes one conversation by ID, asking Slack: for
	// a message from a conversation the list does not know. It is also
	// reported through Run as a ConversationOpened.
	Conversation(ctx context.Context, id string) (Conversation, error)
	// SearchChannels asks Slack which channels match query, joined or
	// not (edgeapi channels/search): for finding one the user is not
	// in. Callers debounce (one request per pause in typing); an empty
	// query is nil with no request.
	SearchChannels(ctx context.Context, query string) ([]ChannelResult, error)
	// Join joins a channel (a no-op when the user is in it already) and
	// describes it. It is also reported through Run as a
	// ConversationOpened.
	Join(ctx context.Context, channelID string) (Conversation, error)
	// OpenConversation opens the DM with one person, or the group DM
	// with up to MaxRecipients (conversations.open): an existing one is
	// reused. The user and repeats are left out of userIDs. A new one is
	// also reported through Run as a ConversationOpened.
	OpenConversation(ctx context.Context, userIDs []string) (Conversation, error)
	// Details fetches a conversation's topic, purpose and size.
	Details(ctx context.Context, channelID string) (Details, error)
	// Members lists who is in a conversation: from the cache when it
	// was listed lately, else from Slack, at most a few pages (5,000
	// people) so a huge channel costs a bounded number of requests.
	Members(ctx context.Context, channelID string) (Members, error)
	// SetNotify sets a conversation's notify level; Run reports it as a
	// NotifyChanged.
	SetNotify(ctx context.Context, channelID string, level NotifyLevel) error
	// WebURL is the conversation in Slack's web client, or "" if there
	// is none.
	WebURL(channelID string) string
	// Presence reports whether a person is active now. Changes for the
	// people the user has DMs with arrive through Run.
	Presence(ctx context.Context, userID string) (active bool, err error)
	// SelfStatus asks Slack for the user's own presence and
	// do-not-disturb (users.getPresence, dnd.info). Changes arrive
	// through Run as SelfPresenceChanged.
	SelfStatus(ctx context.Context) (SelfStatus, error)
	// SetPresence sets the user away, or back to active (Slack's
	// "auto"). Snooze turns do-not-disturb on for minutes; EndSnooze
	// ends a snooze; EndDND ends a snooze and a scheduled
	// do-not-disturb both. Each is optimistic: the change is reported
	// through Run at once and taken back if Slack refuses it.
	SetPresence(ctx context.Context, away bool) error
	Snooze(ctx context.Context, minutes int) error
	EndSnooze(ctx context.Context) error
	EndDND(ctx context.Context) error
	// Download makes sure an image or file is on disk, at
	// AttachmentPath(a), and returns that path. Asking again is free.
	Download(ctx context.Context, a Attachment) (path string, err error)
	// CustomEmoji is the workspace's own emoji: name to picture URL, or
	// "alias:<name>" (see emoji.BuildCustomEmojiURL).
	CustomEmoji(ctx context.Context) (map[string]string, error)
	// Search finds cached messages matching q, in order, at most limit
	// of them; replies included. Without text to rank by the order is
	// newest first.
	Search(ctx context.Context, q SearchQuery, limit int, order SearchOrder) ([]SearchHit, error)
	// SyncStatus reports what the cache holds and what is being fetched.
	SyncStatus(ctx context.Context) (SyncStatus, error)
	// StartBackfill fills the cache back to horizon (0 for everything)
	// in the background; an error when one is already running. Progress
	// arrives as SyncChanged events.
	StartBackfill(horizon time.Duration) error
	// StopBackfill ends a running backfill.
	StopBackfill()
}
