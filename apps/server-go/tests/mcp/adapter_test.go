// Tool adapter: naming, tier resolution, and registry ordering.
package tests

import (
	"context"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

func TestToolNameSanitizedAndCapped(t *testing.T) {
	if got := mcp.ToolName("atl", "search issues.v2"); got != "mcp__atl__search_issues_v2" {
		t.Fatalf("got %q", got)
	}
	long := mcp.ToolName("atl", string(make([]byte, 200)))
	if len(long) > 64 {
		t.Fatalf("name too long: %d", len(long))
	}
}

func TestResolveTierOrder(t *testing.T) {
	cfg := mcp.ServerConfig{ID: "a", TierOverrides: map[string]string{"delete_jira": "read", "get_x": "exec"}}
	cases := []struct {
		tool mcp.RemoteTool
		want tools.ToolTier
	}{
		{mcp.RemoteTool{Name: "delete_jira"}, tools.TierRead},              // override wins over heuristic
		{mcp.RemoteTool{Name: "get_x"}, tools.TierExec},                    // override wins over heuristic
		{mcp.RemoteTool{Name: "anything", ReadOnly: true}, tools.TierRead}, // readOnly annotation
		{mcp.RemoteTool{Name: "search_issues"}, tools.TierRead},            // naming heuristic
		{mcp.RemoteTool{Name: "delete_page"}, tools.TierExec},              // naming heuristic
		{mcp.RemoteTool{Name: "create_issue"}, tools.TierWrite},            // default
	}
	for _, c := range cases {
		if got := mcp.ResolveTier(cfg, c.tool); got != c.want {
			t.Errorf("%s: got %s want %s", c.tool.Name, got, c.want)
		}
	}
}

func TestRegistryAddKeepsBasePrefixStable(t *testing.T) {
	mk := func(name string) tools.Tool {
		return tools.NewTool(name, name, tools.TierRead, func(_ context.Context, _ struct{}) (any, error) { return nil, nil })
	}
	reg := tools.NewRegistry(mk("zulu"), mk("alpha"))
	before := reg.Definitions()
	reg.Add(mk("mcp__x__b"), mk("mcp__x__a"), mk("alpha")) // duplicate ignored
	after := reg.Definitions()

	names := []string{}
	for _, d := range after {
		names = append(names, d.Name)
	}
	want := []string{"alpha", "zulu", "mcp__x__b", "mcp__x__a"}
	if len(names) != len(want) {
		t.Fatalf("got %v want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v want %v", names, want)
		}
	}
	for i, d := range before {
		if after[i].Name != d.Name {
			t.Fatalf("base prefix changed at %d", i)
		}
	}
}
