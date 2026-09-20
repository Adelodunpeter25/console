// Agent loop: drives turns against a provider, executing tool calls until
// the model stops. Port of apps/server/agent/src/service/agent-loop.ts
// (initial slice: sequential turns, session persistence, event stream).
package loop

import (
	"context"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// Provider is the Phase 3 seam: a streaming model backend. The loop only
// depends on this interface, so real providers (claude, codex...) plug in
// without loop changes.
type Provider interface {
	// RunTurn streams one assistant turn for the given history. Events
	// arrive in order; the final event determines the stop reason.
	RunTurn(ctx context.Context, req TurnRequest, stream *streamOf) error
}

type TurnRequest struct {
	Model        string
	SystemPrompt string
	Messages     []any // UserMessage | AssistantMessage | ToolResultMessage
	Tools        []tools.Definition
	// Phase 3 provider options (all optional, zero value = provider default).
	// BaseURL overrides the provider's default endpoint (e.g. tests).
	BaseURL string
	// CacheRetention controls prompt-cache behavior ("short"|"long"|"none").
	CacheRetention CacheRetention
	// ConversationID is the stable per-conversation session id reused
	// across turns to keep the provider's prompt cache warm.
	ConversationID string
	// ThinkingLevel maps to the provider's reasoning effort (e.g. codex).
	ThinkingLevel string
}

// Events a turn can emit (mirrors the TS event stream vocabulary).
type EventKind string

const (
	EventText       EventKind = "text"
	EventThinking   EventKind = "thinking"
	EventToolCall   EventKind = "toolCall"
	EventToolResult EventKind = "toolResult"
	EventError      EventKind = "error"
	EventTurnDone   EventKind = "turnDone"
	EventUsage      EventKind = "usage"
	// EventAskQuestion carries an AskQuestionRequest awaiting the answer route.
	EventAskQuestion EventKind = "askQuestion"
	// EventPermissionRequest carries a permission Request awaiting approval.
	EventPermissionRequest EventKind = "permissionRequest"
)

type Event struct {
	Kind       EventKind         `json:"kind"`
	Text       string            `json:"text,omitempty"`
	Call       *tools.ToolCall   `json:"call,omitempty"`
	Result     *tools.ToolResult `json:"result,omitempty"`
	StopReason StopReason        `json:"stopReason,omitempty"`
	Message    any               `json:"message,omitempty"`
	Usage      *TurnUsage        `json:"usage,omitempty"`
	Ask        *tools.AskQuestionRequest `json:"ask,omitempty"`
	Permission *permissions.Request      `json:"permission,omitempty"`
}

// streamOf is a thin alias over the generic stream for loop events.
type streamOf = stream.Stream[Event]

// Agent runs the loop for one user prompt: provider turns + tool execution
// until the model stops, persisting every message into the session.
type Agent struct {
	provider Provider
	executor *Executor
	sessions *services.SessionService
	maxTurns int
	// Run options forwarded to the provider each turn. Zero values mean
	// provider defaults; the run service sets them per session/model.
	SystemPrompt   string
	Model          string
	CacheRetention CacheRetention
	ConversationID string
	ThinkingLevel  string
}

func New(provider Provider, executor *Executor, sessions *services.SessionService) *Agent {
	return &Agent{provider: provider, executor: executor, sessions: sessions, maxTurns: 20}
}

// Run processes the user prompt and streams events until the final turn.
func (a *Agent) Run(ctx context.Context, sessionID string, userText string, toolsList []tools.Definition) (*stream.Stream[Event], error) {
	return a.RunWithHistory(ctx, sessionID, nil, UserMessage{Role: RoleUser, Content: userText}, toolsList)
}

// RunWithHistory processes a user message with preloaded conversation
// history (decoded from session storage by the run service).
func (a *Agent) RunWithHistory(ctx context.Context, sessionID string, history []any, user UserMessage, toolsList []tools.Definition) (*stream.Stream[Event], error) {
	stream := stream.New[Event]()
	go a.run(ctx, sessionID, history, user, toolsList, stream)
	return stream, nil
}

func (a *Agent) run(ctx context.Context, sessionID string, history []any, user UserMessage, toolsList []tools.Definition, events *stream.Stream[Event]) {
	// Persist the user message first.
	if err := a.sessions.AppendMessage(sessionID, types.AgentMessage{
		ID:   newMessageID(),
		Role: string(RoleUser),
		Data: messageJSON(user),
	}); err != nil {
		events.Fail(fmt.Errorf("persist user message: %w", err))
		return
	}

	history = append(append([]any{}, history...), user)
	for turn := 0; turn < a.maxTurns; turn++ {
		assistant, err := a.turn(ctx, sessionID, history, toolsList, events)
		if err != nil {
			events.Fail(err)
			return
		}
		history = append(history, assistant)

		if assistant.StopReason != StopToolUse {
			events.Push(Event{Kind: EventTurnDone, StopReason: assistant.StopReason, Message: assistant})
			events.Complete()
			return
		}

		// Execute every tool call the model requested this turn.
		results := make([]tools.ToolResult, 0, len(assistant.Content))
		for _, part := range assistant.Content {
			if callPart, ok := part.(ToolCallPart); ok {
				result, err := a.executor.Execute(ctx, callPart.Call)
				if err != nil {
					events.Fail(err)
					return
				}
				results = append(results, result)
				events.Push(Event{Kind: EventToolResult, Result: &result})
			}
		}
		resultMsg := ToolResultMessage{Role: RoleToolResult, Results: results}
		if err := a.sessions.AppendMessage(sessionID, types.AgentMessage{
			ID:   newMessageID(),
			Role: string(RoleToolResult),
			Data: messageJSON(resultMsg),
		}); err != nil {
			events.Fail(err)
			return
		}
		history = append(history, resultMsg)
	}
	events.Fail(fmt.Errorf("agent exceeded %d turns", a.maxTurns))
}

// turn runs one provider streaming turn, accumulating parts into an
// AssistantMessage and re-emitting stream events.
func (a *Agent) turn(ctx context.Context, sessionID string, history []any, toolsList []tools.Definition, events *stream.Stream[Event]) (AssistantMessage, error) {
	assistant := AssistantMessage{Role: RoleAssistant, ID: newMessageID(), Content: []any{}}
	turnStream := stream.New[Event]()
	done := make(chan error, 1)
	go func() {
		done <- a.provider.RunTurn(ctx, TurnRequest{
			Model:          a.Model,
			SystemPrompt:   a.SystemPrompt,
			Messages:       history,
			Tools:          toolsList,
			CacheRetention: a.CacheRetention,
			ConversationID: a.ConversationID,
			ThinkingLevel:  a.ThinkingLevel,
		}, turnStream)
	}()

	for {
		event, err, ok := turnStream.Next()
		if !ok {
			if err != nil {
				return assistant, err
			}
			break
		}
		switch event.Kind {
		case EventText:
			assistant.Content = append(assistant.Content, TextPart{Type: "text", Text: event.Text})
		case EventThinking:
			assistant.Content = append(assistant.Content, ThinkingPart{Type: "thinking", Text: event.Text})
		case EventToolCall:
			if event.Call != nil {
				assistant.Content = append(assistant.Content, ToolCallPart{Type: "toolCall", Call: *event.Call})
				assistant.StopReason = StopToolUse
			}
		case EventUsage:
			if event.Usage != nil {
				assistant.Usage = event.Usage
			}
		case EventError:
			assistant.StopReason = StopError
		}
		events.Push(event)
	}
	if err := <-done; err != nil {
		return assistant, err
	}
	if assistant.StopReason == "" {
		assistant.StopReason = StopStop
	}

	if err := a.sessions.AppendMessage(sessionID, types.AgentMessage{
		ID:   assistant.ID,
		Role: string(RoleAssistant),
		Data: messageJSON(assistant),
	}); err != nil {
		return assistant, err
	}
	return assistant, nil
}
