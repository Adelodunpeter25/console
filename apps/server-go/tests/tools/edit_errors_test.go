// Coverage for tool error quality: editFile recovery hints and required-field
// validation, plus ANSI/carriage-return stripping in bash output (Task 3.2/3.3).
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

func editErrorText(t *testing.T, args map[string]any) string {
	t.Helper()
	raw, _ := json.Marshal(args)
	_, err := tools.EditFile.Execute(context.Background(), raw)
	if err == nil {
		t.Fatal("expected an error")
	}
	return err.Error()
}

func TestEditFileNotFoundSuggestsClosestLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte("package main\n\nfunc Bar() int {\n\treturn 1\n}\n\nfunc Baz() {\n\tBar()\n}\n"), 0o644)

	s := editErrorText(t, map[string]any{"path": path, "oldContent": "func Bar() int  {\n\treturn 1\n}", "newContent": "x"})
	if !strings.Contains(s, "whitespace-insensitive match") {
		t.Fatalf("expected a whitespace hint, got: %s", s)
	}
	if !strings.Contains(s, "line 3") {
		t.Fatalf("expected the match location, got: %s", s)
	}

	// A genuinely different string gets the nearest candidates instead.
	s = editErrorText(t, map[string]any{"path": path, "oldContent": "func Barr() int {\n\treturn 2\n}", "newContent": "x"})
	if !strings.Contains(s, "Closest matching lines") || !strings.Contains(s, "3: func Bar() int {") {
		t.Fatalf("expected closest-line candidates, got: %s", s)
	}
}

func TestEditFileAmbiguousListsLineNumbers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte("a\nb\nc\nb\nd\nb\n"), 0o644)

	s := editErrorText(t, map[string]any{"path": path, "oldContent": "b", "newContent": "x"})
	if !strings.Contains(s, "3 times") || !strings.Contains(s, "lines: 2, 4, 6") {
		t.Fatalf("expected occurrence line numbers, got: %s", s)
	}
}

func TestEditFileMissingRequiredFieldNamesIt(t *testing.T) {
	s := editErrorText(t, map[string]any{"path": "/tmp/whatever"})
	if !strings.Contains(s, "Missing required fields for editFile: oldContent, newContent") {
		t.Fatalf("expected missing-field error, got: %s", s)
	}
}

func TestBashStripsANSIAndCarriageReturns(t *testing.T) {
	args, _ := json.Marshal(map[string]any{
		"command": `printf '\033[31mred\033[0m plain\nloading 10%%\rloading 99%%\rdone\n'`,
	})
	out, err := tools.Bash.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := resultText(t, out)
	if strings.Contains(s, "\x1b") {
		t.Fatalf("ANSI escapes reached the result: %q", s)
	}
	if !strings.Contains(s, "red plain") || !strings.Contains(s, "done") {
		t.Fatalf("lost real content: %q", s)
	}
	if strings.Contains(s, "loading 10%") || strings.Contains(s, "loading 99%") {
		t.Fatalf("progress frames not collapsed: %q", s)
	}
}
