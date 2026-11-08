// Direct decoding of split skeleton + parts into loop domain messages,
// bypassing monolithic JSON reconstruction and redundant base64 round-trips.
package loop

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
)

type partMarker struct {
	Slot *int `json:"$part"`
}

func parsePartSlot(raw []byte) (int, bool) {
	if len(raw) == 0 || raw[0] != '{' {
		return 0, false
	}
	var m partMarker
	if err := json.Unmarshal(raw, &m); err == nil && m.Slot != nil {
		return *m.Slot, true
	}
	return 0, false
}

type rawRootMessage struct {
	User       *rawUserMessage       `json:"user"`
	Assistant  *rawAssistantMessage  `json:"assistant"`
	ToolResult *rawToolResultMessage `json:"toolResult"`
}

type rawUserMessage struct {
	Content      string               `json:"content"`
	Synthetic    bool                 `json:"synthetic"`
	ContextFiles []string             `json:"contextFiles"`
	Attachments  []rawImageAttachment `json:"attachments"`
	Annotations  []json.RawMessage    `json:"annotations"`
}

type rawImageAttachment struct {
	Data     json.RawMessage `json:"data"`
	MimeType string          `json:"mimeType"`
}

type rawAssistantMessage struct {
	ID         string                `json:"id"`
	StopReason string                `json:"stopReason"`
	TokPerSec  *float64              `json:"tokPerSec"`
	Content    []rawAssistantContent `json:"content"`
}

type rawAssistantContent struct {
	Text *struct {
		Text             string `json:"text"`
		ThoughtSignature string `json:"thoughtSignature"`
	} `json:"text"`
	Thinking *struct {
		Text string `json:"text"`
	} `json:"thinking"`
	ToolCall *struct {
		ID               string          `json:"id"`
		Name             string          `json:"name"`
		Arguments        json.RawMessage `json:"arguments"`
		ThoughtSignature string          `json:"thoughtSignature"`
	} `json:"toolCall"`
	Image *struct {
		Data     json.RawMessage `json:"data"`
		MimeType string          `json:"mimeType"`
	} `json:"image"`
}

type rawToolResultMessage struct {
	Results []rawToolResult `json:"results"`
}

type rawToolResult struct {
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Content    json.RawMessage `json:"content"`
	IsError    bool            `json:"isError"`
	Visual     *struct {
		Version uint32          `json:"version"`
		Spec    json.RawMessage `json:"spec"`
	} `json:"visual"`
}

// MessageFromSplit decodes a message directly from its skeleton and extracted parts.
// Tool arguments and results are read directly as raw bytes without base64 re-encoding.
func MessageFromSplit(skeleton []byte, parts []msgparts.Part) (any, error) {
	if len(skeleton) == 0 {
		return nil, fmt.Errorf("empty skeleton")
	}

	bySlot := make(map[int]msgparts.Part, len(parts))
	for _, p := range parts {
		bySlot[p.Slot] = p
	}

	var root rawRootMessage
	if err := json.Unmarshal(skeleton, &root); err != nil {
		// Fallback to canonical assemble if custom skeleton unmarshaling fails.
		rebuilt, assembleErr := msgparts.Assemble(skeleton, parts)
		if assembleErr != nil {
			return nil, err
		}
		return MessageFromProtoBytes(rebuilt)
	}

	if root.Assistant != nil {
		assistant := AssistantMessage{
			Role:       RoleAssistant,
			ID:         root.Assistant.ID,
			StopReason: StopReason(root.Assistant.StopReason),
			TokPerSec:  root.Assistant.TokPerSec,
		}
		for _, c := range root.Assistant.Content {
			if c.Text != nil {
				assistant.Content = append(assistant.Content, TextPart{
					Text:             c.Text.Text,
					ThoughtSignature: c.Text.ThoughtSignature,
				})
			} else if c.Thinking != nil {
				assistant.Content = append(assistant.Content, ThinkingPart{
					Text: c.Thinking.Text,
				})
			} else if c.ToolCall != nil {
				var args json.RawMessage
				if slot, ok := parsePartSlot(c.ToolCall.Arguments); ok {
					if p, found := bySlot[slot]; found {
						args = append(json.RawMessage(nil), p.Body...)
					}
				} else if len(c.ToolCall.Arguments) > 0 {
					var b64 string
					if json.Unmarshal(c.ToolCall.Arguments, &b64) == nil {
						if decoded, err := base64.StdEncoding.DecodeString(b64); err == nil {
							args = decoded
						} else {
							args = append(json.RawMessage(nil), c.ToolCall.Arguments...)
						}
					} else {
						args = append(json.RawMessage(nil), c.ToolCall.Arguments...)
					}
				}
				assistant.Content = append(assistant.Content, ToolCallPart{
					Type: "toolCall",
					Call: tools.ToolCall{
						ID:               c.ToolCall.ID,
						Name:             c.ToolCall.Name,
						Arguments:        args,
						ThoughtSignature: c.ToolCall.ThoughtSignature,
					},
				})
			} else if c.Image != nil {
				var data string
				if slot, ok := parsePartSlot(c.Image.Data); ok {
					if p, found := bySlot[slot]; found {
						data = base64.StdEncoding.EncodeToString(p.Body)
					}
				} else if len(c.Image.Data) > 0 {
					_ = json.Unmarshal(c.Image.Data, &data)
				}
				assistant.Content = append(assistant.Content, ImageAttachment{
					Data:     data,
					MimeType: c.Image.MimeType,
				})
			}
		}
		return assistant, nil
	}

	if root.ToolResult != nil {
		toolMsg := ToolResultMessage{
			Role: RoleToolResult,
		}
		for _, res := range root.ToolResult.Results {
			var content any
			var rawBytes []byte
			if slot, ok := parsePartSlot(res.Content); ok {
				if p, found := bySlot[slot]; found {
					rawBytes = p.Body
				}
			} else if len(res.Content) > 0 {
				var b64 string
				if json.Unmarshal(res.Content, &b64) == nil {
					if decoded, err := base64.StdEncoding.DecodeString(b64); err == nil {
						rawBytes = decoded
					} else {
						rawBytes = res.Content
					}
				} else {
					rawBytes = res.Content
				}
			}
			if len(rawBytes) > 0 {
				if err := json.Unmarshal(rawBytes, &content); err != nil {
					return nil, fmt.Errorf("tool result content: %w", err)
				}
			}
			var visual *tools.VisualArtifact
			if res.Visual != nil && res.Visual.Version > 0 {
				var specBytes []byte
				var b64 string
				if json.Unmarshal(res.Visual.Spec, &b64) == nil {
					specBytes, _ = base64.StdEncoding.DecodeString(b64)
				} else {
					specBytes = res.Visual.Spec
				}
				if json.Valid(specBytes) {
					visual = &tools.VisualArtifact{
						Version: res.Visual.Version,
						Spec:    specBytes,
					}
				}
			}
			toolMsg.Results = append(toolMsg.Results, tools.ToolResult{
				ToolCallID: res.ToolCallID,
				ToolName:   res.ToolName,
				Content:    content,
				IsError:    res.IsError,
				Visual:     visual,
			})
		}
		return toolMsg, nil
	}

	if root.User != nil {
		userMsg := UserMessage{
			Role:         RoleUser,
			Content:      root.User.Content,
			Synthetic:    root.User.Synthetic,
			ContextFiles: append([]string(nil), root.User.ContextFiles...),
		}
		for _, att := range root.User.Attachments {
			data := ""
			if slot, ok := parsePartSlot(att.Data); ok {
				if p, found := bySlot[slot]; found {
					data = base64.StdEncoding.EncodeToString(p.Body)
				}
			} else if len(att.Data) > 0 {
				_ = json.Unmarshal(att.Data, &data)
			}
			userMsg.Attachments = append(userMsg.Attachments, ImageAttachment{
				Data:     data,
				MimeType: att.MimeType,
			})
		}
		for _, annRaw := range root.User.Annotations {
			var annMap map[string]json.RawMessage
			if err := json.Unmarshal(annRaw, &annMap); err == nil {
				if scRaw, ok := annMap["screenshotBase64"]; ok {
					if slot, ok := parsePartSlot(scRaw); ok {
						if p, found := bySlot[slot]; found {
							b64 := base64.StdEncoding.EncodeToString(p.Body)
							annMap["screenshotBase64"], _ = json.Marshal(b64)
							annRaw, _ = json.Marshal(annMap)
						}
					}
				}
			}
			var ann consolev1.BrowserAnnotation
			if err := protoUnmarshal.Unmarshal(annRaw, &ann); err == nil {
				userMsg.Annotations = append(userMsg.Annotations, protoToAnnotation(&ann))
			}
		}
		return userMsg, nil
	}

	// Fallback to canonical assemble and proto unmarshal.
	rebuilt, err := msgparts.Assemble(skeleton, parts)
	if err != nil {
		return nil, err
	}
	return MessageFromProtoBytes(rebuilt)
}
