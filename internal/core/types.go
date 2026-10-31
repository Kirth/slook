// Package core holds message values shared by the Slack client, the
// cache, the export and the GUI. It began as the TUI's ports; the TUI and
// its service interfaces were removed, and these values are what remained
// in use.
//
// core declares values only; it does no I/O itself.
package core

import (
	"github.com/gammons/slk/internal/core/blocks"
)

// MessageItem is one message in the form the export and the GUI read.
type MessageItem struct {
	TS          string
	UserName    string
	UserID      string
	Text        string
	Timestamp   string // formatted display time (e.g. "3:04 PM")
	DateStr     string // date string for grouping (e.g. "2026-04-23")
	ThreadTS    string
	ReplyCount  int
	Reactions   []ReactionItem
	Attachments []Attachment
	IsEdited    bool
	// Subtype mirrors Slack's `subtype` field on a message event.
	// We act on "thread_broadcast" (a thread reply that was also sent to
	// the channel), to render a label above it, and on "me_message" (a
	// /me line), to render its body in italics.
	Subtype string

	// Blocks holds parsed Slack Block Kit blocks. Rendered between
	// the body Text and the file Attachments by Phase 5.
	Blocks []blocks.Block

	// LegacyAttachments holds parsed entries from the legacy
	// `attachments` field (color stripe + title + fields style bot
	// cards). Rendered after Blocks.
	LegacyAttachments []blocks.LegacyAttachment

	// Ephemeral is set for a message only the current user can see
	// (Slack's is_ephemeral). It is never cached and never a
	// read-marking target; Slack does not return it from history.
	Ephemeral bool

	// BotID is the Slack bot_id of the app that posted the message. Set for
	// ephemerals; sent as service_id when one of their buttons is pressed.
	BotID string
}

// Attachment represents a file or image attached to a message.
// Kind is "image" for image/* mimetypes, "file" otherwise.
// URL is the user-facing permalink (preferred) or fallback to url_private.
type Attachment struct {
	Kind string // "image" or "file"
	Name string // display filename / title
	URL  string // permalink (preferred) or url_private

	// DownloadURL is the auth-gated url_private, used by the `d`
	// download keybinding. Size is the file size in bytes (0 when
	// Slack didn't provide one); shown in the file picker.
	DownloadURL string
	Size        int64

	// Populated only for Kind == "image":
	FileID string      // Slack file ID for cache key
	Mime   string      // e.g. "image/png"
	Thumbs []ThumbSpec // sorted ascending; empty for non-image
}

// ThumbSpec is one Slack thumbnail variant.
//
// This is intentionally distinct from image.ThumbSpec in the internal/image
// package to avoid coupling message data to the image package's internal
// type. A converter helper bridges the two where needed.
type ThumbSpec struct {
	URL string
	W   int
	H   int
}

type ReactionItem struct {
	Emoji      string // emoji name without colons, e.g. "thumbsup"
	Count      int
	HasReacted bool     // whether the current user has reacted with this emoji
	UserIDs    []string // user IDs who reacted with this emoji
}

// PendingAttachment is a file (or in-memory image) waiting to be
// uploaded with the next send. Bytes and Path are mutually exclusive:
// Bytes is set for clipboard-pasted images; Path is set for
// file-path-pasted files (read at upload time, not at attach time).
type PendingAttachment struct {
	Filename string
	Bytes    []byte // non-nil for clipboard images
	Path     string // non-empty for file-path attachments
	Mime     string
	Size     int64
}

// ReadState captures the per-channel read-state values that drive the
// unread dot, the mention badge, and the "new messages" line. It is the
// canonical type for passing read state across package boundaries.
type ReadState struct {
	LastReadTS string
	HasUnread  bool
	// MentionCount is the number of unread direct mentions: an explicit
	// @user or an @here/@channel/@everyone broadcast. For DMs and group
	// DMs (Slack's ims and mpims), client.counts is believed to report
	// every unread message here, which is what makes the sidebar badge
	// match the official client without a client-side branch — that
	// reading is unverified against a live capture; see UnreadInfo's doc
	// in internal/slack/client.go. Rendering gates it on HasUnread.
	MentionCount int
}

// ThreadSummary is one row in the Threads view: a thread the user is
// involved in (authored, replied to, or @-mentioned in). Computed from
// the local cache; v1 has no Slack-side authoritative data.
type ThreadSummary struct {
	ChannelID    string
	ChannelName  string
	ChannelType  string // "channel" | "private" | "dm" | "group_dm"
	ThreadTS     string
	ParentUserID string
	ParentText   string
	ParentTS     string
	ReplyCount   int // number of replies (does not count the parent)
	LastReplyTS  string
	LastReplyBy  string
	Unread       bool
}

// ActivityItem is a flattened, UI-friendly representation of a single
// activity.feed entry. The per-type extraction happens in the Slack
// client so the UI never touches the raw (and wildly type-dependent)
// JSON.
type ActivityItem struct {
	Key       string // stable dedupe/selection id
	Type      string // at_user, at_channel, thread_v2, message_reaction, dm, bot_dm_bundle, ...
	IsUnread  bool
	IsBot     bool
	FeedTS    string // sort key (newest first)
	ChannelID string // resolved target channel (from message OR bundle payload)
	TS        string // resolved target message ts (may be "")
	ThreadTS  string // set for thread_v2 (== thread root)
	AuthorID  string // message author (mentions); "" when unknown
	Reaction  string // emoji short name, for message_reaction only
	Subtype   string // saved_reminder's item_type ("reminder"); "" for other types
}

// ActivityMessage is a hydrated message body for one activity ref
// (channel + ts). The Activity feed itself returns only references; the
// body text and author are fetched separately via messages.list.
type ActivityMessage struct {
	Text   string
	UserID string
}

// ActivityMsgKey is the map key for a hydrated activity message,
// combining channel ID and message ts. Shared by the Slack client and
// the activity view so both sides agree on the lookup key.
func ActivityMsgKey(channelID, ts string) string {
	return channelID + "\x00" + ts
}

// CommandError is how adapters report Slack refusing a slash command
// (chat.command's {"ok":false,"error":Code}), so the UI never parses
// Slack error strings. Code is Slack's, e.g. "unknown_command".
type CommandError struct {
	Code string
}

func (e *CommandError) Error() string { return e.Code }

// Theme holds the user's color overrides from config.
type Theme struct {
	Primary     string `toml:"primary"`
	Accent      string `toml:"accent"`
	Warning     string `toml:"warning"`
	Error       string `toml:"error"`
	Background  string `toml:"background"`
	Surface     string `toml:"surface"`
	SurfaceDark string `toml:"surface_dark"`
	Text        string `toml:"text"`
	TextMuted   string `toml:"text_muted"`
	Border      string `toml:"border"`
}
