// Shared JSON/HTTP utilities for providers.
package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// NumberField extracts a numeric field accepting float/int/json.Number.
func NumberField(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// NumberFieldOr returns the numeric field or a fallback.
func NumberFieldOr(m map[string]any, key string, fallback float64) float64 {
	if v, ok := NumberField(m, key); ok {
		return v
	}
	return fallback
}

// ToolResultText stringifies tool result content for provider input.
func ToolResultText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	raw, err := json.Marshal(RedactImages(content))
	if err != nil {
		return fmt.Sprint(content)
	}
	return string(raw)
}

// RedactImages swaps inline image parts for a short placeholder so base64
// never lands in a text-only tool result.
func RedactImages(content any) any {
	switch v := content.(type) {
	case []map[string]any:
		out := make([]map[string]any, len(v))
		for i, item := range v {
			out[i] = item
			if item["type"] == "image" {
				out[i] = map[string]any{"type": "text", "text": "[image omitted: provider does not support images in tool results]"}
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
			if m, ok := item.(map[string]any); ok && m["type"] == "image" {
				out[i] = map[string]any{"type": "text", "text": "[image omitted: provider does not support images in tool results]"}
			}
		}
		return out
	}
	return content
}

// HTTPError formats a non-2xx response with a capped body snippet.
func HTTPError(provider string, statusCode int, status string, r io.Reader) error {
	raw, _ := io.ReadAll(io.LimitReader(r, 1<<20))
	return fmt.Errorf("%s request failed (%d %s): %s", provider, statusCode, status, strings.TrimSpace(string(raw)))
}
