// Antigravity wire conversion: loop messages/tools to Gemini Content
// format. Port of apps/server/providers/src/shared/convert-messages.ts and
// convert-tools.ts.
package antigravity

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// LegacyThoughtSignature is used only for legacy histories created before
// signatures were persisted.
const LegacyThoughtSignature = "skip_thought_signature_validator"

// GeminiContent is one Gemini conversation turn.
type GeminiContent struct {
	Role  string           `json:"role"` // "user" | "model"
	Parts []map[string]any `json:"parts"`
}

var toolCallIDRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// normalizeToolCallID mirrors normalizeToolCallId: non-alphanumeric chars
// become underscores, capped at 64 chars.
func normalizeToolCallID(id string) string {
	out := toolCallIDRe.ReplaceAllString(id, "_")
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func textPart(text, thoughtSignature string) map[string]any {
	part := map[string]any{"text": text}
	if thoughtSignature != "" {
		part["thoughtSignature"] = thoughtSignature
	}
	return part
}

func inlineDataPart(data, mimeType string) map[string]any {
	return map[string]any{"inlineData": map[string]any{"mimeType": mimeType, "data": data}}
}

func functionCallPart(name string, args map[string]any, id, thoughtSignature string) map[string]any {
	part := map[string]any{"functionCall": map[string]any{"name": name, "args": args, "id": id}}
	if thoughtSignature != "" {
		part["thoughtSignature"] = thoughtSignature
	}
	return part
}

func functionResponsePart(name, id string, content any) map[string]any {
	return map[string]any{"functionResponse": map[string]any{
		"name": name, "id": id, "response": map[string]any{"content": content},
	}}
}

func decodeArgs(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	return m
}

// ConvertMessagesOptions mirrors ConvertMessagesOptions.
type ConvertMessagesOptions struct {
	// RequireUserTerminator: Claude-on-CCA models reject requests that end
	// with an assistant/model turn.
	RequireUserTerminator bool
}

// ConvertMessages maps loop history to Gemini Content turns.
func ConvertMessages(messages []any, opts ConvertMessagesOptions) []GeminiContent {
	// 1. Build toolCallId -> toolName lookup from assistant tool calls in history.
	toolNameByCallID := map[string]string{}
	for _, m := range messages {
		if am, ok := m.(loop.AssistantMessage); ok {
			for _, part := range am.Content {
				if call, ok := part.(loop.ToolCallPart); ok {
					toolNameByCallID[call.Call.ID] = call.Call.Name
					toolNameByCallID[normalizeToolCallID(call.Call.ID)] = call.Call.Name
				}
			}
		}
	}

	var rawTurns []GeminiContent

	for _, m := range messages {
		switch msg := m.(type) {
		case loop.UserMessage:
			text := msg.Content
			if strings.TrimSpace(text) == "" && len(msg.Attachments) == 0 {
				continue
			}
			parts := []map[string]any{}
			if strings.TrimSpace(text) == "" {
				parts = append(parts, textPart("(see attached images)", ""))
			} else {
				parts = append(parts, textPart(text, ""))
			}
			for _, att := range msg.Attachments {
				parts = append(parts, inlineDataPart(att.Data, att.MimeType))
			}
			rawTurns = append(rawTurns, GeminiContent{Role: "user", Parts: parts})

		case loop.AssistantMessage:
			var textParts, funcParts []map[string]any
			for _, part := range msg.Content {
				switch p := part.(type) {
				case loop.TextPart:
					if p.Text != "" || p.ThoughtSignature != "" {
						textParts = append(textParts, textPart(p.Text, p.ThoughtSignature))
					}
				case loop.ThinkingPart:
					// Prior thinking is replayed unsigned, which strict
					// signing endpoints reject — demote to text.
					if p.Text != "" {
						textParts = append(textParts, textPart(p.Text, ""))
					}
				case loop.ToolCallPart:
					normalizedID := normalizeToolCallID(p.Call.ID)
					sig := p.Call.ThoughtSignature
					if sig == "" {
						sig = LegacyThoughtSignature
					}
					funcParts = append(funcParts, functionCallPart(p.Call.Name, decodeArgs(p.Call.Arguments), normalizedID, sig))
				}
			}
			// All text must precede functionCall parts.
			parts := append(textParts, funcParts...)
			if len(parts) > 0 {
				rawTurns = append(rawTurns, GeminiContent{Role: "model", Parts: parts})
			}

		case loop.ToolResultMessage:
			var parts []map[string]any
			for _, r := range msg.Results {
				toolName := r.ToolName
				if toolName == "" {
					toolName = toolNameByCallID[r.ToolCallID]
				}
				if toolName == "" {
					toolName = toolNameByCallID[normalizeToolCallID(r.ToolCallID)]
				}
				if toolName == "" {
					toolName = r.ToolCallID
				}
				parts = append(parts, functionResponsePart(toolName, normalizeToolCallID(r.ToolCallID), r.Content))
			}
			if len(parts) > 0 {
				rawTurns = append(rawTurns, GeminiContent{Role: "user", Parts: parts})
			}
		}
	}

	// 2. Merge adjacent same-role turns (Gemini requires strict alternation).
	var merged []GeminiContent
	for _, turn := range rawTurns {
		if len(turn.Parts) == 0 {
			continue
		}
		if n := len(merged); n > 0 && merged[n-1].Role == turn.Role {
			merged[n-1].Parts = append(merged[n-1].Parts, turn.Parts...)
		} else {
			merged = append(merged, GeminiContent{Role: turn.Role, Parts: append([]map[string]any{}, turn.Parts...)})
		}
	}

	// 3. Enforce start/end-with-user when required by the provider.
	if len(merged) == 0 {
		if opts.RequireUserTerminator {
			merged = append(merged, GeminiContent{Role: "user", Parts: []map[string]any{textPart("(continue)", "")}})
		}
	} else {
		if merged[0].Role != "user" {
			merged = append([]GeminiContent{{Role: "user", Parts: []map[string]any{textPart("(session started)", "")}}}, merged...)
		}
		if merged[len(merged)-1].Role != "user" {
			merged = append(merged, GeminiContent{Role: "user", Parts: []map[string]any{textPart("(continue)", "")}})
		}
	}

	// 4. Sanitize function responses: every functionResponse needs a
	// matching functionCall in the immediately preceding model turn.
	for i := range merged {
		if merged[i].Role != "user" {
			continue
		}
		validIDs := map[string]bool{}
		if i > 0 && merged[i-1].Role == "model" {
			for _, part := range merged[i-1].Parts {
				if fc, ok := part["functionCall"].(map[string]any); ok {
					if id, _ := fc["id"].(string); id != "" {
						validIDs[id] = true
					}
				}
			}
		}
		for j, part := range merged[i].Parts {
			fr, ok := part["functionResponse"].(map[string]any)
			if !ok {
				continue
			}
			id, _ := fr["id"].(string)
			if validIDs[id] {
				continue
			}
			name, _ := fr["name"].(string)
			var text string
			if resp, ok := fr["response"].(map[string]any); ok {
				if s, ok := resp["content"].(string); ok {
					text = s
				} else {
					raw, _ := json.Marshal(resp["content"])
					text = string(raw)
				}
			}
			if name == "" {
				name = "tool"
			}
			merged[i].Parts[j] = textPart("[Tool result for "+name+": "+text+"]", "")
		}
	}

	return merged
}

// GeminiFunctionDeclaration is one CCA tool declaration.
type GeminiFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

var ccaBannedKeywords = map[string]bool{
	// $id is Go-specific: invopop/jsonschema emits a top-level $id that the
	// TS zod-to-json-schema generator never produces; CCA rejects it too.
	"$id":     true,
	"$schema": true, "$ref": true, "$defs": true, "$dynamicRef": true, "$dynamicAnchor": true,
	"exclusiveMinimum": true, "exclusiveMaximum": true, "minimum": true, "maximum": true,
	"multipleOf": true, "minLength": true, "maxLength": true, "minItems": true, "maxItems": true,
	"pattern": true, "format": true, "minProperties": true, "maxProperties": true,
	"patternProperties": true, "propertyNames": true, "unevaluatedProperties": true,
	"unevaluatedItems": true, "dependencies": true, "dependentSchemas": true,
	"dependentRequired": true, "additionalProperties": true, "examples": true, "prefixItems": true,
}

var ccaPropertyMapKeys = map[string]bool{
	"properties": true, "definitions": true, "$defs": true, "dependentSchemas": true,
}

var ccaSnakeToCamel = map[string]string{
	"additional_properties": "additionalProperties",
	"any_of":                "anyOf",
	"one_of":                "oneOf",
	"prefix_items":          "prefixItems",
}

// normalizeSchemaForCCA mirrors normalizeSchemaForCCA: strips banned
// keywords, collapses anyOf/oneOf, converts type arrays to nullable.
func normalizeSchemaForCCA(value any, isPropertyMap bool) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeSchemaForCCA(item, false)
		}
		return out
	case map[string]any:
		result := map[string]any{}
		for key, val := range v {
			normalizedKey := key
			if mapped, ok := ccaSnakeToCamel[key]; ok {
				normalizedKey = mapped
			}
			if !isPropertyMap && ccaBannedKeywords[normalizedKey] {
				continue
			}
			if !isPropertyMap && (normalizedKey == "anyOf" || normalizedKey == "oneOf") {
				if variants, ok := val.([]any); ok && len(variants) > 0 {
					merged := map[string]any{"type": "object"}
					var props map[string]any
					for _, variant := range variants {
						vm, ok := variant.(map[string]any)
						if !ok {
							continue
						}
						if vm["type"] == "object" {
							if vp, ok := vm["properties"].(map[string]any); ok {
								if props == nil {
									props = map[string]any{}
								}
								for pk, pv := range vp {
									props[pk] = pv
								}
							}
						}
					}
					if props != nil {
						merged["properties"] = props
					}
					if normalized, ok := normalizeSchemaForCCA(merged, false).(map[string]any); ok {
						for mk, mv := range normalized {
							result[mk] = mv
						}
					}
					continue
				}
			}
			if !isPropertyMap && normalizedKey == "type" {
				if types, ok := val.([]any); ok {
					var nonNull []any
					hasNull := false
					for _, t := range types {
						if t == "null" {
							hasNull = true
						} else {
							nonNull = append(nonNull, t)
						}
					}
					if len(nonNull) == 1 {
						result["type"] = nonNull[0]
						if hasNull {
							result["nullable"] = true
						}
						continue
					}
				}
			}
			nextIsPropertyMap := !isPropertyMap && ccaPropertyMapKeys[normalizedKey]
			result[normalizedKey] = normalizeSchemaForCCA(val, nextIsPropertyMap)
		}
		if !isPropertyMap {
			if required, ok := result["required"].([]any); ok {
				if props, ok := result["properties"].(map[string]any); ok {
					kept := make([]any, 0, len(required))
					for _, name := range required {
						if s, ok := name.(string); ok {
							if _, exists := props[s]; exists {
								kept = append(kept, name)
							}
						}
					}
					if len(kept) == 0 {
						delete(result, "required")
					} else {
						result["required"] = kept
					}
				}
			}
		}
		return result
	default:
		return value
	}
}

// ConvertTools maps tool definitions to Gemini function declarations,
// normalizing JSON Schema for CCA compatibility.
func ConvertTools(defs []tools.Definition) []GeminiFunctionDeclaration {
	out := make([]GeminiFunctionDeclaration, 0, len(defs))
	for _, t := range defs {
		schema := map[string]any{"type": "object"}
		if t.InputSchema != nil {
			if normalized, ok := normalizeSchemaForCCA(t.InputSchema, false).(map[string]any); ok {
				schema = normalized
			}
		}
		out = append(out, GeminiFunctionDeclaration{Name: t.Name, Description: t.Description, Parameters: schema})
	}
	return out
}
