// Antigravity CCA streaming provider. Port of
// apps/server/providers/src/antigravity/stream-fn.ts and
// apps/server/providers/src/shared/stream-core.ts. Implements
// loop.Provider so the agent loop drives turns without changes.
package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// antigravitySystemInstruction is injected for Gemini 3 + Claude models.
// Source: oh-my-pi/packages/catalog/src/wire/gemini-headers.ts.
const antigravitySystemInstruction = "You are Antigravity, a powerful agentic AI coding assistant designed by the Google Deepmind team working on Advanced Agentic Coding." +
	"You are pair programming with a USER to solve their coding task. The task may require creating a new codebase, modifying or debugging an existing codebase, or simply answering a question." +
	"**Absolute paths only**" +
	"**Proactiveness**"

// modelMaxOutputTokens mirrors MODEL_MAX_OUTPUT_TOKENS.
var modelMaxOutputTokens = map[string]int{
	"gemini-3.5-flash-extra-low": 65536,
	"gemini-3.5-flash-low":       65536,
	"gemini-3-flash-agent":       65536,
	"gemini-3.1-pro-low":         65535,
	"gemini-pro-agent":           65535,
	"claude-sonnet-4-6":          64000,
	"claude-opus-4-6-thinking":   64000,
}

// IsClaudeModel reports whether a wire model id targets Claude-on-CCA.
func IsClaudeModel(modelID string) bool {
	return strings.Contains(strings.ToLower(modelID), "claude")
}

func shouldInjectSystemInstruction(modelID string) bool {
	lower := strings.ToLower(modelID)
	return strings.Contains(lower, "claude") || strings.Contains(lower, "gemini-3")
}

// Provider streams Antigravity CCA turns. HTTPClient and BaseURL are
// overridable for tests; CredentialLoader defaults to OAuth file/env.
// SessionState is created once per Provider instance so requestId/stepIndex
// stay stable across turns in the same run, mirroring createAntigravityStreamFn.
type Provider struct {
	BaseURL          string
	HTTPClient       *http.Client
	CredentialLoader func() (ParsedCredential, error)
	sessionState     *SessionState
}

func (p *Provider) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	return BaseURL
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

func (p *Provider) session() *SessionState {
	if p.sessionState == nil {
		p.sessionState = NewSessionState()
	}
	return p.sessionState
}

// buildSystemInstruction mirrors buildSystemInstruction.
func buildSystemInstruction(modelID, userPrompt string) map[string]any {
	var parts []map[string]any
	if shouldInjectSystemInstruction(modelID) {
		parts = append(parts, map[string]any{"text": antigravitySystemInstruction})
	}
	if strings.TrimSpace(userPrompt) != "" {
		parts = append(parts, map[string]any{"text": userPrompt})
	}
	instruction := map[string]any{"parts": parts}
	if IsClaudeModel(modelID) {
		instruction["role"] = "user"
	}
	return instruction
}

// buildToolConfig mirrors buildToolConfig.
func buildToolConfig(modelID string) map[string]any {
	mode := "AUTO"
	if IsClaudeModel(modelID) {
		mode = "VALIDATED"
	}
	return map[string]any{"functionCallingConfig": map[string]any{"mode": mode}}
}

// buildRequestBody assembles the CCA request envelope.
func buildRequestBody(projectID, modelID, systemPrompt string, contents []GeminiContent, toolDefs []GeminiFunctionDeclaration, state *SessionState, thinkingLevel string) map[string]any {
	envelope := BuildEnvelope(state, modelID)
	maxOutputTokens, ok := modelMaxOutputTokens[modelID]
	if !ok {
		maxOutputTokens = 65536
	}

	generationConfig := map[string]any{"maxOutputTokens": maxOutputTokens}
	if thinkingLevel != "" {
		generationConfig["thinkingConfig"] = map[string]any{
			"includeThoughts": true,
			"thinkingLevel":   strings.ToUpper(thinkingLevel),
		}
	}

	rawContents := make([]map[string]any, len(contents))
	for i, c := range contents {
		rawContents[i] = map[string]any{"role": c.Role, "parts": c.Parts}
	}

	payload := map[string]any{
		"contents":          rawContents,
		"sessionId":         envelope.SessionID,
		"systemInstruction": buildSystemInstruction(modelID, systemPrompt),
		"generationConfig":  generationConfig,
		"labels":            envelope.Labels,
	}
	if len(toolDefs) > 0 {
		payload["tools"] = []map[string]any{{"functionDeclarations": toolDefs}}
	}
	if len(toolDefs) > 0 || IsClaudeModel(modelID) {
		payload["toolConfig"] = buildToolConfig(modelID)
	}

	return map[string]any{
		"project":     projectID,
		"model":       modelID,
		"request":     payload,
		"requestId":   envelope.RequestID,
		"requestType": "agent",
		"userAgent":   "antigravity",
	}
}

// EndpointURL mirrors buildEndpointUrl.
func EndpointURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/v1internal:streamGenerateContent?alt=sse"
}

// NormalizeUsage maps CCA usageMetadata to agent TurnUsage. Missing
// metadata → nil (caller falls back to "unknown"/"unsupported").
func NormalizeUsage(metadata map[string]any, retention loop.CacheRetention) *loop.TurnUsage {
	if metadata == nil {
		return nil
	}
	prompt, hasPrompt := shared.NumberField(metadata, "promptTokenCount")
	total, hasTotal := shared.NumberField(metadata, "totalTokenCount")
	if !hasPrompt && !hasTotal {
		if retention == loop.CacheNone {
			return &loop.TurnUsage{CacheStatus: loop.CacheUnsupported}
		}
		return nil
	}
	cached, hasCached := shared.NumberField(metadata, "cachedContentTokenCount")
	output := shared.NumberFieldOr(metadata, "candidatesTokenCount", 0)
	thoughts, hasThoughts := shared.NumberField(metadata, "thoughtsTokenCount")

	hasCacheBreakdown := hasPrompt && hasCached
	cacheRead := 0.0
	input := prompt
	if hasCacheBreakdown {
		cacheRead = cached
		input = prompt - cached
	}

	var status loop.CacheStatus
	switch {
	case !hasCacheBreakdown:
		if retention == loop.CacheNone {
			status = loop.CacheUnsupported
		} else {
			status = loop.CacheUnknown
		}
	case cached > 0:
		status = loop.CacheHit
	case cached == 0:
		status = loop.CacheMiss
	default:
		status = loop.CacheUnknown
	}

	if !hasTotal {
		total = prompt + output + thoughts
	}

	usage := &loop.TurnUsage{
		Input: int(input), CacheRead: int(cacheRead), CacheWrite: 0,
		Output: int(output), TotalTokens: int(total), CacheStatus: status,
	}
	if hasThoughts {
		rt := int(thoughts)
		usage.ReasoningTokens = &rt
	}
	return usage
}

// RunTurn implements loop.Provider: one streaming assistant turn.
func (p *Provider) RunTurn(ctx context.Context, req loop.TurnRequest, events *stream.Stream[loop.Event]) error {
	cred, err := p.loadCredential()
	if err != nil {
		events.Fail(err)
		return err
	}

	requireUserTerminator := IsClaudeModel(req.Model)
	contents := ConvertMessages(req.Messages, ConvertMessagesOptions{RequireUserTerminator: requireUserTerminator})
	toolDefs := ConvertTools(req.Tools)
	body := buildRequestBody(cred.ProjectID, req.Model, req.SystemPrompt, contents, toolDefs, p.session(), req.ThinkingLevel)

	rawBody, err := json.Marshal(body)
	if err != nil {
		events.Fail(err)
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", EndpointURL(p.baseURL()), strings.NewReader(string(rawBody)))
	if err != nil {
		events.Fail(err)
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("User-Agent", UserAgent())

	resp, err := p.httpClient().Do(httpReq)
	if err != nil {
		events.Fail(err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := shared.HTTPError("Antigravity", resp.StatusCode, resp.Status, io.LimitReader(resp.Body, 64*1024))
		events.Fail(err)
		return err
	}

	var lastUsage map[string]any
	var lastResponseID string
	syntheticCallIndex := 0

	parseErr := shared.ParseSSE(ctx, resp.Body, func(chunk map[string]any) error {
		if errBlock, ok := chunk["error"].(map[string]any); ok {
			code, _ := errBlock["code"].(float64)
			status, _ := errBlock["status"].(string)
			msg, _ := errBlock["message"].(string)
			if msg == "" {
				msg = "unknown"
			}
			return fmt.Errorf("CCA stream error (%s %s): %s", strconv.FormatFloat(code, 'f', -1, 64), status, msg)
		}

		response, ok := chunk["response"].(map[string]any)
		if !ok {
			return nil
		}
		if usage, ok := response["usageMetadata"].(map[string]any); ok {
			lastUsage = usage
		}
		if respID, ok := response["responseId"].(string); ok && respID != "" {
			lastResponseID = respID
		}

		candidates, _ := response["candidates"].([]any)
		if len(candidates) == 0 {
			return nil
		}
		candidate, _ := candidates[0].(map[string]any)
		content, _ := candidate["content"].(map[string]any)
		if content == nil {
			return nil
		}
		parts, _ := content["parts"].([]any)
		for _, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			if thought, _ := part["thought"].(bool); thought {
				continue
			}
			thoughtSignature, _ := part["thoughtSignature"].(string)

			if fc, ok := part["functionCall"].(map[string]any); ok {
				name, _ := fc["name"].(string)
				id, _ := fc["id"].(string)
				if id == "" {
					id = fmt.Sprintf("call-%d", syntheticCallIndex)
					syntheticCallIndex++
				}
				args, _ := fc["args"].(map[string]any)
				argsJSON, _ := json.Marshal(args)
				events.Push(loop.Event{Kind: loop.EventToolCall, Call: &tools.ToolCall{
					ID: id, Name: name, Arguments: argsJSON, ThoughtSignature: thoughtSignature,
				}})
				continue
			}

			text, hasText := part["text"].(string)
			if (hasText && text != "") || thoughtSignature != "" {
				events.Push(loop.Event{Kind: loop.EventText, Text: text, ThoughtSignature: thoughtSignature})
			}
		}
		return nil
	})

	UpdateLastExecutionID(p.session(), lastResponseID)

	usage := NormalizeUsage(lastUsage, req.CacheRetention)
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
