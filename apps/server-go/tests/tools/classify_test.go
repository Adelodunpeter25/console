// Tool-call presentation classification: labels, kind flags, icon keys.
// The server derives these once (classify.go) so clients render instead of
// keeping their own tool-name tables.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Labels for the names the server really registers. There are no aliases.
func TestToolLabels(t *testing.T) {
	cases := map[string]string{
		"read_file": "Read File", "editFile": "Edit File",
		"batchWrite": "Batch Write", "write_file": "Write File",
		"bash": "Run Command", "bashJob": "Bash Job",
		"grep": "Search Code", "glob": "Find Files",
		"list_dir": "List Directory",
		"webFetch": "Fetch URL", "webSearch": "Web Search",
		"browser": "Browser", "render": "Render", "preview": "Preview", "subagent": "Subagent",
		"ask": "Ask Question", "askMany": "Ask Questions", "todo": "Todo",
		"memory": "Memory", "read_chat": "Read Chat", "readSkill": "Read Skill", "loadTools": "Load Tools",
		"code": "Code", "code_reset": "Reset Code",
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

func TestToolCallLabel(t *testing.T) {
	cases := []struct {
		name string
		args string
		want string
	}{
		{"code", `{"code":"await $'git status --porcelain'"}`, "$ git status --porcelain"},
		{"code", `{"code":"const f = await file(\"src/main.go\")"}`, "file(src/main.go)"},
		{"code", `{"code":"await edit(\"config.json\", \"old\", \"new\")"}`, "edit(config.json)"},
		{"code", `{"code":"const page = await browser.open({url: \"https://example.com\"})"}`, "browser.open"},
		{"code", `{"code":"let sum = 0;\nfor (let i = 0; i < 10; i++) sum += i;"}`, "sum = 0;"},
		{"code", `{"code":"const r = await fileRaw(\"a.rs\");\nconsole.log(r)"}`, "fileRaw(a.rs)"},
		{"code", `{"code":"console.parts.list(); console.parts.list(); console.parts.read({})"}`, "console.parts.list, console.parts.read"},
		{"code", `{"code":"const n = await fileRaw(\"x.rs\");\nfor (let i=0;i<3;i++) console.log(i)"}`, "fileRaw(x.rs)"},
		{"code", `{"code":"// console.log()\nconst r = await fileRaw(\"y.rs\")"}`, "fileRaw(y.rs)"},
		{"code", `{}`, "Code"},
		{"read_file", `{"path":"a.txt"}`, "Read File"},
	}
	for _, c := range cases {
		if got := tools.ToolCallLabel(c.name, []byte(c.args)); got != c.want {
			t.Errorf("ToolCallLabel(%q, %q) = %q, want %q", c.name, c.args, got, c.want)
		}
	}
}

func TestToolKindFlags(t *testing.T) {
	cases := []struct {
		name  string
		flags uint32
	}{
		{"editFile", tools.KindFlagEdit | tools.KindFlagFileTarget},
		{"batchWrite", tools.KindFlagWrite | tools.KindFlagFileTarget},
		{"write_file", tools.KindFlagWrite | tools.KindFlagFileTarget},
		{"read_file", tools.KindFlagRead | tools.KindFlagFileTarget},
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
		"bash": "terminal", "bashJob": "terminal",
		"editFile": "pencil", "write_file": "pencil", "batchWrite": "pencil",
		"read_file": "file", "list_dir": "folder", "grep": "search", "glob": "search",
		"webSearch": "globe", "webFetch": "globe", "browser": "globe",
		"render": "chart", "preview": "chart", "memory": "memory", "read_chat": "memory", "todo": "list",
		"code": "code", "code_reset": "code",
		"loadTools": "package", "mcp__gh__x": "package",
		"unknown_thing": "wrench",
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

// writePlan takes {name, content}, not a path, so it must not be classed as a file
// write: the clients would try to diff it against a path that is not there.
func TestWritePlanIsNotClassedAsAFileWrite(t *testing.T) {
	if flags := tools.ToolKindFlags("writePlan"); flags != 0 {
		t.Fatalf("writePlan flags = %d, want 0", flags)
	}
}

func TestToolCallSummary(t *testing.T) {
	cases := []struct{ name, args, want string }{
		{"read_file", `{"path":"a.txt"}`, "a.txt"},
		{"bash", `{"command":"git status"}`, "git status"},
		{"memory", `{"op":"search","query":"foo"}`, "search \"foo\""},
		{"code", `{"code":"console.sessions.list(); console.sessions.read({})"}`, "console.sessions.list, console.sessions.read"},
		{"code", `{}`, ""},
	}
	for _, c := range cases {
		if got := tools.ToolCallSummary(c.name, []byte(c.args)); got != c.want {
			t.Errorf("ToolCallSummary(%q, %q) = %q, want %q", c.name, c.args, got, c.want)
		}
	}
}
