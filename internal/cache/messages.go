package cache

import (
	"database/sql"
	"fmt"

	"github.com/gammons/slk/internal/debuglog"
)

type Message struct {
	TS          string
	ChannelID   string
	WorkspaceID string
	UserID      string
	Text        string
	ThreadTS    string
	ReplyCount  int
	EditedAt    string
	IsDeleted   bool
	RawJSON     string
	CreatedAt   int64
	// Subtype mirrors Slack's `subtype` field. We persist it so that
	// thread_broadcast messages (thread replies that the author also
	// posted to the channel) survive a restart and can be labeled in
	// the main channel feed.
	Subtype string
	// Version is the opaque stamp conversations.history's latest_updates
	// gave this message, echoed back as cached_latest_updates so the
	// server can answer "unchanged" instead of resending the body. Empty
	// when Slack has not versioned it (a WebSocket delivery); an upsert
	// with an empty Version keeps the stored one.
	Version string
}

// messageColumns is the column list every message SELECT reads, in the
// order scanMessage expects.
const messageColumns = `ts, channel_id, workspace_id, user_id, text, thread_ts, reply_count, edited_at, is_deleted, raw_json, created_at, subtype, version`

// mainFeedShape selects the rows conversations.history returns: plain
// top-level messages (thread_ts = ”), thread parents (thread_ts = ts,
// the form a parent takes once it has replies) and thread broadcasts.
// Plain replies (thread_ts != ” && thread_ts != ts && not a broadcast)
// belong to the thread pane, not the main feed.
const mainFeedShape = `(thread_ts = '' OR thread_ts = ts OR subtype = 'thread_broadcast')`

// upsertMessageSQL is the one statement every message write uses. It is
// authoritative: what the API returns replaces what is stored,
// including is_deleted (a message the server still returns is not
// deleted). version is the exception: a write that carries none keeps
// the stored stamp, so a WebSocket delivery does not unversion a
// message the server versioned.
const upsertMessageSQL = `
	INSERT INTO messages (ts, channel_id, workspace_id, user_id, text, thread_ts, reply_count, edited_at, is_deleted, raw_json, created_at, subtype, version)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(ts, channel_id) DO UPDATE SET
		user_id=excluded.user_id,
		text=excluded.text,
		thread_ts=excluded.thread_ts,
		reply_count=excluded.reply_count,
		edited_at=excluded.edited_at,
		is_deleted=excluded.is_deleted,
		raw_json=excluded.raw_json,
		subtype=excluded.subtype,
		version=CASE WHEN excluded.version != '' THEN excluded.version ELSE messages.version END`

func upsertMessageArgs(m Message) []any {
	return []any{m.TS, m.ChannelID, m.WorkspaceID, m.UserID, m.Text, m.ThreadTS,
		m.ReplyCount, m.EditedAt, boolToInt(m.IsDeleted), m.RawJSON, m.CreatedAt, m.Subtype, m.Version}
}

func (db *DB) UpsertMessage(m Message) error {
	_, err := db.conn.Exec(upsertMessageSQL, upsertMessageArgs(m)...)
	if err != nil {
		debuglog.Cache("UpsertMessage: channel=%s ts=%s ERR=%v", m.ChannelID, m.TS, err)
		return fmt.Errorf("upserting message: %w", err)
	}
	debuglog.Cache("UpsertMessage: channel=%s ts=%s thread_ts=%s subtype=%q deleted=%v edited=%q",
		m.ChannelID, m.TS, m.ThreadTS, m.Subtype, m.IsDeleted, m.EditedAt)
	return nil
}

// UpsertMessages writes a page of messages in one transaction: either
// every row lands or none does, so a reader never sees half a page
// (and a coverage span is never recorded over rows that are not there).
func (db *DB) UpsertMessages(ms []Message) error {
	if len(ms) == 0 {
		return nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("upserting messages: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(upsertMessageSQL)
	if err != nil {
		return fmt.Errorf("upserting messages: %w", err)
	}
	defer stmt.Close()
	for _, m := range ms {
		if _, err := stmt.Exec(upsertMessageArgs(m)...); err != nil {
			debuglog.Cache("UpsertMessages: channel=%s ts=%s ERR=%v", m.ChannelID, m.TS, err)
			return fmt.Errorf("upserting message %s/%s: %w", m.ChannelID, m.TS, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("upserting messages: %w", err)
	}
	debuglog.Cache("UpsertMessages: channel=%s count=%d", ms[0].ChannelID, len(ms))
	return nil
}

// Edit is what a message_changed event carries for a message the cache
// already holds. Zero values mean "not in the event", not "clear".
type Edit struct {
	Text    string
	RawJSON string
	// EditedAt is the edit's ts (edited.ts); empty when the change was
	// not a user edit (the server updating a parent's reply_count).
	EditedAt string
	// ThreadTS is set when the event carries one; a parent gaining its
	// first reply arrives as a change with thread_ts = ts.
	ThreadTS string
	// ReplyCount is applied as a maximum: it is idempotent with the
	// increment a reply event itself causes, so neither path can count
	// a reply twice.
	ReplyCount int
}

// ApplyEdit applies a message_changed to a cached row. It touches only
// what the event is about: text, raw_json and edited_at, plus thread_ts
// and reply_count when the event carries them. It never touches
// subtype (a changed frame reports "" for a thread_broadcast's) or
// version (vouching a stale version makes the server send the new body,
// so leaving it is self-healing). Returns false, and writes nothing,
// when the row is absent: an edit is not enough to make a message.
func (db *DB) ApplyEdit(channelID, ts string, e Edit) (bool, error) {
	res, err := db.conn.Exec(`
		UPDATE messages SET
			text = ?,
			raw_json = ?,
			edited_at = CASE WHEN ? != '' THEN ? ELSE edited_at END,
			thread_ts = CASE WHEN ? != '' THEN ? ELSE thread_ts END,
			reply_count = MAX(reply_count, ?)
		WHERE channel_id = ? AND ts = ?`,
		e.Text, e.RawJSON, e.EditedAt, e.EditedAt, e.ThreadTS, e.ThreadTS, e.ReplyCount, channelID, ts)
	if err != nil {
		debuglog.Cache("ApplyEdit: channel=%s ts=%s ERR=%v", channelID, ts, err)
		return false, fmt.Errorf("applying edit: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("applying edit: %w", err)
	}
	debuglog.Cache("ApplyEdit: channel=%s ts=%s applied=%v", channelID, ts, n > 0)
	return n > 0, nil
}

// IncrementReplyCount counts a live reply on its cached parent. A
// missing parent is left alone: the next history page brings the
// server's count.
func (db *DB) IncrementReplyCount(channelID, parentTS string) error {
	_, err := db.conn.Exec(`UPDATE messages SET reply_count = reply_count + 1 WHERE channel_id = ? AND ts = ?`,
		channelID, parentTS)
	if err != nil {
		return fmt.Errorf("incrementing reply count: %w", err)
	}
	return nil
}

// GetMessages returns the NEWEST `limit` messages for a channel,
// ordered by ts ascending in the result. If beforeTS is non-empty,
// only considers messages with ts < beforeTS (for pagination).
//
// We pick newest-first inside a subquery and re-sort ascending in the
// outer query so callers (UI render path, history backfill) get the
// recency-anchored window they want without having to reverse it
// themselves. A naive `ORDER BY ts ASC LIMIT N` would return the
// OLDEST N rows, which is wrong for any cache that keeps growing as
// new messages arrive.
func (db *DB) GetMessages(channelID string, limit int, beforeTS string) ([]Message, error) {
	inner := `
		SELECT ` + messageColumns + `
		FROM messages
		WHERE channel_id = ? AND is_deleted = 0
		  AND ` + mainFeedShape
	args := []any{channelID}

	if beforeTS != "" {
		inner += " AND ts < ?"
		args = append(args, beforeTS)
	}

	inner += " ORDER BY ts DESC LIMIT ?"
	args = append(args, limit)

	query := "SELECT * FROM (" + inner + ") ORDER BY ts ASC"

	return db.queryMessages(query, args...)
}

// GetMessagesSince returns the cached main-feed messages at or after
// sinceTS, oldest first. The row-shape filter matches GetMessages: plain
// top-level messages, thread parents, and thread broadcasts, but not
// plain thread replies, which belong to the thread panel.
//
// Inclusive of sinceTS, because its caller is mark-unread: the message
// the user selected is itself unread.
//
// This reads the cache, not Slack, so it can only see messages slk has
// already fetched. For the case it exists to serve — counting mentions
// below a boundary the user just picked off their own screen — those
// messages are cached by construction. Marking something unread far
// enough back can undercount; client.counts corrects it on the next
// reconnect.
func (db *DB) GetMessagesSince(channelID, sinceTS string) ([]Message, error) {
	if sinceTS == "" {
		return nil, nil
	}
	return db.queryMessages(`
		SELECT `+messageColumns+`
		FROM messages
		WHERE channel_id = ? AND is_deleted = 0 AND ts >= ?
		  AND `+mainFeedShape+`
		ORDER BY ts ASC`, channelID, sinceTS)
}

// GetMessagesBetween returns the cached main-feed messages with
// oldest <= ts <= latest, oldest first. Either bound may be "" to leave
// that side open.
func (db *DB) GetMessagesBetween(channelID, oldest, latest string) ([]Message, error) {
	query := `
		SELECT ` + messageColumns + `
		FROM messages
		WHERE channel_id = ? AND is_deleted = 0
		  AND ` + mainFeedShape
	args := []any{channelID}
	if oldest != "" {
		query += " AND ts >= ?"
		args = append(args, oldest)
	}
	if latest != "" {
		query += " AND ts <= ?"
		args = append(args, latest)
	}
	query += " ORDER BY ts ASC"
	return db.queryMessages(query, args...)
}

// OldestCachedTS returns the ts of the oldest main-feed message cached
// for the channel, or "" when there is none.
func (db *DB) OldestCachedTS(channelID string) (string, error) {
	var ts sql.NullString
	err := db.conn.QueryRow(`
		SELECT MIN(ts) FROM messages
		WHERE channel_id = ? AND is_deleted = 0 AND `+mainFeedShape, channelID).Scan(&ts)
	if err != nil {
		return "", fmt.Errorf("oldest cached ts: %w", err)
	}
	if !ts.Valid {
		return "", nil
	}
	return ts.String, nil
}

// GetMessage returns the message with the given (channel_id, ts) primary
// key. Returns sql.ErrNoRows if no such message exists.
func (db *DB) GetMessage(channelID, ts string) (Message, error) {
	row := db.conn.QueryRow(`
		SELECT `+messageColumns+`
		FROM messages
		WHERE channel_id = ? AND ts = ?
	`, channelID, ts)
	return scanMessage(row)
}

func (db *DB) GetThreadReplies(channelID, threadTS string) ([]Message, error) {
	query := `
		SELECT ` + messageColumns + `
		FROM messages
		WHERE channel_id = ? AND thread_ts = ? AND is_deleted = 0
		ORDER BY ts ASC`

	return db.queryMessages(query, channelID, threadTS)
}

// PruneThreadReplies soft-deletes the thread's cached replies that are
// not in keep, the ts set of a complete conversations.replies response.
// Without it a reply Slack no longer returns (a Slackbot ephemeral that
// arrived over the WebSocket, a deletion missed while offline) stays in
// the thread forever, and when it is the newest row it outranks every
// read cursor the thread can be marked with.
//
// Only rows cached before insertedBefore (unix seconds, the moment the
// fetch was issued) are candidates: a reply the WebSocket delivered
// while the request was in flight is missing from the response because
// it is newer than it, not because Slack dropped it. The parent row is
// never touched. Returns the number of rows pruned.
func (db *DB) PruneThreadReplies(channelID, threadTS string, keep []string, insertedBefore int64) (int, error) {
	pruned, err := db.PruneThreadRepliesTS(channelID, threadTS, keep, insertedBefore)
	return len(pruned), err
}

// PruneThreadRepliesTS is PruneThreadReplies returning the ts of the
// replies it soft-deleted, for a caller that tells a view about them.
func (db *DB) PruneThreadRepliesTS(channelID, threadTS string, keep []string, insertedBefore int64) ([]string, error) {
	candidates, err := db.queryMessages(`
		SELECT `+messageColumns+`
		FROM messages
		WHERE channel_id = ? AND thread_ts = ? AND ts != thread_ts
		  AND is_deleted = 0 AND created_at < ?`, channelID, threadTS, insertedBefore)
	if err != nil {
		return nil, fmt.Errorf("pruning thread replies: %w", err)
	}
	return db.pruneNotKept(channelID, candidates, keep)
}

// PruneChannelRange is PruneThreadReplies for the channel feed: it
// soft-deletes the cached main-feed rows with oldest <= ts <= latest
// that keep does not name, where keep is every ts a history page
// covering exactly that range returned or confirmed unchanged. A
// deletion missed while offline is thereby applied the next time the
// range is fetched.
//
// The bounds must be the page's own (its oldest and newest ts, widened
// to an anchor the page was asked to include), never "to now": a live
// message newer than the page is not in it because it is newer, not
// because it is gone. The created_at < insertedBefore guard covers the
// same race within the bounds. Thread broadcasts are not candidates:
// the official client asks history with ignore_replies, and whether
// that hides broadcasts is unmeasured, so a missing broadcast is not
// evidence of deletion. Thread replies are never candidates. Returns the
// ts of the rows pruned.
func (db *DB) PruneChannelRange(channelID, oldest, latest string, keep []string, insertedBefore int64) ([]string, error) {
	if oldest == "" || latest == "" {
		return nil, fmt.Errorf("pruning channel range: bounds are required")
	}
	candidates, err := db.queryMessages(`
		SELECT `+messageColumns+`
		FROM messages
		WHERE channel_id = ? AND ts >= ? AND ts <= ?
		  AND is_deleted = 0 AND created_at < ?
		  AND (thread_ts = '' OR thread_ts = ts)
		  AND subtype != 'thread_broadcast'`, channelID, oldest, latest, insertedBefore)
	if err != nil {
		return nil, fmt.Errorf("pruning channel range: %w", err)
	}
	return db.pruneNotKept(channelID, candidates, keep)
}

// pruneNotKept soft-deletes the candidates keep does not name and
// returns their ts.
func (db *DB) pruneNotKept(channelID string, candidates []Message, keep []string) ([]string, error) {
	kept := make(map[string]bool, len(keep))
	for _, ts := range keep {
		kept[ts] = true
	}
	var pruned []string
	for _, m := range candidates {
		if kept[m.TS] {
			continue
		}
		if err := db.DeleteMessage(channelID, m.TS); err != nil {
			return pruned, err
		}
		pruned = append(pruned, m.TS)
	}
	return pruned, nil
}

func (db *DB) DeleteMessage(channelID, ts string) error {
	_, err := db.conn.Exec(`UPDATE messages SET is_deleted = 1 WHERE channel_id = ? AND ts = ?`, channelID, ts)
	if err != nil {
		debuglog.Cache("DeleteMessage: channel=%s ts=%s ERR=%v", channelID, ts, err)
		return fmt.Errorf("deleting message: %w", err)
	}
	debuglog.Cache("DeleteMessage: channel=%s ts=%s", channelID, ts)
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMessage(r rowScanner) (Message, error) {
	var m Message
	var isDeleted int
	if err := r.Scan(&m.TS, &m.ChannelID, &m.WorkspaceID, &m.UserID, &m.Text,
		&m.ThreadTS, &m.ReplyCount, &m.EditedAt, &isDeleted, &m.RawJSON, &m.CreatedAt, &m.Subtype, &m.Version); err != nil {
		return m, err
	}
	m.IsDeleted = isDeleted == 1
	return m, nil
}

func (db *DB) queryMessages(query string, args ...any) ([]Message, error) {
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// MessageCounts returns, per channel, how many non-deleted messages the
// workspace has cached, thread replies included.
func (db *DB) MessageCounts(workspaceID string) (map[string]int, error) {
	rows, err := db.conn.Query(`
		SELECT channel_id, count(*) FROM messages
		WHERE workspace_id = ? AND is_deleted = 0 GROUP BY channel_id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("counting messages: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var ch string
		var n int
		if err := rows.Scan(&ch, &n); err != nil {
			return nil, fmt.Errorf("scanning message count: %w", err)
		}
		out[ch] = n
	}
	return out, rows.Err()
}
