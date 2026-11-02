// Store tests over a fake Chrome tree: profile naming rules, the urls
// history query, and cookie row reading (tombstones and partition keys).
package chromeimport

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func newFakeChrome(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	def := filepath.Join(base, "Default")
	if err := os.MkdirAll(def, 0o755); err != nil {
		t.Fatal(err)
	}
	// A data file marks the folder as a real profile (hasProfileData).
	if err := os.WriteFile(filepath.Join(def, "History"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	localState := `{"profile":{"info_cache":{"Default":{"gaia_name":"Peter","name":"Person 1"}}}}`
	if err := os.WriteFile(filepath.Join(base, "Local State"), []byte(localState), 0o644); err != nil {
		t.Fatal(err)
	}
	return base
}

func TestDetectProfilesAt(t *testing.T) {
	base := newFakeChrome(t)
	alt := filepath.Join(base, "Profile 1")
	if err := os.MkdirAll(alt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alt, "History"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// Chrome creates snapshot/FIT folders too; without data they are not
	// profiles.
	if err := os.MkdirAll(filepath.Join(base, "System Profile", "History"), 0o755); err != nil {
		t.Fatal(err)
	}

	profiles := DetectProfilesAt(base)
	if len(profiles) != 2 {
		t.Fatalf("profiles = %+v, want Default and Profile 1", profiles)
	}
	if profiles[0].ID != "Default" || profiles[0].Name != "Peter" {
		t.Fatalf("Default name = %q, want gaia_name to win over Person 1", profiles[0].Name)
	}
	if profiles[1].ID != "Profile 1" || profiles[1].Name != "Profile 1" {
		t.Fatalf("Profile 1 = %+v, want the folder-name fallback", profiles[1])
	}
}

func TestReadHistoryAt(t *testing.T) {
	base := newFakeChrome(t)
	db := openFixtureDB(t, filepath.Join(base, "Default", "History"))
	for _, schema := range []string{
		`CREATE TABLE urls (url TEXT, title TEXT, visit_count INTEGER)`,
	} {
		if _, err := db.Exec(schema); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range [][]any{
		{"https://go.dev/", "The Go Blog", 40},
		{"https://example.com", "", 5},
	} {
		if _, err := db.Exec(`INSERT INTO urls VALUES (?, ?, ?)`, row...); err != nil {
			t.Fatal(err)
		}
	}

	history, err := ReadHistoryAt(filepath.Join(base, "Default"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history = %+v", history)
	}
	if history[0].URL != "https://go.dev/" || history[0].VisitCount != 40 {
		t.Fatalf("most visited first: %+v", history[0])
	}
	if history[1].Title != "" {
		t.Fatalf("a NULL title must come back empty, got %q", history[1].Title)
	}
}

func TestQueryRowReading(t *testing.T) {
	base := newFakeChrome(t)
	dbPath := filepath.Join(base, "Default", "Cookies")
	db := openFixtureDB(t, dbPath)
	if _, err := db.Exec(`CREATE TABLE cookies (
		host_key TEXT, name TEXT, value TEXT, path TEXT,
		encrypted_value BLOB, is_secure INTEGER, is_httponly INTEGER,
		expires_utc INTEGER, top_frame_site_key TEXT)`); err != nil {
		t.Fatal(err)
	}
	encrypted := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	rows := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO cookies (host_key, name, value, path, encrypted_value, is_secure, is_httponly, expires_utc, top_frame_site_key)
		  VALUES ('a.com', 'sid', 'plain', '/', x'', 0, 0, 946684800000000, ?)`, []any{nil}},
		{`INSERT INTO cookies (host_key, name, value, path, encrypted_value, is_secure, is_httponly, expires_utc, top_frame_site_key)
		  VALUES ('b.com', 'tok', '', '/', ?, 1, 1, 946684800000000, ?)`, []any{encrypted, nil}},
		{`INSERT INTO cookies (host_key, name, value, path, encrypted_value, is_secure, is_httponly, expires_utc, top_frame_site_key)
		  VALUES ('c.com', 'pip', '', '/', ?, 0, 0, 946684800000000, 'https://embed.com')`, []any{encrypted}},
		{`INSERT INTO cookies (host_key, name, value, path, encrypted_value, is_secure, is_httponly, expires_utc, top_frame_site_key)
		  VALUES ('d.com', 'gone', '', '/', x'', 0, 0, 946684800000000, ?)`, []any{nil}},
	}
	for i, row := range rows {
		if _, err := db.Exec(row.query, row.args...); err != nil {
			t.Fatalf("row %d: %v", i, err)
		}
	}

	// Chrome has the database open: the read must work over the copy.
	profileDir := filepath.Join(base, "Default")
	path, cleanup, err := copyForRead(dbPath, "cookies")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	rowsRaw, err := queryCookies(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	// Tombstone skipped; CHIPS row kept but flagged.
	if len(rowsRaw) != 3 {
		t.Fatalf("rows = %+v, want sid, tok, pip", rowsRaw)
	}
	if rowsRaw[0].Value != "plain" {
		t.Fatalf("sid = %+v", rowsRaw[0])
	}
	if rowsRaw[1].Secure != true || rowsRaw[1].HTTPOnly != true || len(rowsRaw[1].Encrypted) != len(encrypted) {
		t.Fatalf("tok = %+v", rowsRaw[1])
	}
	if rowsRaw[2].TopFrameSiteKey != "https://embed.com" {
		t.Fatalf("pip = %+v", rowsRaw[2])
	}
	_ = profileDir
}

func TestRequireProfileGuards(t *testing.T) {
	if _, err := ReadHistory("", 10); err == nil {
		t.Fatal("an empty profile id must fail")
	}
	if _, err := ReadHistory("../etc", 10); err == nil {
		t.Fatal("a path-shaped profile id must fail")
	}
}

func openFixtureDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
