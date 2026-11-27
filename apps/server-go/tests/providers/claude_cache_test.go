// Claude prompt-cache markers: long retention asks for the one-hour lifetime,
// and per-request tail context lands after the cache breakpoint.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
)

func TestClaudeLongRetentionUsesOneHourTTL(t *testing.T) {
	defs := []tools.Definition{{Name: "t"}}
	msgs := []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}}
	for retention, want := range map[loop.CacheRetention]string{loop.CacheShort: "", loop.CacheLong: "1h"} {
		body := claude.BuildRequestBody("m", "sys", msgs, defs, retention, "")
		system := body["system"].([]any)
		marker := system[1].(map[string]any)["cache_control"].(map[string]any)
		if got, _ := marker["ttl"].(string); got != want {
			t.Fatalf("%s: system ttl = %q, want %q", retention, got, want)
		}
		toolMarker := body["tools"].([]claude.ClaudeTool)[0].CacheControl
		if got, _ := toolMarker["ttl"].(string); got != want {
			t.Fatalf("%s: tool ttl = %q, want %q", retention, got, want)
		}
	}
}

func TestClaudeTailLandsAfterTheCacheBreakpoint(t *testing.T) {
	msgs := []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}}
	body := claude.BuildRequestBody("m", "sys", msgs, nil, loop.CacheShort, "")
	claude.AppendTail(body, "mode: plan")
	last := body["messages"].([]claude.ClaudeMessage)
	blocks := last[len(last)-1].Content
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want the user text plus the tail", len(blocks))
	}
	if _, ok := blocks[0]["cache_control"]; !ok {
		t.Fatal("the breakpoint must stay on the last stable block")
	}
	if _, ok := blocks[1]["cache_control"]; ok {
		t.Fatal("the tail must not carry a breakpoint")
	}
}
