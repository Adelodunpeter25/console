// Per-tool statistics for one run: call counts, error counts by class, and
// result bytes (docs/plan/harness-token-efficiency.md, Task 1.3). Unknown
// errors are logged since they usually indicate harness bugs.
package loop

import (
	"log/slog"
	"strings"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Tool error classes.
const (
	ErrClassInvalidArgs = "invalid_args"
	ErrClassEnv         = "env"
	ErrClassProvider    = "provider"
	ErrClassTimeout     = "timeout"
	ErrClassAborted     = "aborted"
	ErrClassDenied      = "denied"
	ErrClassUnknownTool = "unknown_tool"
	ErrClassUnknown     = "unknown"
)

// errorPatterns map lowercase substrings of a tool error to its class.
// Order matters: the first match wins.
var errorPatterns = []struct {
	class    string
	patterns []string
}{
	{ErrClassAborted, []string{"cancelled by user abort", "context canceled", "run ended"}},
	{ErrClassUnknownTool, []string{"unknown tool:"}},
	{ErrClassDenied, []string{"denied permission", "is denied in", "requires approval"}},
	{ErrClassTimeout, []string{"timed out", "timeout", "deadline exceeded"}},
	{ErrClassInvalidArgs, []string{
		"invalid arguments", "invalid ", "is required", "requires ", "must contain", "must be unique",
		"oldcontent", "unknown action", "unknown operation", "unknown scope", "duplicate path",
	}},
	{ErrClassProvider, []string{"http ", "status ", "api key", "rate limit", "429", "firecrawl", "search failed", "fetch failed"}},
	{ErrClassEnv, []string{
		"no such file", "not found", "does not exist", "permission denied", "is a directory",
		"not a directory", "exit code", "not available", "failed to", "cannot ",
	}},
}

// ClassifyToolError maps an error result's text to an error class.
func ClassifyToolError(text string) string {
	lower := strings.ToLower(text)
	for _, group := range errorPatterns {
		for _, pattern := range group.patterns {
			if strings.Contains(lower, pattern) {
				return group.class
			}
		}
	}
	return ErrClassUnknown
}

// ToolStat is one tool's counters within a run.
type ToolStat struct {
	Calls       int            `json:"calls"`
	Errors      int            `json:"errors"`
	ErrorsBy    map[string]int `json:"errorsBy,omitempty"`
	ResultBytes int            `json:"resultBytes"`
}

// ToolStats accumulates per-tool counters. Safe for concurrent use.
type ToolStats struct {
	mu    sync.Mutex
	stats map[string]*ToolStat
}

// Record adds one tool result.
func (s *ToolStats) Record(result tools.ToolResult) {
	text := resultText(result.Content)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stats == nil {
		s.stats = map[string]*ToolStat{}
	}
	name := result.ToolName
	if name == "" {
		name = "unknown"
	}
	stat, ok := s.stats[name]
	if !ok {
		stat = &ToolStat{}
		s.stats[name] = stat
	}
	stat.Calls++
	stat.ResultBytes += len(text)
	if !result.IsError {
		return
	}
	stat.Errors++
	class := ClassifyToolError(text)
	if stat.ErrorsBy == nil {
		stat.ErrorsBy = map[string]int{}
	}
	stat.ErrorsBy[class]++
	if class == ErrClassUnknown {
		snippet := text
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		slog.Warn("unclassified tool error", "tool", name, "error", snippet)
	}
}

// Snapshot returns a copy of the counters keyed by tool name.
func (s *ToolStats) Snapshot() map[string]ToolStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]ToolStat, len(s.stats))
	for name, stat := range s.stats {
		copyStat := *stat
		if stat.ErrorsBy != nil {
			copyStat.ErrorsBy = make(map[string]int, len(stat.ErrorsBy))
			for k, v := range stat.ErrorsBy {
				copyStat.ErrorsBy[k] = v
			}
		}
		out[name] = copyStat
	}
	return out
}

// resultText flattens a tool result's content (string or MCP-style text
// parts) into one string.
func resultText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []map[string]any:
		var b strings.Builder
		for _, part := range c {
			if text, ok := part["text"].(string); ok {
				b.WriteString(text)
			}
		}
		return b.String()
	case []any:
		var b strings.Builder
		for _, item := range c {
			if part, ok := item.(map[string]any); ok {
				if text, ok := part["text"].(string); ok {
					b.WriteString(text)
				}
			}
		}
		return b.String()
	case nil:
		return ""
	default:
		return string(messageJSON(c))
	}
}
