// Chrome SQLite stores: copy-then-read.
//
// Chrome holds History and Cookies open with a WAL, so they cannot be opened
// in place. Each store is copied to a private temp dir first — WAL and SHM
// siblings included so recent writes do not vanish between the copies — read
// through, and the copy removed. A fresh MkdirTemp per call keeps concurrent
// imports from stepping on each other.
package chromeimport

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// copyForRead copies source (plus -wal/-shm) into a fresh temp dir and
// returns the copy path; the caller removes the whole temp dir.
func copyForRead(source, stem string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "console-chrome-import-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	base := filepath.Base(source)
	if base == "" {
		base = fmt.Sprintf("%s.sqlite", stem)
	}
	target := filepath.Join(dir, base)
	data, err := os.ReadFile(source)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		cleanup()
		return "", nil, err
	}
	// Sibling WAL/SHM: copying stale ones is harmless (SQLite ignores a WAL
	// that predates the main file's state), while missing recent ones would
	// lose the newest rows.
	for _, suffix := range []string{"-wal", "-shm"} {
		if d, err := os.ReadFile(source + suffix); err == nil {
			_ = os.WriteFile(target+suffix, d, 0o600)
		}
	}
	return target, cleanup, nil
}

func openReadonly(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	// One connection serializes the read; these are one-shot queries.
	db.SetMaxOpenConns(1)
	return db, nil
}

// queryHistory reads (url, title, visit_count) rows, most visited first.
func queryHistory(ctx context.Context, path string, limit int) ([]ChromeHistoryEntry, error) {
	db, err := openReadonly(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	stmt, err := conn.PrepareContext(ctx,
		"SELECT url, title, visit_count FROM urls ORDER BY visit_count DESC LIMIT ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	if limit <= 0 {
		limit = 1000
	}
	rows, err := stmt.QueryContext(ctx, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChromeHistoryEntry
	for rows.Next() {
		var url string
		var title sql.NullString
		var visitCount int64
		if err := rows.Scan(&url, &title, &visitCount); err != nil {
			return nil, err
		}
		out = append(out, ChromeHistoryEntry{URL: url, Title: title.String, VisitCount: visitCount})
	}
	return out, rows.Err()
}

// rawCookieRow is one cookie row: the plain-text value if Chrome kept one,
// otherwise encrypted_value for the decryptor.
type rawCookieRow struct {
	Host            string
	Name            string
	Value           string
	Encrypted       []byte
	Path            string
	Secure          bool
	HTTPOnly        bool
	ExpiresUTC      int64
	TopFrameSiteKey string
}

// queryCookies reads every cookie row. top_frame_site_key exists only on
// stores with CHIPS; an older store falls back to the shorter column list.
func queryCookies(ctx context.Context, path string) ([]rawCookieRow, error) {
	db, err := openReadonly(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	cols := "host_key, name, value, path, encrypted_value, is_secure, is_httponly, expires_utc"

	if stmt, err := conn.PrepareContext(ctx, "SELECT "+cols+", top_frame_site_key FROM cookies"); err == nil {
		rows, qerr := stmt.QueryContext(ctx)
		if qerr == nil {
			out, serr := readCookieRows(rows, true)
			stmt.Close()
			rows.Close()
			if serr == nil {
				return out, nil
			}
			// Fall through to the plain column list on a scan error; being
			// strict about one odd row is worse than skipping the column.
			_ = qerr
		}
		stmt.Close()
	}

	stmt, err := conn.PrepareContext(ctx, "SELECT "+cols+" FROM cookies")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return readCookieRows(rows, false)
}

func readCookieRows(rows *sql.Rows, partitioned bool) ([]rawCookieRow, error) {
	var out []rawCookieRow
	for rows.Next() {
		var row rawCookieRow
		var name, path, value sql.NullString
		var encrypted []byte
		var secure, httpOnly int64
		var topFrame sql.NullString
		if partitioned {
			if err := rows.Scan(&row.Host, &name, &value, &path, &encrypted, &secure, &httpOnly, &row.ExpiresUTC,
				&topFrame); err != nil {
				return nil, err
			}
			row.TopFrameSiteKey = topFrame.String
		} else {
			if err := rows.Scan(&row.Host, &name, &value, &path, &encrypted, &secure, &httpOnly, &row.ExpiresUTC); err != nil {
				return nil, err
			}
		}
		row.Name = name.String
		row.Path = path.String
		row.Value = value.String
		if len(encrypted) > 0 {
			row.Encrypted = encrypted
		}
		row.Secure = secure != 0
		row.HTTPOnly = httpOnly != 0
		// A cookie with neither a plain value nor ciphertext is a tombstone
		// Chrome uses to delete; the decryptor would fail it anyway.
		if row.Name == "" || (row.Value == "" && len(row.Encrypted) == 0) {
			continue
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func copyAndQueryHistory(profileDir string, limit int) ([]ChromeHistoryEntry, error) {
	source := filepath.Join(profileDir, "History")
	if info, err := os.Stat(source); err != nil || info.IsDir() {
		return nil, fmt.Errorf("no history database")
	}
	path, cleanup, err := copyForRead(source, "history")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return queryHistory(context.Background(), path, limit)
}

func copyAndQueryCookies(profileDir string) ([]rawCookieRow, error) {
	source := filepath.Join(profileDir, "Network", "Cookies")
	if info, err := os.Stat(source); err != nil || info.IsDir() {
		source = filepath.Join(profileDir, "Cookies")
		if info, err := os.Stat(source); err != nil || info.IsDir() {
			return nil, fmt.Errorf("no cookie database")
		}
	}
	path, cleanup, err := copyForRead(source, "cookies")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return queryCookies(context.Background(), path)
}
