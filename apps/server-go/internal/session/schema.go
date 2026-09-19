// SQLite schema + migrations. Port of agent/src/session/schema.ts — the
// DDL must stay byte-compatible with the TS server so both binaries can
// read the same storage directory during the transition.
package session

import "database/sql"

func setPragmas(db *sql.DB, path string) error {
	if path != "" && path != ":memory:" {
		if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			return err
		}
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	return nil
}

func InitGlobalDB(db *sql.DB, path string) error {
	if err := setPragmas(db, path); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			dir TEXT NOT NULL UNIQUE,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			cwd TEXT NOT NULL,
			project_id TEXT,
			model_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			message_count INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'idle',
			approval_mode TEXT NOT NULL DEFAULT 'always-ask',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER
		);

		CREATE INDEX IF NOT EXISTS idx_projects_dir ON projects(dir);
		CREATE INDEX IF NOT EXISTS idx_sessions_cwd ON sessions(cwd);
		CREATE INDEX IF NOT EXISTS idx_sessions_project_id ON sessions(project_id);
		CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at DESC);

		CREATE TABLE IF NOT EXISTS model_favorites (
			provider TEXT NOT NULL,
			model_id TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (provider, model_id)
		);
	`); err != nil {
		return err
	}

	// Migrations matching initGlobalDatabase.
	rows, err := db.Query("PRAGMA table_info(sessions)")
	if err != nil {
		return err
	}
	defer rows.Close()
	var hasStatus, hasApprovalMode, hasDeletedAt bool
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		switch name {
		case "status":
			hasStatus = true
		case "approval_mode":
			hasApprovalMode = true
		case "deleted_at":
			hasDeletedAt = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, m := range []struct {
		has bool
		ddl string
	}{
		{hasStatus, "ALTER TABLE sessions ADD COLUMN status TEXT NOT NULL DEFAULT 'idle'"},
		{hasApprovalMode, "ALTER TABLE sessions ADD COLUMN approval_mode TEXT NOT NULL DEFAULT 'always-ask'"},
		{hasDeletedAt, "ALTER TABLE sessions ADD COLUMN deleted_at INTEGER"},
	} {
		if m.has {
			continue
		}
		if _, err := db.Exec(m.ddl); err != nil {
			return err
		}
	}
	return nil
}

func InitSessionDB(db *sql.DB, path string) error {
	if err := setPragmas(db, path); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS session_meta (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			title TEXT NOT NULL,
			cwd TEXT NOT NULL,
			project_id TEXT,
			model_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			approval_mode TEXT NOT NULL DEFAULT 'always-ask',
			repaired INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);

		CREATE TABLE IF NOT EXISTS session_file_changes (
			path TEXT NOT NULL,
			turn_index INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			additions INTEGER NOT NULL DEFAULT 0,
			deletions INTEGER NOT NULL DEFAULT 0,
			diff_text TEXT,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (path, turn_index)
		);

		CREATE INDEX IF NOT EXISTS idx_session_file_changes_turn
			ON session_file_changes (turn_index, updated_at DESC);

		CREATE TABLE IF NOT EXISTS session_todos (
			id INTEGER PRIMARY KEY,
			content TEXT NOT NULL,
			status TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS session_subagents (
			id TEXT PRIMARY KEY,
			parent_tool_call_id TEXT NOT NULL,
			name TEXT NOT NULL,
			role TEXT NOT NULL,
			prompt TEXT NOT NULL,
			max_turns INTEGER NOT NULL DEFAULT 10,
			current_turn INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			summary TEXT,
			error TEXT,
			activities TEXT NOT NULL DEFAULT '[]',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_session_subagents_created_at ON session_subagents(created_at);

		CREATE TABLE IF NOT EXISTS session_queued_prompt (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			queue_id TEXT NOT NULL,
			prompt TEXT NOT NULL,
			attachments TEXT,
			model_id TEXT,
			provider TEXT,
			approval_mode TEXT,
			created_at INTEGER NOT NULL
		);
	`); err != nil {
		return err
	}

	rows, err := db.Query("PRAGMA table_info(session_meta)")
	if err != nil {
		return err
	}
	defer rows.Close()
	var hasRepaired bool
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == "repaired" {
			hasRepaired = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !hasRepaired {
		if _, err := db.Exec("ALTER TABLE session_meta ADD COLUMN repaired INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	return nil
}
