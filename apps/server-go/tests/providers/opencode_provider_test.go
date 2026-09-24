package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	var modelCalls, chatCalls int
	var chatRequest map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer public" ||
			r.Header.Get("User-Agent") != opencode.DefaultUserAgent ||
			r.Header.Get("x-opencode-client") != "cli" ||
			r.Header.Get("x-opencode-session") != "ses_test_chat" {
			t.Errorf("missing OpenCode headers: %#v", r.Header)
		}
		switch r.URL.Path {
		case "/models":
			modelCalls++
			_, _ = w.Write([]byte(`{"data":[{"id":"space-bunny-free"},{"id":"paid-model"}]}`))
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
		Model:          "space-bunny-free",
		SystemPrompt:   "system",
		Messages:       []any{loop.UserMessage{Role: loop.RoleUser, Content: "read it"}},
		Tools:          openCodeToolDefinitions(),
		ConversationID: "session:opencode:space-bunny-free",
	})
	if err != nil {
		t.Fatal(err)
	}
	if modelCalls != 1 || chatCalls != 1 {
		t.Fatalf("calls: models=%d chat=%d", modelCalls, chatCalls)
	}
	if got := chatRequest["model"]; got != "space-bunny-free" {
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
			_, _ = w.Write([]byte(`{"data":[{"id":"muse-spark-1.3-contributor-free"}]}`))
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
		Model:          "muse-spark-1.3-contributor-free",
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
			_, _ = w.Write([]byte(`{"data":[{"id":"space-bunny-free"}]}`))
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
