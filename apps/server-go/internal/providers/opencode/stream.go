// OpenCode Zen streaming provider. It implements loop.Provider and routes
// model families to either Chat Completions or Responses.
package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// Provider streams direct OpenCode Zen turns. BaseURL and HTTPClient are
// overridable for tests; the default client is intentionally unlimited so
// long-running model streams are not cut off.
type Provider struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (p *Provider) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	return BaseURL()
}

func (p *Provider) httpClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 0}
}

// RunTurn implements loop.Provider. Discovery is a deliberate preflight for
// every inference request, including requests using a test server.
func (p *Provider) RunTurn(ctx context.Context, req loop.TurnRequest, events *stream.Stream[loop.Event]) error {
	if err := p.ensureModel(ctx, req.Model); err != nil {
		events.Fail(err)
		return err
	}

	var err error
	if IsResponsesModel(req.Model) {
		err = p.runResponses(ctx, req, events)
	} else {
		err = p.runChat(ctx, req, events)
	}
	if err != nil {
		events.Fail(err)
	}
	return err
}

func (p *Provider) ensureModel(ctx context.Context, modelID string) error {
	preflightCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	models, err := FetchModels(preflightCtx, p.httpClient(), p.baseURL())
	if err != nil {
		return fmt.Errorf("OpenCode model discovery: %w", err)
	}
	for _, model := range models {
		if model.ID == modelID {
			return nil
		}
	}
	return fmt.Errorf("OpenCode model %q is not available in the free model list", modelID)
}

func (p *Provider) postJSON(ctx context.Context, path string, body map[string]any) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	SetCommonHeaders(req, "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := shared.HTTPError("OpenCode", resp.StatusCode, resp.Status, resp.Body)
		resp.Body.Close()
		return nil, err
	}
	return resp, nil
}

func (p *Provider) runChat(ctx context.Context, req loop.TurnRequest, events *stream.Stream[loop.Event]) error {
	body := map[string]any{
		"model":    req.Model,
		"messages": ConvertChatMessages(req.Messages),
		"stream":   true,
		"stream_options": map[string]any{
			"include_usage": true,
		},
	}
	toolDefinitions := MergeToolDefinitions(req.Tools)
	if len(toolDefinitions) > 0 {
		body["tools"] = ConvertChatTools(toolDefinitions)
	}
	resp, err := p.postJSON(ctx, ChatCompletionsURL(p.baseURL()), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	calls := newChatCallAccumulator()
	var usage any
	parseErr := shared.ParseSSE(ctx, resp.Body, func(event map[string]any) error {
		if err := streamError(event); err != nil {
			return err
		}
		if value, ok := event["usage"]; ok {
			usage = value
		}
		choices, _ := event["choices"].([]any)
		if len(choices) == 0 {
			return nil
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			return nil
		}
		if text := stringValue(delta["content"]); text != "" {
			events.Push(loop.Event{Kind: loop.EventText, Text: text})
		}
		if text := stringValue(delta["reasoning_content"]); text != "" {
			events.Push(loop.Event{Kind: loop.EventThinking, Text: text})
		} else if text := stringValue(delta["reasoning"]); text != "" {
			events.Push(loop.Event{Kind: loop.EventThinking, Text: text})
		}
		rawCalls, _ := delta["tool_calls"].([]any)
		for _, raw := range rawCalls {
			if err := calls.add(raw); err != nil {
				return err
			}
		}
		return nil
	})

	if err := calls.flush(); err != nil {
		return err
	}
	for _, call := range calls.calls() {
		events.Push(loop.Event{Kind: loop.EventToolCall, Call: &tools.ToolCall{
			ID:        call.id,
			Name:      call.name,
			Arguments: json.RawMessage(call.arguments),
		}})
	}
	events.Push(loop.Event{Kind: loop.EventUsage, Usage: NormalizeChatUsage(usage)})
	if parseErr != nil {
		return parseErr
	}
	events.Complete()
	return nil
}

func (p *Provider) runResponses(ctx context.Context, req loop.TurnRequest, events *stream.Stream[loop.Event]) error {
	body := map[string]any{
		"model":  req.Model,
		"input":  ConvertResponsesMessages(req.Messages),
		"stream": true,
		"store":  false,
	}
	if strings.TrimSpace(req.SystemPrompt) != "" {
		body["instructions"] = req.SystemPrompt
	}
	toolDefinitions := MergeToolDefinitions(req.Tools)
	if len(toolDefinitions) > 0 {
		body["tools"] = ConvertResponsesTools(toolDefinitions)
	}
	if req.CacheRetention != loop.CacheNone && req.ConversationID != "" {
		body["prompt_cache_key"] = req.ConversationID
		if req.CacheRetention == loop.CacheLong {
			body["prompt_cache_retention"] = "24h"
		}
	}
	if req.ThinkingLevel != "" {
		body["reasoning"] = map[string]any{"effort": req.ThinkingLevel}
	}

	resp, err := p.postJSON(ctx, ResponsesURL(p.baseURL()), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	acc := shared.NewAccumulator()
	var usage any
	parseErr := shared.ParseSSE(ctx, resp.Body, func(event map[string]any) error {
		if err := streamError(event); err != nil {
			return err
		}
		typ := stringValue(event["type"])
		switch typ {
		case "response.output_text.delta", "response.refusal.delta":
			if delta := stringValue(event["delta"]); delta != "" {
				events.Push(loop.Event{Kind: loop.EventText, Text: delta})
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			if delta := stringValue(event["delta"]); delta != "" {
				events.Push(loop.Event{Kind: loop.EventThinking, Text: delta})
			}
		case "response.output_item.added":
			item, _ := event["item"].(map[string]any)
			if item == nil || stringValue(item["type"]) != "function_call" {
				return nil
			}
			callID := stringValue(item["call_id"])
			name := stringValue(item["name"])
			if callID == "" || name == "" {
				return nil
			}
			itemID := stringValue(item["id"])
			if itemID == "" {
				itemID = callID
			}
			if call := acc.Add(itemID, callID, name, stringValue(item["arguments"])); call != nil {
				if err := emitCall(events, call); err != nil {
					return err
				}
			}
		case "response.function_call_arguments.delta":
			acc.Delta(stringValue(event["item_id"]), stringValue(event["delta"]))
		case "response.function_call_arguments.done":
			if call := acc.Done(stringValue(event["item_id"]), stringValue(event["name"]), stringValue(event["arguments"])); call != nil {
				if err := emitCall(events, call); err != nil {
					return err
				}
			}
		case "response.completed", "response.incomplete":
			response, _ := event["response"].(map[string]any)
			if response != nil {
				if value, ok := response["usage"]; ok {
					usage = value
				}
				if typ == "response.incomplete" {
					if err := responseError(response); err != nil {
						return err
					}
				}
			}
		case "response.failed":
			response, _ := event["response"].(map[string]any)
			if err := responseError(response); err != nil {
				return err
			}
			return fmt.Errorf("OpenCode Responses request failed")
		}
		return nil
	})

	for _, call := range acc.Unfinalized() {
		if err := emitCall(events, call); err != nil {
			return err
		}
	}
	events.Push(loop.Event{Kind: loop.EventUsage, Usage: NormalizeResponsesUsage(usage, req.CacheRetention)})
	if parseErr != nil {
		return parseErr
	}
	events.Complete()
	return nil
}

func streamError(event map[string]any) error {
	if value, ok := event["error"]; ok {
		if text := stringValue(value); text != "" {
			return fmt.Errorf("OpenCode stream error: %s", text)
		}
		if object, ok := value.(map[string]any); ok {
			if text := stringValue(object["message"]); text != "" {
				return fmt.Errorf("OpenCode stream error: %s", text)
			}
		}
	}
	return nil
}

func responseError(response map[string]any) error {
	if response == nil {
		return nil
	}
	if value, ok := response["error"]; ok {
		if text := stringValue(value); text != "" {
			return fmt.Errorf("OpenCode Responses error: %s", text)
		}
		if object, ok := value.(map[string]any); ok {
			if text := stringValue(object["message"]); text != "" {
				return fmt.Errorf("OpenCode Responses error: %s", text)
			}
		}
	}
	return nil
}

func emitCall(events *stream.Stream[loop.Event], call *shared.Call) error {
	if call == nil || call.Emitted {
		return nil
	}
	call.Emitted = true
	arguments := call.Arguments
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	if !json.Valid([]byte(arguments)) {
		return fmt.Errorf("OpenCode returned invalid tool arguments for %s: %q", call.Name, arguments)
	}
	events.Push(loop.Event{Kind: loop.EventToolCall, Call: &tools.ToolCall{
		ID:        call.CallID,
		Name:      call.Name,
		Arguments: json.RawMessage(arguments),
	}})
	return nil
}

type chatCall struct {
	id        string
	name      string
	arguments string
}

type chatCallAccumulator struct {
	byIndex map[int]*chatCall
	order   []int
}

func newChatCallAccumulator() *chatCallAccumulator {
	return &chatCallAccumulator{byIndex: map[int]*chatCall{}}
}

func (a *chatCallAccumulator) add(raw any) error {
	object, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("OpenCode returned an invalid tool call")
	}
	index := intValue(object["index"])
	if _, exists := a.byIndex[index]; !exists {
		a.byIndex[index] = &chatCall{}
		a.order = append(a.order, index)
	}
	call := a.byIndex[index]
	if value := stringValue(object["id"]); value != "" {
		call.id = value
	}
	if function, ok := object["function"].(map[string]any); ok {
		if value := stringValue(function["name"]); value != "" {
			call.name = value
		}
		if value := stringValue(function["arguments"]); value != "" {
			call.arguments += value
		}
	}
	return nil
}

func (a *chatCallAccumulator) flush() error {
	for _, index := range a.order {
		call := a.byIndex[index]
		if call == nil {
			continue
		}
		if call.id == "" {
			call.id = fmt.Sprintf("opencode_call_%d", index)
		}
		arguments := call.arguments
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		if !json.Valid([]byte(arguments)) {
			return fmt.Errorf("OpenCode returned invalid tool arguments for %s: %q", call.name, arguments)
		}
		// Chat stream calls are returned by the caller through a local slice
		// so the loop receives one complete EventToolCall per call.
		a.byIndex[index].arguments = arguments
	}
	return nil
}

func (a *chatCallAccumulator) calls() []*chatCall {
	out := make([]*chatCall, 0, len(a.order))
	for _, index := range a.order {
		out = append(out, a.byIndex[index])
	}
	return out
}

func intValue(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case json.Number:
		parsed, _ := number.Int64()
		return int(parsed)
	default:
		return 0
	}
}

// NormalizeChatUsage maps Chat Completions usage without claiming native
// prompt-cache support when Zen does not report cache details.
func NormalizeChatUsage(value any) *loop.TurnUsage {
	usage, ok := value.(map[string]any)
	if !ok || usage == nil {
		return &loop.TurnUsage{CacheStatus: loop.CacheUnsupported}
	}
	input, hasInput := shared.NumberField(usage, "prompt_tokens")
	if !hasInput {
		return &loop.TurnUsage{CacheStatus: loop.CacheUnsupported}
	}
	output := shared.NumberFieldOr(usage, "completion_tokens", 0)
	total := shared.NumberFieldOr(usage, "total_tokens", input+output)
	cacheRead, hasCache := nestedNumber(usage, "prompt_tokens_details", "cached_tokens")
	if !hasCache {
		cacheRead, hasCache = nestedNumber(usage, "prompt_tokens_details", "cache_read_tokens")
	}
	status := loop.CacheUnsupported
	if hasCache {
		status = loop.CacheMiss
		if cacheRead > 0 {
			status = loop.CacheHit
		}
	}
	uncached := input
	if hasCache {
		uncached -= cacheRead
		if uncached < 0 {
			uncached = 0
		}
	}
	return &loop.TurnUsage{
		Input:       int(uncached),
		CacheRead:   int(cacheRead),
		Output:      int(output),
		TotalTokens: int(total),
		CacheStatus: status,
		ReasoningTokens: func() *int {
			value, ok := nestedNumber(usage, "completion_tokens_details", "reasoning_tokens")
			if !ok {
				return nil
			}
			reasoning := int(value)
			return &reasoning
		}(),
	}
}

// NormalizeResponsesUsage maps Responses usage and reports cache status from
// the fields Zen actually returns.
func NormalizeResponsesUsage(value any, retention loop.CacheRetention) *loop.TurnUsage {
	usage, ok := value.(map[string]any)
	if !ok || usage == nil {
		status := loop.CacheUnknown
		if retention == loop.CacheNone {
			status = loop.CacheUnsupported
		}
		return &loop.TurnUsage{CacheStatus: status}
	}
	input, hasInput := shared.NumberField(usage, "input_tokens")
	if !hasInput {
		status := loop.CacheUnknown
		if retention == loop.CacheNone {
			status = loop.CacheUnsupported
		}
		return &loop.TurnUsage{CacheStatus: status}
	}
	output := shared.NumberFieldOr(usage, "output_tokens", 0)
	total := shared.NumberFieldOr(usage, "total_tokens", input+output)
	cacheRead, hasCache := nestedNumber(usage, "input_tokens_details", "cached_tokens")
	uncached := input
	status := loop.CacheUnknown
	if hasCache {
		uncached -= cacheRead
		if uncached < 0 {
			uncached = 0
		}
		status = loop.CacheMiss
		if cacheRead > 0 {
			status = loop.CacheHit
		}
	}
	return &loop.TurnUsage{
		Input:       int(uncached),
		CacheRead:   int(cacheRead),
		Output:      int(output),
		TotalTokens: int(total),
		CacheStatus: status,
		ReasoningTokens: func() *int {
			value, ok := nestedNumber(usage, "output_tokens_details", "reasoning_tokens")
			if !ok {
				return nil
			}
			reasoning := int(value)
			return &reasoning
		}(),
	}
}

func nestedNumber(m map[string]any, parent, key string) (float64, bool) {
	nested, ok := m[parent].(map[string]any)
	if !ok {
		return 0, false
	}
	return shared.NumberField(nested, key)
}
