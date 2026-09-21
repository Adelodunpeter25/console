// LLM compaction summaries via a text-capable provider. Port of
// apps/server/agent/src/compaction/llm-compaction.ts: flatten discarded
// turns to a bounded transcript, pack into one user message, and stream a
// narrative summary. Opt-in via Options.SummaryStrategy "llm"; failures
// fall back to structural (TS createSmolSummarizer parity).
package compaction

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// SummaryInputMaxChars caps transcript characters sent to the summarizer.
const SummaryInputMaxChars = 60000

const summaryLineMaxChars = 2000

const defaultSummarySystemPrompt = "Summarize the prior coding conversation for future continuation. Preserve decisions, files, changes, unresolved work, and important constraints. Return only the summary."

func argSnippet(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var text string
	var v any
	if err := json.Unmarshal(args, &v); err != nil {
		return ""
	}
	if s, ok := v.(string); ok {
		text = s
	} else {
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		text = string(raw)
	}
	flat := strings.Join(strings.Fields(text), " ")
	if len([]rune(flat)) > 200 {
		return string([]rune(flat)[:200]) + "…"
	}
	return flat
}

func sliceLine(text string) string {
	flat := strings.Join(strings.Fields(text), " ")
	if len([]rune(flat)) > summaryLineMaxChars {
		return string([]rune(flat)[:summaryLineMaxChars]) + "…"
	}
	return flat
}

// BuildSummaryTranscript flattens discarded turns to transcript lines,
// oldest first.
func BuildSummaryTranscript(history []any) []string {
	var lines []string
	for _, m := range history {
		switch msg := m.(type) {
		case loop.UserMessage:
			if strings.TrimSpace(msg.Content) != "" {
				lines = append(lines, "[user] "+sliceLine(msg.Content))
			}
			for _, a := range msg.Attachments {
				lines = append(lines, "[user] (image attached: "+a.MimeType+")")
			}
		case *loop.UserMessage:
			if msg != nil {
				if strings.TrimSpace(msg.Content) != "" {
					lines = append(lines, "[user] "+sliceLine(msg.Content))
				}
				for _, a := range msg.Attachments {
					lines = append(lines, "[user] (image attached: "+a.MimeType+")")
				}
			}
		case loop.AssistantMessage:
			lines = append(lines, transcriptParts(msg.Content)...)
		case *loop.AssistantMessage:
			if msg != nil {
				lines = append(lines, transcriptParts(msg.Content)...)
			}
		case loop.ToolResultMessage:
			lines = append(lines, transcriptResults(msg.Results)...)
		case *loop.ToolResultMessage:
			if msg != nil {
				lines = append(lines, transcriptResults(msg.Results)...)
			}
		}
	}
	return lines
}

func transcriptParts(content []any) []string {
	var lines []string
	for _, part := range content {
		switch part := part.(type) {
		case loop.TextPart:
			if strings.TrimSpace(part.Text) != "" {
				lines = append(lines, "[assistant] "+sliceLine(part.Text))
			}
		case loop.ThinkingPart:
			if strings.TrimSpace(part.Text) != "" {
				lines = append(lines, "[assistant thinking] "+sliceLine(part.Text))
			}
		case loop.ToolCallPart:
			args := argSnippet(part.Call.Arguments)
			if args != "" {
				lines = append(lines, "[tool:"+part.Call.Name+"] "+args)
			} else {
				lines = append(lines, "[tool:"+part.Call.Name+"]")
			}
		}
	}
	return lines
}

func transcriptResults(results []tools.ToolResult) []string {
	var lines []string
	for _, res := range results {
		var raw string
		if s, ok := res.Content.(string); ok {
			raw = s
		} else {
			encoded, err := json.Marshal(res.Content)
			if err != nil {
				continue
			}
			raw = string(encoded)
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		tag := "[result]"
		if res.IsError {
			tag = "[result (error)]"
		}
		lines = append(lines, tag+" "+sliceLine(raw))
	}
	return lines
}

// PackSummaryInput packs transcript lines into a single user message within
// budget, keeping the oldest lines.
func PackSummaryInput(history []any, maxChars int) []any {
	if maxChars <= 0 {
		maxChars = SummaryInputMaxChars
	}
	var kept []string
	total := 0
	for _, line := range BuildSummaryTranscript(history) {
		if total+len(line)+1 > maxChars {
			kept = append(kept, "[... newer content omitted for length ...]")
			break
		}
		kept = append(kept, line)
		total += len(line) + 1
	}
	if len(kept) == 0 {
		kept = append(kept, "(no prior content)")
	}
	return []any{loop.UserMessage{Role: loop.RoleUser, Content: strings.Join(kept, "\n")}}
}

// SummarizeWithProvider streams a narrative summary from a provider.
// Empty output is an error (caller falls back to structural).
func SummarizeWithProvider(ctx context.Context, provider loop.Provider, modelID string, history []any, systemPrompt string) (string, error) {
	if systemPrompt == "" {
		systemPrompt = defaultSummarySystemPrompt
	}
	events := stream.New[loop.Event]()
	req := loop.TurnRequest{
		Model: modelID, SystemPrompt: systemPrompt,
		Messages: PackSummaryInput(history, SummaryInputMaxChars),
	}
	if err := provider.RunTurn(ctx, req, events); err != nil {
		return "", err
	}
	var out strings.Builder
	for {
		event, err, ok := events.Next()
		if !ok || err != nil {
			break
		}
		if event.Kind == loop.EventText {
			out.WriteString(event.Text)
		}
	}
	summary := strings.TrimSpace(out.String())
	if summary == "" {
		return "", fmt.Errorf("Empty compaction summary")
	}
	return summary, nil
}
