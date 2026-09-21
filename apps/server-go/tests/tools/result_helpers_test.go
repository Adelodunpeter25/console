package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// resultText extracts the text of the first MCP content block from a tool's
// successful return value, unwrapping tools.Envelope when present. Tools
// return this exact shape (mirroring the TS server's normalizeToolOutput)
// so the desktop UI's result renderer has something to parse.
func resultText(t *testing.T, out any) string {
	t.Helper()
	content := out
	if env, ok := out.(tools.Envelope); ok {
		content = env.Content
	}
	blocks, ok := content.([]map[string]any)
	if !ok || len(blocks) == 0 {
		t.Fatalf("expected MCP content array, got %#v", out)
	}
	text, _ := blocks[0]["text"].(string)
	return text
}

// resultIsError reports the isError flag on a tool's return value.
func resultIsError(out any) bool {
	if env, ok := out.(tools.Envelope); ok {
		return env.IsError
	}
	return false
}
