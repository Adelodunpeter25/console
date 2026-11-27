package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/codemode"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func TestCodemodeFileRawRoundTrips(t *testing.T) {
	dir := t.TempDir()
	rt, err := codemode.NewRuntime(newTestExecutor(t, dir), codemode.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		".gitignore":   "node_modules\n*.log\n",
		"nonl.txt":     "no trailing newline",
		"package.json": "{\n  \"b\": 1,\n  \"a\": [1, 2]\n}\n",
		"blank.txt":    "a\n\n\nb\n",
	}
	for name, content := range cases {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		// Read-modify-write with no modification must be a no-op.
		evalOK(t, rt, "await write("+jsonString(p)+", await fileRaw("+jsonString(p)+"));")
		got, _ := os.ReadFile(p)
		if string(got) != content {
			t.Fatalf("%s mangled by raw round-trip:\nwant %q\n got %q", name, content, got)
		}
	}
}

func TestCodemodeFileIsFormattedForDisplay(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	os.WriteFile(p, []byte("hello\n"), 0o644)
	rt, err := codemode.NewRuntime(newTestExecutor(t, dir), codemode.Options{})
	if err != nil {
		t.Fatal(err)
	}
	out := evalOK(t, rt, "await file("+jsonString(p)+");")
	if !strings.Contains(out, "File: ") || !strings.Contains(out, "1: hello") {
		t.Fatalf("file() should stay display-formatted, got %q", out)
	}
}

func TestCodemodeFileRawRange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "r.txt")
	os.WriteFile(p, []byte("one\ntwo\nthree\nfour\n"), 0o644)
	rt, err := codemode.NewRuntime(newTestExecutor(t, dir), codemode.Options{})
	if err != nil {
		t.Fatal(err)
	}
	out := evalOK(t, rt, "(await fileRaw("+jsonString(p)+", {startLine: 2, endLine: 3})).split(String.fromCharCode(10)).join('|');")
	if !strings.Contains(out, "two|three") || strings.Contains(out, "one") || strings.Contains(out, "four") {
		t.Fatalf("raw range wrong: %q", out)
	}
}

func TestCodemodeFileRawRefusesOversize(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.txt")
	os.WriteFile(p, []byte(strings.Repeat("x", 600*1024)), 0o644)
	rt, err := codemode.NewRuntime(newTestExecutor(t, dir), codemode.Options{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.Eval(context.Background(), "await fileRaw("+jsonString(p)+");", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	env := res.(tools.Envelope)
	if !env.IsError || !strings.Contains(env.Content.(string), "raw") {
		t.Fatalf("oversize raw read must error rather than truncate, got %v", env.Content)
	}
}
