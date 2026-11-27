// Follow-up suggestions: after a run settles cleanly, a smol model proposes
// next prompts and they are pushed over the notification stream (the run
// hub is already closed by then).
package run

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/MoeForge/moego/json"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/roles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/suggestions"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

const (
	suggestionTimeout    = 15 * time.Second
	maxSuggestionTurns   = 3
	maxToolDigestEntries = 8
	maxOpenTodos         = 5
)

// SuggestionsNotification carries the chips for one chat.
func SuggestionsNotification(sessionID string, items []string) *consolev1.NotificationEvent {
	return &consolev1.NotificationEvent{
		Type: "notification", Kind: "suggestions", SessionId: sessionID, Items: items,
	}
}

// startSuggestions generates suggestions off the critical path. It is a
// no-op unless a smol role is configured: unlike titles there is no
// fallback to the chat model, since this would cost a model call per run.
func (s *Service) startSuggestions(sessionID, providerID, modelID string, pins *accountPins) {
	ref := s.roleRef(roles.Smol)
	if !roles.HasConfiguredRole(map[string]string{roles.Smol: ref}, roles.Smol) || s.notifier() == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), suggestionTimeout)
	s.mu.Lock()
	if prev := s.suggestCancel[sessionID]; prev != nil {
		prev()
	}
	if s.suggestCancel == nil {
		s.suggestCancel = map[string]context.CancelFunc{}
	}
	s.suggestCancel[sessionID] = cancel
	s.mu.Unlock()

	go func() {
		defer cancel()
		items := s.generateSuggestions(ctx, sessionID, providerID, modelID, ref, pins)
		s.mu.Lock()
		_, busy := s.active[sessionID]
		current := ctx.Err() == nil
		if current {
			delete(s.suggestCancel, sessionID)
		}
		s.mu.Unlock()
		if len(items) == 0 || busy || !current {
			return
		}
		// Stored before the push so a client that reconnects between the two
		// still finds them.
		if err := s.sessions.SaveSessionSuggestions(sessionID, items); err != nil {
			slog.Warn("run: save suggestions failed", "session", sessionID, "error", err)
		}
		if bus := s.notifier(); bus != nil {
			bus.Push(SuggestionsNotification(sessionID, items))
		}
	}()
}

// cancelSuggestions drops any in-flight generation and the stored set for
// the session; a new run makes both stale.
func (s *Service) cancelSuggestions(sessionID string) {
	if err := s.sessions.ClearSessionSuggestions(sessionID); err != nil {
		slog.Warn("run: clear suggestions failed", "session", sessionID, "error", err)
	}
	s.mu.Lock()
	cancel := s.suggestCancel[sessionID]
	delete(s.suggestCancel, sessionID)
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Service) generateSuggestions(ctx context.Context, sessionID, providerID, modelID, ref string, pins *accountPins) []string {
	history, err := s.LoadHistory(sessionID)
	if err != nil {
		return nil
	}
	in, ok := suggestionInput(history)
	if !ok {
		return nil
	}
	if todos, err := s.sessions.GetSessionTodos(sessionID); err == nil {
		for _, todo := range todos {
			if todo.Status == "completed" {
				continue
			}
			in.OpenTodos = append(in.OpenTodos, fmt.Sprintf("[%s] %s", todo.Status, truncateRunes(todo.Content, 100)))
			if len(in.OpenTodos) == maxOpenTodos {
				break
			}
		}
	}
	model := roles.ResolveRoleModel(roles.Smol, s.resolveModel(ctx, providerID, modelID), ref)
	provider, err := s.Lookup(model.Provider)
	if err != nil {
		return nil
	}
	thinking := minimalThinkingLevel(model.ThinkingLevels)
	return suggestions.Generate(ctx, provider, model.ID, thinking, in, pins.forProvider(model.Provider))
}

func minimalThinkingLevel(supported []string) string {
	for _, candidate := range []string{"none", "off", "minimal", "low"} {
		if slices.Contains(supported, candidate) {
			return candidate
		}
	}
	return ""
}

// suggestionInput distills the last few exchanges, plus tool targets and
// outcomes from the latest turn.
func suggestionInput(history []any) (suggestions.Input, bool) {
	var userIndices []int
	for i, msg := range history {
		if u, ok := msg.(loop.UserMessage); ok && !u.Synthetic {
			userIndices = append(userIndices, i)
		}
	}
	if len(userIndices) == 0 {
		return suggestions.Input{}, false
	}
	start := 0
	if len(userIndices) > maxSuggestionTurns {
		start = len(userIndices) - maxSuggestionTurns
	}
	recent := userIndices[start:]

	var turns []suggestions.Turn
	for idx, uPos := range recent {
		end := len(history)
		if idx+1 < len(recent) {
			end = recent[idx+1]
		}
		isLatest := idx == len(recent)-1
		userLimit, assistantLimit := 300, 300
		if isLatest {
			userLimit, assistantLimit = 600, 1000
		}
		userText := truncateRunes(history[uPos].(loop.UserMessage).Content, userLimit)
		var finalAssistant string
		for _, msg := range history[uPos+1 : end] {
			if assistant, ok := msg.(loop.AssistantMessage); ok {
				if text := assistantText(assistant); text != "" {
					finalAssistant = text
				}
			}
		}
		if isLatest && strings.TrimSpace(finalAssistant) == "" {
			return suggestions.Input{}, false
		}
		turns = append(turns, suggestions.Turn{
			UserText:      userText,
			AssistantText: truncateRunes(finalAssistant, assistantLimit),
		})
	}

	lastUser := recent[len(recent)-1]
	return suggestions.Input{
		Turns:      turns,
		ToolDigest: buildToolDigest(history[lastUser+1:]),
	}, true
}

func buildToolDigest(msgs []any) string {
	failed := map[string]bool{}
	for _, msg := range msgs {
		tr, ok := msg.(loop.ToolResultMessage)
		if !ok {
			continue
		}
		for _, res := range tr.Results {
			if res.IsError {
				failed[res.ToolCallID] = true
			}
		}
	}

	var entries []string
	total := 0
	for _, msg := range msgs {
		assistant, ok := msg.(loop.AssistantMessage)
		if !ok {
			continue
		}
		for _, part := range assistant.Content {
			call, ok := part.(loop.ToolCallPart)
			if !ok {
				continue
			}
			total++
			if len(entries) >= maxToolDigestEntries {
				continue
			}
			entry := call.Call.Name
			if target := toolCallTarget(call.Call.Arguments); target != "" {
				entry += "(" + target + ")"
			}
			if failed[call.Call.ID] {
				entry += " (failed)"
			}
			entries = append(entries, entry)
		}
	}
	if total > maxToolDigestEntries {
		entries = append(entries, fmt.Sprintf("+%d more", total-maxToolDigestEntries))
	}
	return strings.Join(entries, ", ")
}

func toolCallTarget(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return ""
	}
	for _, key := range []string{"path", "file_path", "command", "pattern", "query", "url"} {
		if val, ok := args[key].(string); ok && strings.TrimSpace(val) != "" {
			return truncateRunes(strings.Join(strings.Fields(val), " "), 48)
		}
	}
	return ""
}

func truncateRunes(s string, limit int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}
