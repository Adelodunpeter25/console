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
	raw, err := json.Marshal(content)
	if err != nil {
		return fmt.Sprint(content)
	}
	return string(raw)
}

// HTTPError formats a non-2xx response with a capped body snippet.
func HTTPError(provider string, statusCode int, status string, r io.Reader) error {
	raw, _ := io.ReadAll(io.LimitReader(r, 1<<20))
	return fmt.Errorf("%s request failed (%d %s): %s", provider, statusCode, status, strings.TrimSpace(string(raw)))
}
