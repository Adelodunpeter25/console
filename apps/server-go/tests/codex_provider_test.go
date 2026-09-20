// Codex provider coverage: schema normalization, usage, request body,
// OAuth helpers, and SSE tool-call reassembly against a mock HTTP server.
package tests

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
)

func sseBody(events []map[string]any) string {
	var b strings.Builder
	for _, e := range events {
		raw, _ := json.Marshal(e)
		fmt.Fprintf(&b, "data: %s\n\n", raw)
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

func testCredential() (codex.ParsedCredential, error) {
	return codex.ParsedCredential{AccessToken: "tok", RefreshToken: "", ExpiresAtMs: 1<<62 - 1, AccountID: "acc-1"}, nil
}

func mustTestCredential(t *testing.T) codex.ParsedCredential {
	t.Helper()
	cred, err := testCredential()
	if err != nil {
		t.Fatal(err)
	}
	return cred
}

func collectEvents(t *testing.T, srv *httptest.Server) []loop.Event {
	t.Helper()
	p := &codex.Provider{BaseURL: srv.URL, CredentialLoader: testCredential}
	events := stream.New[loop.Event]()
	req := loop.TurnRequest{Model: "gpt-5.6-luna", SystemPrompt: "Use tools.", Messages: []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}}}
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

func toolEvents(in []loop.Event) []loop.Event {
	var out []loop.Event
	for _, e := range in {
		if e.Kind == loop.EventToolCall {
			out = append(out, e)
		}
	}
	return out
}

func TestCodexToolCallCorrelation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody([]map[string]any{
			{"type": "response.output_item.added", "item": map[string]any{"type": "function_call", "id": "item-1", "call_id": "call-1", "name": "listDir", "arguments": ""}},
			{"type": "response.function_call_arguments.delta", "item_id": "item-1", "delta": `{"path":`},
			{"type": "response.function_call_arguments.delta", "item_id": "item-1", "delta": `"."}`},
			{"type": "response.function_call_arguments.done", "item_id": "item-1", "name": "listDir", "arguments": `{"path":"."}`},
		}))
	}))
	defer srv.Close()
	calls := toolEvents(collectEvents(t, srv))
	if len(calls) != 1 {
		t.Fatalf("expected single finalized tool event, got %d", len(calls))
	}
	if calls[0].Call.ID != "call-1" || calls[0].Call.Name != "listDir" {
		t.Fatalf("call identity: %+v", calls[0].Call)
	}
	var args map[string]any
	if err := json.Unmarshal(calls[0].Call.Arguments, &args); err != nil || args["path"] != "." {
		t.Fatalf("args: %s %v", calls[0].Call.Arguments, err)
	}
}

func TestCodexFinalizedOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody([]map[string]any{
			{"type": "response.output_item.added", "item": map[string]any{"type": "function_call", "id": "item-2", "call_id": "call-2", "name": "listDir"}},
			{"type": "response.function_call_arguments.done", "item_id": "item-2", "name": "listDir", "arguments": `{"path":"."}`},
		}))
	}))
	defer srv.Close()
	calls := toolEvents(collectEvents(t, srv))
	if len(calls) != 1 || calls[0].Call.ID != "call-2" {
		t.Fatalf("finalized-only: %+v", calls)
	}
}

func TestCodexOutOfOrderBuffering(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody([]map[string]any{
			{"type": "response.function_call_arguments.delta", "item_id": "item-3", "delta": `{"path":"."}`, "arguments": ""},
			{"type": "response.output_item.added", "item": map[string]any{"type": "function_call", "id": "item-3", "call_id": "call-3", "name": "listDir"}},
		}))
	}))
	defer srv.Close()
	calls := toolEvents(collectEvents(t, srv))
	if len(calls) != 1 || calls[0].Call.ID != "call-3" {
		t.Fatalf("out-of-order: %+v", calls)
	}
	var args map[string]any
	if err := json.Unmarshal(calls[0].Call.Arguments, &args); err != nil || args["path"] != "." {
		t.Fatalf("buffered args: %s %v", calls[0].Call.Arguments, err)
	}
}

func TestCodexTextAndHitUsage(t *testing.T) {
	var gotSession, gotBody map[string]any
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody([]map[string]any{
			{"type": "response.output_text.delta", "delta": "hi"},
			{"type": "response.completed", "response": map[string]any{"usage": map[string]any{
				"input_tokens": 100, "input_tokens_details": map[string]any{"cached_tokens": 80},
				"output_tokens": 5, "total_tokens": 105,
			}}},
		}))
	}))
	defer srv.Close()
	p := &codex.Provider{BaseURL: srv.URL, CredentialLoader: testCredential}
	events := stream.New[loop.Event]()
	req := loop.TurnRequest{Model: "gpt-5.6-luna", Messages: []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}}, ConversationID: "conv-stable-1", CacheRetention: loop.CacheShort}
	if err := p.RunTurn(context.Background(), req, events); err != nil {
		t.Fatal(err)
	}
	var sawText bool
	var usage *loop.TurnUsage
	for {
		ev, err, ok := events.Next()
		if !ok {
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		if ev.Kind == loop.EventText && ev.Text == "hi" {
			sawText = true
		}
		if ev.Kind == loop.EventUsage {
			usage = ev.Usage
		}
	}
	if !sawText {
		t.Fatal("missing text delta")
	}
	if usage == nil || usage.CacheStatus != loop.CacheHit || usage.CacheRead != 80 || usage.Input != 20 {
		t.Fatalf("usage: %+v", usage)
	}
	_ = gotSession
	if gotHeaders.Get("session_id") != "conv-stable-1" || gotHeaders.Get("conversation_id") != "conv-stable-1" {
		t.Fatalf("session headers: %v", gotHeaders)
	}
	if gotBody["prompt_cache_key"] != "conv-stable-1" {
		t.Fatalf("prompt_cache_key: %v", gotBody)
	}
	if _, ok := gotBody["prompt_cache_retention"]; ok {
		t.Fatalf("short retention must omit 24h field: %v", gotBody)
	}
}

func TestCodexNormalizeUsage(t *testing.T) {
	hit := codex.NormalizeUsage(map[string]any{
		"input_tokens": 100, "input_tokens_details": map[string]any{"cached_tokens": 80},
		"output_tokens": 5, "total_tokens": 105,
	}, loop.CacheShort)
	if hit.CacheStatus != loop.CacheHit || hit.Input != 20 || hit.CacheRead != 80 {
		t.Fatalf("hit: %+v", hit)
	}
	if codex.NormalizeUsage(map[string]any{"output_tokens": 5}, loop.CacheShort) != nil {
		t.Fatal("missing input must return nil")
	}
	none := codex.NormalizeUsage(map[string]any{"input_tokens": 50.0, "output_tokens": 5.0, "total_tokens": 55.0}, loop.CacheNone)
	if none.CacheStatus != loop.CacheUnsupported {
		t.Fatalf("none: %+v", none)
	}
	if codex.NormalizeUsage(map[string]any{"response": map[string]any{}}, loop.CacheShort) != nil {
		t.Fatal("non-usage shape must return nil")
	}
	reason := codex.NormalizeUsage(map[string]any{
		"input_tokens": 100, "input_tokens_details": map[string]any{"cached_tokens": 0},
		"output_tokens": 80, "output_tokens_details": map[string]any{"reasoning_tokens": 60},
		"total_tokens": 180,
	}, loop.CacheShort)
	if reason.CacheStatus != loop.CacheMiss || reason.ReasoningTokens == nil || *reason.ReasoningTokens != 60 {
		t.Fatalf("reasoning: %+v", reason)
	}
}

func TestCodexNormalizeSchema(t *testing.T) {
	out, ok := codex.NormalizeSchema(map[string]any{"exclusiveMinimum": true, "minimum": 2.0}).(map[string]any)
	if !ok || out["exclusiveMinimum"] != 2.0 {
		t.Fatalf("draft upgrade: %v", out)
	}
	out, _ = codex.NormalizeSchema(map[string]any{"exclusiveMinimum": false}).(map[string]any)
	if _, ok := out["exclusiveMinimum"]; ok {
		t.Fatalf("false must be dropped: %v", out)
	}
}

func TestCodexRequestBodyCacheControls(t *testing.T) {
	msgs := []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}}
	longBody := codex.BuildRequestBody("m", "sys", msgs, nil, loop.CacheLong, "conv-1", "low")
	if longBody["prompt_cache_retention"] != "24h" || longBody["prompt_cache_key"] != "conv-1" {
		t.Fatalf("long: %v", longBody)
	}
	if r, ok := longBody["reasoning"].(map[string]any); !ok || r["effort"] != "low" {
		t.Fatalf("reasoning: %v", longBody)
	}
	noneBody := codex.BuildRequestBody("m", "", msgs, nil, loop.CacheNone, "conv-1", "")
	if _, ok := noneBody["prompt_cache_key"]; ok {
		t.Fatalf("none must omit cache key: %v", noneBody)
	}
	defs := []tools.Definition{{Name: "t", Description: "d", InputSchema: map[string]any{"type": "object"}}}
	withTools := codex.BuildRequestBody("m", "", msgs, defs, loop.CacheShort, "c", "")
	toolList, ok := withTools["tools"].([]map[string]any)
	if !ok || len(toolList) != 1 || toolList[0]["name"] != "t" {
		t.Fatalf("tools: %v", withTools)
	}
}

func testJWT(accountID, email string) string {
	payload, _ := json.Marshal(map[string]any{
		"https://api.openai.com/auth":    map[string]any{"chatgpt_account_id": accountID},
		"https://api.openai.com/profile": map[string]any{"email": email},
	})
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestCodexOAuthHelpers(t *testing.T) {
	accountID, email, _ := codex.TokenProfile(testJWT("acc-9", "User@Example.com"), "")
	if accountID != "acc-9" || email != "user@example.com" {
		t.Fatalf("profile: %q %q", accountID, email)
	}
	verifier, challenge, err := codex.GeneratePKCE()
	if err != nil || verifier == "" || challenge == "" || verifier == challenge {
		t.Fatalf("pkce: %q %q %v", verifier, challenge, err)
	}
	authURL, redirect := codex.AuthorizationURL("state-1", challenge)
	if !strings.Contains(authURL, "state-1") || !strings.Contains(authURL, "code_challenge") || !strings.HasSuffix(redirect, "/auth/callback") {
		t.Fatalf("auth url: %s %s", authURL, redirect)
	}
	if _, err := codex.ParseCredential(codex.OAuthCredential{}); err == nil {
		t.Fatal("empty credential must fail")
	}
	dir := t.TempDir()
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "codex-creds.json"))
	cred := codex.OAuthCredential{AccessToken: "a", RefreshToken: "r", ExpiresAt: 123, AccountID: "acc"}
	if err := codex.SaveCredential(cred); err != nil {
		t.Fatal(err)
	}
	if !codex.CredentialExists() {
		t.Fatal("credential should exist after save")
	}
	loaded, err := codex.LoadCredential()
	if err != nil || loaded.AccountID != "acc" {
		t.Fatalf("load: %+v %v", loaded, err)
	}
	fresh, err := codex.RefreshIfNeeded(nil, codex.ParsedCredential{AccessToken: "a", RefreshToken: "r", ExpiresAtMs: 1<<62 - 1, AccountID: "acc"})
	if err != nil || fresh.AccessToken != "a" {
		t.Fatalf("fresh must not refresh: %+v %v", fresh, err)
	}
}

func TestCodexEnvToken(t *testing.T) {
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", testJWT("env-acc", ""))
	cred, err := codex.LoadCredential()
	if err != nil || cred.AccountID != "env-acc" {
		t.Fatalf("env token: %+v %v", cred, err)
	}
	if !codex.CredentialExists() {
		t.Fatal("env token must count as existing")
	}
}

func TestCodexFetchModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("chatgpt-account-id") != "acc-1" {
			t.Errorf("account header: %q", r.Header.Get("chatgpt-account-id"))
		}
		fmt.Fprint(w, `{"models":[{"slug":"gpt-5.6-luna","context_window":272000,"input_modalities":["text","image"]},{"id":"gpt-5-mini"}]}`)
	}))
	defer srv.Close()
	models, err := codex.FetchModels(context.Background(), srv.Client(), srv.URL, mustTestCredential(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "gpt-5.6-luna" || !models[0].SupportsImages || models[1].ContextWindow != 272000 {
		t.Fatalf("models: %+v", models)
	}
}

func TestCodexConvertInput(t *testing.T) {
	msgs := []any{
		loop.UserMessage{Role: loop.RoleUser, Content: "hello"},
		loop.AssistantMessage{Role: loop.RoleAssistant, ID: "m1", Content: []any{loop.TextPart{Type: "text", Text: "hi"}}},
		loop.ToolResultMessage{Role: loop.RoleToolResult, Results: []tools.ToolResult{{ToolCallID: "c1", Content: "out"}}},
	}
	input := codex.ConvertInput(msgs)
	if len(input) != 3 {
		t.Fatalf("input items: %v", input)
	}
	_ = os.Getenv("PATH")
}
