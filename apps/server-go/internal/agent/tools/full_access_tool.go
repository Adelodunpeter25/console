// requestFullAccess: lets a plan-mode run ask the user to upgrade to Full
// Access mid-run, so "now implement it" does not need a stop and re-send.
package tools

import (
	"context"
	"strings"
)

const (
	FullAccessApprove = "Approve"
	FullAccessDeny    = "Deny"
)

type requestFullAccessInput struct {
	Reason string `json:"reason" jsonschema:"required,description=Short reason, e.g. 'implement the auth refactor you asked for'"`
}

// NewRequestFullAccessTool builds requestFullAccess. It asks through handler;
// only an explicit Approve calls onApprove, which performs the switch. With no
// handler (headless) nobody can approve, so it never upgrades.
func NewRequestFullAccessTool(handler AskHandler, onApprove func() error) Tool {
	return NewTool("requestFullAccess",
		"Ask the user to switch this chat from plan mode to Full Access so you can implement. Call it only when the user explicitly asked you to implement. The user approves or declines; on approval you are in Full Access immediately.",
		TierRead,
		func(ctx context.Context, in requestFullAccessInput) (any, error) {
			reason := strings.TrimSpace(in.Reason)
			if reason == "" {
				return nil, NewToolError("reason is required")
			}
			if handler == nil {
				return textResult("Full Access was not granted: no user is attached to approve it. Stay in plan mode."), nil
			}
			notSkippable := false
			answer, err := askOne(ctx, handler, askQuestionInput{
				Question:  "Agent wants to switch to Full Access: " + reason,
				Options:   []string{FullAccessApprove, FullAccessDeny},
				Skippable: &notSkippable,
			}, "")
			if err != nil {
				return nil, err
			}
			if answer.Text != FullAccessApprove {
				return textResult("The user declined Full Access. Stay in plan mode and do not make changes."), nil
			}
			if err := onApprove(); err != nil {
				return nil, NewToolError("could not switch to Full Access: %v", err)
			}
			return textResult("The user approved. You are now in Full Access mode; the plan-mode restrictions no longer apply. Proceed with the implementation."), nil
		})
}
