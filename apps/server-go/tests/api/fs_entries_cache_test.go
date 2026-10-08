// /api/fs/entries caches its encoded body until the fs watcher reports a change.
package tests

import (
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFsEntriesCacheInvalidatedByWatcher(t *testing.T) {
	app := fsRoutesApp(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := func() string {
		req := httptest.NewRequest("GET", "/api/fs/entries?path="+url.QueryEscape(root), nil)
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		return string(raw)
	}

	first := entries()
	if !strings.Contains(first, "a.txt") {
		t.Fatalf("first listing: %s", first)
	}
	if second := entries(); second != first {
		t.Fatalf("repeat listing should be served from cache unchanged:\n%s\n%s", first, second)
	}

	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(entries(), "b.txt") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("new file never appeared: cache not invalidated by watcher")
}
