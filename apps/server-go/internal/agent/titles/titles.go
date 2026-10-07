// Session title generation: generic-title detection,
// truncation fallback, sanitizing, and LLM generation (same-model; there is
// no separate small-model role resolver yet).
package titles

import (
	"context"
	"regexp"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

var (
	leadMarkers = regexp.MustCompile(`^\s*[-*#]+\s*`)
	edgeQuotes  = regexp.MustCompile(`^["'` + "`" + `]+|["'` + "`.]+$")
)

// IsGenericTitle reports placeholder titles not worth keeping.
func IsGenericTitle(title string) bool {
	switch strings.TrimSpace(title) {
	case "", "New Session", "New mobile session", "New Chat", "New chat", "Untitled":
		return true
	default:
		return false
	}
}

// FallbackTitle truncates the prompt to 35 chars. The first letter is
// capitalized so a fallback title matches the generated ones.
func FallbackTitle(prompt string) string {
	compact := strings.Join(strings.Fields(prompt), " ")
	if len([]rune(compact)) > 35 {
		return utils.TitleFirst(string([]rune(compact)[:35]) + "...")
	}
	return utils.TitleFirst(compact)
}

// SanitizeTitle single-lines, strips markers/quotes, keeps ≤8 words/80 chars,
// and capitalizes the first letter so generated titles read consistently
// across desktop and mobile. Capitalization runs last so it never lands on a
// marker or quote the earlier steps strip.
func SanitizeTitle(value string) string {
	singleLine := strings.ReplaceAll(value, "\n", " ")
	singleLine = strings.ReplaceAll(singleLine, "\r", " ")
	singleLine = leadMarkers.ReplaceAllString(singleLine, "")
	singleLine = edgeQuotes.ReplaceAllString(singleLine, "")
	words := strings.Fields(singleLine)
	if len(words) > 8 {
		words = words[:8]
	}
	joined := strings.Join(words, " ")
	if len([]rune(joined)) > 80 {
		joined = strings.TrimSpace(string([]rune(joined)[:80]))
	}
	return utils.TitleFirst(joined)
}

// Generate asks the provider for a ~six-word title. Returns "" when the
// model yields nothing usable (caller falls back to truncation).
func Generate(ctx context.Context, provider loop.Provider, model, prompt string) string {
	events := stream.New[loop.Event]()
	req := loop.TurnRequest{
		Model:        model,
		SystemPrompt: "Create a concise session title from the user's request. Output one single-line sentence of about six words. No quotes, markdown, explanation, or punctuation.",
		Messages:     []any{loop.UserMessage{Role: loop.RoleUser, Content: prompt}},
	}
	if err := provider.RunTurn(ctx, req, events); err != nil {
		return ""
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
	return SanitizeTitle(out.String())
}
