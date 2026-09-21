// editFile and batchWrite tools. Ports of
// apps/server/agent/src/tools/edit-file.ts and batch-write.ts.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
			preview := in.OldContent
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			return nil, NewToolError("The oldContent was not found in %q.\nLooked for:\n%s\n\nUse read_file to verify the current file content before editing.", in.Path, preview)
		}
		if occurrences > 1 {
			return nil, NewToolError("The oldContent appears %d times in %q. Include more surrounding context in oldContent to make it unique.", occurrences, in.Path)
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

// formatBatchWriteResults mirrors batch-write.ts's formatResults.
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
