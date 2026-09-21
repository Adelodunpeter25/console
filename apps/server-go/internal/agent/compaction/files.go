// Cumulative file-operation tracking for summaries. Port of
// apps/server/agent/src/compaction/file-tracker.ts.
package compaction

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
)

// FileOps accumulates touched paths across discarded turns.
type FileOps struct {
	Read    map[string]bool
	Written map[string]bool
	Edited  map[string]bool
}

var (
	readSelector = regexp.MustCompile(`^(?:L?\d+(?:(?:[-+]|\.\.)L?\d+|-|\.\.)?|raw|conflicts)$`)
	urlScheme    = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)
)

// StripReadSelector removes `:1-50`-style line selectors from paths.
func StripReadSelector(path string) string {
	colon := strings.LastIndex(path, ":")
	if colon <= 0 {
		return path
	}
	if readSelector.MatchString(path[colon+1:]) {
		return path[:colon]
	}
	return path
}

// IsURLScheme reports http://-style paths (never tracked as files).
func IsURLScheme(path string) bool {
	return urlScheme.MatchString(path)
}

// ExtractFileOps collects read/written/edited paths from assistant turns.
func ExtractFileOps(history []any) FileOps {
	ops := FileOps{Read: map[string]bool{}, Written: map[string]bool{}, Edited: map[string]bool{}}
	for _, m := range history {
		content := assistantContent(m)
		for _, part := range content {
			call, ok := part.(loop.ToolCallPart)
			if !ok {
				continue
			}
			rawPath := toolArgPath(call.Call.Arguments)
			if rawPath == "" || IsURLScheme(rawPath) {
				continue
			}
			clean := StripReadSelector(strings.TrimSpace(rawPath))
			if clean == "" {
				continue
			}
			switch name := strings.ToLower(call.Call.Name); {
			case strings.Contains(name, "read"):
				ops.Read[clean] = true
			case strings.Contains(name, "write"):
				ops.Written[clean] = true
			case strings.Contains(name, "edit") || strings.Contains(name, "replace") || strings.Contains(name, "patch"):
				ops.Edited[clean] = true
			}
		}
	}
	return ops
}

// FormatFileTree renders cumulative ops as a directory-grouped tree with
// Read/Write/RW markers (max 25 files).
func FormatFileTree(ops FileOps) string {
	mode := map[string]string{}
	for file := range ops.Read {
		mode[file] = "Read"
	}
	modified := append(append([]string{}, keysOf(ops.Written)...), keysOf(ops.Edited)...)
	for _, file := range modified {
		if mode[file] == "Read" {
			mode[file] = "RW"
		} else {
			mode[file] = "Write"
		}
	}
	if len(mode) == 0 {
		return ""
	}
	allFiles := keysOfMode(mode)
	sort.Strings(allFiles)
	display := allFiles
	if len(display) > 25 {
		display = display[:25]
	}
	groups := map[string][]string{}
	var order []string
	for _, file := range display {
		dir, name := splitDir(file)
		if _, ok := groups[dir]; !ok {
			order = append(order, dir)
		}
		groups[dir] = append(groups[dir], name+" ("+mode[file]+")")
	}
	var lines []string
	for _, dir := range order {
		if dir != "" {
			lines = append(lines, "# "+dir)
		}
		lines = append(lines, groups[dir]...)
	}
	if len(allFiles) > 25 {
		lines = append(lines, "[…"+strconv.Itoa(len(allFiles)-25)+" files elided…]")
	}
	return "<files>\n" + strings.Join(lines, "\n") + "\n</files>"
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keysOfMode(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func splitDir(file string) (dir, name string) {
	if i := strings.LastIndex(file, "/"); i > 0 {
		return file[:i+1], file[i+1:]
	}
	return "", file
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func assistantContent(m any) []any {
	switch msg := m.(type) {
	case loop.AssistantMessage:
		return msg.Content
	case *loop.AssistantMessage:
		if msg != nil {
			return msg.Content
		}
	}
	return nil
}

func toolArgPath(args json.RawMessage) string {
	var obj map[string]any
	if err := json.Unmarshal(args, &obj); err != nil {
		return ""
	}
	for _, key := range []string{"path", "filePath", "targetFile", "TargetFile", "absolutePath", "AbsolutePath"} {
		if s, ok := obj[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
