// Coverage for editFile, batchWrite, and readSkill.
package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func TestEditFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Missing match -> tool error.
	args, _ := json.Marshal(map[string]any{"path": path, "oldContent": "nope", "newContent": "x"})
	if _, err := tools.EditFile.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error for missing match")
	}

	// Exact match -> replaced.
	args, _ = json.Marshal(map[string]any{"path": path, "oldContent": "world", "newContent": "there"})
	if _, err := tools.EditFile.Execute(context.Background(), args); err != nil {
		t.Fatalf("execute: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "hello there\n" {
		t.Fatalf("file: %v %q", err, data)
	}

	// Ambiguous match -> tool error.
	if err := os.WriteFile(path, []byte("dup dup\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ = json.Marshal(map[string]any{"path": path, "oldContent": "dup", "newContent": "x"})
	if _, err := tools.EditFile.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error for ambiguous match")
	}
}

func TestBatchWrite(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(map[string]any{
		"files": []map[string]any{
			{"path": filepath.Join(dir, "a.txt"), "content": "A"},
			{"path": filepath.Join(dir, "nested", "b.txt"), "content": "B"},
		},
	})
	out, err := tools.BatchWrite.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	raw, _ := json.Marshal(out)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["isError"] != false {
		t.Fatalf("unexpected error: %+v", m)
	}
	for _, p := range []string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "nested", "b.txt")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}

	// Duplicate paths rejected up front.
	dupArgs, _ := json.Marshal(map[string]any{
		"files": []map[string]any{
			{"path": filepath.Join(dir, "c.txt"), "content": "1"},
			{"path": filepath.Join(dir, "c.txt"), "content": "2"},
		},
	})
	if _, err := tools.BatchWrite.Execute(context.Background(), dupArgs); err == nil {
		t.Fatal("expected error for duplicate paths")
	}
}

func TestReadSkill(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, ".agents", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "deploy.md"),
		[]byte("---\ndescription: Deploy the app\n---\nRun the deploy script."), 0o644); err != nil {
		t.Fatal(err)
	}

	// No name -> catalog listing.
	listArgs, _ := json.Marshal(map[string]any{"cwd": dir})
	listOut, err := tools.ReadSkill.Execute(context.Background(), listArgs)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if s, ok := listOut.(string); !ok || !strings.Contains(s, "deploy") || !strings.Contains(s, "Deploy the app") {
		t.Fatalf("catalog: %v", listOut)
	}

	// Named -> full content.
	readArgs, _ := json.Marshal(map[string]any{"cwd": dir, "name": "deploy"})
	readOut, err := tools.ReadSkill.Execute(context.Background(), readArgs)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if s, ok := readOut.(string); !ok || !strings.Contains(s, "Run the deploy script.") {
		t.Fatalf("content: %v", readOut)
	}

	// Unknown name -> tool error.
	missingArgs, _ := json.Marshal(map[string]any{"cwd": dir, "name": "nope"})
	if _, err := tools.ReadSkill.Execute(context.Background(), missingArgs); err == nil {
		t.Fatal("expected error for unknown skill")
	}
}
