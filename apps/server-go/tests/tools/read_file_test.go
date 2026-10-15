// read_file output limits: default 300-line cap, explicit ranges, and the
// sparse line-number flag.
package tests

import (
	"context"
	"encoding/base64"
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

func execRead(t *testing.T, args map[string]any) (any, error) {
	t.Helper()
	raw, _ := json.Marshal(args)
	return tools.ReadFile.Execute(context.Background(), raw)
}

func TestReadFileImages(t *testing.T) {
	dir := t.TempDir()

	// Supported images come back as [image part, text line] with the payload
	// base64'd and the sniffed mime.
	cases := []struct {
		name string
		mime string
		data []byte
	}{
		{"a.png", "image/png", append([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, []byte("png-payload")...)},
		{"a.jpg", "image/jpeg", append([]byte{0xff, 0xd8, 0xff, 0xe0}, []byte("jpeg-payload")...)},
		{"a.gif", "image/gif", append([]byte("GIF89a"), []byte("gif-payload")...)},
		{"a.webp", "image/webp", append([]byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'}, []byte("webp-payload")...)},
	}
	for _, tc := range cases {
		p := filepath.Join(dir, tc.name)
		if err := os.WriteFile(p, tc.data, 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := execRead(t, map[string]any{"path": p})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		parts, ok := out.([]map[string]any)
		if !ok || len(parts) != 2 {
			t.Fatalf("%s: want [image, text], got %#v", tc.name, out)
		}
		if parts[0]["type"] != "image" || parts[0]["mimeType"] != tc.mime {
			t.Fatalf("%s: image part %#v", tc.name, parts[0])
		}
		if parts[0]["data"] != base64.StdEncoding.EncodeToString(tc.data) {
			t.Fatalf("%s: base64 payload mismatch", tc.name)
		}
		if text, _ := parts[1]["text"].(string); parts[1]["type"] != "text" || !strings.Contains(text, "Image read:") {
			t.Fatalf("%s: text part %#v", tc.name, parts[1])
		}
	}

	// Magic bytes win over extension: a text file named ".png" is still text.
	p := filepath.Join(dir, "text.png")
	if err := os.WriteFile(p, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := execRead(t, map[string]any{"path": p})
	if err != nil {
		t.Fatalf("text-named .png: %v", err)
	}
	text, _ := out.([]map[string]any)[0]["text"].(string)
	if !strings.Contains(text, "1: line1") || !strings.Contains(text, "2: line2") {
		t.Fatalf("text-named .png must read as text: %q", text)
	}

	// A truncated PNG (signature cut short) is invalid UTF-8 and refused.
	truncated := filepath.Join(dir, "trunc.png")
	if err := os.WriteFile(truncated, []byte{0x89, 0x50, 0x4e, 0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := execRead(t, map[string]any{"path": truncated}); err == nil {
		t.Fatal("truncated PNG must be rejected as binary")
	}

	// A NUL-bearing file with an innocuous extension is binary.
	binary := filepath.Join(dir, "data.dat")
	if err := os.WriteFile(binary, []byte("hello\x00world"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := execRead(t, map[string]any{"path": binary}); err == nil {
		t.Fatal("NUL-bearing file must be rejected as binary")
	}

	// PDFs stay a read error (no PDF support).
	pdf := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := execRead(t, map[string]any{"path": pdf}); err == nil {
		t.Fatal("PDF must be rejected as binary")
	}

	// The image ingest ceiling is enforced and honours the option.
	limits := tools.NewReadFileTool(tools.ReadFileOptions{MaxImageBytes: 4})
	img := filepath.Join(dir, "big.png")
	if err := os.WriteFile(img, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 'x'}, 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"path": img})
	if _, err := limits.Execute(context.Background(), raw); err == nil {
		t.Fatal("oversized image must be rejected")
	}

	// Directory, empty-file, and offset-past-EOF behaviour is unchanged.
	dirOut, err := execRead(t, map[string]any{"path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := dirOut.([]map[string]any)[0]["text"].(string); !strings.Contains(text, "is a directory. Use list_dir") {
		t.Fatalf("directory hint: %q", text)
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	emptyOut, err := execRead(t, map[string]any{"path": empty})
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := emptyOut.([]map[string]any)[0]["text"].(string); !strings.Contains(text, "File is empty:") {
		t.Fatalf("empty message: %q", text)
	}
	past := writeLines(t, 3)
	pastOut, err := execRead(t, map[string]any{"path": past, "startLine": 10})
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := pastOut.([]map[string]any)[0]["text"].(string); !strings.Contains(text, "Offset beyond EOF.") {
		t.Fatalf("offset message: %q", text)
	}
}
