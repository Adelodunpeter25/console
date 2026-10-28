// Tool-call presentation classification: labels, kind flags, icon keys.
// The server derives these once (classify.go) so clients render instead of
// keeping their own tool-name tables.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func TestToolLabelAliases(t *testing.T) {
	cases := map[string]string{
		"read_file": "Read File", "readFile": "Read File",
		"editFile": "Edit File", "edit_file": "Edit File", "str_replace": "Edit File",
		"batchWrite": "Batch Write", "write_file": "Write File",
		"bash": "Run Command", "shell": "Run Command", "command": "Run Command",
		"bashJob": "Bash Job", "bash_job": "Bash Job",
		"grep": "Search Code", "glob": "Find Files",
		"list_dir": "List Directory", "listDir": "List Directory", "ls": "List Directory",
		"webFetch": "Fetch URL", "webSearch": "Web Search",
		"browser": "Browser", "visualize": "Visualize", "subagent": "Subagent",
		"ask": "Ask Question", "askMany": "Ask Questions", "todo": "Todo",
		"memory": "Memory", "readSkill": "Read Skill", "loadTools": "Load Tools",
	}
	for name, want := range cases {
		if got := tools.ToolLabel(name); got != want {
			t.Errorf("ToolLabel(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestToolLabelMCPServer(t *testing.T) {
	if got := tools.ToolLabel("mcp__github__create_issue"); got != "create issue (github)" {
		t.Errorf("mcp label: %q", got)
	}
	if got := tools.ToolLabel("mcp__a-b__c_d"); got != "c d (a-b)" {
		t.Errorf("mcp label underscores: %q", got)
	}
	for _, name := range []string{"mcp__", "mcp____tool", "mcp__server__", "mcp__server", "other"} {
		if got := tools.ToolLabel(name); got != name {
			t.Errorf("malformed %q: got %q, want raw name", name, got)
		}
	}
}

func TestToolKindFlags(t *testing.T) {
	cases := []struct {
		name  string
		flags uint32
	}{
		{"edit_file", tools.KindFlagEdit | tools.KindFlagFileTarget},
		{"editFile", tools.KindFlagEdit | tools.KindFlagFileTarget},
		{"batch_write", tools.KindFlagWrite | tools.KindFlagFileTarget},
		{"writeFile", tools.KindFlagWrite | tools.KindFlagFileTarget},
		{"read_file", tools.KindFlagRead | tools.KindFlagFileTarget},
		{"Read", tools.KindFlagRead | tools.KindFlagFileTarget},
		{"view", tools.KindFlagRead | tools.KindFlagFileTarget},
		{"subagent", tools.KindFlagSubagent},
		// A search scope is not a file target.
		{"grep", 0},
		{"bash", 0},
		{"mcp__x__y", 0},
	}
	for _, c := range cases {
		if got := tools.ToolKindFlags(c.name); got != c.flags {
			t.Errorf("ToolKindFlags(%q) = %d, want %d", c.name, got, c.flags)
		}
	}
}

func TestToolIconKey(t *testing.T) {
	cases := map[string]string{
		"bash": "terminal", "edit_file": "pencil", "readFile": "file",
		"list_dir": "folder", "grep": "search", "webSearch": "globe",
		"browser": "globe", "visualize": "chart", "memory": "memory",
		"loadTools": "package", "mcp__gh__x": "package", "plan": "list",
		"reasoning": "sparkle", "unknown_thing": "wrench",
	}
	for name, want := range cases {
		if got := tools.ToolIconKey(name); got != want {
			t.Errorf("ToolIconKey(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestParseMCPToolName(t *testing.T) {
	if s, tl, ok := tools.ParseMCPToolName("mcp__a-b__c_d"); !ok || s != "a-b" || tl != "c_d" {
		t.Errorf("ParseMCPToolName: %q %q %v", s, tl, ok)
	}
	if _, _, ok := tools.ParseMCPToolName("mcp__nodouble"); ok {
		t.Error("single underscore must not split")
	}
}
