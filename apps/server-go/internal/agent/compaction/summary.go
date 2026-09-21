// Deterministic structural summaries of discarded turns. Port of
// apps/server/agent/src/compaction/structural-summary.ts.
package compaction

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

const (
	maxPromptChars      = 300
	maxFirstPromptChars = 600
	maxArgChars         = 100
	maxHighlightsChars  = 3000
	maxFileFacts        = 5
	maxFileFactChars    = 400
)

func formatToolArgs(args json.RawMessage) string {
	var obj map[string]any
	if err := json.Unmarshal(args, &obj); err != nil || obj == nil {
		return ""
	}
	for _, key := range []string{"path", "filePath", "targetFile", "TargetFile", "absolutePath", "AbsolutePath", "command", "CommandLine", "query", "Query", "pattern", "Pattern"} {
		if v, ok := obj[key]; ok && v != nil {
			s := fmt.Sprint(v)
			if len([]rune(s)) > maxArgChars {
				return string([]rune(s)[:maxArgChars]) + "…"
			}
			return s
		}
	}
	return ""
}

func resultText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	if items, ok := content.([]any); ok {
		var texts []string
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == "text" {
					if s, ok := m["text"].(string); ok {
						texts = append(texts, s)
					}
				}
			}
		}
		if len(texts) > 0 {
			return strings.Join(texts, "\n")
		}
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	return string(raw)
}

func formatFileFacts(history []any) string {
	pathByCallID := map[string]string{}
	for _, m := range history {
		for _, part := range assistantContent(m) {
			call, ok := part.(loop.ToolCallPart)
			if !ok || !strings.Contains(strings.ToLower(call.Call.Name), "read") {
				continue
			}
			var args map[string]any
			if err := json.Unmarshal(call.Call.Arguments, &args); err != nil {
				continue
			}
			for _, key := range []string{"path", "filePath", "targetFile"} {
				if s, ok := args[key].(string); ok && strings.TrimSpace(s) != "" {
					pathByCallID[call.Call.ID] = strings.TrimSpace(s)
				}
			}
		}
	}
	type fact struct{ path, text string }
	var results []fact
	for _, m := range history {
		msg, ok := asToolResult(m)
		if !ok {
			continue
		}
		for _, res := range msg.Results {
			path, ok := pathByCallID[res.ToolCallID]
			if !ok || path == "" || res.IsError {
				continue
			}
			text := resultText(res.Content)
			if strings.TrimSpace(text) == "" {
				continue
			}
			results = append(results, fact{path, text})
		}
	}
	seen := map[string]bool{}
	var picked []fact
	for i := len(results) - 1; i >= 0 && len(picked) < maxFileFacts; i-- {
		if seen[results[i].path] {
			continue
		}
		seen[results[i].path] = true
		picked = append([]fact{results[i]}, picked...)
	}
	if len(picked) == 0 {
		return ""
	}
	rendered := make([]string, 0, len(picked))
	for _, f := range picked {
		snippet := f.text
		if len([]rune(snippet)) > maxFileFactChars {
			snippet = string([]rune(snippet)[:maxFileFactChars]) + "…"
		}
		rendered = append(rendered, "## "+f.path+"\n"+snippet)
	}
	return "<file-facts>\n" + strings.Join(rendered, "\n\n") + "\n</file-facts>"
}

func asToolResult(m any) (loop.ToolResultMessage, bool) {
	switch msg := m.(type) {
	case loop.ToolResultMessage:
		return msg, true
	case *loop.ToolResultMessage:
		if msg != nil {
			return *msg, true
		}
	}
	return loop.ToolResultMessage{}, false
}

// BuildStructuralSummary renders the deterministic checkpoint summary.
func BuildStructuralSummary(history []any) string {
	var highlights []string
	userTurnsCount, toolCallsCount := 0, 0

	frequency := map[string]int{}
	for _, m := range history {
		if user, ok := asUser(m); ok {
			if text := flatLines(user.Content); text != "" {
				frequency[text]++
			}
		}
	}
	reported := map[string]bool{}
	firstUserSeen := false

	for _, m := range history {
		switch msg := asMessage(m); {
		case msg.isUser:
			userTurnsCount++
			text := flatLines(msg.text)
			if text != "" {
				if !reported[text] {
					reported[text] = true
					repeats := frequency[text]
					suffix := ""
					if repeats > 1 {
						suffix = fmt.Sprintf(" (%dx)", repeats)
					}
					budget := maxPromptChars
					if !firstUserSeen {
						budget = maxFirstPromptChars
					}
					truncated := text
					if len([]rune(truncated)) > budget {
						truncated = string([]rune(truncated)[:budget]) + "…"
					}
					highlights = append(highlights, fmt.Sprintf("- User requested%s: \"%s\"", suffix, truncated))
				}
			}
			firstUserSeen = true
		case msg.isAssistant:
			var calls []string
			for _, part := range msg.parts {
				if call, ok := part.(loop.ToolCallPart); ok {
					toolCallsCount++
					if summary := formatToolArgs(call.Call.Arguments); summary != "" {
						calls = append(calls, call.Call.Name+"("+summary+")")
					} else {
						calls = append(calls, call.Call.Name)
					}
				}
			}
			if len(calls) > 0 {
				extra := ""
				if len(calls) > 4 {
					extra = fmt.Sprintf(" (+%d more)", len(calls)-4)
					calls = calls[:4]
				}
				highlights = append(highlights, "- Executed: "+strings.Join(calls, ", ")+extra)
			}
			var texts []string
			for _, part := range msg.parts {
				switch part := part.(type) {
				case loop.TextPart:
					if strings.TrimSpace(part.Text) != "" {
						texts = append(texts, strings.TrimSpace(part.Text))
					}
				case loop.ThinkingPart:
					if strings.TrimSpace(part.Text) != "" {
						texts = append(texts, strings.TrimSpace(part.Text))
					}
				}
			}
			joined := strings.Join(texts, " ")
			if joined != "" && !isErrorBoilerplate(joined) {
				if len([]rune(joined)) > maxPromptChars {
					joined = string([]rune(joined)[:maxPromptChars]) + "…"
				}
				highlights = append(highlights, "- Assistant concluded: \""+strings.ReplaceAll(joined, "\n", " ")+"\"")
			}
		case msg.isToolResult:
			for _, res := range msg.results {
				if res.IsError {
					errText := "Failed"
					if s, ok := res.Content.(string); ok {
						errText = s
						if len([]rune(errText)) > 80 {
							errText = string([]rune(errText)[:80])
						}
					}
					highlights = append(highlights, "  ↳ [error]: "+strings.ReplaceAll(errText, "\n", " "))
				}
			}
		}
	}

	var deduped []string
	for _, line := range highlights {
		if len(deduped) == 0 || deduped[len(deduped)-1] != line {
			deduped = append(deduped, line)
		}
	}
	highlightsText := strings.Join(deduped, "\n")
	if len(highlightsText) > maxHighlightsChars {
		highlightsText = highlightsText[:maxHighlightsChars] + "\n[…prior highlights truncated…]"
	}
	if highlightsText == "" {
		highlightsText = "- Completed preliminary exploratory operations."
	}

	fileOps := ExtractFileOps(history)
	fileTree := FormatFileTree(fileOps)

	parts := []string{
		`Prior model work/tool state available.`,
		`MUST build on prior work; NEVER duplicate prior work.`,
		``,
		`<summary>`,
		fmt.Sprintf("[Conversation Checkpoint: Compacted %d messages (%d user requests, %d tool operations)]",
			len(history), userTurnsCount, toolCallsCount),
		``,
		`Key Actions & Context:`,
		highlightsText,
		`</summary>`,
	}
	if fileTree != "" {
		parts = append(parts, "", fileTree)
	}
	if facts := formatFileFacts(history); facts != "" {
		parts = append(parts, "", facts)
	}
	return strings.Join(parts, "\n")
}

type messageView struct {
	isUser, isAssistant, isToolResult bool
	text                              string
	parts                             []any
	results                           []tools.ToolResult
}

func asUser(m any) (loop.UserMessage, bool) {
	switch msg := m.(type) {
	case loop.UserMessage:
		return msg, true
	case *loop.UserMessage:
		if msg != nil {
			return *msg, true
		}
	}
	return loop.UserMessage{}, false
}

func asMessage(m any) messageView {
	switch msg := m.(type) {
	case loop.UserMessage:
		return messageView{isUser: true, text: msg.Content}
	case *loop.UserMessage:
		if msg != nil {
			return messageView{isUser: true, text: msg.Content}
		}
	case loop.AssistantMessage:
		return messageView{isAssistant: true, parts: msg.Content}
	case *loop.AssistantMessage:
		if msg != nil {
			return messageView{isAssistant: true, parts: msg.Content}
		}
	case loop.ToolResultMessage:
		return messageView{isToolResult: true, results: msg.Results}
	case *loop.ToolResultMessage:
		if msg != nil {
			return messageView{isToolResult: true, results: msg.Results}
		}
	}
	return messageView{}
}

// flatLines trims then collapses newline runs to spaces (TS
// content.trim().replace(/\n+/g, " ")).
func flatLines(s string) string {
	var b strings.Builder
	newlines := 0
	for _, r := range s {
		if r == '\n' {
			newlines++
			continue
		}
		if newlines > 0 {
			b.WriteByte(' ')
			newlines = 0
		}
		b.WriteRune(r)
	}
	if newlines > 0 {
		b.WriteByte(' ')
	}
	return strings.TrimSpace(b.String())
}

func isErrorBoilerplate(s string) bool {
	trimmed := strings.TrimLeft(s, " \t\n\r\v\f")
	if len(trimmed) < 6 {
		return false
	}
	return strings.HasPrefix(strings.ToLower(trimmed), "error:")
}
