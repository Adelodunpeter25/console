// Agent loop: drives turns against a provider, executing tool calls until
// the model stops. Port of apps/server/agent/src/service/agent-loop.ts
// (initial slice: sequential turns, session persistence, event stream).
package loop

import (
	"context"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// Provider is the Phase 3 seam: a streaming model backend. The loop only
// depends on this interface, so real providers (claude, codex...) plug in
// without loop changes. Contract: RunTurn must terminate the stream via
// Complete (success) or Fail (error) before returning; otherwise the turn
// blocks forever waiting for events.
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
	// EventQueueUpdated carries the staged QueuedPrompt (or null).
	EventQueueUpdated EventKind = "queueUpdated"
	// EventSessionTitleUpdated carries a freshly generated title.
	EventSessionTitleUpdated EventKind = "sessionTitleUpdated"
	// EventSubagentStart/Activity/End carry subagent lifecycle payloads.
	EventSubagentStart    EventKind = "subagentStart"
	EventSubagentActivity EventKind = "subagentActivity"
	EventSubagentEnd      EventKind = "subagentEnd"
	// Wire-structure markers. The run service broadcasts these around the
	// raw provider stream so the SSE layer can emit the TS/desktop event
	// vocabulary (sessionStart/turnStart/modelStream*/toolExecution*/turnEnd/
	// sessionEnd) instead of internal-turn kinds.
	// EventSessionStart opens a run.
	EventSessionStart EventKind = "sessionStart"
	// EventTurnStart opens a turn; Text carries the prompt.
	EventTurnStart EventKind = "turnStart"
	// EventModelStreamStart opens a model stream; Text carries the turn id.
	EventModelStreamStart EventKind = "modelStreamStart"
	// EventModelStreamPart carries one rendered part in Part.
	EventModelStreamPart EventKind = "modelStreamPart"
	// EventModelStreamEnd closes a model stream; Text is the turn id,
	// Message the completed assistant message.
	EventModelStreamEnd EventKind = "modelStreamEnd"
	// EventToolExecutionStart opens a tool phase; Calls holds the calls.
	EventToolExecutionStart EventKind = "toolExecutionStart"
	// EventToolExecutionResult carries one tool result.
	EventToolExecutionResult EventKind = "toolExecutionResult"
	// EventToolExecutionEnd closes a tool phase; Results holds the results.
	EventToolExecutionEnd EventKind = "toolExecutionEnd"
	// EventTurnEnd closes a turn; Text carries the turn id.
	EventTurnEnd EventKind = "turnEnd"
	// EventSessionEnd closes a run.
	EventSessionEnd EventKind = "sessionEnd"
	// EventTodoUpdate carries the session todo list ({items, action}).
	EventTodoUpdate EventKind = "todoUpdate"
)

type Event struct {
	Kind       EventKind                 `json:"kind"`
	Text       string                    `json:"text,omitempty"`
	Call       *tools.ToolCall           `json:"call,omitempty"`
	Result     *tools.ToolResult         `json:"result,omitempty"`
	StopReason StopReason                `json:"stopReason,omitempty"`
	Message    any                       `json:"message,omitempty"`
	Usage      *TurnUsage                `json:"usage,omitempty"`
	Ask        *tools.AskQuestionRequest `json:"ask,omitempty"`
	Permission *permissions.Request      `json:"permission,omitempty"`
	Queued     *types.QueuedPrompt       `json:"queuedPrompt,omitempty"`
	Title      string                    `json:"title,omitempty"`
	Subagent   any                       `json:"subagent,omitempty"`
	// Items/Action carry the session todo list for EventTodoUpdate.
	Items  []types.TodoItem `json:"items,omitempty"`
	Action string           `json:"action,omitempty"`
	// Part carries a rendered model-stream part ({text}|{thinking}|{toolCall})
	// for EventModelStreamPart. Calls/Results accumulate a tool phase for
	// EventToolExecutionStart/End. All three are consumed by the SSE layer
	// when building wire frames and never serialized as-is.
	Part    any                `json:"-"`
	Calls   []tools.ToolCall   `json:"-"`
	Results []tools.ToolResult `json:"-"`
	// ThoughtSignature accompanies EventText for Gemini's reasoning-
	// continuity token. Unused by Claude/Codex.
	ThoughtSignature string `json:"-"`
}

// streamOf is a thin alias over the generic stream for loop events.
type streamOf = stream.Stream[Event]

// CompactionHooks plugs context management into turns without importing
// the compaction package (which operates on loop types). All fields
// optional; nil Compaction disables the behavior.
type CompactionHooks struct {
	// PreTurn rewrites history before the provider call.
	PreTurn func(ctx context.Context, history []any) []any
	// IsOverflow reports context-window failures for emergency recovery.
	IsOverflow func(err error) bool
	// Emergency rewrites history after an overflow (single retry).
	Emergency func(history []any) []any
}

// Agent runs the loop for one user prompt: provider turns + tool execution
// until the model stops, persisting every message into the session.
type Agent struct {
	provider Provider
	executor *Executor
	// sessions persists turns; nil runs fully in-memory (subagents).
	sessions *services.SessionService
	// Run options forwarded to the provider each turn. Zero values mean
	// provider defaults; the run service sets them per session/model.
	SystemPrompt   string
	Model          string
	CacheRetention CacheRetention
	ConversationID string
	ThinkingLevel  string
	// Compaction optionally rewrites history before each turn.
	Compaction *CompactionHooks
}

func New(provider Provider, executor *Executor, sessions *services.SessionService) *Agent {
	return &Agent{provider: provider, executor: executor, sessions: sessions}
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
	// Persist the user message first (skipped for in-memory subagents).
	if err := a.persist(sessionID, types.AgentMessage{
		ID:   newMessageID(),
		Role: string(RoleUser),
		Data: messageJSON(user),
	}); err != nil {
		events.Fail(fmt.Errorf("persist user message: %w", err))
		return
	}

	history = MaterializeHistory(history)
	history = append(history, MaterializeUserMessage(user))
	for {
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
		if err := a.persist(sessionID, types.AgentMessage{
			ID:   newMessageID(),
			Role: string(RoleToolResult),
			Data: messageJSON(resultMsg),
		}); err != nil {
			events.Fail(err)
			return
		}
		history = append(history, resultMsg)
	}
}

// turn runs one provider streaming turn, accumulating parts into an
// AssistantMessage and re-emitting stream events. A context-overflow
// failure triggers emergency compaction and exactly one retry.
func (a *Agent) turn(ctx context.Context, sessionID string, history []any, toolsList []tools.Definition, events *stream.Stream[Event]) (AssistantMessage, error) {
	overflowRetried := false
	for {
		working := history
		if a.Compaction != nil && a.Compaction.PreTurn != nil {
			working = a.Compaction.PreTurn(ctx, history)
		}
		assistant, err := a.turnOnce(ctx, sessionID, working, toolsList, events)
		if err == nil {
			return assistant, nil
		}
		if overflowRetried || ctx.Err() != nil || a.Compaction == nil ||
			a.Compaction.IsOverflow == nil || a.Compaction.Emergency == nil ||
			!a.Compaction.IsOverflow(err) {
			return assistant, err
		}
		overflowRetried = true
		history = a.Compaction.Emergency(history)
	}
}

// turnOnce streams a single attempt, persisting the assistant turn only on
// success.
func (a *Agent) turnOnce(ctx context.Context, sessionID string, history []any, toolsList []tools.Definition, events *stream.Stream[Event]) (AssistantMessage, error) {
	assistant := AssistantMessage{Role: RoleAssistant, ID: newMessageID(), Content: []any{}}
	// Bracket every provider turn with stream markers (TS streamOneTurn
	// parity) so subscribers get per-turn triplets with the canonical
	// assistant snapshot — the desktop replaces streamed content with it.
	turnID := newTurnID()
	events.Push(Event{Kind: EventModelStreamStart, Text: turnID})
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
			if event.Text == "" {
				break
			}
			// Coalesce consecutive text deltas into a single TextPart.
			// Without this each streamed word persists as its own part,
			// and clients render every part as a separate vertical
			// markdown block (one word per line). Mirrors TS streamOneTurn.
			if n := len(assistant.Content); n > 0 {
				if last, ok := assistant.Content[n-1].(TextPart); ok {
					last.Text += event.Text
					if event.ThoughtSignature != "" {
						last.ThoughtSignature = event.ThoughtSignature
					}
					assistant.Content[n-1] = last
					break
				}
			}
			assistant.Content = append(assistant.Content, TextPart{Type: "text", Text: event.Text, ThoughtSignature: event.ThoughtSignature})
		case EventThinking:
			if event.Text == "" {
				break
			}
			if n := len(assistant.Content); n > 0 {
				if last, ok := assistant.Content[n-1].(ThinkingPart); ok {
					last.Text += event.Text
					assistant.Content[n-1] = last
					break
				}
			}
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

	if err := a.persist(sessionID, types.AgentMessage{
		ID:   assistant.ID,
		Role: string(RoleAssistant),
		Data: messageJSON(assistant),
	}); err != nil {
		return assistant, err
	}
	events.Push(Event{Kind: EventModelStreamEnd, Text: turnID, Message: assistant})
	events.Push(Event{Kind: EventTurnEnd, Text: turnID})
	return assistant, nil
}

// persist appends a message unless the agent runs in-memory (subagents).
func (a *Agent) persist(sessionID string, msg types.AgentMessage) error {
	if a.sessions == nil {
		return nil
	}
	return a.sessions.AppendMessage(sessionID, msg)
}
