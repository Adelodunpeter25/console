// Unified-diff format guarantees for recorded file changes.
//
// The desktop viewer parses diff_text with parse_unified_diff, which ignores
// everything until it sees a "@@" hunk header. A patch without one renders as
// an empty diff, so the file looks changed (+12 -4) while the body shows
// nothing. That mismatch shipped silently: the server emitted headerless
// patches, every stored diff lacked "@@", and no test asserted the format.
//
// These tests pin the contract the viewer depends on — "@@" present, header
// counts equal to the body they govern, and recorded counts equal to a
// re-parse of the same text — and cover the difflib input-normalisation traps
// documented on diffLines in the run package.
package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parsedHunk is one "@@" block of a unified diff, with the counts the header
// claims and the counts the body actually contains.
type parsedHunk struct {
	header    string
	oldStart  int
	newStart  int
	oldClaim  int
	newClaim  int
	oldActual int
	newActual int
	added     int
	removed   int
}

// parseUnifiedDiff mirrors the desktop viewer's parse_unified_diff closely
// enough to act as a stand-in: it skips any line before the first "@@", then
// accumulates + - and space-prefixed lines. It additionally reports the
// header/body count agreement that the viewer does not check, so a corrupt
// patch fails here rather than rendering nonsense.
func parseUnifiedDiff(t *testing.T, raw string) (hunks []parsedHunk, added, removed int) {
	t.Helper()
	var cur *parsedHunk
	flush := func() {
		if cur != nil {
			hunks = append(hunks, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "@@") {
			flush()
			h := parsedHunk{header: line}
			oStart, oLen, nStart, nLen, ok := parseHunkHeader(line)
			if !ok {
				t.Fatalf("unparseable hunk header %q", line)
			}
			h.oldStart, h.oldClaim, h.newStart, h.newClaim = oStart, oLen, nStart, nLen
			cur = &h
			continue
		}
		if cur == nil {
			continue // pre-hunk noise, exactly as the viewer skips it
		}
		if line == "" {
			continue
		}
		switch line[0] {
		case ' ':
			cur.oldActual++
			cur.newActual++
		case '-':
			cur.oldActual++
			cur.removed++
			removed++
		case '+':
			cur.newActual++
			cur.added++
			added++
		default:
			t.Fatalf("diff body line lacks a +/-/space prefix, patch is corrupt: %q", line)
		}
	}
	flush()
	return hunks, added, removed
}

// parseHunkHeader reads "@@ -oldStart[,oldLen] +newStart[,newLen] @@". A
// missing length means 1, per the unified diff spec — the viewer relies on
// this, so the server must be allowed to emit the short form.
func parseHunkHeader(line string) (oldStart, oldLen, newStart, newLen int, ok bool) {
	fields := strings.Fields(strings.Trim(line, "@ "))
	if len(fields) < 2 {
		return 0, 0, 0, 0, false
	}
	oldStart, oldLen, ok = parseRange(strings.TrimPrefix(fields[0], "-"))
	if !ok {
		return 0, 0, 0, 0, false
	}
	newStart, newLen, ok = parseRange(strings.TrimPrefix(fields[1], "+"))
	if !ok {
		return 0, 0, 0, 0, false
	}
	return oldStart, oldLen, newStart, newLen, true
}

func parseRange(s string) (start, length int, ok bool) {
	parts := strings.SplitN(s, ",", 2)
	start = atoiOr(parts[0], -1)
	if start < 0 {
		return 0, 0, false
	}
	length = 1
	if len(parts) == 2 {
		if length = atoiOr(parts[1], -1); length < 0 {
			return 0, 0, false
		}
	}
	return start, length, true
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// assertWellFormed enforces every invariant the viewer needs, and returns the
// re-parsed totals so callers can compare them against the recorded counts.
func assertWellFormed(t *testing.T, label, diff string) (added, removed int) {
	t.Helper()
	if !strings.HasPrefix(diff, "--- ") {
		t.Fatalf("%s: diff must start with a --- header:\n%s", label, diff)
	}
	hunks, added, removed := parseUnifiedDiff(t, diff)
	if len(hunks) == 0 {
		t.Fatalf("%s: diff has no @@ hunk header, the viewer will render it empty:\n%s", label, diff)
	}
	for _, h := range hunks {
		if h.oldClaim != h.oldActual || h.newClaim != h.newActual {
			t.Fatalf("%s: hunk %q claims -%d,+%d but body has %d old and %d new lines:\n%s",
				label, h.header, h.oldClaim, h.newClaim, h.oldActual, h.newActual, diff)
		}
	}
	return added, removed
}

// TestRecordedDiffRoundTripsThroughViewerParser is the core guarantee: a diff
// produced by the server must survive a parse by viewer-shaped code with the
// same additions/deletions the server recorded.
func TestRecordedDiffRoundTripsThroughViewerParser(t *testing.T) {
	cases := []struct {
		name       string
		before     string
		after      string
		wantAdds   int
		wantDels   int
		wantStatus string
	}{
		{
			name: "single line replaced", before: "one\ntwo\nthree\n", after: "one\nTWO\nthree\n",
			wantAdds: 1, wantDels: 1, wantStatus: "modified",
		},
		{
			name: "new file", before: "", after: "alpha\nbeta\n",
			wantAdds: 2, wantDels: 0, wantStatus: "added",
		},
		{
			name: "all lines deleted", before: "gone1\ngone2\n", after: "",
			wantAdds: 0, wantDels: 2, wantStatus: "modified",
		},
		{
			name: "interior line inserted", before: "a\nb\nc\n", after: "a\nb\nX\nc\n",
			wantAdds: 1, wantDels: 0, wantStatus: "modified",
		},
		{
			name: "blank interior line edited", before: "a\n\nb\n", after: "a\n\nB\n",
			wantAdds: 1, wantDels: 1, wantStatus: "modified",
		},
		{
			name: "no trailing newline on either side", before: "a\nb", after: "a\nc",
			wantAdds: 1, wantDels: 1, wantStatus: "modified",
		},
		{
			name: "trailing newline added", before: "a\nb", after: "a\nb\n",
			wantAdds: 0, wantDels: 0, wantStatus: "modified",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.txt")
			if tc.before != "" {
				if err := os.WriteFile(path, []byte(tc.before), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			after := tc.after

			changes := runWrites(t, callFor(t, "write_file", map[string]any{
				"path": path, "content": after,
			}))
			if len(changes) != 1 {
				t.Fatalf("expected 1 row, got %+v", changes)
			}
			got := changes[0]
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", got.Status, tc.wantStatus)
			}
			if got.Additions == 0 && got.Deletions == 0 {
				if got.DiffText != nil && *got.DiffText != "" {
					t.Fatalf("no-op change must not carry a diff body:\n%s", *got.DiffText)
				}
				if tc.wantAdds != 0 || tc.wantDels != 0 {
					t.Fatalf("expected a real change (+%d -%d), got a no-op row", tc.wantAdds, tc.wantDels)
				}
				return
			}
			if got.DiffText == nil || *got.DiffText == "" {
				t.Fatal("a real change must carry a diff body")
			}
			added, removed := assertWellFormed(t, tc.name, *got.DiffText)
			if added != got.Additions || removed != got.Deletions {
				t.Fatalf("re-parsed +%d -%d disagrees with recorded +%d -%d:\n%s",
					added, removed, got.Additions, got.Deletions, *got.DiffText)
			}
			if added != tc.wantAdds || removed != tc.wantDels {
				t.Fatalf("re-parsed counts = +%d -%d, want +%d -%d:\n%s",
					added, removed, tc.wantAdds, tc.wantDels, *got.DiffText)
			}
		})
	}
}

// TestDistantEditsProduceSeparateHunks guards hunk grouping: two changes far
// apart must not be merged into one hunk spanning the untouched middle, and
// each hunk header must still match its own body.
func TestDistantEditsProduceSeparateHunks(t *testing.T) {
	var before, after strings.Builder
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&before, "line %d\n", i)
		fmt.Fprintf(&after, "line %d\n", i)
	}
	afterLines := strings.Split(after.String(), "\n")
	afterLines[4] = "FIRST CHANGE"   // near the top
	afterLines[34] = "SECOND CHANGE" // near the bottom

	path := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(path, []byte(before.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := runWrites(t, callFor(t, "write_file", map[string]any{
		"path": path, "content": strings.Join(afterLines, "\n"),
	}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 row, got %+v", changes)
	}
	if changes[0].DiffText == nil {
		t.Fatal("expected a diff body")
	}
	hunks, _, _ := parseUnifiedDiff(t, *changes[0].DiffText)
	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunks for two well-separated edits, got %d:\n%s",
			len(hunks), *changes[0].DiffText)
	}

	// Context must not have swallowed the unchanged middle of the file.
	if strings.Contains(*changes[0].DiffText, "line 20") {
		t.Fatalf("distant edits should not carry the untouched middle as context:\n%s", *changes[0].DiffText)
	}
}

// TestRepeatedLinesDoNotTriggerAutoJunk is a regression guard for difflib's
// autoJunk heuristic, which counts a line appearing in >1% of a 200+ line
// sequence as junk and excludes it from matching. Source files are full of
// repeated structural lines; with autoJunk on, a one-line edit surrounded by
// them re-emits the entire file as a single rewritten hunk.
func TestRepeatedLinesDoNotTriggerAutoJunk(t *testing.T) {
	var before, after strings.Builder
	for i := 0; i < 400; i++ {
		before.WriteString("}\n")
		after.WriteString("}\n")
	}
	before.WriteString("needle\n")
	after.WriteString("NEEDLE\n")
	for i := 0; i < 400; i++ {
		before.WriteString("}\n")
		after.WriteString("}\n")
	}

	path := filepath.Join(t.TempDir(), "braces.js")
	if err := os.WriteFile(path, []byte(before.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := runWrites(t, callFor(t, "write_file", map[string]any{
		"path": path, "content": after.String(),
	}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 row, got %+v", changes)
	}
	got := changes[0]
	if got.Additions != 1 || got.Deletions != 1 {
		t.Fatalf("one edited line should be +1 -1, got +%d -%d (autoJunk likely enabled)",
			got.Additions, got.Deletions)
	}
	if got.DiffText == nil {
		t.Fatal("expected a diff body")
	}
	added, removed := assertWellFormed(t, "repeated lines", *got.DiffText)
	if added != 1 || removed != 1 {
		t.Fatalf("re-parsed +%d -%d, want +1 -1", added, removed)
	}
	if strings.Count(*got.DiffText, "\n-}") > 5 {
		t.Fatalf("diff re-emitted the whole file as deleted lines, autoJunk is on:\n%s", *got.DiffText)
	}
}

// TestLargeFileDiffIsNotTruncated guards the removed hand-rolled cap, which
// silently dropped every change past 5000 lines. A change near the end of a
// large file must still be reported.
func TestLargeFileDiffIsNotTruncated(t *testing.T) {
	var before, after strings.Builder
	const lines = 6000
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&before, "line %d content\n", i)
		fmt.Fprintf(&after, "line %d content\n", i)
	}
	afterLines := strings.Split(after.String(), "\n")
	afterLines[lines-2] = "TAIL CHANGE" // last content line, well past any 5000 cap

	path := filepath.Join(t.TempDir(), "large.txt")
	if err := os.WriteFile(path, []byte(before.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := runWrites(t, callFor(t, "write_file", map[string]any{
		"path": path, "content": strings.Join(afterLines, "\n"),
	}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 row, got %+v", changes)
	}
	got := changes[0]
	if got.Additions != 1 || got.Deletions != 1 {
		t.Fatalf("change past line 5000 was lost: +%d -%d, want +1 -1", got.Additions, got.Deletions)
	}
	if got.DiffText == nil || !strings.Contains(*got.DiffText, "TAIL CHANGE") {
		t.Fatalf("diff body missing the tail change:\n%+v", got.DiffText)
	}
	_, _ = assertWellFormed(t, "large file", *got.DiffText)
}

// TestEditFileDiffIsViewerParsable applies the same format contract to the
// editFile path, which diffs its own argument snippet rather than a snapshot.
func TestEditFileDiffIsViewerParsable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edit.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := runWrites(t, callFor(t, "editFile", map[string]any{
		"path": path, "oldContent": "b\nc\n", "newContent": "B\nC\nD\n",
	}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 row, got %+v", changes)
	}
	got := changes[0]
	if got.DiffText == nil {
		t.Fatal("expected a diff body")
	}
	added, removed := assertWellFormed(t, "editFile", *got.DiffText)
	if added != got.Additions || removed != got.Deletions {
		t.Fatalf("re-parsed +%d -%d disagrees with recorded +%d -%d:\n%s",
			added, removed, got.Additions, got.Deletions, *got.DiffText)
	}
}
