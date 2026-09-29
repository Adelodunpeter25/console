// Queued-prompt types. Mirrors QueuedPrompt in packages/types/src/api.ts:
// at most one staged prompt per session, auto-run when the active turn
// settles (or steered early).
package types

type QueuedAttachment struct {
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

type RectDimensions struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type BrowserAnnotation struct {
	ID             string            `json:"id"`
	SessionID      string            `json:"sessionId,omitempty"`
	URL            string            `json:"url"`
	Title          string            `json:"title,omitempty"`
	ComponentName  string            `json:"componentName,omitempty"`
	SourceLocation string            `json:"sourceLocation,omitempty"`
	Selector       string            `json:"selector"`
	HTMLSnippet    string            `json:"htmlSnippet"`
	Dimensions     *RectDimensions   `json:"dimensions,omitempty"`
	Role           string            `json:"role,omitempty"`
	AccessibleName string            `json:"accessibleName,omitempty"`
	ComputedStyles map[string]string `json:"computedStyles,omitempty"`
	UserComment    string            `json:"userComment"`
	ImageBase64    string            `json:"screenshotBase64,omitempty"`
}

type QueuedPrompt struct {
	ID           string              `json:"id"`
	SessionID    string              `json:"sessionId"`
	Prompt       string              `json:"prompt"`
	ContextFiles []string            `json:"contextFiles,omitempty"`
	Attachments  []QueuedAttachment  `json:"attachments,omitempty"`
	Annotations  []BrowserAnnotation `json:"annotations,omitempty"`
	ModelID      string              `json:"modelId,omitempty"`
	Provider     string              `json:"provider,omitempty"`
	ApprovalMode string              `json:"approvalMode,omitempty"`
	CreatedAt    string              `json:"createdAt"`
}

