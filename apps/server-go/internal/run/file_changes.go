// File-change tracking: pre-write snapshots, diff generation, and recording
// for session file changes. Mirrors run-file-changes.ts from the TS server.
package run

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/pmezard/go-difflib/difflib"
)

const maxDiffSize = 1_000_000 // 1MB limit for diff storage
const contextLines = 3        // lines of context around each hunk (matches git default)

// writeSnapshot captures the on-disk content of a path immediately before a
// write tool touches it.
//
// Whole-file overwrite tools (write_file, batchWrite) can only produce a real
// before/after diff if the prior content is read BEFORE the write lands.
// Recording runs when the tool result event is observed, which is strictly
// after the tool already wrote the file — so a read there returns the new
// content and the file diffs against itself (empty patch, no additions or
// deletions). The run service therefore snapshots these paths before
// dispatching the call and hands the captured state back here.
//
// A missing file snapshots as ("", false); the bool is what distinguishes a
// genuinely new file from an existing file that happens to be empty.
type writeSnapshot struct {
	Content string
	Existed bool
}

// writeSnapshots holds per-session pre-write content keyed by path. Scoped by
// session so two runs writing the same path concurrently cannot cross
// contaminate each other's diffs. Dropped when the run settles.
type writeSnapshots struct {
	mu     sync.Mutex
	stores map[string]map[string]writeSnapshot
}

func newWriteSnapshots() *writeSnapshots {
	return &writeSnapshots{stores: make(map[string]map[string]writeSnapshot)}
}

func (s *writeSnapshots) put(sessionID, path string, snap writeSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	store, ok := s.stores[sessionID]
	if !ok {
		store = make(map[string]writeSnapshot)
		s.stores[sessionID] = store
	}
	store[path] = snap
}

func (s *writeSnapshots) get(sessionID, path string) (writeSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok := s.stores[sessionID][path]
	return snap, ok
}

func (s *writeSnapshots) drop(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.stores, sessionID)
}

// SnapshotWritePaths reads the current content of each path and stores it as
// the "before" side of the diff for that session. Best-effort: an unreadable
// path is stored as non-existent, which is right for files being created and
// harmless for ones that turn out to be unreadable (the tool would fail too).
//
// Only whole-file overwrite tools need this — editFile carries
// oldContent/newContent in its own arguments.
func (s *writeSnapshots) SnapshotWritePaths(sessionID string, paths []string) {
	if s == nil || len(paths) == 0 {
		return
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		snap := writeSnapshot{}
		if data, err := os.ReadFile(path); err == nil {
			snap = writeSnapshot{Content: string(data), Existed: true}
		}
		s.put(sessionID, path, snap)
	}
}

// generateAndRecordFileChange generates unified diffs for file operations
// and records them in the session database.
//
// snapshots supplies pre-write content for whole-file overwrite tools. A nil
// *writeSnapshots is valid and means "no snapshot available", degrading to
// what the arguments alone can tell us.
func (s *Service) generateAndRecordFileChange(
	snapshots *writeSnapshots,
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
		if !ok || path == "" {
			return nil
		}
		content, _ := args["content"].(string)
		return s.recordWholeFileChange(snapshots, sessionID, path, content, turnIndex)

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
			if !ok || path == "" {
				continue
			}
			content, _ := fileMap["content"].(string)
			if err := s.recordWholeFileChange(snapshots, sessionID, path, content, turnIndex); err != nil {
				return err
			}
		}
		return nil

	case "editFile", "edit_file", "replace_file_content":
		// editFileInput uses "path", "oldContent", "newContent".
		// Older/alternate tool schemas may use "targetFile", "targetContent",
		// "replacementContent" — keep those as fallbacks.
		var targetPath string
		if p, ok := args["path"].(string); ok {
			targetPath = p
		} else if p, ok := args["targetFile"].(string); ok {
			targetPath = p
		}
		if targetPath == "" {
			return nil
		}

		var oldContent, newContent string
		if tc, ok := args["oldContent"].(string); ok {
			oldContent = tc
		} else if tc, ok := args["targetContent"].(string); ok {
			oldContent = tc
		}
		if rc, ok := args["newContent"].(string); ok {
			newContent = rc
		} else if rc, ok := args["replacementContent"].(string); ok {
			newContent = rc
		}

		// For editFile the snippet is the changed region — diff it directly.
		patch := createUnifiedDiff(targetPath, oldContent, newContent)
		adds, dels := countDiffLines(patch)

		return s.sessions.RecordFileChange(sessionID, types.SessionFileChange{
			Path:      targetPath,
			TurnIndex: turnIndex,
			Status:    "modified",
			Additions: adds,
			Deletions: dels,
			DiffText:  diffTextOrNil(patch),
			UpdatedAt: 0,
		})
	}

	return nil
}

// recordWholeFileChange diffs a complete file write (write_file, or one entry
// of a batchWrite) against the content captured before the tool ran.
//
// Status comes from the snapshot, not from a post-write read: "added" when
// the file did not exist before, "modified" when it did. With no snapshot the
// on-disk content is already the new content and reveals nothing about the
// before state, so the change is recorded as added against empty rather than
// diffing the file against itself into a fabricated empty patch.
func (s *Service) recordWholeFileChange(
	snapshots *writeSnapshots,
	sessionID, path, content string,
	turnIndex int,
) error {
	oldContent, status := "", "added"
	if snapshots != nil {
		if snap, ok := snapshots.get(sessionID, path); ok && snap.Existed {
			oldContent, status = snap.Content, "modified"
		}
	}

	patch := createUnifiedDiff(path, oldContent, content)
	adds, dels := countDiffLines(patch)

	return s.sessions.RecordFileChange(sessionID, types.SessionFileChange{
		Path:      path,
		TurnIndex: turnIndex,
		Status:    status,
		Additions: adds,
		Deletions: dels,
		DiffText:  diffTextOrNil(patch),
		UpdatedAt: 0,
	})
}

// diffTextOrNil drops an empty or oversized patch while keeping the row and
// its counts, so a capped diff still reports which files changed by how much.
func diffTextOrNil(patch string) *string {
	if patch == "" || len(patch) > maxDiffSize {
		return nil
	}
	return &patch
}

// createUnifiedDiff produces a standard unified diff with @@ hunk headers,
// matching the output of jsdiff's createPatch in the TS server this mirrors.
//
// It walks difflib's grouped opcodes by hand rather than calling
// GetUnifiedDiffString for one reason: that helper calls NewMatcher, which
// hardcodes autoJunk to true, and there is no way to opt out through its API.
// autoJunk treats any line occurring in >1% of a 200+ line sequence as junk
// and excludes it from matching. That heuristic is tuned for prose, not
// source code: in a file full of repeated structural lines (`}` in JS, `end`
// in Ruby, blank lines) a one-line edit surrounded by them makes the matcher
// discard every one of those lines as junk and re-emit the entire file as a
// single rewritten hunk. NewMatcherWithJunk(false, nil) disables it, so the
// hunk grouping below mirrors WriteUnifiedDiff's format with autoJunk off.
func createUnifiedDiff(filename, oldText, newText string) string {
	oldLines, newLines := diffLines(oldText), diffLines(newText)
	if len(oldLines) == 0 && len(newLines) == 0 {
		return ""
	}

	m := difflib.NewMatcherWithJunk(oldLines, newLines, false, nil)
	groups := m.GetGroupedOpCodes(contextLines)
	if len(groups) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- a/%s\n", filename)
	fmt.Fprintf(&b, "+++ b/%s\n", filename)

	for _, g := range groups {
		first, last := g[0], g[len(g)-1]
		fmt.Fprintf(&b, "@@ -%s +%s @@\n",
			formatRange(first.I1, last.I2),
			formatRange(first.J1, last.J2),
		)
		for _, c := range g {
			switch c.Tag {
			case 'e':
				for _, line := range oldLines[c.I1:c.I2] {
					b.WriteString(" " + line)
				}
			case 'r', 'd':
				for _, line := range oldLines[c.I1:c.I2] {
					b.WriteString("-" + line)
				}
				if c.Tag == 'r' {
					for _, line := range newLines[c.J1:c.J2] {
						b.WriteString("+" + line)
					}
				}
			case 'i':
				for _, line := range newLines[c.J1:c.J2] {
					b.WriteString("+" + line)
				}
			}
		}
	}
	return b.String()
}

// formatRange renders one side of a @@ header. A length of 1 is written
// without the ",1" suffix and a length of 0 as start-1,0, per the unified diff
// spec (formatRangeUnified in difflib). The desktop parser accepts both the
// short and long forms.
func formatRange(start, stop int) string {
	beginning := start + 1 // lines are 1-based
	length := stop - start
	switch length {
	case 0:
		return fmt.Sprintf("%d,0", beginning-1)
	case 1:
		return fmt.Sprintf("%d", beginning)
	default:
		return fmt.Sprintf("%d,%d", beginning, length)
	}
}

// diffLines splits content into lines for difflib, keeping each line's
// trailing newline.
//
// Two details are load-bearing, because difflib's own SplitLines gets both
// wrong for our purposes:
//
//   - SplitLines does strings.SplitAfter(s, "\n") and then unconditionally
//     appends "\n" to the final element. For input that already ends in a
//     newline that element is "", so it becomes a lone "\n" — a phantom
//     trailing blank line. That inflates every hunk header's count by one and
//     appends a bogus context line to the end of the diff.
//   - WriteUnifiedDiff writes each body line by prefixing it and writing it
//     verbatim, so the newline must already be part of the line. A final line
//     with no newline runs into the next line's prefix and corrupts the diff
//     ("-b+c"). Normalising the input to end in a newline avoids that.
func diffLines(s string) []string {
	if s == "" {
		return nil
	}
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
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
