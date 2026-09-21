// Memory subsystem coverage: store CRUD, recall scoring, registry
// isolation, tool flows, and per-run wiring.
package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/memory"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func openTestStore(t *testing.T, dir, name string, scope memory.Scope) *memory.Store {
	t.Helper()
	store, err := memory.OpenStore(dir+"/"+name, scope)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestMemoryStoreCRUD(t *testing.T) {
	dir := t.TempDir()
	store := openTestStore(t, dir, "memory.db", memory.ScopeProject)

	a, err := store.Store("user prefers tabs", []string{"prefs"})
	if err != nil || a.ID == "" {
		t.Fatalf("store: %+v %v", a, err)
	}
	b, err := store.Store("repo uses bun", []string{"repo"})
	if err != nil {
		t.Fatal(err)
	}
	if a.CreatedAt == 0 || a.UpdatedAt == 0 {
		t.Fatal("timestamps must be set")
	}

	got, err := store.Get(a.ID)
	if err != nil || got == nil || got.Content != "user prefers tabs" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if missing, err := store.Get("nope"); err != nil || missing != nil {
		t.Fatalf("missing: %+v %v", missing, err)
	}

	listed, err := store.List(nil)
	if err != nil || len(listed) != 2 {
		t.Fatalf("list: %+v %v", listed, err)
	}
	filtered, err := store.List([]string{"repo"})
	if err != nil || len(filtered) != 1 || filtered[0].ID != b.ID {
		t.Fatalf("filtered: %+v %v", filtered, err)
	}

	content := "user prefers spaces"
	updated, err := store.Update(a.ID, &content, []string{"prefs", "editor"}, true)
	if err != nil || updated == nil || updated.Content != content || len(updated.Tags) != 2 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if missing, err := store.Update("nope", &content, nil, false); err != nil || missing != nil {
		t.Fatalf("update missing: %+v %v", missing, err)
	}

	removed, err := store.Remove(b.ID)
	if err != nil || !removed {
		t.Fatalf("remove: %v %v", removed, err)
	}
	if removed, _ := store.Remove(b.ID); removed {
		t.Fatal("double remove must fail")
	}
}

func TestRecallScoring(t *testing.T) {
	entries := []memory.Entry{
		{ID: "a", Content: "the deploy script uses docker", Tags: []string{"deploy"}},
		{ID: "b", Content: "unrelated fact", Tags: []string{"other"}},
		{ID: "c", Content: "docker compose setup", Tags: []string{"deploy", "docker"}},
	}
	matches := memory.RecallMemories(entries, memory.Query{Text: "docker", Tags: []string{"deploy", "docker"}}, 10)
	if len(matches) != 2 {
		t.Fatalf("matches: %+v", matches)
	}
	// c scores tags(3+3)+text(1)=7, a scores tag(3)+text(1)=4.
	if matches[0].Entry.ID != "c" || matches[0].Score != 7 {
		t.Fatalf("first: %+v", matches[0])
	}
	if matches[1].Entry.ID != "a" || matches[1].Score != 4 {
		t.Fatalf("second: %+v", matches[1])
	}
	if limited := memory.RecallMemories(entries, memory.Query{Tags: []string{"deploy"}}, 1); len(limited) != 1 {
		t.Fatalf("limit: %+v", limited)
	}
	if none := memory.RecallMemories(entries, memory.Query{Text: "zzz"}, 10); len(none) != 0 {
		t.Fatalf("no match: %+v", none)
	}
}

func TestMemoryRegistry(t *testing.T) {
	dir := t.TempDir()
	registry := memory.NewRegistry(dir)
	if registry.StorageDir() != dir {
		t.Fatalf("dir: %s", registry.StorageDir())
	}
	p1, err := registry.ForProject("p1")
	if err != nil {
		t.Fatal(err)
	}
	again, err := registry.ForProject("p1")
	if err != nil {
		t.Fatal(err)
	}
	if p1 != again {
		t.Fatal("project store must be cached")
	}
	g, err := registry.ForGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve(memory.ScopeGlobal, ""); err != nil {
		t.Fatalf("global resolve: %v", err)
	}
	if _, err := registry.Resolve(memory.ScopeProject, ""); err == nil {
		t.Fatal("project resolve without id must fail")
	}
	if _, err := registry.Resolve(memory.ScopeProject, "p1"); err != nil {
		t.Fatalf("project resolve: %v", err)
	}
	if _, err := g.Store("global fact", nil); err != nil {
		t.Fatal(err)
	}
	if listed, err := p1.List(nil); err != nil || len(listed) != 0 {
		t.Fatalf("project/global isolation: %+v %v", listed, err)
	}
}

func memoryArgs(t *testing.T, v map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMemoryToolFlows(t *testing.T) {
	registry := memory.NewRegistry(t.TempDir())
	tool := tools.NewMemoryTool("proj-1", registry)
	ctx := context.Background()

	out, err := tool.Execute(ctx, memoryArgs(t, map[string]any{"op": "store", "content": "tabs over spaces", "tags": []string{"prefs"}}))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	stored, _ := out.(string)
	if stored == "" || len(stored) < 10 {
		t.Fatalf("store output: %q", stored)
	}

	out, err = tool.Execute(ctx, memoryArgs(t, map[string]any{"op": "recall", "query": "tabs"}))
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if text, _ := out.(string); text == "" {
		t.Fatalf("recall output: %q", text)
	}

	out, err = tool.Execute(ctx, memoryArgs(t, map[string]any{"op": "list", "scope": "global"}))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if text, _ := out.(string); text == "" {
		t.Fatalf("list output: %q", text)
	}

	if _, err := tool.Execute(ctx, memoryArgs(t, map[string]any{"op": "store"})); err == nil {
		t.Fatal("store without content must fail")
	}
	if _, err := tool.Execute(ctx, memoryArgs(t, map[string]any{"op": "recall"})); err == nil {
		t.Fatal("recall without query/tags must fail")
	}
	if _, err := tool.Execute(ctx, memoryArgs(t, map[string]any{"op": "bogus"})); err == nil {
		t.Fatal("unknown op must fail")
	}
	if _, err := tool.Execute(ctx, memoryArgs(t, map[string]any{"op": "delete"})); err == nil {
		t.Fatal("delete without id must fail")
	}

	headless := tools.NewMemoryTool("proj-1", nil)
	if _, err := headless.Execute(ctx, memoryArgs(t, map[string]any{"op": "list"})); err == nil {
		t.Fatal("nil registry must fail")
	}
	noProject := tools.NewMemoryTool("", registry)
	if _, err := noProject.Execute(ctx, memoryArgs(t, map[string]any{"op": "list"})); err == nil {
		t.Fatal("project scope without project must fail")
	}
	if _, err := noProject.Execute(ctx, memoryArgs(t, map[string]any{"op": "list", "scope": "global"})); err != nil {
		t.Fatalf("global scope needs no project: %v", err)
	}
}

func TestMemoryToolInRun(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.SetMemories(memory.NewRegistry(t.TempDir()))
	storeArgs, _ := json.Marshal(map[string]any{"op": "store", "content": "remember the alamo", "tags": []string{"test"}})
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event {
				return []loop.Event{{Kind: loop.EventToolCall, Call: &tools.ToolCall{ID: "m1", Name: "memory", Arguments: storeArgs}}}
			},
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "stored"}} },
		}}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "remember this", Provider: "mock", ModelID: "m", ApprovalMode: "full-access"})
	if err != nil {
		t.Fatal(err)
	}
	helpers.WaitSettled(t, hub)
	if hub.Outcome != run.OutcomeDone {
		t.Fatalf("outcome: %s", hub.Outcome)
	}
}
