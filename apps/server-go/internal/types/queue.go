// Queued-prompt types. Mirrors QueuedPrompt in packages/types/src/api.ts:
// at most one staged prompt per session, auto-run when the active turn
// settles (or steered early).
package types

type QueuedAttachment struct {
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

type QueuedPrompt struct {
	ID           string             `json:"id"`
	SessionID    string             `json:"sessionId"`
	Prompt       string             `json:"prompt"`
	ContextFiles []string           `json:"contextFiles,omitempty"`
	Attachments  []QueuedAttachment `json:"attachments,omitempty"`
	ModelID      string             `json:"modelId,omitempty"`
	Provider     string             `json:"provider,omitempty"`
	ApprovalMode string             `json:"approvalMode,omitempty"`
	CreatedAt    string             `json:"createdAt"`
}
