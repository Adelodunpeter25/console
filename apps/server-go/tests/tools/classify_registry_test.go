// The classification tables name tools. This keeps them honest in both
// directions against the real set of tools a run is built from: no name in the
// tables that nothing registers, and no registered tool left out of them.
package tests

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// registered returns the name of every tool the server can offer a run. Tools
// that need per-run wiring are built with throwaway arguments: only the name
// matters here.
func registered(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, tool := range tools.DefaultTools() {
		names[tool.Name()] = true
	}
	for _, tool := range []tools.Tool{
		tools.NewMemoryTool("", nil),
		tools.NewReadChatTool("", nil),
		tools.NewWritePlanTool(t.TempDir()),
		tools.NewPlanModeBashTool(nil, ""),
		tools.NewRequestFullAccessTool(nil, nil),
	} {
		names[tool.Name()] = true
	}
	// Tools living in other packages are named by the run loop; their names are
	// part of the contract this test pins.
	for _, n := range []string{"console", "console_reset", "computer", "computer_reset", "visualize", "loadTools", "browser", "browser_reset"} {
		names[n] = true
	}
	return names
}

// nameInTable matches a quoted tool name in a `case "a", "b":` line.
var quoted = regexp.MustCompile(`"([A-Za-z_]+)"`)

// tableNames reads every tool name out of classify.go's switch cases, so the
// test follows the source rather than a second hand-kept list.
func tableNames(t *testing.T) map[string]bool {
	t.Helper()
	src := readFile(t, "../../internal/agent/tools/classify.go")
	names := map[string]bool{}
	for _, line := range strings.Split(src, "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "case ") {
			continue
		}
		for _, m := range quoted.FindAllStringSubmatch(trim, -1) {
			names[m[1]] = true
		}
	}
	return names
}

func TestEveryNameInTheClassificationTablesIsARegisteredTool(t *testing.T) {
	reg := registered(t)
	// Words that appear in `case` lines for other reasons than naming a tool.
	notTools := map[string]bool{"mcp": true}
	var stale []string
	for name := range tableNames(t) {
		if !reg[name] && !notTools[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("classify.go names tools nothing registers (aliases are not kept; the server is the source of every name): %v", stale)
	}
}

func TestEveryRegisteredToolHasALabelAndAnIcon(t *testing.T) {
	var noLabel, noIcon []string
	for name := range registered(t) {
		if tools.ToolLabel(name) == name {
			noLabel = append(noLabel, name)
		}
		if tools.ToolIconKey(name) == "wrench" {
			noIcon = append(noIcon, name)
		}
	}
	sort.Strings(noLabel)
	sort.Strings(noIcon)
	// `subagent`, `ask` and friends have labels; a wrench is only acceptable for
	// tools that really have no better glyph, which is listed explicitly.
	allowedWrench := map[string]bool{
		"subagent": true, "ask": true, "askMany": true, "readSkill": true, "computer_reset": false,
	}
	var badIcon []string
	for _, n := range noIcon {
		if !allowedWrench[n] {
			badIcon = append(badIcon, n)
		}
	}
	if len(noLabel) > 0 {
		t.Errorf("registered tools with no label (clients would show the raw name): %v", noLabel)
	}
	if len(badIcon) > 0 {
		t.Errorf("registered tools with only the generic wrench icon: %v", badIcon)
	}
}

// What clients rely on: the file tools are recognised by their real names, and
// an alias that used to be accepted is now a plain unknown name.
func TestRegisteredFileToolsAreClassifiedAndAliasesAreNot(t *testing.T) {
	if !tools.IsReadFile("read_file") || !tools.IsEditFile("editFile") || !tools.IsWriteFile("write_file") || !tools.IsWriteFile("batchWrite") {
		t.Fatal("the registered file tools must be classified")
	}
	for _, alias := range []string{"readFile", "view", "Read", "edit_file", "str_replace", "writeFile", "batch_write"} {
		if tools.IsReadFile(alias) || tools.IsEditFile(alias) || tools.IsWriteFile(alias) || tools.IsFileTargetTool(alias) {
			t.Errorf("%q is not a registered tool and must not be classified", alias)
		}
		if tools.ToolLabel(alias) != alias || tools.ToolIconKey(alias) != "wrench" || tools.ToolKindFlags(alias) != 0 {
			t.Errorf("%q must fall back to its raw name, a wrench and no flags", alias)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
