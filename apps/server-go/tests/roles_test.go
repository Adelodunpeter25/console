// Model roles + thinking validation coverage: reference parsing,
// resolution, level inference, validation, and run wiring (fail-fast on
// unsupported levels, smol titles, vision fallback shape).
package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/roles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func TestParseReference(t *testing.T) {
	p, m := roles.ParseReference("codex/gpt-5.5", "antigravity", "gemini-3-flash")
	if p != "codex" || m != "gpt-5.5" {
		t.Fatalf("qualified: %s %s", p, m)
	}
	p, m = roles.ParseReference("gpt-5.5", "codex", "gpt-5.6-luna")
	if p != "codex" || m != "gpt-5.5" {
		t.Fatalf("bare: %s %s", p, m)
	}
	if !roles.IsRole("smol") || !roles.IsRole("vision") || roles.IsRole("large") {
		t.Fatal("role names")
	}
	if !roles.HasConfiguredRole(map[string]string{"smol": "codex/gpt-5.5"}, "smol") {
		t.Fatal("configured ref must count")
	}
}

func TestInferThinkingLevels(t *testing.T) {
	levels, def := roles.InferThinkingLevels("codex", "gpt-5.6-luna")
	if len(levels) != 7 || def != "low" {
		t.Fatalf("codex: %v %s", levels, def)
	}
	levels, def = roles.InferThinkingLevels("claude", "claude-sonnet-4-6")
	if len(levels) != 5 || def != "low" {
		t.Fatalf("claude: %v %s", levels, def)
	}
	levels, def = roles.InferThinkingLevels("antigravity", "gemini-3-flash")
	if len(levels) != 4 || def != "low" {
		t.Fatalf("gemini: %v %s", levels, def)
	}
	if levels, def := roles.InferThinkingLevels("mock", "m"); len(levels) != 0 || def != "" {
		t.Fatalf("unknown: %v %s", levels, def)
	}
}

func TestResolveRoleModel(t *testing.T) {
	fallback := types.Model{ID: "gpt-5.6-luna", Provider: "codex", ContextWindow: 272_000}
	if got := roles.ResolveRoleModel("smol", fallback, ""); got.ID != fallback.ID {
		t.Fatalf("empty ref must fall back: %+v", got)
	}
	got := roles.ResolveRoleModel("smol", fallback, "codex/gpt-5.5")
	if got.ID != "gpt-5.5" || got.Provider != "codex" || got.ContextWindow != 272_000 {
		t.Fatalf("catalog hit: %+v", got)
	}
	got = roles.ResolveRoleModel("vision", fallback, "gpt-5.4-mini")
	if got.ID != "gpt-5.4-mini" || got.Provider != "codex" {
		t.Fatalf("bare id keeps provider: %+v", got)
	}
	got = roles.ResolveRoleModel("smol", fallback, "unknown-provider/m")
	if got.ID != fallback.ID {
		t.Fatalf("unknown provider must fall back: %+v", got)
	}
	got = roles.ResolveRoleModel("smol", fallback, "codex/future-model-9")
	if got.ID != "future-model-9" || got.ContextWindow != 128_000 || len(got.ThinkingLevels) != 7 {
		t.Fatalf("synthetic: %+v", got)
	}
}

func TestValidateLevel(t *testing.T) {
	model := types.Model{ID: "m", Provider: "codex", ThinkingLevels: []string{"low", "high"}}
	if err := roles.ValidateLevel(model, ""); err != nil {
		t.Fatalf("omitted must pass: %v", err)
	}
	if err := roles.ValidateLevel(model, "high"); err != nil {
		t.Fatalf("supported must pass: %v", err)
	}
	if err := roles.ValidateLevel(model, "max"); err == nil || !strings.Contains(err.Error(), "max") {
		t.Fatalf("unsupported must fail: %v", err)
	}
	bare := types.Model{ID: "m", Provider: "mock"}
	if err := roles.ValidateLevel(bare, "low"); err == nil {
		t.Fatal("level on level-less model must fail")
	}
}

func TestFindModel(t *testing.T) {
	found, ok := providers.FindModel("codex", "GPT-5.6-LUNA")
	if !ok || found.ID != "gpt-5.6-luna" {
		t.Fatalf("case-insensitive: %+v %v", found, ok)
	}
	if _, ok := providers.FindModel("codex", "nope"); ok {
		t.Fatal("unknown id must miss")
	}
	if _, ok := providers.FindModel("mock", "m"); ok {
		t.Fatal("unknown provider must miss")
	}
}

func TestRunRejectsUnsupportedThinking(t *testing.T) {
	sessions := newRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(id string) (loop.Provider, error) {
		return queueMock("hi"), nil
	}
	header := createRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "hi", Provider: "mock", ModelID: "mock-model", Thinking: "max"})
	if err != nil {
		t.Fatal(err)
	}
	subID, subCh, _ := hub.Subscribe(nil)
	defer hub.Unsubscribe(subID)
	waitSettled(t, hub)
	if hub.Outcome != run.OutcomeDone {
		t.Fatalf("outcome: %s", hub.Outcome)
	}
	sawError := false
	for f := range subCh {
		if f.Event.Kind == loop.EventError {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("expected an error frame for unsupported thinking level")
	}
	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("failed validation must persist only the user message: %d", len(loaded.Messages))
	}
	if svc.IsActive(header.ID) {
		t.Fatal("run must be inactive")
	}
}

type modelRecorder struct {
	models []string
	mock   *mockProvider
}

func (r *modelRecorder) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	r.models = append(r.models, req.Model)
	return r.mock.RunTurn(ctx, req, s)
}

func TestTitleUsesSmolModel(t *testing.T) {
	dir := t.TempDir()
	settings := `{"modelRoles":{"smol":"codex/gpt-5.5"}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONSOLE_SETTINGS_PATH", filepath.Join(dir, "settings.json"))

	sessions := newRunSessions(t)
	svc := run.NewService(sessions)
	rec := &modelRecorder{mock: &mockProvider{turns: []func() []loop.Event{
		func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "done"}} },
	}}}
	svc.Lookup = func(id string) (loop.Provider, error) { return rec, nil }
	header, err := sessions.Create(types.CreateSessionOptions{
		Cwd: t.TempDir(), ModelID: "mock-model", Provider: "mock", Title: "New Session",
	})
	if err != nil {
		t.Fatal(err)
	}
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "do things", Provider: "mock", ModelID: "mock-model"})
	if err != nil {
		t.Fatal(err)
	}
	waitSettled(t, hub)
	deadline := time.Now().Add(5 * time.Second)
	for {
		seen := false
		for _, m := range rec.models {
			if m == "gpt-5.5" {
				seen = true
			}
		}
		if seen {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("smol model never used: %v", rec.models)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
