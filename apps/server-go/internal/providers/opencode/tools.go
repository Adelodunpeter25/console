// OpenCode tool conversion and the Zen free-tier compatibility catalog.
package opencode

import "github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"

var compatibilityToolNames = []string{
	"edit",
	"glob",
	"grep",
	"question",
	"read",
	"shell",
}

// CompatibilityDefinitions returns provider-only marker tools required by
// Zen's current free-tier validation. They are never registered in the Go
// harness and are never executable.
func CompatibilityDefinitions() []tools.Definition {
	out := make([]tools.Definition, 0, len(compatibilityToolNames))
	for _, name := range compatibilityToolNames {
		out = append(out, tools.Definition{
			Name:        name,
			Description: "OpenCode compatibility tool",
			InputSchema: minimalSchema(),
		})
	}
	return out
}

// IsCompatibilityToolName reports whether name is one of the six wire-only
// marker tools.
func IsCompatibilityToolName(name string) bool {
	for _, compatibilityName := range compatibilityToolNames {
		if name == compatibilityName {
			return true
		}
	}
	return false
}

// MergeToolDefinitions places the six compatibility names first, keeps the
// real glob/grep definitions when present, and appends all other harness
// tools without duplicate names.
func MergeToolDefinitions(real []tools.Definition) []tools.Definition {
	out := CompatibilityDefinitions()
	indexes := make(map[string]int, len(out))
	for i, definition := range out {
		indexes[definition.Name] = i
	}
	for _, definition := range real {
		if definition.Name == "" {
			continue
		}
		if index, exists := indexes[definition.Name]; exists {
			// glob and grep are real harness tools; their full schema is more
			// useful than the marker schema. Other marker names are left alone.
			if definition.Name == "glob" || definition.Name == "grep" {
				out[index] = definition
			}
			continue
		}
		out = append(out, definition)
		indexes[definition.Name] = len(out) - 1
	}
	return out
}

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
