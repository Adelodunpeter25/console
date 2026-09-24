// read_file output limits: default 300-line cap, explicit ranges, and the
// sparse line-number flag.
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func writeLines(t *testing.T, n int) string {
	t.Helper()
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i+1)
	}
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readText(t *testing.T, args map[string]any) string {
	t.Helper()
	raw, _ := json.Marshal(args)
	out, err := tools.ReadFile.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return out.([]map[string]any)[0]["text"].(string)
}

func TestReadFileDefaultCap(t *testing.T) {
	p := writeLines(t, 842)
	text := readText(t, map[string]any{"path": p})
	if !strings.Contains(text, "300: line300") || strings.Contains(text, "line301") {
		t.Fatalf("expected first 300 lines only")
	}
	if !strings.Contains(text, "Output truncated at line 300 of 842. Continue with startLine=301.") {
		t.Fatalf("missing continue hint: %q", text[:200])
	}
}

func TestReadFileShortFileWhole(t *testing.T) {
	p := writeLines(t, 50)
	text := readText(t, map[string]any{"path": p})
	if !strings.Contains(text, "Showing: all 50 lines\n") || strings.Contains(text, "truncated") {
		t.Fatalf("short file should be whole: %q", text[:120])
	}
}

func TestReadFileExplicitRangeUncapped(t *testing.T) {
	p := writeLines(t, 842)
	text := readText(t, map[string]any{"path": p, "startLine": 1, "endLine": 800})
	if !strings.Contains(text, "800: line800") || strings.Contains(text, "truncated") {
		t.Fatalf("explicit range should not hit the default cap")
	}
	text = readText(t, map[string]any{"path": p, "startLine": 301})
	if !strings.Contains(text, "842: line842") {
		t.Fatalf("startLine-only read should reach EOF")
	}
}

func TestReadFileSparseLineNumbers(t *testing.T) {
	p := writeLines(t, 25)
	t.Setenv("CONSOLE_HARNESS_SPARSE_LINE_NUMBERS", "1")
	text := readText(t, map[string]any{"path": p})
	for _, want := range []string{" 1: line1\n", "10: line10\n", "20: line20\n", "25: line25"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing numbered %q in %q", want, text)
		}
	}
	if strings.Contains(text, "2: line2\n") || !strings.Contains(text, "\n    line2\n") {
		t.Fatalf("line 2 should be unnumbered and aligned: %q", text)
	}
}

func TestReadFileDenseByDefault(t *testing.T) {
	p := writeLines(t, 12)
	text := readText(t, map[string]any{"path": p})
	if !strings.Contains(text, " 2: line2\n") {
		t.Fatalf("every line should be numbered when flag is off: %q", text)
	}
}
