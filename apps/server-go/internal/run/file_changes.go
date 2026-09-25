// File change tracking: diff generation and recording for session
// file changes. Mirrors run-file-changes.ts from the TS server.
package run

import (
	"os"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/sergi/go-diff/diffmatchpatch"
)

const maxDiffSize = 1_000_000 // 1MB limit for diff storage

// ExtractAndRecordFileChange generates unified diffs for file operations
// and records them in the session database. Mirrors the TS version.
func ExtractAndRecordFileChange(
	sessions *services.SessionService,
	sessionID string,
	toolName string,
	args map[string]any,
	isError bool,
	turnIndex int,
) error {
	if isError || toolName == "" || args == nil {
		return nil
	}

	switch toolName {
	case "writeFile", "write_file":
		path, ok := args["path"].(string)
		if !ok {
			return nil
		}
		content, _ := args["content"].(string)

		// If the file already existed, read its old content so we can produce
		// a real before/after diff and mark the status as "modified".
		var oldContent string
		var status string
		if existing, err := os.ReadFile(path); err == nil {
			oldContent = string(existing)
			status = "modified"
		} else {
			status = "added"
		}

		additions := strings.Count(content, "\n") + 1
		deletions := strings.Count(oldContent, "\n")
		if oldContent == "" {
			deletions = 0
		}

		var diffText *string
		patch := createPatch(path, oldContent, content)
		if len(patch) <= maxDiffSize {
			diffText = &patch
		}

		return sessions.RecordFileChange(sessionID, types.SessionFileChange{
			Path:      path,
			TurnIndex: turnIndex,
			Status:    status,
			Additions: additions,
			Deletions: deletions,
			DiffText:  diffText,
			UpdatedAt: 0, // Set by RecordFileChange
		})

	case "editFile", "edit_file", "replace_file_content":
		// editFileInput uses "path", "oldContent", "newContent".
		// Older/alternate tool schemas may use "TargetFile", "TargetContent",
		// "ReplacementContent" — keep those as fallbacks.
		var targetPath string
		if p, ok := args["path"].(string); ok {
			targetPath = p
		} else if p, ok := args["TargetFile"].(string); ok {
			targetPath = p
		} else {
			return nil
		}

		var targetContent, replacementContent string
		if tc, ok := args["oldContent"].(string); ok {
			targetContent = tc
		} else if tc, ok := args["TargetContent"].(string); ok {
			targetContent = tc
		}
		if rc, ok := args["newContent"].(string); ok {
			replacementContent = rc
		} else if rc, ok := args["ReplacementContent"].(string); ok {
			replacementContent = rc
		}

		adds := strings.Count(replacementContent, "\n") + 1
		dels := strings.Count(targetContent, "\n") + 1

		// Generate unified diff
		var diffText *string
		patch := createPatch(targetPath, targetContent, replacementContent)
		if len(patch) <= maxDiffSize {
			diffText = &patch
		}

		return sessions.RecordFileChange(sessionID, types.SessionFileChange{
			Path:      targetPath,
			TurnIndex: turnIndex,
			Status:    "modified",
			Additions: adds,
			Deletions: dels,
			DiffText:  diffText,
			UpdatedAt: 0,
		})

	case "batchWrite", "batch_write":
		files, ok := args["files"].([]any)
		if !ok {
			return nil
		}
		for _, file := range files {
			fileMap, ok := file.(map[string]any)
			if !ok {
				continue
			}
			path, ok := fileMap["path"].(string)
			if !ok {
				continue
			}
			content, _ := fileMap["content"].(string)
			lineCount := strings.Count(content, "\n") + 1

			// Generate diff for new file
			var diffText *string
			patch := createPatch(path, "", content)
			if len(patch) <= maxDiffSize {
				diffText = &patch
			}

			if err := sessions.RecordFileChange(sessionID, types.SessionFileChange{
				Path:      path,
				TurnIndex: turnIndex,
				Status:    "added",
				Additions: lineCount,
				Deletions: 0,
				DiffText:  diffText,
				UpdatedAt: 0,
			}); err != nil {
				return err
			}
		}
		return nil
	}

	return nil
}

// createPatch generates a unified diff between two strings using go-diff.
// Mirrors createPatch from the 'diff' npm package.
func createPatch(filename, oldText, newText string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(oldText, newText, true)
	
	// Build unified diff format
	var builder strings.Builder
	builder.WriteString("--- a/")
	builder.WriteString(filename)
	builder.WriteString("\n")
	builder.WriteString("+++ b/")
	builder.WriteString(filename)
	builder.WriteString("\n")
	
	for _, diff := range diffs {
		var prefix string
		switch diff.Type {
		case diffmatchpatch.DiffDelete:
			prefix = "-"
		case diffmatchpatch.DiffInsert:
			prefix = "+"
		case diffmatchpatch.DiffEqual:
			prefix = " "
		}
		
		lines := strings.Split(diff.Text, "\n")
		for _, line := range lines {
			builder.WriteString(prefix)
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	
	return builder.String()
}
