package cache

import (
	"fmt"
	"strings"
)

// UserGroup is one of a workspace's usergroups (the @team behind a
// <!subteam^S…> mention), with its members' user IDs.
type UserGroup struct {
	ID          string
	Handle      string
	Name        string
	Description string
	Members     []string
}

// ReplaceUserGroups makes groups the workspace's usergroups, dropping
// any the cache held that are not among them.
func (db *DB) ReplaceUserGroups(workspaceID string, groups []UserGroup) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("replacing usergroups: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	if _, err := tx.Exec(`DELETE FROM usergroups WHERE workspace_id = ?`, workspaceID); err != nil {
		return fmt.Errorf("replacing usergroups: %w", err)
	}
	for _, g := range groups {
		if _, err := tx.Exec(`INSERT INTO usergroups (workspace_id, id, handle, name, description, members) VALUES (?, ?, ?, ?, ?, ?)`,
			workspaceID, g.ID, g.Handle, g.Name, g.Description, strings.Join(g.Members, ",")); err != nil {
			return fmt.Errorf("replacing usergroups: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replacing usergroups: %w", err)
	}
	return nil
}

// ListUserGroups is the workspace's cached usergroups, by handle.
func (db *DB) ListUserGroups(workspaceID string) ([]UserGroup, error) {
	rows, err := db.conn.Query(`SELECT id, handle, name, description, members FROM usergroups WHERE workspace_id = ? ORDER BY handle`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing usergroups: %w", err)
	}
	defer rows.Close()
	var out []UserGroup
	for rows.Next() {
		var g UserGroup
		var members string
		if err := rows.Scan(&g.ID, &g.Handle, &g.Name, &g.Description, &members); err != nil {
			return nil, fmt.Errorf("listing usergroups: %w", err)
		}
		if members != "" {
			g.Members = strings.Split(members, ",")
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
