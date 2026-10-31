// Package cache: full-text search over the messages table.
//
// messages_fts is an external-content FTS5 table over messages.text,
// kept in sync by AFTER INSERT/UPDATE/DELETE triggers (the standard
// FTS5 external-content pattern). Soft deletes (is_deleted=1) do not
// touch text, so query time filters them via a join back to messages.
//
// WARNING: messages has a TEXT composite PK, so it uses implicit
// rowids, and VACUUM may renumber them — desyncing the external-content
// mapping. Do not VACUUM this database without rebuilding the index via
// INSERT INTO messages_fts(messages_fts) VALUES('rebuild').
package cache

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode"
)

// buildFTSQuery converts raw user input into an FTS5 MATCH expression
// of quoted prefix terms: `foo bar` -> `"foo"* "bar"*` ("messages
// containing words starting with foo AND bar"). Quoting every term
// means user input is never interpreted as FTS5 syntax; embedded
// double quotes are escaped by doubling per SQL string rules.
func buildFTSQuery(input string) string {
	fields := strings.Fields(input)
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, `"`+strings.ReplaceAll(f, `"`, `""`)+`"*`)
	}
	return strings.Join(parts, " ")
}

// migrateSearch creates the FTS5 table, sync triggers, and backfills
// existing rows. Idempotent: a no-op when messages_fts already exists.
func (db *DB) migrateSearch() error {
	var n int
	if err := db.conn.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='messages_fts'`).Scan(&n); err != nil {
		return fmt.Errorf("probing messages_fts: %w", err)
	}
	if n == 1 {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning fts migration: %w", err)
	}
	defer tx.Rollback()

	stmts := []string{
		`CREATE VIRTUAL TABLE messages_fts USING fts5(
			text,
			content='messages',
			content_rowid='rowid',
			tokenize='unicode61 remove_diacritics 2'
		)`,
		`CREATE TRIGGER messages_fts_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
		END`,
		`CREATE TRIGGER messages_fts_au AFTER UPDATE OF text ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
			INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
		END`,
		`CREATE TRIGGER messages_fts_ad AFTER DELETE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
		END`,
		`INSERT INTO messages_fts(rowid, text) SELECT rowid, text FROM messages`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("fts migration: %w", err)
		}
	}
	return tx.Commit()
}

// SearchChannelMessages returns the ts values of non-deleted messages
// in (channelID, workspaceID) whose text matches the query, newest
// first. Matching is word-prefix, case- and accent-insensitive (FTS5
// unicode61 remove_diacritics 2). When FTS is unavailable it degrades
// to a substring LIKE scan (ASCII case-insensitive only).
func (db *DB) SearchChannelMessages(channelID, workspaceID, query string) ([]string, error) {
	if db.ftsDisabled {
		return db.searchChannelMessagesLike(channelID, workspaceID, query)
	}
	match := buildFTSQuery(query)
	if match == "" {
		return nil, nil
	}
	// Mirror GetMessages' channel-feed predicate: plain thread replies
	// (cached when threads are viewed) can't be displayed in the
	// channel pane, so jumping to one would fail. Searching replies is
	// a v2 follow-up.
	rows, err := db.conn.Query(`
		SELECT m.ts
		FROM messages_fts f
		JOIN messages m ON m.rowid = f.rowid
		WHERE messages_fts MATCH ?
		  AND m.channel_id = ? AND m.workspace_id = ? AND m.is_deleted = 0
		  AND (m.thread_ts = '' OR m.thread_ts = m.ts OR m.subtype = 'thread_broadcast')
		ORDER BY m.ts DESC`, match, channelID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("searching messages: %w", err)
	}
	return scanTSColumn(rows)
}

// searchChannelMessagesLike is the degraded path: every term must
// appear as a substring (LIKE is case-insensitive for ASCII).
func (db *DB) searchChannelMessagesLike(channelID, workspaceID, query string) ([]string, error) {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return nil, nil
	}
	// Same channel-feed predicate as the FTS path: thread replies
	// can't be displayed in the channel pane (reply search is a v2
	// follow-up).
	q := `SELECT ts FROM messages WHERE channel_id = ? AND workspace_id = ? AND is_deleted = 0
		AND (thread_ts = '' OR thread_ts = ts OR subtype = 'thread_broadcast')`
	args := []any{channelID, workspaceID}
	for _, term := range terms {
		q += ` AND text LIKE ? ESCAPE '\'`
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term)
		args = append(args, "%"+escaped+"%")
	}
	q += ` ORDER BY ts DESC`
	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("searching messages (like): %w", err)
	}
	return scanTSColumn(rows)
}

func scanTSColumn(rows *sql.Rows) ([]string, error) {
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ts string
		if err := rows.Scan(&ts); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

// SearchOrder is how SearchMessages sorts the messages that match.
type SearchOrder int

const (
	// SearchRelevance is FTS5's bm25 rank (the terms' frequency in the
	// message against their rarity in the cache, shorter messages
	// ahead), newest first among equals.
	SearchRelevance SearchOrder = iota
	// SearchNewest is newest first.
	SearchNewest
)

// SearchMessages returns the workspace's non-deleted messages whose
// text matches the query, in order and at most limit of them, thread
// replies included. It is the workspace-wide twin of
// SearchChannelMessages for a search window, which can open a reply's
// thread, so nothing is filtered by row shape. Matching is as there:
// word-prefix, case- and accent-insensitive. It is SearchMessagesSpec
// with every word required as a prefix.
func (db *DB) SearchMessages(workspaceID, query string, limit int, order SearchOrder) ([]Message, error) {
	var spec SearchSpec
	for _, f := range strings.Fields(query) {
		spec.All = append(spec.All, []string{f + "*"})
	}
	if len(spec.All) == 0 {
		return nil, nil
	}
	return db.SearchMessagesSpec(workspaceID, spec, limit, order)
}

// Tri is a yes/no filter that may be left out.
type Tri int8

const (
	Any Tri = iota // not filtered
	Yes
	No
)

// SearchSpec is what SearchMessagesSpec looks for. Every part that is
// set must hold; an empty spec finds nothing.
type SearchSpec struct {
	// All are groups of alternatives, each group one the text must
	// match. An alternative is whole words in a row; a trailing * makes
	// its last word a prefix: {{"deploy"}, {"build fail*", "red"}} is
	// deploy AND ("build" then a word starting "fail", OR red).
	All [][]string
	// None are alternatives the text must not match.
	None []string
	// Users and Channels are authors and conversations the message must
	// be one of; NotUsers and NotChannels ones it must not.
	Users, NotUsers, Channels, NotChannels []string
	// Mentions are pieces of Slack markup (<@U1>, <!subteam^S1) the
	// text must contain one of; NotMentions any of which it must not.
	Mentions, NotMentions []string
	// Since and Until bound the message's ts, [Since, Until); "" is
	// unbounded.
	Since, Until string
	// Thread is a thread's parent with replies; Reply a reply (not a
	// broadcast); Edited an edited message; Link one with a link in its
	// text; File one with files; Reaction one with any reaction.
	Thread, Reply, Edited, Link, File, Reaction Tri
	// Emoji are reactions the message must have each of; NotEmoji ones
	// it must have none of. Names without colons.
	Emoji, NotEmoji []string
}

// empty is whether s asks for nothing at all.
func (s SearchSpec) empty() bool {
	return len(s.All) == 0 && len(s.None) == 0 && len(s.Users)+len(s.NotUsers)+len(s.Channels)+len(s.NotChannels) == 0 &&
		len(s.Mentions)+len(s.NotMentions)+len(s.Emoji)+len(s.NotEmoji) == 0 && s.Since == "" && s.Until == "" &&
		s.Thread == Any && s.Reply == Any && s.Edited == Any && s.Link == Any && s.File == Any && s.Reaction == Any
}

// ftsPhrase is one alternative as an FTS5 phrase, quoted so nothing in
// it reads as FTS5 syntax, its last word a prefix when it ends in *;
// "" when it has no word FTS5 would index.
func ftsPhrase(alt string) string {
	if !strings.ContainsFunc(alt, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		return ""
	}
	prefix := strings.HasSuffix(alt, "*")
	p := `"` + strings.ReplaceAll(strings.Join(strings.Fields(strings.TrimRight(alt, "*")), " "), `"`, `""`) + `"`
	if prefix {
		p += "*"
	}
	return p
}

// ftsAny is alternatives as one FTS5 expression, "" if none is a word.
func ftsAny(alts []string) string {
	var ps []string
	for _, a := range alts {
		if p := ftsPhrase(a); p != "" {
			ps = append(ps, p)
		}
	}
	switch len(ps) {
	case 0:
		return ""
	case 1:
		return ps[0]
	}
	return "(" + strings.Join(ps, " OR ") + ")"
}

// likePattern is a LIKE pattern for alt anywhere in the text (LIKE has
// no words: whole or prefix, it is a substring).
func likePattern(alt string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.Join(strings.Fields(strings.TrimRight(alt, "*")), " "))
	return "%" + escaped + "%"
}

// SearchMessagesSpec returns the workspace's non-deleted messages that
// match spec, in order and at most limit of them, thread replies
// included. Text matches whole words (a trailing * a prefix), case- and
// accent-insensitively; without text to rank by,
// or when FTS is unavailable (a LIKE scan: substrings, ASCII-only case
// folding), the order is newest first.
func (db *DB) SearchMessagesSpec(workspaceID string, spec SearchSpec, limit int, order SearchOrder) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	if spec.empty() {
		return nil, nil
	}
	cols := "m." + strings.ReplaceAll(messageColumns, ", ", ", m.")
	from := ` FROM messages m`
	where := []string{"m.workspace_id = ?", "m.is_deleted = 0"}
	var args []any
	args = append(args, workspaceID)
	orderBy := "m.ts DESC"

	if db.ftsDisabled {
		for _, group := range spec.All {
			var ors []string
			for _, alt := range group {
				if strings.TrimSpace(alt) == "" {
					continue
				}
				ors = append(ors, `m.text LIKE ? ESCAPE '\'`)
				args = append(args, likePattern(alt))
			}
			if len(ors) == 0 {
				return nil, nil
			}
			where = append(where, "("+strings.Join(ors, " OR ")+")")
		}
		for _, alt := range spec.None {
			if strings.TrimSpace(alt) != "" {
				where = append(where, `m.text NOT LIKE ? ESCAPE '\'`)
				args = append(args, likePattern(alt))
			}
		}
	} else {
		var match []string
		for _, group := range spec.All {
			e := ftsAny(group)
			if e == "" {
				return nil, nil // a group of no words matches nothing
			}
			match = append(match, e)
		}
		if len(match) > 0 {
			from += ` JOIN messages_fts f ON f.rowid = m.rowid`
			where = append(where, "messages_fts MATCH ?")
			args = append(args, strings.Join(match, " AND "))
			if order == SearchRelevance {
				orderBy = "bm25(messages_fts), m.ts DESC" // lower bm25 is better
			}
		}
		if e := ftsAny(spec.None); e != "" {
			where = append(where, "m.rowid NOT IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)")
			args = append(args, e)
		}
	}

	in := func(col string, ids []string, not bool) {
		if len(ids) == 0 {
			return
		}
		op := " IN "
		if not {
			op = " NOT IN "
		}
		where = append(where, col+op+"("+strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}
	in("m.user_id", spec.Users, false)
	in("m.user_id", spec.NotUsers, true)
	in("m.channel_id", spec.Channels, false)
	in("m.channel_id", spec.NotChannels, true)
	if len(spec.Mentions) > 0 {
		var ors []string
		for _, w := range spec.Mentions {
			ors = append(ors, "instr(m.text, ?) > 0")
			args = append(args, w)
		}
		where = append(where, "("+strings.Join(ors, " OR ")+")")
	}
	for _, w := range spec.NotMentions {
		where = append(where, "instr(m.text, ?) = 0")
		args = append(args, w)
	}
	if spec.Since != "" {
		where = append(where, "m.ts >= ?")
		args = append(args, spec.Since)
	}
	if spec.Until != "" {
		where = append(where, "m.ts < ?")
		args = append(args, spec.Until)
	}
	tri := func(t Tri, cond string) {
		switch t {
		case Yes:
			where = append(where, cond)
		case No:
			where = append(where, "NOT ("+cond+")")
		}
	}
	tri(spec.Thread, "m.thread_ts = m.ts AND m.reply_count > 0")
	tri(spec.Reply, "m.thread_ts != '' AND m.thread_ts != m.ts AND m.subtype != 'thread_broadcast'")
	tri(spec.Edited, "m.edited_at != ''")
	tri(spec.Link, "instr(m.text, '<http') > 0")
	tri(spec.File, `instr(m.raw_json, '"files":[{') > 0`)
	const reacted = "EXISTS (SELECT 1 FROM reactions r WHERE r.message_ts = m.ts AND r.channel_id = m.channel_id AND r.count > 0"
	tri(spec.Reaction, reacted+")")
	for _, e := range spec.Emoji {
		where = append(where, reacted+" AND r.emoji = ?)")
		args = append(args, e)
	}
	for _, e := range spec.NotEmoji {
		where = append(where, "NOT "+reacted+" AND r.emoji = ?)")
		args = append(args, e)
	}

	q := "SELECT " + cols + from + " WHERE " + strings.Join(where, " AND ") + " ORDER BY " + orderBy + " LIMIT ?"
	args = append(args, limit)
	return db.queryMessages(q, args...)
}
