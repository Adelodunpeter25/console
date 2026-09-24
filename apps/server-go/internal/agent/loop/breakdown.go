// Per-source request size breakdown: estimates how many tokens each part
// of a provider request contributes (system-prompt sections, tool
// definitions, and history split by kind), so token spend can be ranked
// by source (docs/plan/harness-token-efficiency.md, Task 1.2).
//
// Estimates use the same heuristic as compaction (~4 chars/token prose,
// ~3 chars/token code/tool output); they rank sources, they don't bill.
package loop

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

const (
	breakdownProseChars = 4.0
	breakdownCodeChars  = 3.0
	breakdownImage      = 1000
)

// NamedText is one labeled block of the system prompt.
type NamedText struct {
	Name    string
	Content string
}

// Breakdown is the estimated token count per request source.
type Breakdown struct {
	// System maps system-prompt section name → tokens.
	System map[string]int `json:"system"`
	// Tools maps tool name → tokens (name + description + schema).
	Tools map[string]int `json:"tools"`
	// History buckets: user text, assistant text/thinking, assistant
	// tool-call arguments, compaction summaries.
	UserText      int `json:"userText"`
	AssistantText int `json:"assistantText"`
	ToolCallArgs  int `json:"toolCallArgs"`
	Summaries     int `json:"summaries"`
	// ToolResults maps tool name → tokens of its results in history.
	ToolResults map[string]int `json:"toolResults"`
}

// Total sums every bucket.
func (b Breakdown) Total() int {
	total := b.UserText + b.AssistantText + b.ToolCallArgs + b.Summaries
	for _, m := range []map[string]int{b.System, b.Tools, b.ToolResults} {
		for _, v := range m {
			total += v
		}
	}
	return total
}

func proseTokens(s string) int {
	return int(math.Ceil(float64(len([]rune(s))) / breakdownProseChars))
}

func codeTokens(n int) int {
	return int(math.Ceil(float64(n) / breakdownCodeChars))
}

// summaryMarkers identify compaction-produced user messages.
var summaryMarkers = []string{"[Conversation Checkpoint:", "<file-facts>"}

func isSummary(text string) bool {
	for _, marker := range summaryMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// EstimateBreakdown estimates tokens per source for one request. sections
// may be nil, in which case the whole system prompt is one "system" entry.
func EstimateBreakdown(sections []NamedText, systemPrompt string, defs []tools.Definition, history []any) Breakdown {
	b := Breakdown{System: map[string]int{}, Tools: map[string]int{}, ToolResults: map[string]int{}}
	if len(sections) > 0 {
		for _, s := range sections {
			b.System[s.Name] += proseTokens(s.Content)
		}
	} else if systemPrompt != "" {
		b.System["system"] = proseTokens(systemPrompt)
	}
	for _, d := range defs {
		schema, _ := json.Marshal(d.InputSchema)
		b.Tools[d.Name] += proseTokens(d.Name+d.Description) + codeTokens(len(schema))
	}
	for _, m := range history {
		switch msg := m.(type) {
		case UserMessage:
			b.addUser(msg)
		case *UserMessage:
			if msg != nil {
				b.addUser(*msg)
			}
		case AssistantMessage:
			b.addAssistant(msg)
		case *AssistantMessage:
			if msg != nil {
				b.addAssistant(*msg)
			}
		case ToolResultMessage:
			b.addResults(msg)
		case *ToolResultMessage:
			if msg != nil {
				b.addResults(*msg)
			}
		}
	}
	return b
}

func (b *Breakdown) addUser(msg UserMessage) {
	tokens := proseTokens(msg.Content) + len(msg.Attachments)*breakdownImage
	if isSummary(msg.Content) {
		b.Summaries += tokens
		return
	}
	b.UserText += tokens
}

func (b *Breakdown) addAssistant(msg AssistantMessage) {
	for _, part := range msg.Content {
		switch p := part.(type) {
		case TextPart:
			b.AssistantText += proseTokens(p.Text)
		case ThinkingPart:
			b.AssistantText += proseTokens(p.Text)
		case ToolCallPart:
			b.ToolCallArgs += codeTokens(len(p.Call.Arguments))
		}
	}
}

func (b *Breakdown) addResults(msg ToolResultMessage) {
	for _, r := range msg.Results {
		name := r.ToolName
		if name == "" {
			name = "unknown"
		}
		b.ToolResults[name] += codeTokens(contentChars(r.Content))
	}
}

func contentChars(content any) int {
	if s, ok := content.(string); ok {
		return len([]rune(s))
	}
	if content == nil {
		return 0
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return 0
	}
	return len(raw)
}
