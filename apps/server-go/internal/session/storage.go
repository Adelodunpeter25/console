// SQLite-backed storage facade. Port of agent/src/session/storage.ts.
// One global index DB + one SQLite file per session; each *sql.DB is
// opened with a single connection so concurrent goroutines serialize on
// SQLite writes, mirroring Bun's single-threaded behavior.
package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

const maxCachedSessionDBs = 20

type Storage struct {
	storageDir string
	globalDB   *sql.DB

	mu         sync.Mutex
	sessionDBs map[string]*sql.DB // session id -> db
}

type OpenOptions struct {
	// DBPath overrides the global DB path (":memory:" for tests).
	DBPath string
	// StorageDir overrides the storage root; empty uses ConsoleStorageDir().
	StorageDir string
}

func OpenStorage(opts OpenOptions) (*Storage, error) {
	storageDir := opts.StorageDir
	if storageDir == "" {
		if opts.DBPath == ":memory:" {
			tmp, err := os.MkdirTemp("", "console-storage-")
			if err != nil {
				return nil, err
			}
			storageDir = tmp
		} else {
			storageDir = ConsoleStorageDir()
		}
	}

	globalPath := opts.DBPath
	if globalPath == "" {
		globalPath = filepath.Join(storageDir, "console-global.db")
	}
	if dir := filepath.Dir(globalPath); dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	globalDB, err := openDB(globalPath)
	if err != nil {
		return nil, err
	}
	if err := InitGlobalDB(globalDB, globalPath); err != nil {
		globalDB.Close()
		return nil, fmt.Errorf("init global db: %w", err)
	}

	return &Storage{
		storageDir: storageDir,
		globalDB:   globalDB,
		sessionDBs: make(map[string]*sql.DB),
	}, nil
}

func openDB(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Single connection serializes writers, matching Bun's single-threaded
	// access and avoiding SQLITE_BUSY across goroutines.
	db.SetMaxOpenConns(1)
	return db, nil
}

// Close closes the global DB and every cached session DB.
func (s *Storage) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, db := range s.sessionDBs {
		_ = db.Close()
		delete(s.sessionDBs, id)
	}
	_ = s.globalDB.Close()
}

// sessionDB returns the cached connection for a session, opening and
// initializing it on first use, and evicts the oldest entry past the cache
// limit (matches the TS MAX_CACHED_SESSION_DBS behavior closely enough for
// this slice).
func (s *Storage) sessionDB(sessionID, projectID string) (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if db, ok := s.sessionDBs[sessionID]; ok {
		return db, nil
	}

	var path string
	switch {
	case s.storageDir == ":memory:":
		path = ":memory:"
	case projectID != "":
		path = SessionDBPath(s.storageDir, projectID, sessionID)
	default:
		path = ScratchSessionDBPath(s.storageDir, sessionID)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && path != ":memory:" {
		return nil, err
	}

	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	if err := InitSessionDB(db, path); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init session db %s: %w", sessionID, err)
	}

	if len(s.sessionDBs) >= maxCachedSessionDBs {
		for id, cached := range s.sessionDBs {
			_ = cached.Close()
			delete(s.sessionDBs, id)
			break
		}
	}
	s.sessionDBs[sessionID] = db
	return db, nil
}

func (s *Storage) projectIDBySession(sessionID string) (string, bool) {
	var projectID sql.NullString
	err := s.globalDB.QueryRow(`SELECT project_id FROM sessions WHERE id = ?`, sessionID).Scan(&projectID)
	if err != nil {
		return "", false
	}
	if projectID.Valid && projectID.String != "" && projectID.String != "scratch" {
		return projectID.String, true
	}
	return "", true // found, projectless ("scratch")
}

func (s *Storage) bumpSessionUpdated(sessionID string, now int64, delta int) {
	_, _ = s.globalDB.Exec(
		`UPDATE sessions SET updated_at = ?, message_count = message_count + ? WHERE id = ?`,
		now, delta, sessionID,
	)
}
