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

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
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

// TestEditFileUsesArgumentSnippet confirms a single editFile records the
// file's change against its turn baseline.
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

// TestTwoWritesSamePathCollapseWithinATurn: two writes to one path in a
// turn collapse to one row recording the net change of the turn — diffed
// against the content the file had before the turn first touched it.
func TestTwoWritesSamePathCollapseWithinATurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "twice.ts")
	if err := os.WriteFile(path, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes := runWrites(t,
		callFor(t, "write_file", map[string]any{"path": path, "content": "v2\n"}),
		callFor(t, "write_file", map[string]any{"path": path, "content": "v3\n"}),
	)

	if len(changes) != 1 {
		t.Fatalf("same-path writes in one turn must collapse to 1 row, got %d: %+v", len(changes), changes)
	}
	got := changes[0]
	if got.Additions != 1 || got.Deletions != 1 {
		t.Fatalf("net counts = +%d -%d, want +1 -1", got.Additions, got.Deletions)
	}
	if got.DiffText == nil || !strings.Contains(*got.DiffText, "-v1") || !strings.Contains(*got.DiffText, "+v3") {
		t.Fatalf("diff must be the turn's net change (v1 -> v3): %+v", got.DiffText)
	}
	if strings.Contains(*got.DiffText, "v2") {
		t.Fatalf("intermediate content must not appear in the net diff:\n%s", *got.DiffText)
	}
}

// TestRepeatedEditsRecordNetChange: several editFile calls on one path in a
// turn record the whole-file net change, not just the last snippet.
func TestRepeatedEditsRecordNetChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edits.ts")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes := runWrites(t,
		tools.ToolCall{ID: "e1", Name: "editFile", Arguments: mustJSON(t, map[string]any{"path": path, "oldContent": "a\n", "newContent": "A\n"})},
		tools.ToolCall{ID: "e2", Name: "editFile", Arguments: mustJSON(t, map[string]any{"path": path, "oldContent": "c\n", "newContent": "C\nD\n"})},
	)

	if len(changes) != 1 {
		t.Fatalf("expected 1 row, got %+v", changes)
	}
	got := changes[0]
	if got.Status != "modified" {
		t.Fatalf("status = %q, want modified", got.Status)
	}
	if got.Additions != 3 || got.Deletions != 2 {
		t.Fatalf("net counts = +%d -%d, want +3 -2", got.Additions, got.Deletions)
	}
	if got.DiffText == nil || !strings.Contains(*got.DiffText, "-a") || !strings.Contains(*got.DiffText, "+D") {
		t.Fatalf("net diff missing first or last edit:\n%v", got.DiffText)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
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
	// One turn per user message: the runs land on turns 0 and 1, each linked
	// to the persisted user message that started it.
	byTurn := map[int]types.SessionFileChange{}
	for _, c := range changes {
		byTurn[c.TurnIndex] = c
	}
	first, ok0 := byTurn[0]
	second, ok1 := byTurn[1]
	if !ok0 || !ok1 {
		t.Fatalf("want turn indices 0 and 1, got %+v", changes)
	}
	userIDs := userMessageIDs(t, sessions, header.ID)
	if len(userIDs) != 2 || first.UserMessageID != userIDs[0] || second.UserMessageID != userIDs[1] {
		t.Fatalf("rows must link to their user messages: ids=%v first=%q second=%q",
			userIDs, first.UserMessageID, second.UserMessageID)
	}
	// Each was a modification of content that existed.
	for i, c := range changes {
		if c.Status != "modified" {
			t.Fatalf("row %d status = %q, want modified", i, c.Status)
		}
	}
}

// userMessageIDs returns the persisted user message ids of a session, oldest
// first.
func userMessageIDs(t *testing.T, sessions *services.SessionService, sessionID string) []string {
	t.Helper()
	loaded, err := sessions.Load(sessionID, 0, 0)
	if err != nil || loaded == nil {
		t.Fatalf("load session: %v", err)
	}
	var ids []string
	for _, raw := range loaded.Messages {
		var msg consolev1.AgentMessage
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.GetUser() != nil {
			ids = append(ids, msg.GetId())
		}
	}
	return ids
}
