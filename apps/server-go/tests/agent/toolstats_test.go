// Tool stats: per-tool counters recorded by the loop and error-class
// mapping for real tool error messages.
package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestClassifyToolError(t *testing.T) {
	cases := map[string]string{
		"Invalid arguments for read_file: unexpected EOF": loop.ErrClassInvalidArgs,
		"path is required":                                                   loop.ErrClassInvalidArgs,
		`The oldContent was not found in "a.go".`:                            loop.ErrClassInvalidArgs,
		"'store' operation requires 'content'.":                              loop.ErrClassInvalidArgs,
		"File not found: /tmp/x":                                             loop.ErrClassEnv,
		"Cannot read /tmp/x: permission denied":                              loop.ErrClassEnv,
		"Command timed out after 30000ms":                                    loop.ErrClassTimeout,
		"Tool execution cancelled by user abort.":                            loop.ErrClassAborted,
		"User denied permission for tool 'bash'.":                            loop.ErrClassDenied,
		"Tool 'bash' requires approval but no approver is connected (mode).": loop.ErrClassDenied,
		"Unknown tool: frobnicate":                                           loop.ErrClassUnknownTool,
		"search failed: HTTP 429":                                            loop.ErrClassProvider,
		"something odd happened":                                             loop.ErrClassUnknown,
	}
	for text, want := range cases {
		if got := loop.ClassifyToolError(text); got != want {
			t.Errorf("%q: got %s want %s", text, got, want)
		}
	}
}

func TestToolStatsRecordedByLoop(t *testing.T) {
	good := tools.ToolCall{ID: "c1", Name: "list_dir", Arguments: json.RawMessage(`{"path":"` + t.TempDir() + `"}`)}
	bad := tools.ToolCall{ID: "c2", Name: "read_file", Arguments: json.RawMessage(`{"path":"/definitely/missing/file.txt"}`)}
	unknown := tools.ToolCall{ID: "c3", Name: "frobnicate", Arguments: json.RawMessage(`{}`)}
	provider := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event {
			return []loop.Event{
				{Kind: loop.EventToolCall, Call: &good},
				{Kind: loop.EventToolCall, Call: &bad},
				{Kind: loop.EventToolCall, Call: &unknown},
			}
		},
		func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "done"}} },
	}}
	registry := tools.NewRegistry(tools.DefaultTools()...)
	agent := loop.New(provider, loop.NewExecutor(registry, permissions.FullAccess, nil), nil)
	events, err := agent.Run(context.Background(), "s1", "go", registry.Definitions())
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err, ok := events.Next(); !ok {
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	stats := agent.ToolStats.Snapshot()
	if s := stats["list_dir"]; s.Calls != 1 || s.Errors != 0 || s.ResultBytes == 0 {
		t.Fatalf("list_dir: %+v", s)
	}
	if s := stats["read_file"]; s.Calls != 1 || s.Errors != 1 || s.ErrorsBy[loop.ErrClassEnv] != 1 {
		t.Fatalf("read_file: %+v", s)
	}
	if s := stats["frobnicate"]; s.Errors != 1 || s.ErrorsBy[loop.ErrClassUnknownTool] != 1 {
		t.Fatalf("frobnicate: %+v", s)
	}
}
