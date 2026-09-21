// Cut-point selection: discard old turns without orphaning tool results
// from their calls or breaking provider role alternation. Port of
// apps/server/agent/src/compaction/cut-point.ts.
package compaction

import (
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
)

// CutPointResult is the first-kept index plus boundary quality.
type CutPointResult struct {
	FirstKeptIndex int
	IsUserBoundary bool
}

// IsToolCallSafe reports whether slicing at candidateIndex keeps every
// toolResult paired with its assistant toolCall.
func IsToolCallSafe(history []any, candidateIndex int) bool {
	if candidateIndex <= 0 || candidateIndex >= len(history) {
		return true
	}
	if isToolResultMessage(history[candidateIndex]) {
		return false
	}
	discarded := make(map[string]bool)
	for i := 0; i < candidateIndex; i++ {
		for _, id := range toolCallIDs(history[i]) {
			discarded[id] = true
		}
	}
	for i := candidateIndex; i < len(history); i++ {
		if results, ok := toolResultsOf(history[i]); ok {
			for _, r := range results {
				if discarded[r.ToolCallID] {
					return false
				}
			}
		}
	}
	return true
}

// FindCutPoint locates the first-kept index preserving at least
// keepRecentTokens while honoring protected recent turns and tool pairing.
func FindCutPoint(history []any, keepRecentTokens, minimumRecentTurns int) CutPointResult {
	if keepRecentTokens <= 0 {
		keepRecentTokens = 40000
	}
	if minimumRecentTurns <= 0 {
		minimumRecentTurns = 3
	}
	if len(history) <= 4 {
		return CutPointResult{FirstKeptIndex: 0, IsUserBoundary: true}
	}
	var userIndices []int
	for i, m := range history {
		if isUserMessage(m) {
			userIndices = append(userIndices, i)
		}
	}
	protectedStart := 0
	if len(userIndices) > minimumRecentTurns {
		protectedStart = userIndices[len(userIndices)-minimumRecentTurns]
	}
	accumulated := 0
	targetIndex := 0
	for i := len(history) - 1; i >= 0; i-- {
		accumulated += EstimateMessageTokens(history[i : i+1])
		if accumulated >= keepRecentTokens {
			targetIndex = i
			break
		}
	}
	if protectedStart > 0 && targetIndex > protectedStart {
		targetIndex = protectedStart
	}
	// Preference 1: newest user turn at/after target inside protection.
	for u := len(userIndices) - 1; u >= 0; u-- {
		idx := userIndices[u]
		if idx > 0 && idx >= targetIndex && idx <= protectedStart && IsToolCallSafe(history, idx) {
			return CutPointResult{FirstKeptIndex: idx, IsUserBoundary: true}
		}
	}
	// Preference 2: closest safe user turn before target.
	for u := len(userIndices) - 1; u >= 1; u-- {
		idx := userIndices[u]
		if IsToolCallSafe(history, idx) {
			return CutPointResult{FirstKeptIndex: idx, IsUserBoundary: true}
		}
	}
	// Preference 3: single-user-turn sessions — first safe assistant cut.
	for i := targetIndex; i < len(history)-2; i++ {
		if i > protectedStart {
			continue
		}
		if isAssistantMessage(history[i]) && IsToolCallSafe(history, i) {
			return CutPointResult{FirstKeptIndex: i, IsUserBoundary: false}
		}
	}
	return CutPointResult{FirstKeptIndex: 0, IsUserBoundary: true}
}

func isToolResultMessage(m any) bool {
	switch msg := m.(type) {
	case loop.ToolResultMessage:
		return msg.Role == loop.RoleToolResult
	case *loop.ToolResultMessage:
		return msg != nil && msg.Role == loop.RoleToolResult
	}
	return false
}

func isAssistantMessage(m any) bool {
	switch msg := m.(type) {
	case loop.AssistantMessage:
		return msg.Role == loop.RoleAssistant
	case *loop.AssistantMessage:
		return msg != nil && msg.Role == loop.RoleAssistant
	}
	return false
}

func toolCallIDs(m any) []string {
	var ids []string
	var content []any
	switch msg := m.(type) {
	case loop.AssistantMessage:
		content = msg.Content
	case *loop.AssistantMessage:
		if msg == nil {
			return nil
		}
		content = msg.Content
	default:
		return nil
	}
	for _, part := range content {
		if call, ok := part.(loop.ToolCallPart); ok {
			ids = append(ids, call.Call.ID)
		}
	}
	return ids
}
