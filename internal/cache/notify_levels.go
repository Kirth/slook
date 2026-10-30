package cache

import "fmt"

// SetNotifyLevel records how a conversation notifies in slk, a level
// the frontend defines; 0, the default, removes the row. The level is
// slk's own: nothing of it is sent to Slack.
func (db *DB) SetNotifyLevel(workspaceID, channelID string, level int) error {
	var err error
	if level == 0 {
		_, err = db.conn.Exec(`DELETE FROM notify_levels WHERE workspace_id = ? AND channel_id = ?`, workspaceID, channelID)
	} else {
		_, err = db.conn.Exec(`
INSERT INTO notify_levels (workspace_id, channel_id, level) VALUES (?, ?, ?)
ON CONFLICT(workspace_id, channel_id) DO UPDATE SET level = excluded.level`, workspaceID, channelID, level)
	}
	if err != nil {
		return fmt.Errorf("setting notify level of %s: %w", channelID, err)
	}
	return nil
}

// NotifyLevels is every conversation of the workspace with a level
// other than the default.
func (db *DB) NotifyLevels(workspaceID string) (map[string]int, error) {
	rows, err := db.conn.Query(`SELECT channel_id, level FROM notify_levels WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing notify levels: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var level int
		if err := rows.Scan(&id, &level); err != nil {
			return nil, fmt.Errorf("listing notify levels: %w", err)
		}
		out[id] = level
	}
	return out, rows.Err()
}
