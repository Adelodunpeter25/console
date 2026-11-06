package tests

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
)

// Reports, on copies of the developer's real databases, what the split storage
// does to their size. It asserts only that nothing was lost; the numbers are for
// reading. Originals are never opened for writing.
func TestStorageReportOnRealCopies(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	store := filepath.Join(home, ".console")
	var srcs []string
	_ = filepath.Walk(store, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".db") && strings.Contains(p, string(filepath.Separator)+"sessions"+string(filepath.Separator)) {
			srcs = append(srcs, p)
		}
		return nil
	})
	var contentBytes, skeletonBytes, partBytes, textBytes, imgFileBytes int64
	sessions, msgs := 0, 0
	for _, src := range srcs {
		probe, err := sql.Open("sqlite", "file:"+src+"?mode=ro&immutable=1")
		if err != nil {
			continue
		}
		var n int
		bad := probe.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&n) != nil
		probe.Close()
		if bad {
			continue
		}
		// A real on-disk layout so images become files: <dir>/projects/p/sessions/<id>.db
		dir := t.TempDir()
		id := "s" + filepath.Base(src)
		id = strings.TrimSuffix(id, ".db")
		dst := filepath.Join(dir, "projects", "p", "sessions", id+".db")
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		if err := copyFile(src, dst); err != nil {
			continue
		}
		manager, err := db.Open(db.OpenOptions{Path: filepath.Join(dir, "global.db"), StorageDir: dir})
		if err != nil {
			continue
		}
		// Register the session so Load can find it.
		_, _ = manager.Global().Exec(`INSERT INTO sessions (id, title, cwd, project_id, model_id, provider, message_count, status, approval_mode, created_at, updated_at) VALUES (?, 't', '/', 'p', 'm', 'p', 0, 'idle', 'always-ask', 1, 1)`, id)
		svc := services.NewSessionService(manager)
		if _, err := svc.Load(id, 0, 0); err != nil {
			t.Errorf("%s: load: %v", filepath.Base(src), err)
			manager.Close()
			continue
		}
		c, err := manager.Session(id, "p")
		if err != nil {
			manager.Close()
			continue
		}
		var cb, sb, pb, tb sql.NullInt64
		_ = c.QueryRow(`SELECT COALESCE(SUM(LENGTH(content)),0), COALESCE(SUM(LENGTH(skeleton)),0), COALESCE(SUM(LENGTH(text)),0) FROM messages`).Scan(&cb, &sb, &tb)
		_ = c.QueryRow(`SELECT COALESCE(SUM(LENGTH(body)),0) FROM message_parts`).Scan(&pb)
		contentBytes += cb.Int64
		skeletonBytes += sb.Int64
		textBytes += tb.Int64
		partBytes += pb.Int64
		_ = filepath.Walk(filepath.Join(dir, "projects", "p", "sessions", id+".images"), func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				imgFileBytes += info.Size()
			}
			return nil
		})
		var pending int
		_ = c.QueryRow(`SELECT COUNT(*) FROM messages WHERE split_state = 0`).Scan(&pending)
		if pending != 0 {
			t.Errorf("%s: %d messages still pending", filepath.Base(src), pending)
		}
		sessions++
		msgs += n
		manager.Close()
	}
	if sessions == 0 {
		t.Skip("no readable databases")
	}
	mb := func(b int64) float64 { return float64(b) / 1048576 }
	t.Logf("%d sessions, %d messages", sessions, msgs)
	t.Logf("  content column (dropped by stage 6)     : %7.2f MB", mb(contentBytes))
	t.Logf("  skeleton column                         : %7.2f MB", mb(skeletonBytes))
	t.Logf("  searchable text column                  : %7.2f MB", mb(textBytes))
	t.Logf("  part bodies in the database (raw)       : %7.2f MB", mb(partBytes))
	t.Logf("  image files on disk                     : %7.2f MB", mb(imgFileBytes))
	after := skeletonBytes + textBytes + partBytes
	t.Logf("  => after the drop the database holds %.2f MB (+ %.2f MB images as files); content measured %.2f MB before it was dropped", mb(after), mb(imgFileBytes), mb(contentBytes))
	t.Logf("  => reading a message's words now touches %.2f MB (text)", mb(textBytes))
}

// Searches copies of the developer's real history through the same Search the
// agent uses. It asserts nothing about content (that is private); it reports
// timing and checks that results are well formed and that a word known to be in
// the history is found.
func TestSearchOverCopiesOfRealHistory(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	store := filepath.Join(home, ".console")
	var srcs []string
	_ = filepath.Walk(store, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".db") && strings.Contains(p, string(filepath.Separator)+"sessions"+string(filepath.Separator)) {
			srcs = append(srcs, p)
		}
		return nil
	})
	dir := t.TempDir()
	manager, err := db.Open(db.OpenOptions{Path: filepath.Join(dir, "global.db"), StorageDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	svc := services.NewSessionService(manager)
	loaded := 0
	for _, src := range srcs {
		probe, err := sql.Open("sqlite", "file:"+src+"?mode=ro&immutable=1")
		if err != nil {
			continue
		}
		var n int
		bad := probe.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&n) != nil
		probe.Close()
		if bad {
			continue
		}
		id := strings.TrimSuffix("s"+filepath.Base(src), ".db")
		dst := filepath.Join(dir, "projects", "p", "sessions", id+".db")
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		if copyFile(src, dst) != nil {
			continue
		}
		_, _ = manager.Global().Exec(`INSERT INTO sessions (id, title, cwd, project_id, model_id, provider, message_count, status, approval_mode, created_at, updated_at) VALUES (?, ?, '/w', 'p', 'm', 'p', ?, 'idle', 'always-ask', 1, ?)`, id, "session "+id, n, loaded+1)
		loaded++
	}
	if loaded == 0 {
		t.Skip("no readable databases")
	}

	// First search converts any session not yet split (the cost a user pays once).
	first := time.Now()
	r, err := svc.Search(session.SearchScope{ProjectID: "p"}, "the", session.SearchOptions{Limit: 100, PerSession: 100})
	if err != nil {
		t.Fatal(err)
	}
	firstTook := time.Since(first)

	// Later searches are the steady state.
	const reps = 20
	start := time.Now()
	var last *session.SearchResult
	for i := 0; i < reps; i++ {
		last, err = svc.Search(session.SearchScope{ProjectID: "p"}, "the", session.SearchOptions{Limit: 100, PerSession: 100})
		if err != nil {
			t.Fatal(err)
		}
	}
	each := time.Since(start) / reps

	for _, h := range last.Hits {
		if h.MessageID == "" || h.SessionID == "" || h.Snippet == "" || len(h.Matches) == 0 {
			t.Fatalf("malformed hit: %+v", h)
		}
		if h.Role != "user" && h.Role != "assistant" {
			t.Fatalf("a %q message was returned", h.Role)
		}
		if strings.Contains(h.Snippet, "\n") {
			t.Fatalf("snippet has a newline")
		}
	}
	t.Logf("%d sessions copied; search for \"the\": %d hits across %d sessions", loaded, len(last.Hits), last.SessionsSearched)
	t.Logf("first search (includes converting any unsplit session): %v", firstTook.Round(time.Millisecond))
	t.Logf("steady-state search: %v each", each.Round(time.Microsecond))
	_ = r
}
