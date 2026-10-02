// Claude provider coverage: message/tool conversion, request body, OAuth
// helpers, usage normalization, and Anthropic SSE parsing against a mock
// HTTP server. Mirrors tests/providers/codex_provider_test.go.
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
)

func claudeSSEBody(frames [][2]string) string {
	var b strings.Builder
	for _, f := range frames {
		fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", f[0], f[1])
	}
	return b.String()
}

func testClaudeCredential() (claude.ParsedCredential, error) {
	return claude.ParsedCredential{AccessToken: "tok", RefreshToken: "", ExpiresAtMs: 1<<62 - 1}, nil
}

func collectClaudeEvents(t *testing.T, srv *httptest.Server, model string) []loop.Event {
	t.Helper()
	p := &claude.Provider{BaseURL: srv.URL, CredentialLoader: testClaudeCredential}
	events := stream.New[loop.Event]()
	req := loop.TurnRequest{Model: model, SystemPrompt: "Use tools.", Messages: []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}}}
	if err := p.RunTurn(context.Background(), req, events); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	var out []loop.Event
	for {
		ev, err, ok := events.Next()
		if !ok {
			if err != nil {
				t.Fatalf("stream err: %v", err)
			}
			break
		}
		out = append(out, ev)
	}
	return out
}

func kindsOf(events []loop.Event) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, string(e.Kind))
	}
	return out
}

func TestClaudeTextAndUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q", got)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer tok" {
			t.Errorf("auth = %q", auth)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, claudeSSEBody([][2]string{
			{"message_start", `{"message":{"usage":{"input_tokens":10,"cache_read_input_tokens":4,"cache_creation_input_tokens":2}}}`},
			{"content_block_delta", `{"index":0,"delta":{"type":"text_delta","text":"hello"}}`},
			{"message_delta", `{"usage":{"output_tokens":5}}`},
		}))
	}))
	defer srv.Close()

	events := collectClaudeEvents(t, srv, "claude-sonnet-4-6")
	var texts []string
	var usage *loop.TurnUsage
	for _, e := range events {
		switch e.Kind {
		case loop.EventText:
			texts = append(texts, e.Text)
		case loop.EventUsage:
			usage = e.Usage
		}
	}
	if strings.Join(texts, "") != "hello" {
		t.Fatalf("texts = %q", texts)
	}
	if usage == nil || usage.Input != 6 || usage.CacheRead != 4 || usage.CacheWrite != 2 || usage.Output != 5 || usage.CacheStatus != loop.CacheHit {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestClaudeToolCallReassembly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, claudeSSEBody([][2]string{
			{"content_block_start", `{"index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_file"}}`},
			{"content_block_delta", `{"index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`},
			{"content_block_delta", `{"index":1,"delta":{"type":"input_json_delta","partial_json":"\"a\"}"}}`},
			{"content_block_stop", `{"index":1}`},
		}))
	}))
	defer srv.Close()

	events := collectClaudeEvents(t, srv, "claude-sonnet-4-6")
	var calls []tools.ToolCall
	for _, e := range events {
		if e.Kind == loop.EventToolCall && e.Call != nil {
			calls = append(calls, *e.Call)
		}
	}
	if len(calls) != 1 || calls[0].ID != "toolu_1" || calls[0].Name != "read_file" {
		t.Fatalf("calls = %+v", calls)
	}
	var args map[string]any
	if err := json.Unmarshal(calls[0].Arguments, &args); err != nil || args["path"] != "a" {
		t.Fatalf("args = %s", calls[0].Arguments)
	}
}

func TestClaudeStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, claudeSSEBody([][2]string{
			{"error", `{"error":{"message":"overloaded"}}`},
		}))
	}))
	defer srv.Close()

	p := &claude.Provider{BaseURL: srv.URL, CredentialLoader: testClaudeCredential}
	events := stream.New[loop.Event]()
	req := loop.TurnRequest{Model: "m", Messages: []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}}}
	if err := p.RunTurn(context.Background(), req, events); err == nil {
		t.Fatal("expected stream error")
	}
}

func TestClaudeConvertMessages(t *testing.T) {
	msgs := []any{
		loop.UserMessage{Role: loop.RoleUser, Content: "hi"},
		loop.AssistantMessage{Role: loop.RoleAssistant, ID: "a", Content: []any{
			loop.TextPart{Type: "text", Text: "doing"},
			loop.ToolCallPart{Type: "toolCall", Call: tools.ToolCall{ID: "c1", Name: "read_file", Arguments: json.RawMessage(`{"path":"x"}`)}},
		}, StopReason: loop.StopToolUse},
		loop.ToolResultMessage{Role: loop.RoleToolResult, Results: []tools.ToolResult{
			{ToolCallID: "c1", ToolName: "read_file", Content: "contents"},
		}},
	}
	converted := claude.ConvertMessages(msgs, loop.CacheShort)
	if len(converted) < 3 {
		t.Fatalf("turns = %+v", converted)
	}
	if converted[0].Role != "user" || converted[1].Role != "assistant" || converted[2].Role != "user" {
		t.Fatalf("roles = %v", converted)
	}
	foundToolUse, foundResult := false, false
	for _, block := range converted[1].Content {
		if block["type"] == "tool_use" && block["id"] == "c1" {
			foundToolUse = true
		}
	}
	for _, block := range converted[2].Content {
		if block["type"] == "tool_result" && block["tool_use_id"] == "c1" {
			foundResult = true
		}
	}
	if !foundToolUse || !foundResult {
		t.Fatalf("blocks = %+v", converted)
	}
	last := converted[len(converted)-1]
	if _, ok := last.Content[len(last.Content)-1]["cache_control"]; !ok {
		t.Fatal("missing trailing cache breakpoint")
	}
}

func TestClaudeNormalizeSchema(t *testing.T) {
	schema := map[string]any{"exclusiveMinimum": true, "minimum": 3.0, "exclusiveMaximum": false}
	got, _ := claude.NormalizeSchema(schema).(map[string]any)
	if got["exclusiveMinimum"] != 3.0 {
		t.Fatalf("schema = %v", got)
	}
	if _, ok := got["exclusiveMaximum"]; ok {
		t.Fatalf("schema = %v", got)
	}
}

func TestClaudeThinkingMapping(t *testing.T) {
	if got := claude.MapThinkingLevel("high"); got != "high" {
		t.Fatalf("high = %q", got)
	}
	if got := claude.MapThinkingLevel("none"); got != "" {
		t.Fatalf("none = %q", got)
	}
	if got := claude.MapThinkingLevel("minimal"); got != "" {
		t.Fatalf("minimal = %q", got)
	}
}

func TestClaudeRequestBody(t *testing.T) {
	body := claude.BuildRequestBody("claude-sonnet-4-6", "sys", []any{
		loop.UserMessage{Role: loop.RoleUser, Content: "hi"},
	}, nil, loop.CacheShort, "high")
	if body["model"] != "claude-sonnet-4-6" || body["max_tokens"] != claude.MaxOutputTokens {
		t.Fatalf("body = %v", body)
	}
	thinking, _ := body["thinking"].(map[string]any)
	if thinking["budget_tokens"] != claude.ThinkingBudgetTokens {
		t.Fatalf("thinking = %v", thinking)
	}
	cfg, _ := body["output_config"].(map[string]any)
	if cfg["effort"] != "high" {
		t.Fatalf("output_config = %v", cfg)
	}
	sys, _ := body["system"].([]any)
	if len(sys) != 2 {
		t.Fatalf("system = %v", body["system"])
	}
	// Anthropic rejects OAuth requests whose first system block is not the
	// Claude Code identity line: premium models 429, only Haiku answers.
	first, _ := sys[0].(map[string]any)
	if first["text"] != claude.ClaudeCodeSystemInstruction {
		t.Fatalf("system[0] must be the Claude Code identity: %v", sys[0])
	}
	if _, ok := first["cache_control"]; ok {
		t.Fatalf("identity block must not carry a cache breakpoint: %v", first)
	}
	second, _ := sys[1].(map[string]any)
	if second["text"] != "sys" {
		t.Fatalf("system[1] must be the caller prompt: %v", sys[1])
	}
	noCache := claude.BuildRequestBody("m", "sys", nil, nil, loop.CacheNone, "none")
	if _, ok := noCache["output_config"]; ok {
		t.Fatalf("none/minimal must omit effort: %v", noCache)
	}
}

func TestClaudeOAuthHelpers(t *testing.T) {
	authURL, redirect := claude.AuthorizationURL("state-1", "challenge-1")
	if !strings.Contains(authURL, "claude.ai/oauth/authorize") || !strings.Contains(authURL, "state-1") {
		t.Fatalf("authURL = %s", authURL)
	}
	if !strings.HasPrefix(redirect, "http://localhost:") {
		t.Fatalf("redirect = %s", redirect)
	}
	if _, err := claude.ParseCredential(claude.OAuthCredential{}); err == nil {
		t.Fatal("empty credential must fail")
	}
	cred, err := claude.ParseCredential(claude.OAuthCredential{
		AccessToken: "a", RefreshToken: "r", ExpiresAt: 999,
	})
	if err != nil || cred.AccessToken != "a" {
		t.Fatalf("parse = %+v %v", cred, err)
	}
}

func TestClaudeDiscoveryMapsWindows(t *testing.T) {
	claude.InvalidateModelCache()
	t.Cleanup(claude.InvalidateModelCache)
	dir := t.TempDir()
	credPath := filepath.Join(dir, "claude-creds.json")
	if err := os.WriteFile(credPath, []byte(`{"access_token":"tok","refresh_token":"r","expiresAt":9999999999999}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CREDENTIALS_PATH", credPath)
	prev := claude.DiscoveryBaseURL
	claude.DiscoveryBaseURL = ""
	t.Cleanup(func() { claude.DiscoveryBaseURL = prev })

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if !strings.HasSuffix(r.URL.Path, "/v1/models") {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[
			{"id":"claude-opus-4-6","max_input_tokens":1000000,"capabilities":{"image_input":{"supported":true}}},
			{"id":"claude-haiku-4-5","max_input_tokens":200000},
			{"id":"mystery-model"},
			{"id":"","max_input_tokens":5}
		]}`)
	}))
	defer srv.Close()
	claude.DiscoveryBaseURL = srv.URL

	models := providers.ClaudeModels(context.Background())
	byID := map[string]int{}
	for i, m := range models {
		byID[m.ID] = i
	}
	for id, want := range map[string]int{
		"claude-opus-4-6": 1000000, "claude-haiku-4-5": 200000, "mystery-model": 200000,
	} {
		i, ok := byID[id]
		if !ok {
			t.Fatalf("missing %s: %+v", id, models)
		}
		if models[i].ContextWindow != want || models[i].Provider != "claude" {
			t.Fatalf("%s: %+v", id, models[i])
		}
	}
	if !models[byID["claude-opus-4-6"]].SupportsImages {
		t.Fatalf("images not mapped: %+v", models[byID["claude-opus-4-6"]])
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d; want 1", hits.Load())
	}

	// Second call serves the cache: no second fetch.
	again := providers.ClaudeModels(context.Background())
	if len(again) != len(models) || hits.Load() != 1 {
		t.Fatalf("cache miss: %d models, %d hits", len(again), hits.Load())
	}

	// Case-insensitive hit with the discovered window.
	found, ok := claude.ResolveModel(context.Background(), "CLAUDE-OPUS-4-6")
	if !ok || found.ContextWindow != 1000000 {
		t.Fatalf("resolve: %+v %v", found, ok)
	}
	if _, ok := claude.ResolveModel(context.Background(), "nope"); ok {
		t.Fatal("unknown id must miss")
	}

	// Seed path agrees once the snapshot is warm.
	if found, ok := providers.FindModel("claude", "claude-opus-4-6"); !ok || found.ContextWindow != 1000000 {
		t.Fatalf("find: %+v %v", found, ok)
	}
}

func TestClaudeLoggedOutIsEmpty(t *testing.T) {
	claude.InvalidateModelCache()
	t.Cleanup(claude.InvalidateModelCache)
	t.Setenv("CLAUDE_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "missing.json"))
	if models := providers.ClaudeModels(context.Background()); len(models) != 0 {
		t.Fatalf("logged out: %+v", models)
	}
	raw, err := json.Marshal(claude.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatalf("snapshot serializes as %s; want []", raw)
	}
}

func TestConvertMessages_ToolResultKeepsImageBlock(t *testing.T) {
	msgs := claude.ConvertMessages([]any{
		loop.UserMessage{Role: loop.RoleUser, Content: "look"},
		loop.AssistantMessage{Role: loop.RoleAssistant, ID: "a", Content: []any{
			loop.ToolCallPart{Type: "toolCall", Call: tools.ToolCall{ID: "c1", Name: "browser", Arguments: json.RawMessage(`{"action":"screenshot"}`)}},
		}, StopReason: loop.StopToolUse},
		loop.ToolResultMessage{Role: loop.RoleToolResult, Results: []tools.ToolResult{{
			ToolCallID: "c1", ToolName: "browser",
			Content: []map[string]any{
				{"type": "text", "text": "Screenshot attached."},
				{"type": "image", "data": "AAAA", "mimeType": "image/png"},
			},
		}}},
	}, loop.CacheShort)
	raw, _ := json.Marshal(msgs)
	if !strings.Contains(string(raw), `"media_type":"image/png"`) || strings.Contains(string(raw), `[Tool result:`) {
		t.Fatalf("image block missing from tool_result: %s", raw)
	}
}
