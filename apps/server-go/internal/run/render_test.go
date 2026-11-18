package run

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// A subagent inherits the parent's tools, but a visual made there would have
// no transcript card to render into, so the tool is filtered out.
func TestWithoutToolDropsOnlyTheNamedTool(t *testing.T) {
	list := []tools.Tool{
		tools.NewTool[struct{}]("read", "read", tools.TierRead, nil),
		tools.NewTool[struct{}]("render", "render", tools.TierRead, nil),
		tools.NewTool[struct{}]("write", "write", tools.TierWrite, nil),
	}
	out := withoutTool(list, "render")
	if len(out) != 2 {
		t.Fatalf("tools = %d, want 2", len(out))
	}
	for _, tool := range out {
		if tool.Name() == "render" {
			t.Fatal("render must be filtered out")
		}
	}
}