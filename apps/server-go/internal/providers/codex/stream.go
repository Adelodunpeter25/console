// Codex Responses streaming provider. Port of
// apps/server/providers/src/codex/stream-fn.ts: request body, SSE parsing,
// function-call reassembly, usage normalization. Implements loop.Provider
// so the agent loop drives turns without changes.
//
// Generic SSE, JSON-number, and call-reassembly helpers live in
// providers/shared; this file keeps Codex request/response shapes.
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// Provider streams Codex Responses turns. HTTPClient and BaseURL are
// overridable for tests; CredentialLoader defaults to OAuth file/env.
type Provider struct {
	BaseURL          string
	HTTPClient       *http.Client
	CredentialLoader func() (ParsedCredential, error)
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

func (p *Provider) loadCredential() (ParsedCredential, error) {
	if p.CredentialLoader != nil {
		return p.CredentialLoader()
	}
	cred, err := LoadCredential()
	if err != nil {
		return ParsedCredential{}, err
	}
	return RefreshIfNeeded(nil, cred)
}

// ConvertInput maps loop messages to Codex Responses `input` items.
func ConvertInput(messages []any) []map[string]any {
	var input []map[string]any
	for _, m := range messages {
		switch msg := m.(type) {
		case loop.UserMessage:
			content := []map[string]any{{"type": "input_text", "text": msg.Content}}
			for _, a := range msg.Attachments {
				content = append(content, map[string]any{
					"type":      "input_image",
					"image_url": fmt.Sprintf("data:%s;base64,%s", a.MimeType, a.Data),
					"detail":    "auto",
				})
			}
			input = append(input, map[string]any{"role": "user", "content": content})
		case *loop.UserMessage:
			if msg != nil {
				input = append(input, ConvertInput([]any{*msg})...)
			}
		case loop.AssistantMessage:
			for _, part := range msg.Content {
				switch part := part.(type) {
				case loop.TextPart:
					if part.Text != "" {
						input = append(input, map[string]any{
							"role":    "assistant",
							"content": []map[string]any{{"type": "output_text", "text": part.Text}},
						})
					}
				case map[string]any:
					if part["type"] == "text" {
						if text, _ := part["text"].(string); text != "" {
							input = append(input, map[string]any{
								"role":    "assistant",
								"content": []map[string]any{{"type": "output_text", "text": text}},
							})
						}
					} else if part["type"] == "toolCall" {
						if call, ok := part["call"].(map[string]any); ok {
							args := ""
							switch v := call["arguments"].(type) {
							case string:
								args = v
							case json.RawMessage:
								args = string(v)
							default:
								if v != nil {
									raw, _ := json.Marshal(v)
									args = string(raw)
								}
							}
							input = append(input, map[string]any{
								"type":      "function_call",
								"call_id":   call["id"],
								"name":      call["name"],
								"arguments": args,
							})
						}
					}
				case loop.ToolCallPart:
					args := string(part.Call.Arguments)
					if args == "" {
						args = "{}"
					}
					input = append(input, map[string]any{
						"type":      "function_call",
						"call_id":   part.Call.ID,
						"name":      part.Call.Name,
						"arguments": args,
					})
				}
			}
		case *loop.AssistantMessage:
			if msg != nil {
				input = append(input, ConvertInput([]any{*msg})...)
			}
		case loop.ToolResultMessage:
			for _, r := range msg.Results {
				input = append(input, map[string]any{
					"type":    "function_call_output",
					"call_id": r.ToolCallID,
					"output":  shared.ToolResultText(r.Content),
				})
			}
		case *loop.ToolResultMessage:
			if msg != nil {
				input = append(input, ConvertInput([]any{*msg})...)
			}
		}
	}
	if input == nil {
		return []map[string]any{}
	}
	return input
}

// ConvertTools maps tool definitions to Codex function tools.
func ConvertTools(defs []tools.Definition) []map[string]any {
	out := make([]map[string]any, 0, len(defs))
	for _, t := range defs {
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		out = append(out, map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters":  NormalizeSchema(schema),
			"strict":      false,
		})
	}
	return out
}

// NormalizeSchema upgrades draft-07 boolean exclusiveMinimum/Maximum to
// draft-2020-12 numeric form, mirroring normalizeCodexSchema in TS.
func NormalizeSchema(value any) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = NormalizeSchema(item)
		}
		return out
	case map[string]any:
		normalized := make(map[string]any, len(v))
		for key, child := range v {
			normalized[key] = NormalizeSchema(child)
		}
		if em, ok := normalized["exclusiveMinimum"]; ok {
			if b, ok := em.(bool); ok {
				if b {
					if min, ok := normalized["minimum"].(float64); ok {
						normalized["exclusiveMinimum"] = min
					} else {
						delete(normalized, "exclusiveMinimum")
					}
				} else {
					delete(normalized, "exclusiveMinimum")
				}
			}
		}
		if em, ok := normalized["exclusiveMaximum"]; ok {
			if b, ok := em.(bool); ok {
				if b {
					if max, ok := normalized["maximum"].(float64); ok {
						normalized["exclusiveMaximum"] = max
					} else {
						delete(normalized, "exclusiveMaximum")
					}
				} else {
					delete(normalized, "exclusiveMaximum")
				}
			}
		}
		return normalized
	default:
		return value
	}
}

// NormalizeUsage converts a Codex Responses usage block to TurnUsage.
// Missing usage returns nil so callers can report cacheStatus "unknown".
func NormalizeUsage(usage any, retention loop.CacheRetention) *loop.TurnUsage {
	m, ok := usage.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	input, hasInput := shared.NumberField(m, "input_tokens")
	output := shared.NumberFieldOr(m, "output_tokens", 0)
	total := shared.NumberFieldOr(m, "total_tokens", input+output)
	if !hasInput {
		return nil
	}
	var cached *float64
	var reasoning *int
	if details, ok := m["input_tokens_details"].(map[string]any); ok {
		if c, ok := shared.NumberField(details, "cached_tokens"); ok {
			cached = &c
		}
	}
	if outDetails, ok := m["output_tokens_details"].(map[string]any); ok {
		if r, ok := shared.NumberField(outDetails, "reasoning_tokens"); ok {
			ri := int(r)
			reasoning = &ri
		}
	}
	cacheRead := 0
	uncached := int(input)
	var status loop.CacheStatus
	if cached != nil {
		cacheRead = int(*cached)
		uncached = int(input) - cacheRead
		if uncached < 0 {
			uncached = 0
		}
		if *cached > 0 {
			status = loop.CacheHit
		} else {
			status = loop.CacheMiss
		}
	} else if retention == loop.CacheNone {
		status = loop.CacheUnsupported
	} else {
		status = loop.CacheUnknown
	}
	return &loop.TurnUsage{
		Input:           uncached,
		CacheRead:       cacheRead,
		CacheWrite:      0,
		Output:          int(output),
		ReasoningTokens: reasoning,
		TotalTokens:     int(total),
		CacheStatus:     status,
	}
}

// BuildRequestBody assembles the Codex Responses request body with prompt
// cache controls and optional reasoning effort.
func BuildRequestBody(modelID, systemPrompt string, messages []any, toolDefs []tools.Definition, retention loop.CacheRetention, promptCacheKey, thinkingLevel string) map[string]any {
	body := map[string]any{
		"model":  modelID,
		"input":  ConvertInput(messages),
		"stream": true,
		"store":  false,
	}
	if strings.TrimSpace(systemPrompt) != "" {
		body["instructions"] = systemPrompt
	}
	if len(toolDefs) > 0 {
		body["tools"] = ConvertTools(toolDefs)
	}
	if retention != loop.CacheNone {
		body["prompt_cache_key"] = promptCacheKey
		if retention == loop.CacheLong {
			body["prompt_cache_retention"] = "24h"
		}
	}
	if thinkingLevel != "" {
		body["reasoning"] = map[string]any{"effort": thinkingLevel}
	}
	return body
}

// emitCall pushes a single EventToolCall per call. Empty arguments fall back
// to {} (mirrors TS parseToolCallArguments: "" → {}). The Emitted flag
// prevents the done-handler + final flush from double-emitting; the Go loop
// appends one ToolCallPart per event, so unlike the TS streamFn we must NOT
// emit an initial empty event.
func emitCall(c *shared.Call) *loop.Event {
	if c.Emitted {
		return nil
	}
	c.Emitted = true
	args := c.Arguments
	if args == "" {
		args = "{}"
	}
	return &loop.Event{Kind: loop.EventToolCall, Call: &tools.ToolCall{ID: c.CallID, Name: c.Name, Arguments: json.RawMessage(args)}}
}

// RunTurn implements loop.Provider: one streaming assistant turn.
func (p *Provider) RunTurn(ctx context.Context, req loop.TurnRequest, events *stream.Stream[loop.Event]) error {
	cred, err := p.loadCredential()
	if err != nil {
		return err
	}
	sessionID := req.ConversationID
	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	promptCacheKey := sessionID
	body := BuildRequestBody(req.Model, req.SystemPrompt, req.Messages, req.Tools, req.CacheRetention, promptCacheKey, req.ThinkingLevel)
	rawBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", ResponsesURL(p.baseURL()), bytes.NewReader(rawBody))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	httpReq.Header.Set("chatgpt-account-id", cred.AccountID)
	httpReq.Header.Set("OpenAI-Beta", "responses=experimental")
	httpReq.Header.Set("originator", "pi")
	httpReq.Header.Set("version", ClientVersion)
	httpReq.Header.Set("session_id", sessionID)
	httpReq.Header.Set("conversation_id", sessionID)
	httpReq.Header.Set("x-client-request-id", sessionID)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient().Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return shared.HTTPError("Codex", resp.StatusCode, resp.Status, resp.Body)
	}

	acc := shared.NewAccumulator()
	var lastUsage any

	flushUsage := func() {
		usage := NormalizeUsage(lastUsage, req.CacheRetention)
		if usage == nil {
			status := loop.CacheUnknown
			if req.CacheRetention == loop.CacheNone {
				status = loop.CacheUnsupported
			}
			usage = &loop.TurnUsage{CacheStatus: status}
		}
		events.Push(loop.Event{Kind: loop.EventUsage, Usage: usage})
	}

	parseErr := shared.ParseSSE(ctx, resp.Body, func(event map[string]any) error {
		typ, _ := event["type"].(string)
		switch typ {
		case "error":
			msg := "Codex stream error"
			if e, ok := event["error"].(map[string]any); ok {
				if m, ok := e["message"].(string); ok && m != "" {
					msg = m
				}
			}
			return fmt.Errorf("%s", msg)
		case "response.output_text.delta", "response.refusal.delta":
			if delta, ok := event["delta"].(string); ok && delta != "" {
				events.Push(loop.Event{Kind: loop.EventText, Text: delta})
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			if delta, ok := event["delta"].(string); ok && delta != "" {
				events.Push(loop.Event{Kind: loop.EventThinking, Text: delta})
			}
		case "response.output_item.added":
			item, _ := event["item"].(map[string]any)
			if item == nil {
				break
			}
			if item["type"] != "function_call" {
				break
			}
			callID, _ := item["call_id"].(string)
			name, _ := item["name"].(string)
			if callID == "" || name == "" {
				break
			}
			itemID, _ := item["id"].(string)
			if itemID == "" {
				itemID = callID
			}
			args, _ := item["arguments"].(string)
			// Single emit only when already finalized (out-of-order done
			// arrived first). No initial empty event: the Go loop records
			// one ToolCallPart per event, so an empty + finalized pair
			// would execute the tool twice.
			if fin := acc.Add(itemID, callID, name, args); fin != nil {
				if ev := emitCall(fin); ev != nil {
					events.Push(*ev)
				}
			}
		case "response.function_call_arguments.delta":
			itemID, _ := event["item_id"].(string)
			fragment, _ := event["delta"].(string)
			acc.Delta(itemID, fragment)
		case "response.function_call_arguments.done":
			itemID, _ := event["item_id"].(string)
			arguments, _ := event["arguments"].(string)
			name, _ := event["name"].(string)
			if fin := acc.Done(itemID, name, arguments); fin != nil {
				if ev := emitCall(fin); ev != nil {
					events.Push(*ev)
				}
			}
		case "response.completed", "response.incomplete":
			if responseBlock, ok := event["response"].(map[string]any); ok {
				if u, ok := responseBlock["usage"]; ok {
					lastUsage = u
				}
				if typ == "response.incomplete" {
					if re, ok := responseBlock["error"].(map[string]any); ok {
						if msg, ok := re["message"].(string); ok && msg != "" {
							return fmt.Errorf("%s", msg)
						}
					}
				}
			}
		case "response.failed":
			if responseBlock, ok := event["response"].(map[string]any); ok {
				if re, ok := responseBlock["error"].(map[string]any); ok {
					if msg, ok := re["message"].(string); ok && msg != "" {
						return fmt.Errorf("%s", msg)
					}
				}
			}
			return fmt.Errorf("Codex response failed")
		}
		return nil
	})

	// Flush incomplete calls (stream ended with only deltas) and the final
	// usage delta before terminating — mirrors the TS finally block. The
	// usage push must precede Complete/Fail (Push after termination drops).
	for _, c := range acc.Unfinalized() {
		if ev := emitCall(c); ev != nil {
			events.Push(*ev)
		}
	}
	flushUsage()
	if parseErr != nil {
		events.Fail(parseErr)
		return parseErr
	}
	events.Complete()
	return nil
}
