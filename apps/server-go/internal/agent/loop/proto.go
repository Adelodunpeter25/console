// Canonical proto conversions for conversation messages.
//
// The loop owns provider logic on its own structs (UserMessage,
// AssistantMessage, ToolResultMessage); this schema is the storage +
// client contract. These converters are the only bridge: persist sites
// encode through ToProtoBytes, replay decodes through
// MessageFromProtoBytes, and everything between works on loop structs
// exactly as before. Turn usage stays loop-internal (dropped from the
// schema); tool args/result content cross as raw JSON bytes.
package loop

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

var protoMarshal = protojson.MarshalOptions{}
var protoUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// ToProtoMessage converts a loop message struct to the canonical wire type.
func ToProtoMessage(v any) (*consolev1.AgentMessage, error) {
	switch m := v.(type) {
	case UserMessage:
		return &consolev1.AgentMessage{Message: &consolev1.AgentMessage_User{
			User: userToProto(m),
		}}, nil
	case *UserMessage:
		if m == nil {
			return nil, fmt.Errorf("nil user message")
		}
		return ToProtoMessage(*m)
	case AssistantMessage:
		return &consolev1.AgentMessage{Message: &consolev1.AgentMessage_Assistant{
			Assistant: AssistantToProto(m),
		}}, nil
	case *AssistantMessage:
		if m == nil {
			return nil, fmt.Errorf("nil assistant message")
		}
		return ToProtoMessage(*m)
	case ToolResultMessage:
		results, err := toolResultsToProto(m.Results)
		if err != nil {
			return nil, err
		}
		return &consolev1.AgentMessage{Message: &consolev1.AgentMessage_ToolResult{
			ToolResult: &consolev1.AgentToolResultMessage{Results: results},
		}}, nil
	case *ToolResultMessage:
		if m == nil {
			return nil, fmt.Errorf("nil tool result message")
		}
		return ToProtoMessage(*m)
	default:
		return nil, fmt.Errorf("unknown message type %T", v)
	}
}

// ToProtoBytes marshals a loop message struct straight to canonical storage
// bytes. It fails loudly on unencodable content: persist callers treat that
// like any other persist error.
func ToProtoBytes(v any) ([]byte, error) {
	msg, err := ToProtoMessage(v)
	if err != nil {
		return nil, err
	}
	return protoMarshal.Marshal(msg)
}

// MessageFromProtoBytes parses canonical bytes back into loop structs for
// replay. Unknown fields are tolerated; unparseable rows are an error and
// the caller skips them, matching the old role-probe leniency.
func MessageFromProtoBytes(raw []byte) (any, error) {
	var msg consolev1.AgentMessage
	if err := protoUnmarshal.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	switch event := msg.GetMessage().(type) {
	case *consolev1.AgentMessage_User:
		return protoToUser(event.User), nil
	case *consolev1.AgentMessage_Assistant:
		assistant, err := protoToAssistant(event.Assistant)
		if err != nil {
			return nil, err
		}
		return assistant, nil
	case *consolev1.AgentMessage_ToolResult:
		results, err := protoToToolResults(event.ToolResult.GetResults())
		if err != nil {
			return nil, err
		}
		return ToolResultMessage{Role: RoleToolResult, Results: results}, nil
	default:
		return nil, fmt.Errorf("empty agent message")
	}
}

func userToProto(m UserMessage) *consolev1.AgentUserMessage {
	out := &consolev1.AgentUserMessage{Content: m.Content}
	out.ContextFiles = append(out.ContextFiles, m.ContextFiles...)
	for _, a := range m.Attachments {
		out.Attachments = append(out.Attachments, &consolev1.ImageAttachment{
			Data: a.Data, MimeType: a.MimeType,
		})
	}
	for _, a := range m.Annotations {
		out.Annotations = append(out.Annotations, AnnotationToProto(a))
	}
	return out
}

func AnnotationToProto(a types.BrowserAnnotation) *consolev1.BrowserAnnotation {
	out := &consolev1.BrowserAnnotation{
		Id: a.ID, Url: a.URL, Selector: a.Selector,
		HtmlSnippet: a.HTMLSnippet, UserComment: a.UserComment,
	}
	if a.SessionID != "" {
		out.SessionId = &a.SessionID
	}
	if a.Title != "" {
		out.Title = &a.Title
	}
	if a.ComponentName != "" {
		out.ComponentName = &a.ComponentName
	}
	if a.SourceLocation != "" {
		out.SourceLocation = &a.SourceLocation
	}
	if a.Dimensions != nil {
		out.Dimensions = &consolev1.BrowserAnnotationRect{
			X: a.Dimensions.X, Y: a.Dimensions.Y,
			Width: a.Dimensions.Width, Height: a.Dimensions.Height,
		}
	}
	if a.Role != "" {
		out.Role = &a.Role
	}
	if a.AccessibleName != "" {
		out.AccessibleName = &a.AccessibleName
	}
	for k, v := range a.ComputedStyles {
		if out.ComputedStyles == nil {
			out.ComputedStyles = map[string]string{}
		}
		out.ComputedStyles[k] = v
	}
	if a.ImageBase64 != "" {
		out.ScreenshotBase64 = &a.ImageBase64
	}
	return out
}

func AssistantToProto(m AssistantMessage) *consolev1.AgentAssistantMessage {
	out := &consolev1.AgentAssistantMessage{
		Id: m.ID, StopReason: string(m.StopReason),
	}
	for _, part := range m.Content {
		switch p := part.(type) {
		case TextPart:
			tp := &consolev1.TextPart{Text: p.Text}
			if p.ThoughtSignature != "" {
				tp.ThoughtSignature = &p.ThoughtSignature
			}
			out.Content = append(out.Content, &consolev1.AssistantContentPart{
				Part: &consolev1.AssistantContentPart_Text{Text: tp},
			})
		case ThinkingPart:
			out.Content = append(out.Content, &consolev1.AssistantContentPart{
				Part: &consolev1.AssistantContentPart_Thinking{
					Thinking: &consolev1.ThinkingPart{Text: p.Text},
				},
			})
		case ToolCallPart:
			call := &consolev1.ToolCall{Id: p.Call.ID, Name: p.Call.Name}
			if len(p.Call.Arguments) > 0 {
				call.Arguments = append([]byte(nil), p.Call.Arguments...)
			}
			if p.Call.ThoughtSignature != "" {
				call.ThoughtSignature = &p.Call.ThoughtSignature
			}
			out.Content = append(out.Content, &consolev1.AssistantContentPart{
				Part: &consolev1.AssistantContentPart_ToolCall{ToolCall: call},
			})
		}
	}
	return out
}

func toolResultsToProto(results []tools.ToolResult) ([]*consolev1.ToolResult, error) {
	out := make([]*consolev1.ToolResult, 0, len(results))
	for _, r := range results {
		content, err := json.Marshal(r.Content)
		if err != nil {
			return nil, fmt.Errorf("tool result content: %w", err)
		}
		item := &consolev1.ToolResult{ToolCallId: r.ToolCallID, Content: content}
		if r.ToolName != "" {
			item.ToolName = &r.ToolName
		}
		if r.IsError {
			item.IsError = &r.IsError
		}
		out = append(out, item)
	}
	return out, nil
}

func protoToUser(m *consolev1.AgentUserMessage) UserMessage {
	out := UserMessage{Role: RoleUser, Content: m.GetContent()}
	out.ContextFiles = append(out.ContextFiles, m.GetContextFiles()...)
	for _, a := range m.GetAttachments() {
		out.Attachments = append(out.Attachments, ImageAttachment{
			Data: a.GetData(), MimeType: a.GetMimeType(),
		})
	}
	for _, a := range m.GetAnnotations() {
		out.Annotations = append(out.Annotations, protoToAnnotation(a))
	}
	return out
}

func protoToAnnotation(a *consolev1.BrowserAnnotation) types.BrowserAnnotation {
	out := types.BrowserAnnotation{
		ID: a.GetId(), URL: a.GetUrl(), Selector: a.GetSelector(),
		HTMLSnippet: a.GetHtmlSnippet(), UserComment: a.GetUserComment(),
	}
	if a.SessionId != nil {
		out.SessionID = *a.SessionId
	}
	if a.Title != nil {
		out.Title = *a.Title
	}
	if a.ComponentName != nil {
		out.ComponentName = *a.ComponentName
	}
	if a.SourceLocation != nil {
		out.SourceLocation = *a.SourceLocation
	}
	if a.Dimensions != nil {
		out.Dimensions = &types.RectDimensions{
			X: a.Dimensions.GetX(), Y: a.Dimensions.GetY(),
			Width: a.Dimensions.GetWidth(), Height: a.Dimensions.GetHeight(),
		}
	}
	if a.Role != nil {
		out.Role = *a.Role
	}
	if a.AccessibleName != nil {
		out.AccessibleName = *a.AccessibleName
	}
	for k, v := range a.GetComputedStyles() {
		if out.ComputedStyles == nil {
			out.ComputedStyles = map[string]string{}
		}
		out.ComputedStyles[k] = v
	}
	if a.ScreenshotBase64 != nil {
		out.ImageBase64 = *a.ScreenshotBase64
	}
	return out
}

func protoToAssistant(m *consolev1.AgentAssistantMessage) (AssistantMessage, error) {
	out := AssistantMessage{Role: RoleAssistant, ID: m.GetId(), StopReason: StopReason(m.GetStopReason())}
	for _, part := range m.GetContent() {
		switch p := part.GetPart().(type) {
		case *consolev1.AssistantContentPart_Text:
			out.Content = append(out.Content, TextPart{
				Text: p.Text.GetText(), ThoughtSignature: p.Text.GetThoughtSignature(),
			})
		case *consolev1.AssistantContentPart_Thinking:
			out.Content = append(out.Content, ThinkingPart{Text: p.Thinking.GetText()})
		case *consolev1.AssistantContentPart_ToolCall:
			var args json.RawMessage
			if len(p.ToolCall.GetArguments()) > 0 {
				args = append(json.RawMessage(nil), p.ToolCall.GetArguments()...)
			}
			out.Content = append(out.Content, ToolCallPart{
				Type: "toolCall",
				Call: tools.ToolCall{
					ID: p.ToolCall.GetId(), Name: p.ToolCall.GetName(),
					Arguments:        args,
					ThoughtSignature: p.ToolCall.GetThoughtSignature(),
				},
			})
		case *consolev1.AssistantContentPart_Image:
			out.Content = append(out.Content, ImageAttachment{
				Data: p.Image.GetData(), MimeType: p.Image.GetMimeType(),
			})
		default:
			return AssistantMessage{}, fmt.Errorf("unknown content part")
		}
	}
	return out, nil
}

func protoToToolResults(items []*consolev1.ToolResult) ([]tools.ToolResult, error) {
	out := make([]tools.ToolResult, 0, len(items))
	for _, r := range items {
		var content any
		if len(r.GetContent()) > 0 {
			if err := json.Unmarshal(r.GetContent(), &content); err != nil {
				return nil, fmt.Errorf("tool result content: %w", err)
			}
		}
		out = append(out, tools.ToolResult{
			ToolCallID: r.GetToolCallId(), ToolName: r.GetToolName(),
			Content: content, IsError: r.GetIsError(),
		})
	}
	return out, nil
}
