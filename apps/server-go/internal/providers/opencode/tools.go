// OpenCode tool conversion.
package opencode

import "github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"

func minimalSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": true,
	}
}

func schemaOrObject(schema map[string]any) map[string]any {
	if schema == nil {
		return minimalSchema()
	}
	return schema
}

// ConvertChatTools converts definitions to /chat/completions wire shape.
func ConvertChatTools(definitions []tools.Definition) []map[string]any {
	out := make([]map[string]any, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        definition.Name,
				"description": definition.Description,
				"parameters":  schemaOrObject(definition.InputSchema),
			},
		})
	}
	return out
}

// ConvertResponsesTools converts definitions to /responses wire shape.
func ConvertResponsesTools(definitions []tools.Definition) []map[string]any {
	out := make([]map[string]any, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, map[string]any{
			"type":        "function",
			"name":        definition.Name,
			"description": definition.Description,
			"parameters":  schemaOrObject(definition.InputSchema),
		})
	}
	return out
}
