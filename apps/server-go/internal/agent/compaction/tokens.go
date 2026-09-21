// Token estimation for compaction decisions. Port of
// apps/server/agent/src/compaction/token-estimator.ts: ~4 chars per token
// for prose, ~3 for code/JSON/tool output, ~1,000 per inline image. No
// tokenizer dependency; provider-native counting can plug in later.
package compaction

import (
	"encoding/json"
	"math"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

const (
	charsPerProseToken = 4.0
	charsPerCodeToken  = 3.0
	imageTokensEach    = 1000
	// Flat per-tool allowance for the JSON input schema.
	toolSchemaOverheadTokens = 400
	defaultWireMarginTokens  = 2000
)

// PayloadEstimate is a wire-payload token estimate with its source.
type PayloadEstimate struct {
	Tokens int
	Source string // "local" (heuristic) — "provider" plugs in later
}

// EstimateMessageTokens counts history tokens (message bodies only).
func EstimateMessageTokens(history []any) int {
	totalChars, imageTokens := 0, 0
	for _, m := range history {
		switch msg := m.(type) {
		case loop.UserMessage:
			totalChars += len([]rune(msg.Content))
			imageTokens += len(msg.Attachments) * imageTokensEach
		case *loop.UserMessage:
			if msg != nil {
				totalChars += len([]rune(msg.Content))
				imageTokens += len(msg.Attachments) * imageTokensEach
			}
		case loop.AssistantMessage:
			totalChars += assistantChars(msg.Content)
		case *loop.AssistantMessage:
			if msg != nil {
				totalChars += assistantChars(msg.Content)
			}
		case loop.ToolResultMessage:
			for _, r := range msg.Results {
				totalChars += resultChars(r.Content)
			}
		case *loop.ToolResultMessage:
			if msg != nil {
				for _, r := range msg.Results {
					totalChars += resultChars(r.Content)
				}
			}
		}
	}
	return int(math.Ceil(float64(totalChars)/charsPerProseToken)) + imageTokens
}

// EstimatePayloadTokens counts the full wire payload: messages plus the
// system prompt, tool definitions, and a flat wire margin.
func EstimatePayloadTokens(history []any, systemPrompt string, defs []tools.Definition) PayloadEstimate {
	proseChars := len([]rune(systemPrompt))
	codeChars, imageTokens := 0, 0
	for _, m := range history {
		switch msg := m.(type) {
		case loop.UserMessage:
			proseChars += len([]rune(msg.Content))
			imageTokens += len(msg.Attachments) * imageTokensEach
		case *loop.UserMessage:
			if msg != nil {
				proseChars += len([]rune(msg.Content))
				imageTokens += len(msg.Attachments) * imageTokensEach
			}
		case loop.AssistantMessage:
			p, c := assistantSplit(msg.Content)
			proseChars += p
			codeChars += c
		case *loop.AssistantMessage:
			if msg != nil {
				p, c := assistantSplit(msg.Content)
				proseChars += p
				codeChars += c
			}
		case loop.ToolResultMessage:
			for _, r := range msg.Results {
				codeChars += resultChars(r.Content)
			}
		case *loop.ToolResultMessage:
			if msg != nil {
				for _, r := range msg.Results {
					codeChars += resultChars(r.Content)
				}
			}
		}
	}
	toolTokens := 0
	for _, d := range defs {
		toolTokens += int(math.Ceil(float64(len([]rune(d.Name))+len([]rune(d.Description))) / charsPerProseToken))
		toolTokens += toolSchemaOverheadTokens
	}
	tokens := int(math.Ceil(float64(proseChars)/charsPerProseToken)) +
		int(math.Ceil(float64(codeChars)/charsPerCodeToken)) +
		imageTokens + toolTokens + defaultWireMarginTokens
	return PayloadEstimate{Tokens: tokens, Source: "local"}
}

func assistantChars(content []any) int {
	p, c := assistantSplit(content)
	return p + c
}

func assistantSplit(content []any) (prose, code int) {
	for _, part := range content {
		switch part := part.(type) {
		case loop.TextPart:
			prose += len([]rune(part.Text))
		case loop.ThinkingPart:
			prose += len([]rune(part.Text))
		case loop.ToolCallPart:
			prose += len([]rune(part.Call.Name))
			code += argsChars(part.Call.Arguments)
		case map[string]any:
			if t, _ := part["type"].(string); t == "text" || t == "thinking" {
				if s, _ := part["text"].(string); s != "" {
					prose += len([]rune(s))
				}
			}
		}
	}
	return prose, code
}

func argsChars(args json.RawMessage) int {
	if len(args) == 0 {
		return 0
	}
	// Raw JSON counts as code chars; invalid JSON gets the TS fallback.
	var v any
	if err := json.Unmarshal(args, &v); err != nil {
		return 50
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return 50
	}
	return len(raw)
}

func resultChars(content any) int {
	if s, ok := content.(string); ok {
		return len([]rune(s))
	}
	if content == nil {
		return 0
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return 100
	}
	return len(raw)
}
