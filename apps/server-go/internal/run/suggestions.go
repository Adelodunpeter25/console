// Follow-up suggestions: after a run settles cleanly, a smol model proposes
// next prompts and they are pushed over the notification stream (the run
// hub is already closed by then). Not persisted.
package run

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/roles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/suggestions"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
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
	ctx, cancel := context.WithCancel(context.Background())
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
		if bus := s.notifier(); bus != nil {
			bus.Push(SuggestionsNotification(sessionID, items))
		}
	}()
}

// cancelSuggestions drops any in-flight generation for the session; a new
// run or a deletion makes its result stale.
func (s *Service) cancelSuggestions(sessionID string) {
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
	if !ok || suggestions.EndsWithQuestion(in.AssistantText) {
		return nil
	}
	model := roles.ResolveRoleModel(roles.Smol, s.resolveModel(ctx, providerID, modelID), ref)
	provider, err := s.Lookup(model.Provider)
	if err != nil {
		return nil
	}
	return suggestions.Generate(ctx, provider, model.ID, in, pins.forProvider(model.Provider))
}

// suggestionInput distills the latest exchange: the last real user message,
// the assistant's final text after it, and which tools ran in between.
func suggestionInput(history []any) (suggestions.Input, bool) {
	lastUser := -1
	for i := len(history) - 1; i >= 0; i-- {
		if u, ok := history[i].(loop.UserMessage); ok && !u.Synthetic {
			lastUser = i
			break
		}
	}
	if lastUser < 0 {
		return suggestions.Input{}, false
	}
	in := suggestions.Input{UserText: history[lastUser].(loop.UserMessage).Content}
	counts := map[string]int{}
	for _, msg := range history[lastUser+1:] {
		assistant, ok := msg.(loop.AssistantMessage)
		if !ok {
			continue
		}
		if text := assistantText(assistant); text != "" {
			in.AssistantText = text
		}
		for _, part := range assistant.Content {
			if call, ok := part.(loop.ToolCallPart); ok {
				counts[call.Call.Name]++
			}
		}
	}
	if strings.TrimSpace(in.AssistantText) == "" {
		return suggestions.Input{}, false
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s x%d", name, counts[name]))
	}
	in.ToolDigest = strings.Join(parts, ", ")
	return in, true
}
