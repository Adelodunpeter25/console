// Approval-mode → per-call policy resolution. Port of
// apps/server/agent/src/permissions/approval.ts (initial slice).
package permissions

import (
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type Mode string

const (
	AlwaysAsk   Mode = "always-ask"
	AcceptEdits Mode = "accept-edits"
	PlanMode    Mode = "plan-mode"
	FullAccess  Mode = "full-access"
)

type Policy string

const (
	Allow  Policy = "allow"
	Deny   Policy = "deny"
	Prompt Policy = "prompt"
)

// Request is emitted when a tool call needs user approval.
type Request struct {
	RequestID  string         `json:"requestId"`
	ToolCallID string         `json:"toolCallId"`
	ToolName   string         `json:"toolName"`
	Args       any            `json:"args"`
	Tier       tools.ToolTier `json:"tier"`
	Reason     string         `json:"reason,omitempty"`
}

// Resolve maps (mode, tier) to a policy.
func Resolve(mode Mode, tier tools.ToolTier) Policy {
	switch mode {
	case FullAccess:
		return Allow
	case AcceptEdits:
		switch tier {
		case tools.TierWrite:
			return Allow
		case tools.TierExec:
			return Prompt
		default:
			return Allow
		}
	case PlanMode:
		switch tier {
		case tools.TierExec:
			return Deny
		case tools.TierWrite:
			return Deny
		default:
			return Allow
		}
	default: // always-ask
		switch tier {
		case tools.TierRead:
			return Allow
		default:
			return Prompt
		}
	}
}

// Decision carries the outcome for one tool call.
type Decision struct {
	Policy Policy
	Reason string
}

func (d Decision) Error() error {
	if d.Policy == Deny {
		return tools.NewToolError("Permission denied by approval mode: %s", d.Reason)
	}
	return fmt.Errorf("approval required")
}
