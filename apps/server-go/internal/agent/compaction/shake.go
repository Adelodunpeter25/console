// Tool-output shaking: mechanically truncate bloated tool results before
// throwing away whole turns. Port of apps/server/agent/src/compaction/shake.ts.
package compaction

import (
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// EmergencyChars is the per-result budget during overflow recovery.
const EmergencyChars = 2000

const shakenMarker = "\n[…tool output shaken for context space…]"

// ShakeConversation truncates oversized string tool results outside the
// protected suffix (or everywhere when emergency is set). Error results
// and short outputs pass through untouched. Messages are never mutated in
// place — truncated turns are copied.
func ShakeConversation(history []any, maxChars, protectedFrom int, emergency bool) []any {
	out := make([]any, len(history))
	changed := false
	for i, m := range history {
		results, ok := toolResultsOf(m)
		if !ok || (!emergency && i >= protectedFrom) {
			out[i] = m
			continue
		}
		shaken, truncated := shakeResults(results, maxChars)
		if !truncated {
			out[i] = m
			continue
		}
		changed = true
		out[i] = loop.ToolResultMessage{Role: loop.RoleToolResult, Results: shaken}
	}
	if !changed {
		return history
	}
	return out
}

func toolResultsOf(m any) ([]tools.ToolResult, bool) {
	switch msg := m.(type) {
	case loop.ToolResultMessage:
		return msg.Results, true
	case *loop.ToolResultMessage:
		if msg != nil {
			return msg.Results, true
		}
	}
	return nil, false
}

func shakeResults(results []tools.ToolResult, maxChars int) ([]tools.ToolResult, bool) {
	out := make([]tools.ToolResult, len(results))
	truncated := false
	for i, r := range results {
		out[i] = r
		content, ok := r.Content.(string)
		if !ok || r.IsError || len([]rune(content)) <= maxChars {
			continue
		}
		truncated = true
		out[i].Content = string([]rune(content)[:maxChars]) + shakenMarker
	}
	if !truncated {
		return nil, false
	}
	return out, true
}

// ProtectedRecentStart returns the earliest message index of the final
// turnCount user turns (0 when fewer exist).
func ProtectedRecentStart(history []any, turnCount int) int {
	if turnCount <= 0 {
		turnCount = 3
	}
	var userIndices []int
	for i, m := range history {
		if isUserMessage(m) {
			userIndices = append(userIndices, i)
		}
	}
	if len(userIndices) > turnCount {
		return userIndices[len(userIndices)-turnCount]
	}
	return 0
}

func isUserMessage(m any) bool {
	switch msg := m.(type) {
	case loop.UserMessage:
		return msg.Role == loop.RoleUser
	case *loop.UserMessage:
		return msg != nil && msg.Role == loop.RoleUser
	}
	return false
}
