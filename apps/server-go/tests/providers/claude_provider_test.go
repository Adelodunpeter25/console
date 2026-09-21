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
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
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
	if len(sys) != 1 {
		t.Fatalf("system = %v", body["system"])
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

func TestClaudeModelsSeed(t *testing.T) {
	seed := claude.DefaultModels()
	if len(seed) != 4 {
		t.Fatalf("seed = %+v", seed)
	}
	for _, m := range seed {
		if m.Provider != "claude" || len(m.ThinkingLevels) != 5 || m.DefaultThinking != "low" {
			t.Fatalf("model = %+v", m)
		}
	}
}
