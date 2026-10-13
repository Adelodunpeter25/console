// Run-event wire encoding, shared by the SSE routes and the WebTransport
// server. Every hub event maps to an (event name, JSON body) pair; the SSE
// path frames it with an `id:` sequence line while the WT path packs the
// body bytes into a WtFrame payload — both byte-identical bodies, so client
// parsers are shared across transports.
//
// Payloads source every structured field from generated console.v1 types via
// protojson. Map envelopes here are map[string]any (fiber.Map is the same
// underlying type); encoding/json sorts map keys, so output is deterministic.
package run

import (
	"encoding/json"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var wireProtoMarshal = protojson.MarshalOptions{}

// marshalProtoList encodes a slice of messages as a JSON array of protojson
// objects.
func marshalProtoList[T proto.Message](items []T) (json.RawMessage, error) {
	elements := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		raw, err := wireProtoMarshal.Marshal(item)
		if err != nil {
			return nil, err
		}
		elements = append(elements, raw)
	}
	return json.Marshal(elements)
}

// modelStreamPartFrame is the one wire frame emitted per model token, so it
// is a struct rather than a map: encoding/json reflects over maps and sorts
// their keys on every call. The part itself is protojson (shared schema).
type modelStreamPartFrame struct {
	Type string          `json:"type"`
	Part json.RawMessage `json:"part"`
}

// queuedPromptToProto converts a staged prompt row to the canonical wire
// type. Attachments/annotations reuse the message schema; created_at stays
// the RFC3339 string the server stores; empty optionals stay absent.
func queuedPromptToProto(qp *types.QueuedPrompt) *consolev1.QueuedPrompt {
	if qp == nil {
		return nil
	}
	out := &consolev1.QueuedPrompt{
		Id: qp.ID, SessionId: qp.SessionID, Prompt: qp.Prompt, CreatedAt: qp.CreatedAt,
	}
	out.ContextFiles = append(out.ContextFiles, qp.ContextFiles...)
	for _, a := range qp.Attachments {
		out.Attachments = append(out.Attachments, &consolev1.ImageAttachment{
			Data: a.Data, MimeType: a.MimeType,
		})
	}
	for _, a := range qp.Annotations {
		out.Annotations = append(out.Annotations, loop.AnnotationToProto(a))
	}
	if qp.ModelID != "" {
		out.ModelId = &qp.ModelID
	}
	if qp.Provider != "" {
		out.Provider = &qp.Provider
	}
	if qp.ApprovalMode != "" {
		out.ApprovalMode = &qp.ApprovalMode
	}
	return out
}

// QueuedPromptData marshals a staged row for the success envelope,
// preserving the old data:null when nothing is staged.
func QueuedPromptData(qp *types.QueuedPrompt) (any, error) {
	if qp == nil {
		return nil, nil
	}
	raw, err := wireProtoMarshal.Marshal(queuedPromptToProto(qp))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// toolCallToProto converts a live tool call to the canonical wire type,
// reusing the transcript schema: arguments cross as raw JSON bytes. Empty
// or literal-null argument payloads stay absent, as omitempty did.
func toolCallToProto(c tools.ToolCall) *consolev1.ToolCall {
	out := &consolev1.ToolCall{Id: c.ID, Name: c.Name}
	if len(c.Arguments) > 0 && string(c.Arguments) != "null" {
		out.Arguments = append([]byte(nil), c.Arguments...)
	}
	if c.ThoughtSignature != "" {
		out.ThoughtSignature = &c.ThoughtSignature
	}
	return out
}

// toolResultToProto converts a live tool result to the canonical wire type.
// Content re-encodes from its parsed form (encoding/json sorts map keys, so
// the bytes match what the old direct marshal emitted). The loop's echoed
// call args are dropped — no client reads them (message.proto notes this
// for the transcript rows too).
func toolResultToProto(r tools.ToolResult) *consolev1.ToolResult {
	out := &consolev1.ToolResult{ToolCallId: r.ToolCallID}
	if r.ToolName != "" {
		out.ToolName = &r.ToolName
	}
	if r.Content != nil {
		if raw, err := json.Marshal(r.Content); err == nil && string(raw) != "null" {
			out.Content = raw
		}
	}
	if r.IsError {
		out.IsError = &r.IsError
	}
	return out
}

// SnapshotToProto converts a context-window snapshot to the canonical wire
// type. Counts narrow to int32 and stay JSON numbers.
func SnapshotToProto(snap loop.ContextSnapshot) *consolev1.ContextSnapshot {
	return &consolev1.ContextSnapshot{
		UsedTokens: int32(snap.UsedTokens), ContextWindow: int32(snap.ContextWindow),
		PercentUsed: snap.PercentUsed, ThresholdRatio: snap.ThresholdRatio,
		ModelId: snap.ModelID, Provider: snap.Provider, Source: snap.Source,
	}
}

// permissionToProto converts a pending tool approval to the canonical wire
// type. Args cross as raw JSON bytes; null args stay absent (decodes as
// null client-side, as before).
func permissionToProto(r permissions.Request) *consolev1.PermissionRequest {
	out := &consolev1.PermissionRequest{
		RequestId: r.RequestID, ToolCallId: r.ToolCallID,
		ToolName: r.ToolName, Tier: string(r.Tier),
	}
	if r.Args != nil {
		if raw, err := json.Marshal(r.Args); err == nil && string(raw) != "null" {
			out.Args = raw
		}
	}
	if r.Reason != "" {
		out.Reason = &r.Reason
	}
	return out
}

// askToProto converts a pending agent question. skippable is always set:
// the old shape had no omitempty and clients tell explicit false apart
// from absent.
func askToProto(q tools.AskQuestionRequest) *consolev1.AskQuestionRequest {
	skippable := q.Skippable
	out := &consolev1.AskQuestionRequest{
		RequestId: q.RequestID, Question: q.Question, Skippable: &skippable,
	}
	out.Options = append(out.Options, q.Options...)
	if q.IsMultiSelect {
		out.IsMultiSelect = true
	}
	if q.BatchID != "" {
		out.BatchId = &q.BatchID
	}
	return out
}

// browserToProto converts a pending browser action. Zero timeout and false
// submit stay absent, as omitempty did.
func browserToProto(b tools.BrowserActionRequest) *consolev1.BrowserActionRequest {
	out := &consolev1.BrowserActionRequest{RequestId: b.RequestID, Action: b.Action}
	if b.URL != "" {
		out.Url = &b.URL
	}
	if b.Script != "" {
		out.Script = &b.Script
	}
	if b.Selector != "" {
		out.Selector = &b.Selector
	}
	if b.TabID != "" {
		out.TabId = &b.TabID
	}
	if b.URLContains != "" {
		out.UrlContains = &b.URLContains
	}
	if b.Text != "" {
		out.Text = &b.Text
	}
	if b.TimeoutMs != 0 {
		out.TimeoutMs = int32(b.TimeoutMs)
	}
	if b.Ref != "" {
		out.Ref = &b.Ref
	}
	if b.Submit {
		out.Submit = true
	}
	return out
}

// Subagent lifecycle converters. Fields stay flattened under the frame's
// type tag (no nested payload object), matching the old wire. Args re-encode
// from their parsed form (encoding/json sorts map keys), and null args stay
// absent.
func subagentStartToProto(v loop.SubagentStartInfo) *consolev1.SubagentStartEvent {
	out := &consolev1.SubagentStartEvent{
		SubagentId: v.SubagentID, Name: v.Name, Role: v.Role,
		Prompt: v.Prompt, MaxTurns: int32(v.MaxTurns),
	}
	if v.ParentToolCall != "" {
		out.ParentToolCallId = &v.ParentToolCall
	}
	return out
}

func subagentActivityToProto(v loop.SubagentActivityInfo) *consolev1.SubagentActivityEvent {
	out := &consolev1.SubagentActivityEvent{
		SubagentId: v.SubagentID, TurnIndex: int32(v.TurnIndex), Status: v.Status,
	}
	if v.ToolCallID != "" {
		out.ToolCallId = &v.ToolCallID
	}
	if v.ToolName != "" {
		out.ToolName = &v.ToolName
	}
	if v.Args != nil {
		if raw, err := json.Marshal(v.Args); err == nil && string(raw) != "null" {
			out.Args = raw
		}
	}
	if v.Error != "" {
		out.Error = &v.Error
	}
	return out
}

func subagentEndToProto(v loop.SubagentEndInfo) *consolev1.SubagentEndEvent {
	out := &consolev1.SubagentEndEvent{
		SubagentId: v.SubagentID, Status: v.Status, TotalTurns: int32(v.TotalTurns),
	}
	if v.Summary != "" {
		out.Summary = &v.Summary
	}
	if v.Error != "" {
		out.Error = &v.Error
	}
	return out
}

// BackgroundWorkEventToProto converts a background-work lifecycle event for
// WtFrame payloads. Kind/status/timestamps stay plain strings, matched
// literally; the SSE wire keeps its existing hand-shaped envelope and never
// calls this.
func BackgroundWorkEventToProto(e services.BackgroundWorkEvent) *consolev1.BackgroundWorkEvent {
	work := e.Work
	out := &consolev1.BackgroundWorkEvent{
		Type: string(e.Type),
		Work: &consolev1.BackgroundWork{
			Id: work.ID, SessionId: work.SessionID,
			Kind: string(work.Kind), Status: string(work.Status),
			CreatedAt: work.CreatedAt,
		},
	}
	if work.ParentRunID != "" {
		out.Work.ParentRunId = &work.ParentRunID
	}
	if work.ParentCallID != "" {
		out.Work.ParentCallId = &work.ParentCallID
	}
	if work.StartedAt != "" {
		out.Work.StartedAt = &work.StartedAt
	}
	if work.FinishedAt != "" {
		out.Work.FinishedAt = &work.FinishedAt
	}
	if work.Summary != "" {
		out.Work.Summary = &work.Summary
	}
	if work.Error != "" {
		out.Work.Error = &work.Error
	}
	for k, v := range work.Metadata {
		if out.Work.Metadata == nil {
			out.Work.Metadata = map[string]string{}
		}
		out.Work.Metadata[k] = v
	}
	return out
}

// WireFrame maps a hub event to its wire event name and JSON body, the shape
// both SSE (`{"type": ...}`) and WT (frame payload bytes) emit. Internal-only
// kinds (token accounting, raw provider deltas) drop; drop=true means emit
// nothing.
func WireFrame(e loop.Event) (string, any, bool) {
	switch e.Kind {
	case loop.EventSessionStart:
		return "sessionStart", map[string]any{"type": "sessionStart"}, false
	case loop.EventTurnStart:
		return "turnStart", map[string]any{"type": "turnStart", "prompt": e.Text}, false
	case loop.EventModelStreamStart:
		return "modelStreamStart", map[string]any{"type": "modelStreamStart", "turnId": e.Text}, false
	case loop.EventModelStreamPart:
		part, ok := e.Part.(*consolev1.ModelStreamPart)
		if !ok {
			return "", nil, true
		}
		raw, err := wireProtoMarshal.Marshal(part)
		if err != nil {
			return "", nil, true
		}
		return "modelStreamPart", modelStreamPartFrame{Type: "modelStreamPart", Part: json.RawMessage(raw)}, false
	case loop.EventModelStreamEnd:
		var turn any
		if m, ok := e.Message.(loop.AssistantMessage); ok {
			raw, err := wireProtoMarshal.Marshal(loop.AssistantToProto(m))
			if err != nil {
				return "", nil, true
			}
			turn = json.RawMessage(raw)
		}
		return "modelStreamEnd", map[string]any{"type": "modelStreamEnd", "turnId": e.Text, "turn": turn}, false
	case loop.EventToolExecutionStart:
		calls := make([]*consolev1.ToolCall, 0, len(e.Calls))
		for _, c := range e.Calls {
			calls = append(calls, toolCallToProto(c))
		}
		data, err := marshalProtoList(calls)
		if err != nil {
			return "", nil, true
		}
		return "toolExecutionStart", map[string]any{"type": "toolExecutionStart", "calls": data}, false
	case loop.EventToolExecutionResult:
		if e.Result == nil {
			return "", nil, true
		}
		raw, err := wireProtoMarshal.Marshal(toolResultToProto(*e.Result))
		if err != nil {
			return "", nil, true
		}
		return "toolExecutionResult", map[string]any{"type": "toolExecutionResult", "result": json.RawMessage(raw)}, false
	case loop.EventToolExecutionEnd:
		results := make([]*consolev1.ToolResult, 0, len(e.Results))
		for _, r := range e.Results {
			results = append(results, toolResultToProto(r))
		}
		data, err := marshalProtoList(results)
		if err != nil {
			return "", nil, true
		}
		return "toolExecutionEnd", map[string]any{"type": "toolExecutionEnd", "results": data}, false
	case loop.EventTurnEnd:
		return "turnEnd", map[string]any{"type": "turnEnd", "turnId": e.Text}, false
	case loop.EventContextUpdate:
		if e.Context == nil {
			return "", nil, true
		}
		raw, err := wireProtoMarshal.Marshal(SnapshotToProto(*e.Context))
		if err != nil {
			return "", nil, true
		}
		return "contextUpdate", map[string]any{"type": "contextUpdate", "context": json.RawMessage(raw)}, false
	case loop.EventSessionEnd:
		return "sessionEnd", map[string]any{"type": "sessionEnd"}, false
	case loop.EventSessionTitleUpdated:
		return "sessionTitleUpdated", map[string]any{"type": "sessionTitleUpdated", "title": e.Title}, false
	case loop.EventQueueUpdated:
		data, err := QueuedPromptData(e.Queued)
		if err != nil {
			return "", nil, true
		}
		return "queueUpdated", map[string]any{"type": "queueUpdated", "queuedPrompt": data}, false
	case loop.EventAskQuestion:
		if e.Ask == nil {
			return "", nil, true
		}
		raw, err := wireProtoMarshal.Marshal(askToProto(*e.Ask))
		if err != nil {
			return "", nil, true
		}
		return "askQuestion", map[string]any{"type": "askQuestion", "request": json.RawMessage(raw)}, false
	case loop.EventBrowserAction:
		if e.Browser == nil {
			return "", nil, true
		}
		raw, err := wireProtoMarshal.Marshal(browserToProto(*e.Browser))
		if err != nil {
			return "", nil, true
		}
		return "browserAction", map[string]any{"type": "browserAction", "request": json.RawMessage(raw)}, false
	case loop.EventPermissionRequest:
		if e.Permission == nil {
			return "", nil, true
		}
		raw, err := wireProtoMarshal.Marshal(permissionToProto(*e.Permission))
		if err != nil {
			return "", nil, true
		}
		return "permissionRequest", map[string]any{"type": "permissionRequest", "request": json.RawMessage(raw)}, false
	case loop.EventTodoUpdate:
		items := make([]*consolev1.TodoItem, 0, len(e.Items))
		for _, item := range e.Items {
			items = append(items, &consolev1.TodoItem{
				Id: int32(item.ID), Content: item.Content, Status: item.Status,
			})
		}
		data, err := marshalProtoList(items)
		if err != nil {
			return "", nil, true
		}
		return "todoUpdate", map[string]any{"type": "todoUpdate", "items": data, "action": e.Action}, false
	case loop.EventSubagentStart, loop.EventSubagentActivity, loop.EventSubagentEnd:
		return SubagentFrame(e)
	case loop.EventBackgroundWork:
		work, ok := e.Message.(services.BackgroundWorkEvent)
		if !ok {
			return "", nil, true
		}
		return string(work.Type), map[string]any{
			"type": string(work.Type),
			"work": work.Work,
		}, false
	case loop.EventError:
		raw, err := wireProtoMarshal.Marshal(&consolev1.ErrorPayload{Message: e.Text})
		if err != nil {
			return "", nil, true
		}
		return "error", map[string]any{"type": "error", "error": json.RawMessage(raw)}, false
	default:
		// Internal provider kinds (text/thinking/toolCall/usage/toolResult/
		// turnDone) never reach subscribers; drop defensively.
		return "", nil, true
	}
}

// subagentFrame marshals one subagent lifecycle payload. The old helper
// round-tripped through a map to flatten it; the schema carries the flat
// shape directly, so the map dance is gone.
func SubagentFrame(e loop.Event) (string, any, bool) {
	var msg proto.Message
	switch v := e.Subagent.(type) {
	case loop.SubagentStartInfo:
		msg = subagentStartToProto(v)
	case loop.SubagentActivityInfo:
		msg = subagentActivityToProto(v)
	case loop.SubagentEndInfo:
		msg = subagentEndToProto(v)
	default:
		return "", nil, true
	}
	raw, err := wireProtoMarshal.Marshal(msg)
	if err != nil {
		return "", nil, true
	}
	// Flatten: splice the payload object into the frame envelope alongside
	// the type tag, which is what clients decode.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "", nil, true
	}
	fields["type"], _ = json.Marshal(string(e.Kind))
	return string(e.Kind), fields, false
}
