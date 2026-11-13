// editFile and batchWrite tools.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type editFileInput struct {
	Path       string `json:"path" jsonschema:"required,description=Absolute path to the file to edit"`
	OldContent string `json:"oldContent" jsonschema:"required,description=The exact string to find in the file (including whitespace and newlines). Must match exactly once."`
	NewContent string `json:"newContent" jsonschema:"required,description=The replacement string to insert in place of oldContent"`
}

// EditFile replaces one exact occurrence of oldContent with newContent.
// Errors (not found / ambiguous) surface as tool errors so the model can
// re-read the file and retry with a unique anchor.
var EditFile = NewTool("editFile", "Edit a file by replacing an exact string. Read the file first to get the exact match. For complete rewrites, use write_file.", TierWrite,
	func(ctx context.Context, in editFileInput) (any, error) {
		if in.Path == "" {
			return nil, NewToolError("path is required")
		}
		data, err := os.ReadFile(in.Path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, NewToolError("File not found: %s", in.Path)
			}
			return nil, NewToolError("Cannot read %s: %v", in.Path, err)
		}
		original := string(data)

		occurrences := strings.Count(original, in.OldContent)
		if occurrences == 0 {
			return nil, NewToolError("%s", editNotFoundMessage(in.Path, original, in.OldContent))
		}
		if occurrences > 1 {
			msg := fmt.Sprintf("The oldContent appears %d times in %q. Include more surrounding context in oldContent to make it unique.", occurrences, in.Path)
			if lines := lineNumbersOf(original, in.OldContent); len(lines) > 0 {
				msg += fmt.Sprintf("\nOccurrences at lines: %s.", joinLineNumbers(lines, 8))
			}
			return nil, NewToolError("%s", msg)
		}

		updated := strings.Replace(original, in.OldContent, in.NewContent, 1)
		if err := os.WriteFile(in.Path, []byte(updated), 0o644); err != nil {
			return nil, NewToolError("Cannot write %s: %v", in.Path, err)
		}

		oldLines := strings.Count(in.OldContent, "\n") + 1
		newLines := strings.Count(in.NewContent, "\n") + 1
		lineDelta := newLines - oldLines
		deltaStr := "no line count change"
		if lineDelta > 0 {
			deltaStr = fmt.Sprintf("+%d lines", lineDelta)
		} else if lineDelta < 0 {
			deltaStr = fmt.Sprintf("%d lines", lineDelta)
		}
		return textResult(fmt.Sprintf("Edited: %s\n  Replaced %d line(s) with %d line(s) (%s)", in.Path, oldLines, newLines, deltaStr)), nil
	})

// editNotFoundMessage builds the not-found error with recovery hints: a
// whitespace-insensitive match check (the most common real cause) and the
// closest matching lines in the file.
func editNotFoundMessage(path, original, oldContent string) string {
	preview := oldContent
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}
	msg := fmt.Sprintf("The oldContent was not found in %q.\nLooked for:\n%s", path, preview)
	if hint := editNotFoundHint(original, oldContent); hint != "" {
		msg += "\n\n" + hint
	}
	return msg + "\n\nUse read_file to verify the current file content before editing."
}

// editNotFoundHint explains near-misses. Empty when the text is genuinely
// absent.
func editNotFoundHint(original, oldContent string) string {
	if strings.Contains(strings.ReplaceAll(original, "\r\n", "\n"), oldContent) {
		return "The text exists but the file uses CRLF line endings — match the file's line endings."
	}
	if idx := strings.Index(normalizeWhitespace(original), normalizeWhitespace(oldContent)); idx >= 0 {
		return fmt.Sprintf("A whitespace-insensitive match exists near line %d — indentation, trailing whitespace or line endings differ.",
			1+strings.Count(normalizeWhitespace(original)[:idx], "\n"))
	}
	if candidates := closestLines(original, oldContent, 3); len(candidates) > 0 {
		var b strings.Builder
		b.WriteString("Closest matching lines:")
		for _, c := range candidates {
			line := c.text
			if len(line) > 120 {
				line = line[:120] + "..."
			}
			fmt.Fprintf(&b, "\n  %d: %s", c.number, strings.TrimSpace(line))
		}
		return b.String()
	}
	return ""
}

// normalizeWhitespace collapses runs of spaces/tabs and trims each line, so
// an edit that only differs in indentation still finds its target.
func normalizeWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.Join(lines, "\n")
}

type lineCandidate struct {
	number int
	text   string
	score  float64
}

// closestLines ranks file lines by similarity to the needle's first
// meaningful line, so a failed edit points at where the text probably lives.
func closestLines(original, oldContent string, want int) []lineCandidate {
	needle := ""
	for _, line := range strings.Split(oldContent, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			needle = t
			break
		}
	}
	if needle == "" {
		return nil
	}
	if len(needle) > 160 {
		needle = needle[:160]
	}
	var out []lineCandidate
	for i, line := range strings.Split(original, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		score := similarity(t, needle)
		if score < 0.6 {
			continue
		}
		out = append(out, lineCandidate{number: i + 1, text: line, score: score})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].score > out[j].score })
	if len(out) > want {
		out = out[:want]
	}
	return out
}

// similarity is 1 - Levenshtein distance over the longer string, with an
// early length filter to keep the scan cheap on big files.
func similarity(a, b string) float64 {
	if len(a) > len(b) {
		a, b = b, a
	}
	if len(b) > 0 && float64(len(b)-len(a))/float64(len(b)) > 0.5 {
		return 0
	}
	dist := levenshtein(a, b)
	if len(b) == 0 {
		return 0
	}
	return 1 - float64(dist)/float64(len(b))
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

// lineNumbersOf maps every occurrence of needle to its 1-based line number.
func lineNumbersOf(original, needle string) []int {
	var lines []int
	for idx := 0; len(needle) > 0 && idx <= len(original)-len(needle); {
		found := strings.Index(original[idx:], needle)
		if found < 0 {
			break
		}
		lines = append(lines, 1+strings.Count(original[:idx+found], "\n"))
		idx += found + len(needle)
	}
	return lines
}

// joinLineNumbers renders at most max line numbers, then "…".
func joinLineNumbers(lines []int, max int) string {
	parts := make([]string, 0, min(len(lines), max))
	for i, n := range lines {
		if i == max {
			parts = append(parts, fmt.Sprintf("… (%d more)", len(lines)-max))
			break
		}
		parts = append(parts, fmt.Sprintf("%d", n))
	}
	return strings.Join(parts, ", ")
}

type batchWriteFile struct {
	Path    string `json:"path" jsonschema:"required,description=File path to write"`
	Content string `json:"content" jsonschema:"required,description=Full content of the file"`
}

type batchWriteInput struct {
	Files       []batchWriteFile `json:"files" jsonschema:"required,description=List of files to write. All writes happen concurrently unless stopOnError is set."`
	StopOnError bool             `json:"stopOnError,omitempty" jsonschema:"description=Stop after the first failure instead of attempting every write (default false)"`
}

type batchWriteResult struct {
	Path   string
	Status string // "written" | "failed"
	Bytes  int
	Lines  int
	Error  string
}

func writeOneFile(path, content string) batchWriteResult {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return batchWriteResult{Path: path, Status: "failed", Error: err.Error()}
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return batchWriteResult{Path: path, Status: "failed", Error: err.Error()}
	}
	return batchWriteResult{Path: path, Status: "written", Bytes: len(content), Lines: strings.Count(content, "\n") + 1}
}

// formatBatchWriteResults renders the per-file batchWrite results as text.
func formatBatchWriteResults(results []batchWriteResult) string {
	var lines []string
	written := 0
	for _, r := range results {
		if r.Status == "written" {
			written++
		}
	}
	lines = append(lines, fmt.Sprintf("Summary: %d/%d files written successfully.", written, len(results)), "")

	if written > 0 {
		lines = append(lines, "✓ Written:")
		for _, r := range results {
			if r.Status == "written" {
				lines = append(lines, fmt.Sprintf("  %s  [%dB, %d lines]", r.Path, r.Bytes, r.Lines))
			}
		}
	}
	failed := len(results) - written
	if failed > 0 {
		lines = append(lines, "", "✗ Failed:")
		for _, r := range results {
			if r.Status != "written" {
				lines = append(lines, fmt.Sprintf("  %s", r.Path), fmt.Sprintf("    Error: %s", r.Error))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// BatchWrite writes multiple files in one call, useful for scaffolding or
// multi-file creation. Duplicate paths are rejected up front.
var BatchWrite = NewTool("batchWrite", "Write multiple files at once. Useful for project scaffolding or multi-file creation.", TierWrite,
	func(ctx context.Context, in batchWriteInput) (any, error) {
		if len(in.Files) == 0 {
			return nil, NewToolError("files must contain at least one entry")
		}
		seen := map[string]bool{}
		for _, f := range in.Files {
			if seen[f.Path] {
				return nil, NewToolError("Duplicate path detected in batchWrite: %s — each path must be unique.", f.Path)
			}
			seen[f.Path] = true
		}

		results := make([]batchWriteResult, 0, len(in.Files))
		anyFailed := false
		if in.StopOnError {
			// Sequential — stop at the first failure, mark the rest skipped.
			for _, f := range in.Files {
				result := writeOneFile(f.Path, f.Content)
				results = append(results, result)
				if result.Status == "failed" {
					anyFailed = true
					for _, remaining := range in.Files[len(results):] {
						results = append(results, batchWriteResult{Path: remaining.Path, Status: "failed", Error: "Skipped due to stopOnError"})
					}
					break
				}
			}
		} else {
			// Concurrent — attempt every write regardless of earlier failures.
			results = make([]batchWriteResult, len(in.Files))
			var wg sync.WaitGroup
			for i, f := range in.Files {
				wg.Add(1)
				go func(i int, f batchWriteFile) {
					defer wg.Done()
					results[i] = writeOneFile(f.Path, f.Content)
				}(i, f)
			}
			wg.Wait()
			for _, r := range results {
				if r.Status == "failed" {
					anyFailed = true
				}
			}
		}

		return Envelope{Content: textResult(formatBatchWriteResults(results)), IsError: anyFailed}, nil
	})
