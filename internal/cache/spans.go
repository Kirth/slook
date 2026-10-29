package cache

import (
	"fmt"
	"strings"
)

// Span records that every channel-feed message (the rows
// conversations.history returns: top-level messages, thread parents,
// broadcasts) with OldestTS <= ts <= LatestTS is in the messages table
// (as a row, possibly soft-deleted). Spans are what lets a reader tell
// "the cache has this range" from "the cache has some rows in this
// range", which a growing cache fed by pages and live events otherwise
// cannot distinguish.
//
// A channel usually has one span. It has more after a fetch that could
// not be joined to what was cached, for example a reconnect's head page
// when more than a page was posted while offline; the hole between two
// spans is filled by the next page fetched across it, which merges them.
type Span struct {
	ChannelID   string
	WorkspaceID string
	OldestTS    string // inclusive
	LatestTS    string // inclusive
	// ReachesStart means OldestTS is the channel's first message: a page
	// walking backwards ended with has_more=false.
	ReachesStart bool
	// Live means the span's LatestTS is the channel's head and the
	// WebSocket is connected, so every new message extends it. Cleared
	// for the workspace on disconnect: the socket does not replay.
	Live bool
}

// Spans returns the channel's coverage spans, oldest first.
func (db *DB) Spans(channelID string) ([]Span, error) {
	rows, err := db.conn.Query(`
		SELECT channel_id, workspace_id, oldest_ts, latest_ts, reaches_start, is_live
		FROM message_spans WHERE channel_id = ? ORDER BY oldest_ts ASC`, channelID)
	if err != nil {
		return nil, fmt.Errorf("listing spans: %w", err)
	}
	defer rows.Close()
	var out []Span
	for rows.Next() {
		var s Span
		var reaches, live int
		if err := rows.Scan(&s.ChannelID, &s.WorkspaceID, &s.OldestTS, &s.LatestTS, &reaches, &live); err != nil {
			return nil, fmt.Errorf("scanning span: %w", err)
		}
		s.ReachesStart, s.Live = reaches == 1, live == 1
		out = append(out, s)
	}
	return out, rows.Err()
}

// AddSpan records s, merging it with every stored span it overlaps
// (sharing at least one ts). The merged span is the union of the
// bounds; it reaches the start or is live if any part did. Spans that
// do not overlap stay separate: whether two ranges are adjacent is the
// caller's knowledge (a page anchored at a span's boundary), and the
// caller expresses it by widening s to touch the boundary.
//
// ts strings compare as strings here, as they do everywhere in this
// package (AdvanceChannelLatestSyncedTS): Slack emits them at a fixed
// width, so lexical order is numeric order.
func (db *DB) AddSpan(s Span) error {
	if s.ChannelID == "" || s.OldestTS == "" || s.LatestTS == "" {
		return fmt.Errorf("adding span: channel and both bounds are required")
	}
	if strings.Compare(s.OldestTS, s.LatestTS) > 0 {
		return fmt.Errorf("adding span: oldest %s is after latest %s", s.OldestTS, s.LatestTS)
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("adding span: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT oldest_ts, latest_ts, reaches_start, is_live FROM message_spans
		WHERE channel_id = ? AND oldest_ts <= ? AND latest_ts >= ?`,
		s.ChannelID, s.LatestTS, s.OldestTS)
	if err != nil {
		return fmt.Errorf("adding span: %w", err)
	}
	var overlapping []string
	for rows.Next() {
		var oldest, latest string
		var reaches, live int
		if err := rows.Scan(&oldest, &latest, &reaches, &live); err != nil {
			rows.Close()
			return fmt.Errorf("adding span: %w", err)
		}
		overlapping = append(overlapping, oldest)
		if strings.Compare(oldest, s.OldestTS) < 0 {
			s.OldestTS = oldest
		}
		if strings.Compare(latest, s.LatestTS) > 0 {
			s.LatestTS = latest
		}
		s.ReachesStart = s.ReachesStart || reaches == 1
		s.Live = s.Live || live == 1
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("adding span: %w", err)
	}
	rows.Close()

	for _, oldest := range overlapping {
		if _, err := tx.Exec(`DELETE FROM message_spans WHERE channel_id = ? AND oldest_ts = ?`, s.ChannelID, oldest); err != nil {
			return fmt.Errorf("adding span: %w", err)
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO message_spans (channel_id, workspace_id, oldest_ts, latest_ts, reaches_start, is_live)
		VALUES (?, ?, ?, ?, ?, ?)`,
		s.ChannelID, s.WorkspaceID, s.OldestTS, s.LatestTS, boolToInt(s.ReachesStart), boolToInt(s.Live)); err != nil {
		return fmt.Errorf("adding span: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("adding span: %w", err)
	}
	return nil
}

// ExtendLiveSpan moves the channel's live span's latest bound forward
// to toTS, for a message the WebSocket just delivered. It reports false
// when the channel has no live span, in which case the caller records
// a new live span for the message itself: the socket guarantees nothing
// about what came before it.
func (db *DB) ExtendLiveSpan(channelID, toTS string) (bool, error) {
	res, err := db.conn.Exec(`
		UPDATE message_spans SET latest_ts = MAX(latest_ts, ?)
		WHERE channel_id = ? AND is_live = 1`, toTS, channelID)
	if err != nil {
		return false, fmt.Errorf("extending live span: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("extending live span: %w", err)
	}
	return n > 0, nil
}

// ClearLiveSpans marks every span of the workspace as not live: the
// WebSocket dropped, so no span is known to reach the head any more.
func (db *DB) ClearLiveSpans(workspaceID string) error {
	if _, err := db.conn.Exec(`UPDATE message_spans SET is_live = 0 WHERE workspace_id = ? AND is_live = 1`, workspaceID); err != nil {
		return fmt.Errorf("clearing live spans: %w", err)
	}
	return nil
}

// Covers reports whether one span holds the whole of [oldest, latest].
func (db *DB) Covers(channelID, oldest, latest string) (bool, error) {
	var n int
	err := db.conn.QueryRow(`
		SELECT count(*) FROM message_spans
		WHERE channel_id = ? AND oldest_ts <= ? AND latest_ts >= ?`, channelID, oldest, latest).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking span coverage: %w", err)
	}
	return n > 0, nil
}

// SpansForWorkspace returns every span of the workspace, by channel and
// then oldest first: what a coverage view reads in one query.
func (db *DB) SpansForWorkspace(workspaceID string) ([]Span, error) {
	rows, err := db.conn.Query(`
		SELECT channel_id, workspace_id, oldest_ts, latest_ts, reaches_start, is_live
		FROM message_spans WHERE workspace_id = ? ORDER BY channel_id, oldest_ts ASC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing workspace spans: %w", err)
	}
	defer rows.Close()
	var out []Span
	for rows.Next() {
		var s Span
		var reaches, live int
		if err := rows.Scan(&s.ChannelID, &s.WorkspaceID, &s.OldestTS, &s.LatestTS, &reaches, &live); err != nil {
			return nil, fmt.Errorf("scanning span: %w", err)
		}
		s.ReachesStart, s.Live = reaches == 1, live == 1
		out = append(out, s)
	}
	return out, rows.Err()
}
