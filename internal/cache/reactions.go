package cache

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ReactionRow struct {
	Emoji   string
	UserIDs []string
	Count   int
}

func (db *DB) UpsertReaction(messageTS, channelID, emoji string, userIDs []string, count int) error {
	userIDsJSON, err := json.Marshal(userIDs)
	if err != nil {
		return fmt.Errorf("marshal user_ids: %w", err)
	}

	_, err = db.conn.Exec(`
		INSERT INTO reactions (message_ts, channel_id, emoji, user_ids, count)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(message_ts, channel_id, emoji)
		DO UPDATE SET user_ids = excluded.user_ids, count = excluded.count`,
		messageTS, channelID, emoji, string(userIDsJSON), count,
	)
	if err != nil {
		return fmt.Errorf("upserting reaction: %w", err)
	}
	return nil
}

func (db *DB) GetReactions(messageTS, channelID string) ([]ReactionRow, error) {
	rows, err := db.conn.Query(`
		SELECT emoji, user_ids, count
		FROM reactions
		WHERE message_ts = ? AND channel_id = ?`,
		messageTS, channelID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying reactions: %w", err)
	}
	defer rows.Close()

	var result []ReactionRow
	for rows.Next() {
		var r ReactionRow
		var userIDsJSON string
		if err := rows.Scan(&r.Emoji, &userIDsJSON, &r.Count); err != nil {
			return nil, fmt.Errorf("scanning reaction row: %w", err)
		}
		if err := json.Unmarshal([]byte(userIDsJSON), &r.UserIDs); err != nil {
			return nil, fmt.Errorf("unmarshal user_ids: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (db *DB) DeleteReaction(messageTS, channelID, emoji string) error {
	_, err := db.conn.Exec(`
		DELETE FROM reactions
		WHERE message_ts = ? AND channel_id = ? AND emoji = ?`,
		messageTS, channelID, emoji,
	)
	if err != nil {
		return fmt.Errorf("deleting reaction: %w", err)
	}
	return nil
}

// reactionsChunk bounds how many message timestamps one ReactionsFor
// query binds.
const reactionsChunk = 500

// ReactionsFor returns the reactions on each of the channel's messages
// named by ts, keyed by message ts, each message's in the order they
// were first added (rowid; an upsert keeps it). A message with none is
// absent. It is one query per reactionsChunk messages, for a page.
func (db *DB) ReactionsFor(channelID string, ts []string) (map[string][]ReactionRow, error) {
	out := map[string][]ReactionRow{}
	for len(ts) > 0 {
		n := min(len(ts), reactionsChunk)
		chunk := ts[:n]
		ts = ts[n:]
		args := make([]any, 0, n+1)
		args = append(args, channelID)
		for _, t := range chunk {
			args = append(args, t)
		}
		rows, err := db.conn.Query(`
			SELECT message_ts, emoji, user_ids, count
			FROM reactions
			WHERE channel_id = ? AND message_ts IN (?`+strings.Repeat(",?", n-1)+`)
			ORDER BY rowid`, args...)
		if err != nil {
			return nil, fmt.Errorf("querying reactions: %w", err)
		}
		for rows.Next() {
			var ts, userIDsJSON string
			var r ReactionRow
			if err := rows.Scan(&ts, &r.Emoji, &userIDsJSON, &r.Count); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scanning reaction row: %w", err)
			}
			if err := json.Unmarshal([]byte(userIDsJSON), &r.UserIDs); err != nil {
				rows.Close()
				return nil, fmt.Errorf("unmarshal user_ids: %w", err)
			}
			out[ts] = append(out[ts], r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
