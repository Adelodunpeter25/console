// File change tracking: diff generation and recording for session
// file changes. Mirrors run-file-changes.ts from the TS server.
package run

import (
	"fmt"
	"os"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const maxDiffSize = 1_000_000 // 1MB limit for diff storage
const contextLines = 3        // lines of context around each hunk (matches git default)

// ExtractAndRecordFileChange generates unified diffs for file operations
// and records them in the session database.
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

		// If the file already existed on disk, produce a real before/after diff
		// and mark it modified. For brand new files, diff against empty.
		var oldContent string
		var status string
		if existing, err := os.ReadFile(path); err == nil {
			oldContent = string(existing)
			status = "modified"
		} else {
			status = "added"
		}

		patch := createUnifiedDiff(path, oldContent, content)
		adds, dels := countDiffLines(patch)

		var diffText *string
		if len(patch) <= maxDiffSize {
			diffText = &patch
		}

		return sessions.RecordFileChange(sessionID, types.SessionFileChange{
			Path:      path,
			TurnIndex: turnIndex,
			Status:    status,
			Additions: adds,
			Deletions: dels,
			DiffText:  diffText,
			UpdatedAt: 0,
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

		var oldContent, newContent string
		if tc, ok := args["oldContent"].(string); ok {
			oldContent = tc
		} else if tc, ok := args["TargetContent"].(string); ok {
			oldContent = tc
		}
		if rc, ok := args["newContent"].(string); ok {
			newContent = rc
		} else if rc, ok := args["ReplacementContent"].(string); ok {
			newContent = rc
		}

		// For editFile the snippet is the changed region — diff it directly.
		patch := createUnifiedDiff(targetPath, oldContent, newContent)
		adds, dels := countDiffLines(patch)

		var diffText *string
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

			patch := createUnifiedDiff(path, "", content)
			adds, _ := countDiffLines(patch)

			var diffText *string
			if len(patch) <= maxDiffSize {
				diffText = &patch
			}

			if err := sessions.RecordFileChange(sessionID, types.SessionFileChange{
				Path:      path,
				TurnIndex: turnIndex,
				Status:    "added",
				Additions: adds,
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

// createUnifiedDiff produces a standard unified diff with @@ hunk headers
// that parse_unified_diff on the desktop side can consume. Uses a simple
// line-level LCS via Myers-style patience approach (good enough for our
// sizes; go-diff was dropped because it produced character-level output
// without proper hunk headers).
func createUnifiedDiff(filename, oldText, newText string) string {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	hunks := computeHunks(oldLines, newLines, contextLines)
	if len(hunks) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- a/%s\n", filename)
	fmt.Fprintf(&b, "+++ b/%s\n", filename)

	for _, h := range hunks {
		oldCount := h.oldEnd - h.oldStart
		newCount := h.newEnd - h.newStart
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n",
			h.oldStart+1, oldCount,
			h.newStart+1, newCount,
		)
		for _, line := range h.lines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// countDiffLines counts +/- lines in a unified diff string.
func countDiffLines(diff string) (adds int, dels int) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			adds++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			dels++
		}
	}
	return
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// Remove trailing empty element from a trailing newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type hunk struct {
	oldStart, oldEnd int
	newStart, newEnd int
	lines            []string
}

// computeHunks diffs oldLines vs newLines and groups changes into hunks
// with ctx lines of context on each side.
func computeHunks(oldLines, newLines []string, ctx int) []hunk {
	edits := lcsEdits(oldLines, newLines)

	// Group into hunks.
	var hunks []hunk
	i := 0
	for i < len(edits) {
		// Skip unchanged lines until we hit a change.
		if edits[i].kind == ' ' {
			i++
			continue
		}
		// Found a change — collect context before it.
		start := i
		for start > 0 && edits[start-1].kind == ' ' && i-start < ctx {
			start--
		}
		// Walk forward collecting changes + trailing context.
		end := i
		for end < len(edits) {
			if edits[end].kind != ' ' {
				end++
				// Collect up to ctx context lines after this change.
				trail := 0
				for end < len(edits) && edits[end].kind == ' ' && trail < ctx {
					end++
					trail++
				}
				continue
			}
			// Context line — stop here (already counted above).
			break
		}

		// Build hunk.
		h := hunk{}
		firstOld, firstNew := -1, -1
		lastOld, lastNew := -1, -1
		for _, e := range edits[start:end] {
			if e.old >= 0 {
				if firstOld < 0 {
					firstOld = e.old
				}
				lastOld = e.old
			}
			if e.new >= 0 {
				if firstNew < 0 {
					firstNew = e.new
				}
				lastNew = e.new
			}
			switch e.kind {
			case ' ':
				h.lines = append(h.lines, " "+e.text)
			case '+':
				h.lines = append(h.lines, "+"+e.text)
			case '-':
				h.lines = append(h.lines, "-"+e.text)
			}
		}
		if firstOld < 0 {
			firstOld = 0
		}
		if firstNew < 0 {
			firstNew = 0
		}
		h.oldStart = firstOld
		h.oldEnd = lastOld + 1
		h.newStart = firstNew
		h.newEnd = lastNew + 1
		hunks = append(hunks, h)

		i = end
	}
	return hunks
}

type edit struct {
	kind rune
	old  int
	new  int
	text string
}

// lcsEdits returns a flat edit list (context/add/remove) via DP LCS.
// Capped at 5000 lines each side to avoid O(N²) blowup on huge files.
func lcsEdits(old, new []string) []edit {
	const cap = 5000
	if len(old) > cap {
		old = old[:cap]
	}
	if len(new) > cap {
		new = new[:cap]
	}

	m, n := len(old), len(new)
	// dp[i][j] = LCS length of old[:i] and new[:j]
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if old[i] == new[j] {
				dp[i][j] = 1 + dp[i+1][j+1]
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var edits []edit
	i, j := 0, 0
	for i < m || j < n {
		switch {
		case i < m && j < n && old[i] == new[j]:
			edits = append(edits, edit{' ', i, j, old[i]})
			i++
			j++
		case j < n && (i >= m || dp[i][j+1] >= dp[i+1][j]):
			edits = append(edits, edit{'+', -1, j, new[j]})
			j++
		default:
			edits = append(edits, edit{'-', i, -1, old[i]})
			i++
		}
	}
	return edits
}
