// Database manager: one reusable connector for the global index DB plus a
// cache of per-session connections. Holds no business logic — services
// call Session()/Global() and run their own SQL.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
	_ "modernc.org/sqlite"
)

const maxCachedSessionDBs = 20

type DB struct {
	storageDir string
	global     *sql.DB

	mu         sync.Mutex
	sessionDBs map[string]*sql.DB
}

type OpenOptions struct {
	// Path overrides the global DB path (":memory:" for tests).
	Path string
	// StorageDir overrides the storage root; empty uses the resolved
	// console storage dir.
	StorageDir string
}

func Open(opts OpenOptions) (*DB, error) {
	storageDir := opts.StorageDir
	if storageDir == "" {
		if opts.Path == ":memory:" {
			tmp, err := os.MkdirTemp("", "console-storage-")
			if err != nil {
				return nil, err
			}
			storageDir = tmp
		} else {
			storageDir = utils.ConsoleStorageDir()
		}
	}

	globalPath := opts.Path
	if globalPath == "" {
		globalPath = utils.GlobalDBPath()
	}
	if dir := filepath.Dir(globalPath); dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	global, err := openConnector(globalPath)
	if err != nil {
		return nil, err
	}
	if err := InitGlobalDB(global, globalPath); err != nil {
		global.Close()
		return nil, fmt.Errorf("init global db: %w", err)
	}

	return &DB{
		storageDir: storageDir,
		global:     global,
		sessionDBs: make(map[string]*sql.DB),
	}, nil
}

func openConnector(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Single connection serializes writers, matching Bun's single-threaded
	// access and avoiding SQLITE_BUSY across goroutines.
	conn.SetMaxOpenConns(1)
	return conn, nil
}

// Global returns the shared global index database connection.
func (d *DB) Global() *sql.DB {
	return d.global
}

func (d *DB) StorageDir() string {
	return d.storageDir
}

// Session returns the cached connection for a session, opening and
// initializing it on first use. projectID == "" means a scratch session.
func (d *DB) Session(sessionID, projectID string) (*sql.DB, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if conn, ok := d.sessionDBs[sessionID]; ok {
		return conn, nil
	}

	var path string
	switch {
	case d.storageDir == ":memory:":
		path = ":memory:"
	case projectID != "":
		path = utils.SessionDBPath(d.storageDir, projectID, sessionID)
	default:
		path = utils.ScratchSessionDBPath(d.storageDir, sessionID)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && path != ":memory:" {
		return nil, err
	}

	conn, err := openConnector(path)
	if err != nil {
		return nil, err
	}
	if err := InitSessionDB(conn, path); err != nil {
		conn.Close()
		return nil, fmt.Errorf("init session db %s: %w", sessionID, err)
	}

	if len(d.sessionDBs) >= maxCachedSessionDBs {
		for id, cached := range d.sessionDBs {
			cached.Close()
			delete(d.sessionDBs, id)
			break
		}
	}
	d.sessionDBs[sessionID] = conn
	return conn, nil
}

// CloseSession drops a cached session connection (used before file ops
// such as project deletion).
func (d *DB) CloseSession(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if conn, ok := d.sessionDBs[sessionID]; ok {
		conn.Close()
		delete(d.sessionDBs, sessionID)
	}
}

// Close closes the global DB and every cached session DB.
func (d *DB) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, conn := range d.sessionDBs {
		conn.Close()
		delete(d.sessionDBs, id)
	}
	d.global.Close()
}
