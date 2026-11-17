// requestFullAccess: lets a plan-mode run ask the user to upgrade to Full
// Access mid-run, so "now implement it" does not need a stop and re-send.
// The tool is registered in every mode so a stray call outside plan mode
// gets a clear answer instead of an unknown-tool error.
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

// IsPlanModeFunc reports whether the run is currently in plan mode. Defined
// here because permissions imports tools, so tools cannot import permissions
// back; the loop.Executor satisfies it via IsPlanMode.
type IsPlanModeFunc func() bool

// NewRequestFullAccessTool builds requestFullAccess. It asks through handler;
// only an explicit Approve calls onApprove, which performs the switch. With no
// handler (headless) nobody can approve, so it never upgrades. The tool is
// registered in every mode: outside plan mode it answers directly instead of
// surfacing an unknown-tool error to the model.
func NewRequestFullAccessTool(handler AskHandler, onApprove func() error, isPlanMode IsPlanModeFunc) Tool {
	return NewTool("requestFullAccess",
		"Ask the user to switch this chat from plan mode to Full Access so you can implement. Only meaningful in plan mode; in any other mode it reports that no upgrade is needed. Call it only when the user explicitly asked you to implement. The user approves or declines; on approval you are in Full Access immediately.",
		TierRead,
		func(ctx context.Context, in requestFullAccessInput) (any, error) {
			reason := strings.TrimSpace(in.Reason)
			if reason == "" {
				return nil, NewToolError("reason is required")
			}
			if isPlanMode != nil && !isPlanMode() {
				return textResult("Plan mode is not active, so there is nothing to upgrade — you already have Full Access. Proceed with the implementation."), nil
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
