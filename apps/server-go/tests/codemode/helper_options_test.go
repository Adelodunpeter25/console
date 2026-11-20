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

func evalOK(t *testing.T, rt *codemode.Runtime, code string) string {
	t.Helper()
	res, err := rt.Eval(context.Background(), code, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	env := res.(tools.Envelope)
	if env.IsError {
		t.Fatalf("eval failed: %v", env.Content)
	}
	return env.Content.(string)
}

func TestCodemodeFileLineRange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "lines.txt")
	var c strings.Builder
	for i := 1; i <= 400; i++ {
		c.WriteString("row-" + itoa(i) + "\n")
	}
	os.WriteFile(p, []byte(c.String()), 0o644)
	rt, err := codemode.NewRuntime(newTestExecutor(t, dir), codemode.Options{})
	if err != nil {
		t.Fatal(err)
	}
	q := jsonString(p)
	out := evalOK(t, rt, "await file("+q+", 350, 352);")
	if !strings.Contains(out, "row-350") || !strings.Contains(out, "row-352") || strings.Contains(out, "row-10\n") {
		t.Fatalf("positional range not honored: %s", out)
	}
	out = evalOK(t, rt, "await file("+q+", {startLine: 399, endLine: 400});")
	if !strings.Contains(out, "row-399") || strings.Contains(out, "row-398") {
		t.Fatalf("object range not honored: %s", out)
	}
}

func TestCodemodeHelperOptionsPassThrough(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644)
	rt, err := codemode.NewRuntime(newTestExecutor(t, dir), codemode.Options{})
	if err != nil {
		t.Fatal(err)
	}
	q := jsonString(dir)
	if out := evalOK(t, rt, "JSON.stringify(await list("+q+"))"); strings.Contains(out, ".hidden") {
		t.Fatalf("hidden file listed without showHidden: %s", out)
	}
	if out := evalOK(t, rt, "JSON.stringify(await list("+q+", {showHidden: true}))"); !strings.Contains(out, ".hidden") {
		t.Fatalf("showHidden not forwarded: %s", out)
	}
	if out := evalOK(t, rt, "(await glob('*.txt', {root: "+q+", maxResults: 1})).length"); !strings.HasPrefix(strings.TrimSpace(out), "1") {
		t.Fatalf("maxResults not forwarded: %s", out)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
