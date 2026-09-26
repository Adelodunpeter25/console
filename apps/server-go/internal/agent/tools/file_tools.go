// Initial file tools: read_file, write_file, list_dir, glob, grep.
// Each tool is a struct + run function; schemas come from the struct tags.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/fff"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// fffManager is set once at startup (see SetFffManager) so glob/grep can use
// fff's native, frecency-ranked search instead of walking the filesystem.
var fffManager *fff.Manager

// SetFffManager wires the shared fff manager into the glob/grep tools.
func SetFffManager(m *fff.Manager) { fffManager = m }

type readFileInput struct {
	Path      string `json:"path" jsonschema:"required,description=Absolute path to the file to read"`
	StartLine int    `json:"startLine,omitempty" jsonschema:"description=1-based start line (inclusive)"`
	EndLine   int    `json:"endLine,omitempty" jsonschema:"description=1-based end line (inclusive)"`
}

// readMaxBytes/readMaxLines mirror the TS readFileTool's MAX_BYTES/MAX_LINES
// safety ceilings (engine.ts). Go reads the capped bytes in one shot rather
// than streaming, so only the *content* ceiling matters here.
const (
	readMaxBytes = 512 * 1024
	readMaxLines = 2000
	// readDefaultLines caps a read with no startLine/endLine, so a whole
	// file isn't pulled into history when only part of it is needed.
	readDefaultLines = 300
)

// sparseLineNumbers reports whether read_file should number only the first
// line, every 10th line and the last line (CONSOLE_HARNESS_SPARSE_LINE_NUMBERS=1).
func sparseLineNumbers() bool {
	return os.Getenv("CONSOLE_HARNESS_SPARSE_LINE_NUMBERS") == "1"
}

// textResult wraps a formatted string as the MCP-style content array the TS
// server always sends over the wire (tool-output.ts normalizeToolOutput
// strips everything except this array + isError before it reaches the
// client) — the desktop UI parses this exact shape, not raw Go structs.
func textResult(text string) []map[string]any {
	return []map[string]any{{"type": "text", "text": text}}
}

var ReadFile = NewTool("read_file", "Read a file on disk. Without startLine/endLine, returns the first 300 lines; pass a range to read further.", TierRead,
	func(ctx context.Context, in readFileInput) (any, error) {
		if in.Path == "" {
			return nil, NewToolError("path is required")
		}
		info, err := os.Stat(in.Path)
		if err != nil {
			return nil, NewToolError("Cannot read %s: %v", in.Path, err)
		}
		if info.IsDir() {
			return textResult(fmt.Sprintf("%q is a directory. Use list_dir to browse directories.", in.Path)), nil
		}
		sizeBytes := info.Size()
		data, err := os.ReadFile(in.Path)
		if err != nil {
			return nil, NewToolError("Cannot read %s: %v", in.Path, err)
		}
		byteCapped := false
		if len(data) > readMaxBytes {
			data = data[:readMaxBytes]
			byteCapped = true
		}
		if len(data) == 0 {
			return textResult(fmt.Sprintf("File is empty: %s", in.Path)), nil
		}

		allLines := strings.Split(string(data), "\n")
		totalLines := len(allLines)
		start := in.StartLine
		if start < 1 {
			start = 1
		}
		if start > totalLines {
			return textResult(fmt.Sprintf("Offset beyond EOF. The file has %d lines. Try a smaller startLine.", totalLines)), nil
		}
		naturalEnd := totalLines
		if in.EndLine > 0 && in.EndLine < naturalEnd {
			naturalEnd = in.EndLine
		}
		if start > naturalEnd {
			return textResult(fmt.Sprintf("startLine (%d) must be less than or equal to endLine (%d).", start, naturalEnd)), nil
		}
		end := naturalEnd
		maxLines := readMaxLines
		if in.StartLine == 0 && in.EndLine == 0 {
			maxLines = readDefaultLines
		}
		if emitEnd := start + maxLines - 1; end > emitEnd {
			end = emitEnd
		}
		truncated := byteCapped || end < naturalEnd

		lines := allLines[start-1 : end]
		eofReached := !truncated && end == totalLines
		rangeDescription := fmt.Sprintf("lines %d–%d", start, end)
		if eofReached {
			if start == 1 {
				rangeDescription = fmt.Sprintf("all %d lines", totalLines)
			} else {
				rangeDescription = fmt.Sprintf("lines %d–%d of %d", start, end, totalLines)
			}
		}

		showing := "Showing: " + rangeDescription
		if truncated {
			showing += " (truncated)"
		}
		header := []string{
			"File: " + in.Path,
			showing,
			fmt.Sprintf("Size: %d bytes", sizeBytes),
		}
		if truncated {
			header = append(header, fmt.Sprintf("Output truncated at line %d of %d. Continue with startLine=%d.", end, totalLines, end+1))
		}
		header = append(header, "")

		width := len(fmt.Sprintf("%d", end))
		sparse := sparseLineNumbers()
		pad := strings.Repeat(" ", width+2)
		numbered := make([]string, len(lines))
		for i, line := range lines {
			n := start + i
			if sparse && n != start && n != end && n%10 != 0 {
				numbered[i] = pad + line
				continue
			}
			numbered[i] = fmt.Sprintf("%*d: %s", width, n, line)
		}

		return textResult(strings.Join(header, "\n") + strings.Join(numbered, "\n")), nil
	})

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
		lineCount := strings.Count(in.Content, "\n") + 1
		return textResult(fmt.Sprintf("Written: %s\n  Bytes: %d\n  Lines: %d", in.Path, len(in.Content), lineCount)), nil
	})

type listDirInput struct {
	Path       string `json:"path" jsonschema:"required,description=Directory to list"`
	ShowHidden bool   `json:"showHidden,omitempty" jsonschema:"description=Include dotfiles"`
}

type dirEntry struct {
	Name  string
	IsDir bool
	Size  int64
}

// formatBytes mirrors packages/types/src/fs.ts formatBytes.
func formatBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.0f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

// renderDirTree mirrors list-dir.ts's renderTree: box-drawing connectors,
// "N/" for directories, "  [size]" for files.
func renderDirTree(entries []dirEntry) []string {
	lines := make([]string, 0, len(entries))
	for i, e := range entries {
		connector := "├── "
		if i == len(entries)-1 {
			connector = "└── "
		}
		if e.IsDir {
			lines = append(lines, connector+e.Name+"/")
		} else {
			lines = append(lines, fmt.Sprintf("%s%s  [%s]", connector, e.Name, formatBytes(e.Size)))
		}
	}
	return lines
}

var ListDir = NewTool("list_dir", "List files and directories at a path.", TierRead,
	func(ctx context.Context, in listDirInput) (any, error) {
		if in.Path == "" {
			return nil, NewToolError("path is required")
		}
		info, err := os.Stat(in.Path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, NewToolError("Directory not found: %s", in.Path)
			}
			return nil, NewToolError("%v", err)
		}
		if !info.IsDir() {
			return nil, NewToolError("%q is not a directory. Use read_file to read files.", in.Path)
		}
		entries, err := os.ReadDir(in.Path)
		if err != nil {
			return nil, NewToolError("Cannot read directory %q: %v", in.Path, err)
		}
		out := make([]dirEntry, 0, len(entries))
		for _, e := range entries {
			if !in.ShowHidden && strings.HasPrefix(e.Name(), ".") {
				continue
			}
			entry := dirEntry{Name: e.Name(), IsDir: e.IsDir()}
			if !e.IsDir() {
				if fi, err := e.Info(); err == nil {
					entry.Size = fi.Size()
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

		header := fmt.Sprintf("Directory: %s (top-level only)\n", in.Path)
		body := "(empty directory)"
		if len(out) > 0 {
			body = strings.Join(renderDirTree(out), "\n")
		}
		return textResult(header + body), nil
	})

type globInput struct {
	Pattern    string `json:"pattern" jsonschema:"required,description=Glob pattern relative to root, e.g. src/**/*.ts"`
	Root       string `json:"root,omitempty" jsonschema:"description=Directory the pattern is relative to (default: current dir)"`
	MaxResults int    `json:"maxResults,omitempty" jsonschema:"description=Maximum number of results to return (default 200)"`
}

var Glob = NewTool("glob", "Find files matching a glob pattern (e.g. 'src/**/*.ts', '**/*.json').", TierRead,
	func(ctx context.Context, in globInput) (any, error) {
		if in.Pattern == "" {
			return nil, NewToolError("pattern is required")
		}
		root := in.Root
		if root == "" {
			root = "."
		}
		maxResults := in.MaxResults
		if maxResults <= 0 {
			maxResults = 200
		}
		if fffManager != nil && fffManager.Enabled() {
			if resolvedRoot, err := filepath.Abs(root); err == nil {
				if lease, err := fffManager.GetOrCreate(resolvedRoot); err == nil {
					defer lease.Release()
					items, err := lease.Glob(in.Pattern, maxResults)
					if err == nil {
						matches := make([]string, 0, len(items))
						for _, item := range items {
							matches = append(matches, item.RelPath)
						}
						sort.Strings(matches)
						return textResult(formatGlobMatches(matches, in.Pattern, resolvedRoot)), nil
					}
				}
			}
		}
		// Fallback: plain filesystem glob when fff is unavailable or errors.
		pattern := in.Pattern
		if in.Root != "" {
			pattern = filepath.Join(in.Root, in.Pattern)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, NewToolError("Invalid glob pattern %s: %v", in.Pattern, err)
		}
		sort.Strings(matches)
		resolvedRoot, _ := filepath.Abs(root)
		return textResult(formatGlobMatches(matches, in.Pattern, resolvedRoot)), nil
	})

// formatGlobMatches mirrors glob.ts's result text.
func formatGlobMatches(matches []string, pattern, searchRoot string) string {
	if len(matches) == 0 {
		return fmt.Sprintf("No files matched pattern %q in %s", pattern, searchRoot)
	}
	header := fmt.Sprintf("Found %d file(s) matching %q in %s:\n", len(matches), pattern, searchRoot)
	return header + strings.Join(matches, "\n")
}

type grepInput struct {
	Pattern         string `json:"pattern" jsonschema:"required,description=Regular expression to search for"`
	Root            string `json:"root,omitempty" jsonschema:"description=Directory to search recursively (default: current dir)"`
	Include         string `json:"include,omitempty" jsonschema:"description=File name glob filter, e.g. *.go"`
	MaxResults      int    `json:"maxResults,omitempty" jsonschema:"description=Maximum matches to return (default 50)"`
	Mode            string `json:"mode,omitempty" jsonschema:"description=\"regex\" (default)\\, \"plain\"\\, or \"fuzzy\""`
	CaseInsensitive bool   `json:"caseInsensitive,omitempty" jsonschema:"description=Case-insensitive search (legacy; prefer caseMode)"`
	CaseMode        string `json:"caseMode,omitempty" jsonschema:"description=Case mode: smart (default)\\, sensitive\\, or insensitive"`
	WholeWord       bool   `json:"wholeWord,omitempty" jsonschema:"description=Match whole words only (plain and regex modes)"`
	ContextLines    int    `json:"contextLines,omitempty" jsonschema:"description=Lines of context around each match (default 2)"`
}

type grepMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

var Grep = NewTool("grep", "Search file contents by pattern. Use for finding definitions, usages, or references.", TierRead,
	func(ctx context.Context, in grepInput) (any, error) {
		if in.Pattern == "" {
			return nil, NewToolError("pattern is required")
		}
		root := in.Root
		if root == "" {
			root = "."
		}
		max := in.MaxResults
		if max <= 0 {
			max = 50
		}
		if fffManager != nil && fffManager.Enabled() {
			if resolvedRoot, err := filepath.Abs(root); err == nil {
				if lease, err := fffManager.GetOrCreate(resolvedRoot); err == nil {
					defer lease.Release()
					mode := fff.GrepModeRegex
					switch in.Mode {
					case "plain":
						mode = fff.GrepModePlain
					case "fuzzy":
						mode = fff.GrepModeFuzzy
					}
					caseMode, cmErr := fff.ParseCaseMode(in.CaseMode)
					if cmErr != nil {
						return nil, NewToolError("%v", cmErr)
					}
					if in.CaseInsensitive && caseMode == fff.CaseSmart {
						caseMode = fff.CaseInsensitive
					}
					contextLines := in.ContextLines
					if contextLines <= 0 {
						contextLines = 2
					}
					result, err := lease.GrepWithOptions(in.Pattern, fff.GrepOptions{
						Mode:         mode,
						Case:         caseMode,
						WholeWord:    in.WholeWord,
						ContextLines: contextLines,
						MaxMatches:   max,
					})
					if err == nil {
						matches := make([]grepMatch, 0, len(result.Matches))
						for _, m := range result.Matches {
							matches = append(matches, grepMatch{
								Path: m.RelPath, Line: int(m.LineNumber), Text: m.LineContent,
							})
						}
						text := formatGrepMatches(matches, result.FilesSearched, max, in.Pattern, root)
						if result.RegexError != nil {
							text += fmt.Sprintf("\nNote: regex was invalid (%s); results are from a literal fallback.", result.RegexError.Message)
						}
						return textResult(text), nil
					}
				}
			}
		}
		// Fallback: plain filesystem walk with Go regexp when fff is
		// unavailable or errors. Only regex mode is meaningful here.
		re, err := regexp.Compile(in.Pattern)
		if err != nil {
			return nil, NewToolError("Invalid regex %s: %v", in.Pattern, err)
		}
		var includeRe *regexp.Regexp
		if in.Include != "" {
			if includeRe, err = regexp.Compile(globToRegex(in.Include)); err != nil {
				return nil, NewToolError("Invalid include filter %s: %v", in.Include, err)
			}
		}
		matches := make([]grepMatch, 0)
		filesSearched := 0
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if len(matches) >= max {
				return filepath.SkipAll
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") || utils.IsPathIgnored(d.Name()) {
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
				filesSearched++
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
		return textResult(formatGrepMatches(matches, filesSearched, max, in.Pattern, root)), nil
	})

// formatGrepMatches mirrors the TS grepTool's result text: a header line,
// then matches grouped by file under "── path ──" with "→ line: text" rows.
func formatGrepMatches(matches []grepMatch, filesSearched, max int, pattern, searchPath string) string {
	if len(matches) == 0 {
		return fmt.Sprintf("No matches found for %q in %s\n(searched %d files)", pattern, searchPath, filesSearched)
	}
	truncated := len(matches) >= max
	lines := make([]string, 0, len(matches)+8)
	header := fmt.Sprintf("Found %d match(es) across files", len(matches))
	if truncated {
		header += fmt.Sprintf(" (showing first %d)", max)
	}
	header += fmt.Sprintf("  [searched %d files]\n", filesSearched)
	lines = append(lines, header)

	currentFile := ""
	for _, m := range matches {
		if m.Path != currentFile {
			if currentFile != "" {
				lines = append(lines, "")
			}
			lines = append(lines, fmt.Sprintf("── %s ──", m.Path))
			currentFile = m.Path
		}
		lines = append(lines, fmt.Sprintf("→ %4d: %s", m.Line, m.Text))
	}
	return strings.Join(lines, "\n")
}

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
	return []Tool{ReadFile, WriteFile, ListDir, Glob, Grep, EditFile, BatchWrite, ReadSkill, Fetch, WebSearch, Ask, AskMany, Todo, Bash, BashJob, Subagent, Ports, ProjectScripts, Browser}
}
