// Request dumps: CONSOLE_DUMP_REQUESTS writes numbered pretty-JSON bodies
// per conversation, and does nothing when unset.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

func TestDumpRequestWritesNumberedFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(shared.DumpRequestsEnv, dir)
	shared.DumpRequest("claude", "sess:claude/haiku", []byte(`{"model":"x"}`))
	shared.DumpRequest("claude", "sess:claude/haiku", []byte(`{"model":"y"}`))
	run := filepath.Join(dir, "sess_claude_haiku")
	first, err := os.ReadFile(filepath.Join(run, "0001-claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "\"model\": \"x\"") {
		t.Fatalf("first dump not pretty JSON: %s", first)
	}
	if _, err := os.Stat(filepath.Join(run, "0002-claude.json")); err != nil {
		t.Fatalf("second dump: %v", err)
	}
}

func TestDumpRequestDisabledByDefault(t *testing.T) {
	t.Setenv(shared.DumpRequestsEnv, "")
	cwd := t.TempDir()
	t.Chdir(cwd)
	shared.DumpRequest("codex", "c1", []byte(`{}`))
	entries, _ := os.ReadDir(cwd)
	if len(entries) != 0 {
		t.Fatalf("unexpected files: %v", entries)
	}
}
