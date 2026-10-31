package onboard

import (
	"fmt"

	"github.com/gammons/slk/internal/config"
	slackclient "github.com/gammons/slk/internal/slack"
)

// SaveResult says what SaveWorkspace did beyond saving the token, for the
// caller to report.
type SaveResult struct {
	// Updated is true when config.toml already had a block for the team:
	// only the token was refreshed.
	Updated bool
	// LoadErr is set when config.toml does not load; it was left as is.
	LoadErr error
	// WriteErr is set when appending the new block failed.
	WriteErr error
}

// SaveWorkspace persists a validated token and appends its
// [workspaces.<slug>] config block to configPath (best-effort: a config
// failure is reported in the result, not as an error). Re-adding a
// workspace already in config.toml (the browser flow's documented recovery
// when its session expires) only refreshes the token: a second block for
// the same team_id would make config.Load reject the file. The error is
// non-nil only when the token could not be saved.
func SaveWorkspace(store *slackclient.TokenStore, tok slackclient.Token, configPath string) (SaveResult, error) {
	if err := store.Save(tok); err != nil {
		return SaveResult{}, fmt.Errorf("saving token for %s: %w", tok.TeamName, err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		// Appending to a file that already fails to load (a duplicate an
		// earlier run left) cannot fix it, and may add one more block.
		return SaveResult{LoadErr: err}, nil
	}
	if _, ok := cfg.WorkspaceByTeamID(tok.TeamID); ok {
		return SaveResult{Updated: true}, nil
	}
	slug := UniqueSlug(config.Slugify(tok.TeamName), ExistingSlugs(configPath))
	return SaveResult{WriteErr: AppendWorkspaceConfigBlock(configPath, slug, tok.TeamID, tok.TeamName)}, nil
}
