// Initial file tools: read_file, write_file, list_dir, glob, grep.
// Each tool is a struct + run function; schemas come from the struct tags.
package tools

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type readFileInput struct {
	Path      string `json:"path" jsonschema:"required,description=Absolute path to the file to read"`
	StartLine int    `json:"startLine,omitempty" jsonschema:"description=1-based start line (inclusive)"`
	EndLine   int    `json:"endLine,omitempty" jsonschema:"description=1-based end line (inclusive)"`
}

type readFileOutput struct {
	Content   string `json:"content"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated,omitempty"`
}

const (
	readMaxBytes  = 512 * 1024
	readMaxOutput = 256 * 1024
)

var ReadFile = NewTool("read_file", "Read the content of a file on disk. Use startLine/endLine for large files.", TierRead,
	func(ctx context.Context, in readFileInput) (any, error) {
		if in.Path == "" {
			return nil, NewToolError("path is required")
		}
		data, err := os.ReadFile(in.Path)
		if err != nil {
			return nil, NewToolError("Cannot read %s: %v", in.Path, err)
		}
		if len(data) > readMaxBytes {
			data = data[:readMaxBytes]
		}
		content := string(data)
		if in.StartLine > 0 || in.EndLine > 0 {
			content = sliceLines(content, in.StartLine, in.EndLine)
		}
		out := readFileOutput{Content: content, Bytes: len(data)}
		if len(out.Content) > readMaxOutput {
			out.Content = out.Content[:readMaxOutput]
			out.Truncated = true
		}
		return out, nil
	})

func sliceLines(text string, startLine, endLine int) string {
	lines := strings.Split(text, "\n")
	count := len(lines)
	start0 := startLine
	if start0 < 1 {
		start0 = 1
	}
	endExcl := endLine
	if endExcl <= 0 || endExcl > count {
		endExcl = count
	}
	if start0-1 >= count || endExcl <= start0-1 {
		return ""
	}
	return strings.Join(lines[start0-1:endExcl], "\n")
}

type writeFileInput struct {
	Path    string `json:"path" jsonschema:"required,description=Absolute file path to write"`
	Content string `json:"content" jsonschema:"required,description=Full file content to write (overwrites)"`
}

var WriteFile = NewTool("write_file", "Create or overwrite a file with the given content. Parent directories are created automatically.", TierWrite,
	func(ctx context.Context, in writeFileInput) (any, error) {
		if in.Path == "" {
			return nil, NewToolError("path is required")
		}
		if dir := filepath.Dir(in.Path); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, NewToolError("Cannot create parent dir for %s: %v", in.Path, err)
			}
		}
		if err := os.WriteFile(in.Path, []byte(in.Content), 0o644); err != nil {
			return nil, NewToolError("Cannot write %s: %v", in.Path, err)
		}
		return map[string]any{"path": in.Path, "bytes": len(in.Content), "written": true}, nil
	})

type listDirInput struct {
	Path       string `json:"path" jsonschema:"required,description=Directory to list"`
	ShowHidden bool   `json:"showHidden,omitempty" jsonschema:"description=Include dotfiles"`
}

type dirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size,omitempty"`
}

var ListDir = NewTool("list_dir", "List the immediate children of a directory.", TierRead,
	func(ctx context.Context, in listDirInput) (any, error) {
		if in.Path == "" {
			return nil, NewToolError("path is required")
		}
		entries, err := os.ReadDir(in.Path)
		if err != nil {
			return nil, NewToolError("Cannot list %s: %v", in.Path, err)
		}
		out := make([]dirEntry, 0, len(entries))
		for _, e := range entries {
			if !in.ShowHidden && strings.HasPrefix(e.Name(), ".") {
				continue
			}
			entry := dirEntry{Name: e.Name(), IsDir: e.IsDir()}
			if !e.IsDir() {
				if info, err := e.Info(); err == nil {
					entry.Size = info.Size()
				}
			}
			out = append(out, entry)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].IsDir != out[j].IsDir {
				return out[i].IsDir
			}
			return out[i].Name < out[j].Name
		})
		return out, nil
	})

type globInput struct {
	Pattern string `json:"pattern" jsonschema:"required,description=Glob pattern relative to root, e.g. src/**/*.ts"`
	Root    string `json:"root,omitempty" jsonschema:"description=Directory the pattern is relative to (default: current dir)"`
}

var Glob = NewTool("glob", "Find files matching a glob pattern.", TierRead,
	func(ctx context.Context, in globInput) (any, error) {
		if in.Pattern == "" {
			return nil, NewToolError("pattern is required")
		}
		pattern := in.Pattern
		if in.Root != "" {
			pattern = filepath.Join(in.Root, in.Pattern)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, NewToolError("Invalid glob pattern %s: %v", in.Pattern, err)
		}
		sort.Strings(matches)
		return matches, nil
	})

type grepInput struct {
	Pattern    string `json:"pattern" jsonschema:"required,description=Regular expression to search for"`
	Root       string `json:"root,omitempty" jsonschema:"description=Directory to search recursively (default: current dir)"`
	Include    string `json:"include,omitempty" jsonschema:"description=File name glob filter, e.g. *.go"`
	MaxResults int    `json:"maxResults,omitempty" jsonschema:"description=Maximum matches to return (default 50)"`
}

type grepMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

var Grep = NewTool("grep", "Search file contents recursively for a regular expression.", TierRead,
	func(ctx context.Context, in grepInput) (any, error) {
		if in.Pattern == "" {
			return nil, NewToolError("pattern is required")
		}
		re, err := regexp.Compile(in.Pattern)
		if err != nil {
			return nil, NewToolError("Invalid regex %s: %v", in.Pattern, err)
		}
		root := in.Root
		if root == "" {
			root = "."
		}
		max := in.MaxResults
		if max <= 0 {
			max = 50
		}
		var includeRe *regexp.Regexp
		if in.Include != "" {
			if includeRe, err = regexp.Compile(globToRegex(in.Include)); err != nil {
				return nil, NewToolError("Invalid include filter %s: %v", in.Include, err)
			}
		}
		matches := make([]grepMatch, 0)
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if len(matches) >= max {
				return filepath.SkipAll
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" {
					if path != root {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if includeRe != nil && !includeRe.MatchString(d.Name()) {
				return nil
			}
			if d.Type().IsRegular() {
				data, rerr := os.ReadFile(path)
				if rerr != nil || strings.ContainsRune(string(data[:min(len(data), 4096)]), 0) {
					return nil // binary or unreadable
				}
				for i, line := range strings.Split(string(data), "\n") {
					if re.MatchString(line) {
						matches = append(matches, grepMatch{Path: path, Line: i + 1, Text: line})
						if len(matches) >= max {
							return filepath.SkipAll
						}
					}
				}
			}
			return nil
		})
		return matches, nil
	})

// globToRegex converts a simple *.ext glob into a name regex.
func globToRegex(glob string) string {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return b.String()
}

// DefaultTools is the initial Phase 2 toolset.
func DefaultTools() []Tool {
	return []Tool{ReadFile, WriteFile, ListDir, Glob, Grep}
}
