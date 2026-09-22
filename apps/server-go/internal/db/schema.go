// SQLite schema. The DDL is idempotent, with small additive migrations for
// fields introduced after an existing session database was created.
package db

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
	_, err := db.Exec(`
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
			deleted_at INTEGER,
			worktree_path TEXT,
			worktree_branch TEXT,
			worktree_repo TEXT
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
	`)
	if err != nil {
		return err
	}
	// Additive migration for session DBs created before worktrees: add the
	// ownership columns when missing.
	for _, col := range []string{
		"worktree_path TEXT", "worktree_branch TEXT", "worktree_repo TEXT",
	} {
		name := col[:len(col)-len(" TEXT")]
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = ?`, name,
		).Scan(&count); err != nil || count > 0 {
			if err != nil {
				return err
			}
			continue
		}
		if _, err := db.Exec(`ALTER TABLE sessions ADD COLUMN ` + col); err != nil {
			return err
		}
	}
	return nil
}

func InitSessionDB(db *sql.DB, path string) error {
	if err := setPragmas(db, path); err != nil {
		return err
	}
	_, err := db.Exec(`
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
			context_files TEXT,
			attachments TEXT,
			model_id TEXT,
			provider TEXT,
			approval_mode TEXT,
			created_at INTEGER NOT NULL
		);
	`)
	if err != nil {
		return err
	}

	rows, err := db.Query(`PRAGMA table_info(session_queued_prompt)`)
	if err != nil {
		return err
	}
	hasContextFiles := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "context_files" {
			hasContextFiles = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !hasContextFiles {
		_, err = db.Exec(`ALTER TABLE session_queued_prompt ADD COLUMN context_files TEXT`)
	}
	return err
}
