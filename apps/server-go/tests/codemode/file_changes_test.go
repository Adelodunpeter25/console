package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/codemode"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func TestCodemodeFileWritesReachExecutorHook(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	os.WriteFile(p, []byte("before\n"), 0644)
	reg := tools.NewRegistry(tools.DefaultTools()...)
	exec := loop.NewExecutor(reg, permissions.FullAccess)
	exec.Guard = permissions.NewGuard(dir)
	var calls []string
	exec.OnAfterExecute = func(call tools.ToolCall, result tools.ToolResult) { calls = append(calls, call.Name) }
	rt, err := codemode.NewRuntime(exec, codemode.Options{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.Eval(context.Background(), `await write(`+jsonStr(p)+`, "after\n");`, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if env := res.(tools.Envelope); env.IsError {
		t.Fatal(env.Content)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "after\n" {
		t.Fatalf("got %q", data)
	}
	found := false
	for _, c := range calls {
		if c == "writeFile" {
			found = true
		}
	}
	if !found {
		t.Fatalf("writeFile not seen by OnAfterExecute: %v", calls)
	}
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
