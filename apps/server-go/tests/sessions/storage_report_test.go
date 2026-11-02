package tests

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
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
	t.Logf("  content column (kept during transition) : %7.2f MB", mb(contentBytes))
	t.Logf("  skeleton column                         : %7.2f MB", mb(skeletonBytes))
	t.Logf("  searchable text column                  : %7.2f MB", mb(textBytes))
	t.Logf("  part bodies in the database (raw)       : %7.2f MB", mb(partBytes))
	t.Logf("  image files on disk                     : %7.2f MB", mb(imgFileBytes))
	after := skeletonBytes + textBytes + partBytes
	t.Logf("  => AFTER dropping content: database holds %.2f MB (+ %.2f MB images as files) vs %.2f MB today", mb(after), mb(imgFileBytes), mb(contentBytes))
	t.Logf("  => reading a message's words now touches %.2f MB (text) instead of %.2f MB", mb(textBytes), mb(contentBytes))
}
