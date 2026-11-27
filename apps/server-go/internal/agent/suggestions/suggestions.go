// Follow-up prompt suggestions: a small model proposes what the user might
// say next, shown as one-click chips once a run settles.
package suggestions

import (
	"context"
	"github.com/MoeForge/moego/json"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
)

const (
	MaxItems   = 2
	maxRunes   = 80
	systemText = "You suggest what the user might say next in a coding-agent chat. " +
		"Reply with only a JSON array of at most 2 strings. Each string is a short, concrete " +
		"follow-up the user could send, under 80 characters, written in the user's voice " +
		"(imperative or direct reply, no quotes or markdown). " +
		"If the assistant offered an action or asked a yes/no or choice question (e.g. \"Want me to run the tests?\"), " +
		"suggest concrete replies (e.g. \"Yes, run the tests\"). " +
		"Only reply with [] if the assistant is asking for open-ended information only the user knows, or nothing useful follows."
)

// Turn is one user/assistant exchange in the recent conversation.
type Turn struct {
	UserText      string
	AssistantText string
}

// Input is the context the suggestion model sees.
type Input struct {
	Turns      []Turn
	ToolDigest string
	OpenTodos  []string
}

// Parse extracts up to MaxItems usable suggestions from model output.
// Malformed output yields nothing; there is no retry.
func Parse(raw string) []string {
	raw = strings.TrimSpace(raw)
	start, end := strings.Index(raw, "["), strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil
	}
	var candidates []string
	if err := json.Unmarshal([]byte(raw[start:end+1]), &candidates); err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range candidates {
		c = strings.Trim(strings.Join(strings.Fields(c), " "), "\"'`")
		key := strings.ToLower(c)
		if c == "" || seen[key] {
			continue
		}
		if len([]rune(c)) > maxRunes {
			continue
		}
		seen[key] = true
		out = append(out, c)
		if len(out) == MaxItems {
			break
		}
	}
	return out
}

// Generate asks the provider for suggestions. accountID pins the call to
// the run's account, like title generation.
func Generate(ctx context.Context, provider loop.Provider, model, thinkingLevel string, in Input, accountID string) []string {
	var prompt strings.Builder
	for i, turn := range in.Turns {
		if i > 0 {
			prompt.WriteString("\n\n---\n\n")
		}
		prompt.WriteString("User said:\n" + turn.UserText + "\n\nAssistant replied:\n" + turn.AssistantText)
	}
	if in.ToolDigest != "" {
		prompt.WriteString("\n\nWork done: " + in.ToolDigest)
	}
	if len(in.OpenTodos) > 0 {
		prompt.WriteString("\n\nOpen tasks:\n- " + strings.Join(in.OpenTodos, "\n- "))
	}
	events := stream.New[loop.Event]()
	req := loop.TurnRequest{
		Model:         model,
		SystemPrompt:  systemText,
		Messages:      []any{loop.UserMessage{Role: loop.RoleUser, Content: prompt.String()}},
		AccountID:     accountID,
		ThinkingLevel: thinkingLevel,
	}
	if err := provider.RunTurn(ctx, req, events); err != nil {
		return nil
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
	return Parse(out.String())
}
