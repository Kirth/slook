package cache

import "fmt"

// ThreadRef names a thread by its channel and parent ts.
type ThreadRef struct {
	ChannelID string
	TS        string
}

// ThreadsNeedingReplies lists the workspace's cached thread parents, at
// or after sinceTS and newest first, whose reply_count says the thread
// has more replies than the cache holds: the threads a backfill should
// fetch. A thread the user opened is complete and is not listed; a
// parent whose stale reply_count overstates its thread is listed once
// and corrected when the replies fetch upserts it. sinceTS "" means no
// lower bound.
func (db *DB) ThreadsNeedingReplies(workspaceID, sinceTS string, limit int) ([]ThreadRef, error) {
	rows, err := db.conn.Query(`
		SELECT p.channel_id, p.ts FROM messages p
		WHERE p.workspace_id = ? AND p.is_deleted = 0 AND p.reply_count > 0 AND p.ts >= ?
		  AND p.reply_count > (
			SELECT count(*) FROM messages r
			WHERE r.channel_id = p.channel_id AND r.thread_ts = p.ts AND r.ts != p.ts AND r.is_deleted = 0)
		ORDER BY p.ts DESC LIMIT ?`, workspaceID, sinceTS, limit)
	if err != nil {
		return nil, fmt.Errorf("listing threads needing replies: %w", err)
	}
	defer rows.Close()
	var out []ThreadRef
	for rows.Next() {
		var r ThreadRef
		if err := rows.Scan(&r.ChannelID, &r.TS); err != nil {
			return nil, fmt.Errorf("scanning thread ref: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
