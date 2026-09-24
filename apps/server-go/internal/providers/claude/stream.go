// Claude Messages streaming provider. Port of
// apps/server/providers/src/claude/stream-fn.ts: request body, Anthropic
// SSE parsing, tool_use reassembly, usage normalization. Implements
// loop.Provider so the agent loop drives turns without changes.
package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// Provider streams Claude Messages turns. HTTPClient and BaseURL are
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

func mapStainlessOS(platform string) string {
	switch strings.ToLower(platform) {
	case "darwin":
		return "MacOS"
	case "windows", "win32":
		return "Windows"
	case "linux":
		return "Linux"
	case "freebsd":
		return "FreeBSD"
	default:
		return "Other:" + strings.ToLower(platform)
	}
}

func mapStainlessArch(arch string) string {
	switch strings.ToLower(arch) {
	case "amd64", "x64":
		return "x64"
	case "arm64", "aarch64":
		return "arm64"
	case "386", "x86", "ia32":
		return "x86"
	default:
		return "other:" + strings.ToLower(arch)
	}
}

// buildHeaders assembles the request headers, including the per-session
// identity (X-Claude-Code-Session-Id) that lets Anthropic distinguish
// concurrent sessions under one OAuth subscription — mirroring Claude
// Code's own CLI. sessionID is omitted from the headers when empty.
func buildHeaders(accessToken, sessionID string) map[string]string {
	headers := map[string]string{
		"Authorization":   "Bearer " + accessToken,
		"Content-Type":    "application/json",
		"Accept":          "text/event-stream",
		"anthropic-version": "2023-06-01",
		"anthropic-beta":    strings.Join(OAuthBetas, ","),
		"anthropic-dangerous-direct-browser-access": "true",
		"User-Agent":           UserAgent(),
		"X-Stainless-Arch":     mapStainlessArch(runtime.GOARCH),
		"X-Stainless-Lang":     "go",
		"X-Stainless-OS":       mapStainlessOS(runtime.GOOS),
		"X-Stainless-Package-Version": SDKVersion,
		"X-Stainless-Retry-Count":     "0",
		"X-Stainless-Runtime":         "go",
		"X-Stainless-Runtime-Version": runtime.Version(),
		"X-Stainless-Timeout":         "600",
		"x-app":                       "cli",
	}
	if sessionID != "" {
		headers["X-Claude-Code-Session-Id"] = sessionID
	}
	return headers
}

// MapThinkingLevel maps Console thinking levels to Anthropic output
// effort. Claude has no "none"/"minimal" — those omit the param.
func MapThinkingLevel(level string) string {
	switch level {
	case "low", "medium", "high", "xhigh", "max":
		return level
	default:
		return ""
	}
}

// ClaudeCodeSystemInstruction is the identity block Anthropic requires as
// the first system block on OAuth (subscription) requests. Without it the
// premium models reject the call with an opaque 429 rate_limit_error;
// only Haiku answers. Claude Code's own CLI sends the same line.
const ClaudeCodeSystemInstruction = "You are Claude Code, Anthropic's official CLI for Claude."

// BuildRequestBody assembles the Messages request: the required Claude Code
// identity block ahead of the caller's system prompt, always-on extended
// thinking (medium budget, interleaved beta keeps tool use working),
// optional effort mapping, ephemeral cache breakpoints.
func BuildRequestBody(modelID, systemPrompt string, messages []any, toolDefs []tools.Definition, retention loop.CacheRetention, thinkingLevel string) map[string]any {
	trimmed := strings.TrimSpace(systemPrompt)
	body := map[string]any{
		"model":      modelID,
		"max_tokens": MaxOutputTokens,
		"thinking":   map[string]any{"type": "enabled", "budget_tokens": ThinkingBudgetTokens},
	}
	if effort := MapThinkingLevel(thinkingLevel); effort != "" {
		body["output_config"] = map[string]any{"effort": effort}
	}
	system := []any{map[string]any{"type": "text", "text": ClaudeCodeSystemInstruction}}
	if trimmed != "" {
		block := map[string]any{"type": "text", "text": trimmed}
		if retention != loop.CacheNone {
			block["cache_control"] = map[string]any{"type": "ephemeral"}
		}
		system = append(system, block)
	}
	body["system"] = system
	body["messages"] = ConvertMessages(messages, retention)
	if converted := ConvertTools(toolDefs, retention); len(converted) > 0 {
		body["tools"] = converted
	}
	body["stream"] = true
	return body
}

// sseFrame is one named Anthropic SSE frame. Unlike the shared parser, the
// event name (message_start, content_block_delta, ...) is significant.
type sseFrame struct {
	event string
	data  map[string]any
}

// parseAnthropicSSE yields named frames, skipping [DONE]/blanks/malformed
// chunks. Port of parseAnthropicSse in TS.
func parseAnthropicSSE(ctx context.Context, r io.Reader, handle func(sseFrame) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	currentEvent := ""
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "event:"):
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			event := currentEvent
			if event == "" {
				event = "message"
			}
			currentEvent = ""
			if payload == "" || payload == "[DONE]" {
				continue
			}
			var data map[string]any
			if err := json.Unmarshal([]byte(payload), &data); err != nil {
				continue
			}
			if err := handle(sseFrame{event: event, data: data}); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

// toolCallState accumulates one tool_use block across content events.
type toolCallState struct {
	index     int
	id        string
	name      string
	arguments string
	finalized bool
}

// usageWire mirrors the Anthropic usage block shape.
type usageWire struct {
	InputTokens              *int `json:"input_tokens"`
	OutputTokens             *int `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
}

// NormalizeUsage maps Anthropic usage blocks to agent TurnUsage. Missing
// usage → cacheStatus "unknown", never a forced miss; retention "none"
// with no cache fields → "unsupported".
func NormalizeUsage(input, output map[string]any, retention loop.CacheRetention) *loop.TurnUsage {
	if input == nil && output == nil {
		return nil
	}
	prompt, hasPrompt := shared.NumberField(input, "input_tokens")
	if !hasPrompt {
		return nil
	}
	cached, hasCached := shared.NumberField(input, "cache_read_input_tokens")
	created, _ := shared.NumberField(input, "cache_creation_input_tokens")
	out, _ := shared.NumberField(output, "output_tokens")
	cacheRead := 0.0
	if hasCached {
		cacheRead = cached
	}
	var status loop.CacheStatus
	switch {
	case hasCached:
		if cached > 0 {
			status = loop.CacheHit
		} else if cached == 0 {
			status = loop.CacheMiss
		} else {
			status = loop.CacheUnknown
		}
	case retention == loop.CacheNone:
		status = loop.CacheUnsupported
	default:
		status = loop.CacheUnknown
	}
	in := prompt
	if hasCached {
		in = prompt - cached
		if in < 0 {
			in = 0
		}
	}
	return &loop.TurnUsage{
		Input: int(in), CacheRead: int(cacheRead), CacheWrite: int(created),
		Output: int(out), TotalTokens: int(prompt + created + out),
		CacheStatus: status,
	}
}

// doRequest posts one Messages request with retry on 429/529. A 429 that
// survives the retry budget is surfaced to the caller as-is: the selected
// model is the only model that answers.
// Headers are rebuilt on every attempt (cheap) so a version adopted via
// AdoptRequiredClaudeCodeVersion mid-loop is picked up immediately.
func (p *Provider) doRequest(ctx context.Context, url, accessToken, sessionID string, body map[string]any) (*http.Response, error) {
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	shared.DumpRequest("claude", sessionID, rawBody)
	attempt := func(payload []byte) (*http.Response, error) {
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		for k, v := range buildHeaders(accessToken, sessionID) {
			httpReq.Header.Set(k, v)
		}
		return p.httpClient().Do(httpReq)
	}
	var resp *http.Response
	for attemptN := 0; attemptN < 3; attemptN++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		resp, err = attempt(rawBody)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		retryable := resp.StatusCode == 429 || resp.StatusCode == 529 ||
			resp.Header.Get("x-should-retry") == "true"
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		if AdoptRequiredClaudeCodeVersion(string(raw)) && attemptN < 2 {
			// The pinned wire version is stale; retry immediately with the
			// server-named version instead of spending the attempt budget
			// on a rejection that would just repeat.
			continue
		}
		if !retryable || attemptN == 2 {
			return nil, shared.HTTPError("Claude", resp.StatusCode, resp.Status, strings.NewReader(string(raw)))
		}
		wait := time.Duration(1000*(1<<attemptN)) * time.Millisecond
		if wait > 6*time.Second {
			wait = 6 * time.Second
		}
		if retryAfter := resp.Header.Get("retry-after"); retryAfter != "" {
			var secs float64
			if _, err := fmt.Sscanf(retryAfter, "%f", &secs); err == nil && secs > 0 {
				wait = time.Duration(secs * float64(time.Second))
				if wait > 10*time.Second {
					wait = 10 * time.Second
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, fmt.Errorf("Claude request exceeded retry limit")
}

// RunTurn implements loop.Provider: one streaming assistant turn.
func (p *Provider) RunTurn(ctx context.Context, req loop.TurnRequest, events *stream.Stream[loop.Event]) error {
	cred, err := p.loadCredential()
	if err != nil {
		events.Fail(err)
		return err
	}
	body := BuildRequestBody(req.Model, req.SystemPrompt, req.Messages, req.Tools, req.CacheRetention, req.ThinkingLevel)
	if userID := metadataUserID(req.ConversationID); userID != "" {
		body["metadata"] = map[string]any{"user_id": userID}
	}
	resp, err := p.doRequest(ctx, MessagesURL(p.baseURL()), cred.AccessToken, req.ConversationID, body)
	if err != nil {
		events.Fail(err)
		return err
	}
	defer resp.Body.Close()

	calls := map[int]*toolCallState{}
	var inputUsage, outputUsage map[string]any

	emitFinal := func(state *toolCallState) {
		if state.finalized || state.id == "" || state.name == "" {
			return
		}
		state.finalized = true
		args := state.arguments
		if args == "" {
			args = "{}"
		}
		events.Push(loop.Event{Kind: loop.EventToolCall, Call: &tools.ToolCall{
			ID: state.id, Name: state.name, Arguments: json.RawMessage(args),
		}})
	}

	parseErr := parseAnthropicSSE(ctx, resp.Body, func(f sseFrame) error {
		switch f.event {
		case "error":
			msg := "Claude stream error"
			if e, ok := f.data["error"].(map[string]any); ok {
				if m, ok := e["message"].(string); ok && m != "" {
					msg = m
				}
			}
			return fmt.Errorf("%s", msg)
		case "message_start":
			if message, ok := f.data["message"].(map[string]any); ok {
				if usage, ok := message["usage"].(map[string]any); ok {
					inputUsage = usage
				}
			}
		case "content_block_start":
			index, _ := f.data["index"].(float64)
			block, _ := f.data["content_block"].(map[string]any)
			if block == nil {
				break
			}
			if block["type"] != "tool_use" {
				break
			}
			id, _ := block["id"].(string)
			name, _ := block["name"].(string)
			if id == "" || name == "" {
				break
			}
			state := &toolCallState{index: int(index), id: id, name: name}
			calls[int(index)] = state
		case "content_block_delta":
			index, _ := f.data["index"].(float64)
			delta, _ := f.data["delta"].(map[string]any)
			if delta == nil {
				break
			}
			switch delta["type"] {
			case "text_delta":
				if text, _ := delta["text"].(string); text != "" {
					events.Push(loop.Event{Kind: loop.EventText, Text: text})
				}
			case "thinking_delta":
				if text, _ := delta["thinking"].(string); text != "" {
					events.Push(loop.Event{Kind: loop.EventThinking, Text: text})
				}
			case "input_json_delta":
				if fragment, ok := delta["partial_json"].(string); ok {
					if state, ok := calls[int(index)]; ok {
						state.arguments += fragment
					}
				}
			}
		case "content_block_stop":
			index, _ := f.data["index"].(float64)
			if state, ok := calls[int(index)]; ok {
					emitFinal(state)
			}
		case "message_delta":
			if usage, ok := f.data["usage"].(map[string]any); ok {
				outputUsage = usage
			}
		}
		return nil
	})

	// Flush incomplete calls (truncated stream) before terminating.
	for _, state := range calls {
		emitFinal(state)
	}
	usage := NormalizeUsage(inputUsage, outputUsage, req.CacheRetention)
	if usage == nil {
		status := loop.CacheUnknown
		if req.CacheRetention == loop.CacheNone {
			status = loop.CacheUnsupported
		}
		usage = &loop.TurnUsage{CacheStatus: status}
	}
	events.Push(loop.Event{Kind: loop.EventUsage, Usage: usage})
	if parseErr != nil {
		events.Fail(parseErr)
		return parseErr
	}
	events.Complete()
	return nil
}
