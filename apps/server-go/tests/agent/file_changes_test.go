// Session file-change recording for whole-file overwrite tools.
//
// Regression coverage: write_file/batchWrite must diff against the content
// captured BEFORE the tool wrote, not against the file's already-updated
// on-disk state. The bug read the file back after the write, so it diffed the
// file against itself — empty patch, zero additions and deletions, and an
// overwrite mislabelled as a fresh "added" file.
//
// These drive the real run pipeline (StartRun -> executor -> tools) so the
// snapshot hook, the tool, and the recorder are all exercised together.
package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// toolCallingProvider emits one scripted tool call on its first turn. The
// loop sees StopToolUse, executes the call, then requests a second turn to
// finish the run.
type toolCallingProvider struct {
	calls []tools.ToolCall
	n     int
}

func (p *toolCallingProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	if p.n == 0 {
		for i := range p.calls {
			call := p.calls[i]
			s.Push(loop.Event{Kind: loop.EventToolCall, Call: &call})
		}
	} else {
		s.Push(loop.Event{Kind: loop.EventText, Text: "done"})
	}
	p.n++
	s.Complete()
	return nil
}

// runWrites runs a session through a provider that issues the given tool
// calls, then returns the recorded file changes.
func runWrites(t *testing.T, calls ...tools.ToolCall) []types.SessionFileChange {
	t.Helper()
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &toolCallingProvider{calls: calls}, nil
	}
	header := helpers.CreateRunSession(t, sessions)

	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "go", Provider: "mock", ModelID: "m", ApprovalMode: string(permissions.FullAccess)})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	helpers.WaitSettled(t, hub)

	changes, err := sessions.GetSessionFileChanges(header.ID, -1)
	if err != nil {
		t.Fatalf("GetSessionFileChanges: %v", err)
	}
	return changes
}

func callFor(t *testing.T, name string, args map[string]any) tools.ToolCall {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return tools.ToolCall{ID: "call_" + name, Name: name, Arguments: raw}
}

// TestWriteFileOverwriteRecordsRealDiff is the core regression: overwriting
// an existing file must be "modified" with a real before/after diff.
func TestWriteFileOverwriteRecordsRealDiff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.ts")
	const before = "line one\nline two\nline three\n"
	const after = "line one\nline TWO\nline three\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}

	changes := runWrites(t, callFor(t, "write_file", map[string]any{
		"path": path, "content": after,
	}))

	if len(changes) != 1 {
		t.Fatalf("expected 1 change row, got %d: %+v", len(changes), changes)
	}
	got := changes[0]
	if got.Status != "modified" {
		t.Fatalf("status = %q, want modified (file existed before the write)", got.Status)
	}
	if got.Additions != 1 || got.Deletions != 1 {
		t.Fatalf("counts = +%d -%d, want +1 -1 (empty diff means the before-side was lost)",
			got.Additions, got.Deletions)
	}
	if got.DiffText == nil || *got.DiffText == "" {
		t.Fatal("diff text must not be empty for an overwrite")
	}
	if !strings.Contains(*got.DiffText, "-line two") || !strings.Contains(*got.DiffText, "+line TWO") {
		t.Fatalf("diff text missing before/after lines:\n%s", *got.DiffText)
	}
	// And the file really was written by the tool.
	if onDisk, _ := os.ReadFile(path); string(onDisk) != after {
		t.Fatalf("tool did not write the file: %q", string(onDisk))
	}
}

// TestWriteFileNewFileIsAdded covers the create path.
func TestWriteFileNewFileIsAdded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.ts")
	const content = "alpha\nbeta\n"

	changes := runWrites(t, callFor(t, "write_file", map[string]any{
		"path": path, "content": content,
	}))

	if len(changes) != 1 {
		t.Fatalf("expected 1 row, got %+v", changes)
	}
	if changes[0].Status != "added" {
		t.Fatalf("status = %q, want added", changes[0].Status)
	}
	if changes[0].Additions != 2 || changes[0].Deletions != 0 {
		t.Fatalf("counts = +%d -%d, want +2 -0", changes[0].Additions, changes[0].Deletions)
	}
	if changes[0].DiffText == nil || *changes[0].DiffText == "" {
		t.Fatal("a new file must carry a diff body")
	}
}

// TestBatchWriteOverwriteIsModified covers the batchWrite half: each entry
// gets its own pre-write snapshot, so overwrites are "modified" with real
// deletions while genuine new files stay "added".
func TestBatchWriteOverwriteIsModified(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.ts")
	created := filepath.Join(dir, "created.ts")

	const beforeExisting = "one\ntwo\nthree\n"
	const afterExisting = "one\nthree\n"
	const createdContent = "new file\n"

	if err := os.WriteFile(existing, []byte(beforeExisting), 0o644); err != nil {
		t.Fatal(err)
	}

	changes := runWrites(t, callFor(t, "batchWrite", map[string]any{
		"files": []any{
			map[string]any{"path": existing, "content": afterExisting},
			map[string]any{"path": created, "content": createdContent},
		},
	}))

	if len(changes) != 2 {
		t.Fatalf("expected 2 rows, got %d: %+v", len(changes), changes)
	}
	byPath := map[string]types.SessionFileChange{}
	for _, c := range changes {
		byPath[c.Path] = c
	}

	got, ok := byPath[existing]
	if !ok {
		t.Fatalf("no row for the overwritten file: %+v", changes)
	}
	if got.Status != "modified" {
		t.Fatalf("overwritten file status = %q, want modified", got.Status)
	}
	// "one\ntwo\nthree" -> "one\nthree": a pure deletion of "two" — the
	// surviving lines are context, so there is no corresponding addition.
	if got.Deletions != 1 {
		t.Fatalf("overwritten file deletions = %d, want 1", got.Deletions)
	}
	if got.Additions != 0 {
		t.Fatalf("overwritten file additions = %d, want 0 (net deletion)", got.Additions)
	}
	if got.DiffText == nil || !strings.Contains(*got.DiffText, "-two") {
		t.Fatalf("overwritten file diff missing the removed line: %+v", got.DiffText)
	}

	fresh, ok := byPath[created]
	if !ok {
		t.Fatalf("no row for the created file: %+v", changes)
	}
	if fresh.Status != "added" {
		t.Fatalf("created file status = %q, want added", fresh.Status)
	}
	if fresh.Additions != 1 || fresh.Deletions != 0 {
		t.Fatalf("created file counts = +%d -%d, want +1 -0", fresh.Additions, fresh.Deletions)
	}
}

// TestWriteFileIdenticalRewriteIsNoOpDiff covers re-writing unchanged content:
// a genuine diff of equal text is empty, so the row reports zero counts rather
// than fabricating additions.
func TestWriteFileIdenticalRewriteIsNoOpDiff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "same.ts")
	const content = "identical\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	changes := runWrites(t, callFor(t, "write_file", map[string]any{
		"path": path, "content": content,
	}))

	if len(changes) != 1 {
		t.Fatalf("expected 1 row, got %+v", changes)
	}
	if changes[0].Additions != 0 || changes[0].Deletions != 0 {
		t.Fatalf("identical rewrite should be 0 changes, got +%d -%d",
			changes[0].Additions, changes[0].Deletions)
	}
	if changes[0].Status != "modified" {
		t.Fatalf("status = %q, want modified (the file existed)", changes[0].Status)
	}
}

// TestFailedWriteRecordsNothing confirms an errored tool call creates no row
// (a write that failed on disk changed nothing).
func TestFailedWriteRecordsNothing(t *testing.T) {
	// A path whose parent is an existing *file* makes os.MkdirAll fail.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	impossible := filepath.Join(blocker, "child.ts")

	changes := runWrites(t, callFor(t, "write_file", map[string]any{
		"path": impossible, "content": "x",
	}))

	if len(changes) != 0 {
		t.Fatalf("failed write must record nothing, got %+v", changes)
	}
}

// TestEditFileUsesArgumentSnippet confirms editFile is unaffected — it diffs
// its own oldContent/newContent and needs no snapshot.
func TestEditFileUsesArgumentSnippet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edit.ts")
	if err := os.WriteFile(path, []byte("foo\nbar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes := runWrites(t, callFor(t, "editFile", map[string]any{
		"path": path, "oldContent": "foo\n", "newContent": "baz\n",
	}))

	if len(changes) != 1 {
		t.Fatalf("expected 1 row, got %+v", changes)
	}
	if changes[0].Status != "modified" {
		t.Fatalf("status = %q, want modified", changes[0].Status)
	}
	if changes[0].Additions != 1 || changes[0].Deletions != 1 {
		t.Fatalf("counts = +%d -%d, want +1 -1", changes[0].Additions, changes[0].Deletions)
	}
}

// TestTwoWritesSamePathCollapseWithinATurn documents the turn-index
// semantics that decide whether two writes to one path survive as separate
// rows or overwrite each other.
//
// Every tool result in a run is stamped with `len(history)` captured once,
// before the run starts (see turns.go). So all writes in one run share a
// turn index, and the table's PRIMARY KEY (path, turn_index) upserts the
// second write over the first — the run keeps only its final effect on that
// path, which is the correct end state, not a lost update.
func TestTwoWritesSamePathCollapseWithinATurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "twice.ts")
	if err := os.WriteFile(path, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes := runWrites(t,
		callFor(t, "write_file", map[string]any{"path": path, "content": "v2\n"}),
		callFor(t, "write_file", map[string]any{"path": path, "content": "v3\n"}),
	)

	// One row: same (path, turnIndex) key, so the last write wins.
	if len(changes) != 1 {
		t.Fatalf("same-path writes in one turn must collapse to 1 row, got %d: %+v", len(changes), changes)
	}
	// The snapshot is refreshed before each write, so the surviving row
	// reflects the LAST write (v2 -> v3), not the pre-run content.
	if changes[0].Deletions != 1 {
		t.Fatalf("surviving row deletions = %d, want 1 (v2 removed)", changes[0].Deletions)
	}
	if changes[0].DiffText == nil {
		t.Fatal("surviving row must keep a diff body")
	}
	if strings.Contains(*changes[0].DiffText, "-v1") {
		t.Fatalf("diff must be against the previous write's output, not the pre-run content:\n%s", *changes[0].DiffText)
	}
	if final, _ := os.ReadFile(path); string(final) != "v3\n" {
		t.Fatalf("on-disk content = %q, want the last write's content", string(final))
	}
}

// TestSamePathAcrossRunsKeepsBothRows is the counterpart: a new run sees a
// longer history, so the same path lands on a different turn index and both
// versions are preserved for review (this is what the "Previous Turn" scope
// reads).
func TestSamePathAcrossRunsKeepsBothRows(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	header := helpers.CreateRunSession(t, sessions)

	path := filepath.Join(t.TempDir(), "across.ts")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run 1: write "second".
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &toolCallingProvider{calls: []tools.ToolCall{
			{ID: "c1", Name: "write_file", Arguments: []byte(`{"path":"` + path + `","content":"second\n"}`)},
		}}, nil
	}
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "one", Provider: "mock", ModelID: "m", ApprovalMode: string(permissions.FullAccess)})
	if err != nil {
		t.Fatalf("StartRun 1: %v", err)
	}
	helpers.WaitSettled(t, hub)

	// Run 2: write "third".
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &toolCallingProvider{calls: []tools.ToolCall{
			{ID: "c2", Name: "write_file", Arguments: []byte(`{"path":"` + path + `","content":"third\n"}`)},
		}}, nil
	}
	hub2, err := svc.StartRun(header.ID, run.Prompt{Text: "two", Provider: "mock", ModelID: "m", ApprovalMode: string(permissions.FullAccess)})
	if err != nil {
		t.Fatalf("StartRun 2: %v", err)
	}
	helpers.WaitSettled(t, hub2)

	changes, err := sessions.GetSessionFileChanges(header.ID, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("two runs on one path must keep both rows, got %d: %+v", len(changes), changes)
	}
	// Distinct turn indices — this is the whole point of the column.
	if changes[0].TurnIndex == changes[1].TurnIndex {
		t.Fatalf("both rows share turn_index %d; they must differ", changes[0].TurnIndex)
	}
	// Each was a modification of content that existed.
	for i, c := range changes {
		if c.Status != "modified" {
			t.Fatalf("row %d status = %q, want modified", i, c.Status)
		}
	}
}
