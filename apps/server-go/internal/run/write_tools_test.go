package run

import (
	"reflect"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// writePaths decides which files get a pre-write snapshot, and so whether the
// review tab can show a real "before". It must recognise every registered tool
// that writes files, by its real name.
func TestWritePathsRecognisesEveryRegisteredWriteTool(t *testing.T) {
	for _, c := range []struct {
		name string
		args string
		want []string
	}{
		{"write_file", `{"path":"/p/a.txt","content":"x"}`, []string{"/p/a.txt"}},
		{"editFile", `{"path":"/p/b.txt","oldContent":"a","newContent":"b"}`, []string{"/p/b.txt"}},
		{"batchWrite", `{"files":[{"path":"/p/c.txt","content":"x"},{"path":"/p/d.txt","content":"y"}]}`, []string{"/p/c.txt", "/p/d.txt"}},
	} {
		got := writePaths(tools.ToolCall{Name: c.name, Arguments: []byte(c.args)})
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: writePaths = %v, want %v", c.name, got, c.want)
		}
	}
}

// Anything that does not write files touches nothing, whatever its arguments say.
func TestWritePathsIgnoresToolsThatDoNotWriteFiles(t *testing.T) {
	for _, name := range []string{"read_file", "bash", "grep", "list_dir", "console", "writePlan", "memory", "browser"} {
		if got := writePaths(tools.ToolCall{Name: name, Arguments: []byte(`{"path":"/p/a.txt","content":"x"}`)}); len(got) != 0 {
			t.Errorf("%s: writePaths = %v, want none", name, got)
		}
	}
	if got := writePaths(tools.ToolCall{Name: "write_file"}); len(got) != 0 {
		t.Errorf("empty arguments must yield no paths: %v", got)
	}
}

// The tool names writePaths and the file-change recorder accept must be exactly
// the registered tools that write files: one drifting from the other means a
// write the review tab never sees.
func TestEveryRegisteredFileWriteToolIsTracked(t *testing.T) {
	for _, tool := range []tools.Tool{tools.WriteFile, tools.EditFile, tools.BatchWrite} {
		args := `{"path":"/p/x","files":[{"path":"/p/x","content":"c"}]}`
		if got := writePaths(tools.ToolCall{Name: tool.Name(), Arguments: []byte(args)}); len(got) == 0 {
			t.Errorf("%s writes files but writePaths ignores it", tool.Name())
		}
		if !(tools.IsWriteFile(tool.Name()) || tools.IsEditFile(tool.Name())) {
			t.Errorf("%s writes files but classify.go does not say so", tool.Name())
		}
	}
}
