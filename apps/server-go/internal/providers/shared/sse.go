// Shared SSE parsing for provider event streams. Ports
// apps/server/providers/src/shared/sse-parser.ts: each `data: {...}` line
// yields a JSON object; [DONE]/blank/malformed lines are skipped.
package shared

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
)

// ParseSSE yields JSON objects from data: lines, skipping [DONE]/blanks.
func ParseSSE(ctx context.Context, r io.Reader, handle func(map[string]any) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		if err := handle(event); err != nil {
			return err
		}
	}
	return scanner.Err()
}
