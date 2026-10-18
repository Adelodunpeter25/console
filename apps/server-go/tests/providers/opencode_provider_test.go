package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/opencode"
)

func TestOpenCodeDiscoveryAndChatProvider(t *testing.T) {
	t.Setenv("OPENCODE_USER_AGENT", opencode.DefaultUserAgent)
	t.Setenv("OPENCODE_SESSION_ID", "ses_test_chat")
	// Set an explicit test key rather than relying on the ambient store, so the
	// header assertion below checks a value this test owns. TestMain already
	// redirects the store to a temp dir; setting the variable too keeps the
	// expected value visible next to the assertion.
	t.Setenv("OPENCODE_API_KEY", "oc_sk_test_key")
	var modelCalls, chatCalls int
	var chatRequest map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Compared one at a time rather than dumping the whole header set on
		// failure: a dump prints the Authorization value, and if the store were
		// ever misconfigured that writes a real credential into the test log.
		if got := r.Header.Get("Authorization"); got != "Bearer oc_sk_test_key" {
			t.Errorf("Authorization: got %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != opencode.DefaultUserAgent {
			t.Errorf("User-Agent: got %q", got)
		}
		if got := r.Header.Get("x-opencode-client"); got != "cli" {
			t.Errorf("x-opencode-client: got %q", got)
		}
		if got := r.Header.Get("x-opencode-session"); got != "ses_test_chat" {
			t.Errorf("x-opencode-session: got %q", got)
		}
		switch r.URL.Path {
		case "/models":
			modelCalls++
			_, _ = w.Write([]byte(`{"data":[{"id":"space-bunny"},{"id":"kimi-k3"}]}`))
		case "/chat/completions":
			chatCalls++
			if r.Method != http.MethodPost {
				t.Errorf("chat method: %s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&chatRequest); err != nil {
				t.Errorf("decode chat request: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think\",\"content\":\"hello\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\"}}]}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"/tmp\\\"}\"}}]}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":3,\"total_tokens\":13}}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := &opencode.Provider{BaseURL: server.URL, HTTPClient: server.Client()}
	events, err := runOpenCodeTurn(provider, loop.TurnRequest{
		Model:          "space-bunny",
		SystemPrompt:   "system",
		Messages:       []any{loop.UserMessage{Role: loop.RoleUser, Content: "read it"}},
		Tools:          openCodeToolDefinitions(),
		ConversationID: "session:opencode:space-bunny",
	})
	if err != nil {
		t.Fatal(err)
	}
	if modelCalls != 1 || chatCalls != 1 {
		t.Fatalf("calls: models=%d chat=%d", modelCalls, chatCalls)
	}
	if got := chatRequest["model"]; got != "space-bunny" {
		t.Fatalf("model: %v", got)
	}
	messages, _ := chatRequest["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages: %#v", chatRequest["messages"])
	}
	if first, _ := messages[0].(map[string]any); first["role"] != "system" || first["content"] != "system" {
		t.Fatalf("system message: %#v", messages[0])
	}
	if second, _ := messages[1].(map[string]any); second["role"] != "user" {
		t.Fatalf("user message: %#v", messages[1])
	}
	toolNames := chatToolNames(chatRequest["tools"])
	if !reflect.DeepEqual(toolNames[:6], []string{"edit", "glob", "grep", "question", "read", "shell"}) {
		t.Fatalf("compatibility tools: %v", toolNames)
	}
	if !contains(toolNames, "read_file") || !contains(toolNames, "custom_tool") {
		t.Fatalf("real tools missing: %v", toolNames)
	}
	if countName(toolNames, "glob") != 1 || countName(toolNames, "grep") != 1 {
		t.Fatalf("duplicate tools: %v", toolNames)
	}
	if len(events) != 4 || events[0].Kind != loop.EventText || events[0].Text != "hello" || events[1].Kind != loop.EventThinking || events[2].Kind != loop.EventToolCall || events[3].Kind != loop.EventUsage {
		t.Fatalf("events: %+v", events)
	}
	if events[2].Call.ID != "call_1" || events[2].Call.Name != "read_file" || string(events[2].Call.Arguments) != `{"path":"/tmp"}` {
		t.Fatalf("tool call: %+v", events[2].Call)
	}
	if events[3].Usage == nil || events[3].Usage.Input != 10 || events[3].Usage.Output != 3 {
		t.Fatalf("usage: %+v", events[3].Usage)
	}
}

func TestOpenCodeResponsesProviderAndCaching(t *testing.T) {
	t.Setenv("OPENCODE_USER_AGENT", opencode.DefaultUserAgent)
	t.Setenv("OPENCODE_SESSION_ID", "ses_test_responses")
	var modelCalls, responsesCalls int
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			modelCalls++
			_, _ = w.Write([]byte(`{"data":[{"id":"muse-spark-1.3-contributor"}]}`))
		case "/responses":
			responsesCalls++
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode responses request: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"answer\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"item-1\",\"type\":\"function_call\",\"call_id\":\"call-1\",\"name\":\"read_file\",\"arguments\":\"\"}}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"item-1\",\"delta\":\"{\\\"path\\\":\\\"/tmp\\\"}\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.function_call_arguments.done\",\"item_id\":\"item-1\",\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\\\"/tmp\\\"}\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":20,\"output_tokens\":4,\"total_tokens\":24,\"input_tokens_details\":{\"cached_tokens\":5}}}}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := &opencode.Provider{BaseURL: server.URL, HTTPClient: server.Client()}
	events, err := runOpenCodeTurn(provider, loop.TurnRequest{
		Model:          "muse-spark-1.3-contributor",
		SystemPrompt:   "system",
		Messages:       []any{loop.UserMessage{Role: loop.RoleUser, Content: "use a tool"}},
		Tools:          openCodeToolDefinitions(),
		ConversationID: "session:opencode:muse",
		CacheRetention: loop.CacheLong,
	})
	if err != nil {
		t.Fatal(err)
	}
	if modelCalls != 1 || responsesCalls != 1 {
		t.Fatalf("calls: models=%d responses=%d", modelCalls, responsesCalls)
	}
	if request["instructions"] != "system" || request["prompt_cache_key"] != "session:opencode:muse" || request["prompt_cache_retention"] != "24h" {
		t.Fatalf("responses cache/request: %#v", request)
	}
	if len(events) != 3 || events[0].Kind != loop.EventText || events[1].Kind != loop.EventToolCall || events[2].Kind != loop.EventUsage {
		t.Fatalf("events: %+v", events)
	}
	if string(events[1].Call.Arguments) != `{"path":"/tmp"}` {
		t.Fatalf("tool arguments: %s", events[1].Call.Arguments)
	}
	if events[2].Usage == nil || events[2].Usage.CacheRead != 5 || events[2].Usage.Input != 15 || events[2].Usage.CacheStatus != loop.CacheHit {
		t.Fatalf("usage: %+v", events[2].Usage)
	}
}

func TestOpenCodeUnknownModelFailsBeforeInference(t *testing.T) {
	var inferenceCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"space-bunny"}]}`))
			return
		}
		inferenceCalls++
		http.Error(w, "unexpected inference", http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := &opencode.Provider{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := runOpenCodeTurn(provider, loop.TurnRequest{Model: "not-listed-free", Messages: []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("error: %v", err)
	}
	if inferenceCalls != 0 {
		t.Fatalf("inference calls: %d", inferenceCalls)
	}
}

func TestOpenCodeConversions(t *testing.T) {
	messages := []any{
		loop.UserMessage{Role: loop.RoleUser, Content: "inspect", Attachments: []loop.ImageAttachment{{MimeType: "image/png", Data: "abc"}}},
		loop.AssistantMessage{Role: loop.RoleAssistant, Content: []any{
			loop.ThinkingPart{Type: "thinking", Text: "hidden"},
			loop.TextPart{Type: "text", Text: "answer"},
			loop.ToolCallPart{Type: "toolCall", Call: tools.ToolCall{ID: "c1", Name: "read_file", Arguments: json.RawMessage(`{"path":"/tmp"}`)}},
		}},
		loop.ToolResultMessage{Role: loop.RoleToolResult, Results: []tools.ToolResult{{ToolCallID: "c1", Content: []map[string]any{{"type": "text", "text": "done"}}}}},
	}
	chat := opencode.ConvertChatMessages(messages)
	if len(chat) != 3 {
		t.Fatalf("chat messages: %#v", chat)
	}
	if _, ok := chat[1]["tool_calls"]; !ok {
		t.Fatalf("chat tool call missing: %#v", chat[1])
	}
	responses := opencode.ConvertResponsesMessages(messages)
	if len(responses) != 4 {
		t.Fatalf("responses messages: %#v", responses)
	}
	if responses[1]["content"] == nil || responses[2]["type"] != "function_call" || responses[3]["type"] != "function_call_output" {
		t.Fatalf("responses shape: %#v", responses)
	}
	fallback := opencode.ConvertChatMessages(nil)
	if fallback[0]["content"] != "(continue)" {
		t.Fatalf("fallback: %#v", fallback)
	}
}

func TestOpenCodeThinkingLevelsComeFromMetadata(t *testing.T) {
	// Seeded rather than hardcoded: effort vocabularies are upstream's to
	// publish, not ours to invent.
	opencode.UseMetadata(map[string]opencode.ModelMetadata{
		"muse-spark-1.3-contributor": {
			ThinkingLevels: []string{"minimal", "low", "medium", "high", "xhigh"},
		},
		"deepseek-v4.1-flash": {
			// The real vocabulary. Note there is no "medium": this is exactly
			// the case a hardcoded default got wrong.
			ThinkingLevels: []string{"low", "high", "max"},
		},
		"beam-free": {},
	})
	t.Cleanup(func() { opencode.UseMetadata(nil) })

	if got := opencode.ThinkingLevelsFor("muse-spark-1.3-contributor"); len(got) != 5 {
		t.Fatalf("muse levels: %#v", got)
	}
	if got := opencode.ThinkingLevelsFor("deepseek-v4.1-flash"); !reflect.DeepEqual(got, []string{"low", "high", "max"}) {
		t.Fatalf("deepseek levels: %#v", got)
	}
	// A model the catalogue knows but that does no tunable reasoning offers no
	// picker at all.
	if got := opencode.ThinkingLevelsFor("beam-free"); got != nil {
		t.Fatalf("expected no levels for beam-free, got %#v", got)
	}
	// A model the catalogue has never heard of is also nil — which callers
	// read as "use the provider default", not as a claim that it has no
	// reasoning.
	if got := opencode.ThinkingLevelsFor("some-future-model"); got != nil {
		t.Fatalf("expected no levels for an uncatalogued model, got %#v", got)
	}
}

func TestOpenCodeDefaultThinkingIsAMemberOfTheLevels(t *testing.T) {
	// The bug this replaces: a hardcoded "medium" default was shipped for every
	// model, including ones whose vocabulary is low/high/max.
	cases := []struct {
		levels []string
		want   string
	}{
		{[]string{"low", "high", "max"}, "high"},
		{[]string{"minimal", "low", "medium", "high", "xhigh"}, "medium"},
		{[]string{"low", "medium", "high", "xhigh", "max"}, "high"},
		{[]string{"only"}, "only"},
		{nil, ""},
	}
	for _, tc := range cases {
		got := opencode.DefaultThinkingFor(tc.levels)
		if got != tc.want {
			t.Fatalf("DefaultThinkingFor(%v) = %q, want %q", tc.levels, got, tc.want)
		}
		if got != "" && !slices.Contains(tc.levels, got) {
			t.Fatalf("default %q is not a member of %v", got, tc.levels)
		}
	}
}

func TestOpenCodeFetchModelsUsesMetadataForCapabilities(t *testing.T) {
	opencode.UseMetadata(map[string]opencode.ModelMetadata{
		"kimi-k3": {
			ContextWindow:  262144,
			SupportsImages: true,
			ThinkingLevels: []string{"low", "high", "max"},
		},
		"glm-5.3": {
			ContextWindow: 204800,
		},
	})
	t.Cleanup(func() { opencode.UseMetadata(nil) })

	t.Setenv("OPENCODE_USER_AGENT", opencode.DefaultUserAgent)
	t.Setenv("OPENCODE_API_KEY", "sk-test")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"kimi-k3","object":"model"},
			{"id":"glm-5.3","object":"model"},
			{"id":"uncovered-model","object":"model"}
		]}`))
	}))
	defer server.Close()

	discovered, err := opencode.FetchModels(t.Context(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetch models: %v", err)
	}
	// Every model the endpoint lists is returned. The old implementation kept
	// only "-free" ids, which silently discarded 35 of Go's 37.
	if len(discovered) != 3 {
		t.Fatalf("discovered: %#v", discovered)
	}

	byID := map[string]int{}
	for i, model := range discovered {
		byID[model.ID] = i
		if model.Provider != "opencode" {
			t.Fatalf("provider: %#v", model)
		}
	}

	kimi := discovered[byID["kimi-k3"]]
	if kimi.ContextWindow != 262144 || !kimi.SupportsImages {
		t.Fatalf("kimi capabilities: %#v", kimi)
	}
	if kimi.DefaultThinking != "high" {
		t.Fatalf("kimi default should be the midpoint of low/high/max, got %q", kimi.DefaultThinking)
	}

	glm := discovered[byID["glm-5.3"]]
	if glm.ContextWindow != 204800 {
		t.Fatalf("glm context: %#v", glm)
	}
	if glm.SupportsImages {
		t.Fatalf("glm should not claim image support: %#v", glm)
	}

	// A model the catalogue does not cover is still listed — with a
	// conservative window and no levels — rather than hidden or defaulted to 0.
	uncovered := discovered[byID["uncovered-model"]]
	if uncovered.ContextWindow != opencode.DefaultContextWindow {
		t.Fatalf("uncovered context window: %#v", uncovered)
	}
	if len(uncovered.ThinkingLevels) != 0 || uncovered.DefaultThinking != "" {
		t.Fatalf("uncovered model must not invent levels: %#v", uncovered)
	}
}

func TestOpenCodeSendsThinkingLevelOnBothRoutes(t *testing.T) {
	t.Setenv("OPENCODE_USER_AGENT", opencode.DefaultUserAgent)
	t.Setenv("OPENCODE_SESSION_ID", "ses_test_thinking")
	var chatBody, responsesBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"space-bunny"},{"id":"muse-spark-1.3-contributor"}]}`))
		case "/chat/completions":
			if err := json.NewDecoder(r.Body).Decode(&chatBody); err != nil {
				t.Errorf("decode chat: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case "/responses":
			if err := json.NewDecoder(r.Body).Decode(&responsesBody); err != nil {
				t.Errorf("decode responses: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := &opencode.Provider{BaseURL: server.URL, HTTPClient: server.Client()}

	if _, err := runOpenCodeTurn(provider, loop.TurnRequest{
		Model:         "space-bunny",
		Messages:      []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}},
		ThinkingLevel: "high",
	}); err != nil {
		t.Fatalf("chat turn: %v", err)
	}
	if chatBody["reasoning_effort"] != "high" {
		t.Fatalf("chat reasoning_effort: %#v", chatBody)
	}

	if _, err := runOpenCodeTurn(provider, loop.TurnRequest{
		Model:         "muse-spark-1.3-contributor",
		Messages:      []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}},
		ThinkingLevel: "xhigh",
	}); err != nil {
		t.Fatalf("responses turn: %v", err)
	}
	reasoning, _ := responsesBody["reasoning"].(map[string]any)
	if reasoning["effort"] != "xhigh" {
		t.Fatalf("responses reasoning: %#v", responsesBody)
	}
}

func TestOpenCodeCapabilitiesFallBackWhenUncatalogued(t *testing.T) {
	// Replaces a test that pinned a hand-written 1M floor onto one specific
	// model id. With capabilities sourced from models.dev there is no curated
	// floor left to assert; what matters now is that an uncatalogued model is
	// still listed, with a conservative window and no invented claims.
	opencode.UseMetadata(map[string]opencode.ModelMetadata{
		"space-bunny": {
			ContextWindow:  1_000_000,
			SupportsImages: true,
			ThinkingLevels: []string{"low", "medium", "high", "xhigh", "max"},
		},
		// Advertised without a context limit: must fall back, not report zero.
		"no-window-model": {SupportsImages: false},
	})
	t.Cleanup(func() { opencode.UseMetadata(nil) })

	t.Setenv("OPENCODE_USER_AGENT", opencode.DefaultUserAgent)
	t.Setenv("OPENCODE_API_KEY", "oc_sk_test_key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"space-bunny"},{"id":"no-window-model"},{"id":"uncatalogued"}]}`))
	}))
	defer server.Close()

	discovered, err := opencode.FetchModels(t.Context(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetch models: %v", err)
	}
	byID := map[string]int{}
	for i, model := range discovered {
		byID[model.ID] = i
	}

	bunny := discovered[byID["space-bunny"]]
	if bunny.ContextWindow != 1_000_000 || !bunny.SupportsImages {
		t.Fatalf("space bunny: %+v", bunny)
	}
	if bunny.DefaultThinking != "high" {
		t.Fatalf("space bunny default: %q", bunny.DefaultThinking)
	}

	// A catalogued model with no published limit must not report 0, which
	// would silently break every context calculation downstream.
	if got := discovered[byID["no-window-model"]].ContextWindow; got != opencode.DefaultContextWindow {
		t.Fatalf("no-window-model context: %d", got)
	}
	if discovered[byID["no-window-model"]].SupportsImages {
		t.Fatal("no-window-model must not claim image support")
	}

	// Uncatalogued: listed, conservative, no fabricated capabilities.
	unc := discovered[byID["uncatalogued"]]
	if unc.ContextWindow != opencode.DefaultContextWindow || unc.SupportsImages {
		t.Fatalf("uncatalogued: %+v", unc)
	}
	if len(unc.ThinkingLevels) != 0 || unc.DefaultThinking != "" {
		t.Fatalf("uncatalogued must not invent levels: %+v", unc)
	}
}

func TestOpenCodeOmitsThinkingWhenUnset(t *testing.T) {
	t.Setenv("OPENCODE_USER_AGENT", opencode.DefaultUserAgent)
	t.Setenv("OPENCODE_SESSION_ID", "ses_test_unset")
	var chatBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"space-bunny"}]}`))
		case "/chat/completions":
			if err := json.NewDecoder(r.Body).Decode(&chatBody); err != nil {
				t.Errorf("decode chat: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := &opencode.Provider{BaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := runOpenCodeTurn(provider, loop.TurnRequest{
		Model:    "space-bunny",
		Messages: []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}},
	}); err != nil {
		t.Fatalf("chat turn: %v", err)
	}
	if _, ok := chatBody["reasoning_effort"]; ok {
		t.Fatalf("reasoning_effort should be omitted when unset: %#v", chatBody)
	}
}

func openCodeToolDefinitions() []tools.Definition {
	registry := tools.NewRegistry(tools.Glob, tools.Grep, tools.ReadFile, tools.EditFile, tools.Bash, tools.Ask)
	return append(registry.Definitions(), tools.Definition{Name: "custom_tool", Description: "custom", InputSchema: map[string]any{"type": "object"}})
}

func runOpenCodeTurn(provider *opencode.Provider, req loop.TurnRequest) ([]loop.Event, error) {
	events := stream.New[loop.Event]()
	err := provider.RunTurn(context.Background(), req, events)
	var out []loop.Event
	for {
		event, streamErr, ok := events.Next()
		if !ok {
			if streamErr != nil && err == nil {
				err = streamErr
			}
			return out, err
		}
		out = append(out, event)
	}
}

func chatToolNames(value any) []string {
	items, _ := value.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		object, _ := item.(map[string]any)
		function, _ := object["function"].(map[string]any)
		name, _ := function["name"].(string)
		out = append(out, name)
	}
	return out
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func countName(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}
