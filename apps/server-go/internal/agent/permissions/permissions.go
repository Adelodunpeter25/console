// Approval-mode → per-call policy resolution.
package permissions

import (
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type Mode string

// Two modes: Plan (read-only, the default) and Full Access (everything runs,
// nothing asks). Older stored values (always-ask, accept-edits) are not modes
// any more; Normalize maps them to PlanMode.
const (
	PlanMode   Mode = "plan-mode"
	FullAccess Mode = "full-access"
)

// Normalize returns the mode for a stored or requested value, falling back to
// PlanMode for empty and legacy values: failing closed, never open.
func Normalize(value string) Mode {
	if Mode(value) == FullAccess {
		return FullAccess
	}
	return PlanMode
}

type Policy string

const (
	Allow Policy = "allow"
	Deny  Policy = "deny"
)

// Resolve maps (mode, tier) to a policy.
//
// PlanMode is read-only and enforced here, not just asked of the model: write
// and exec tools are denied outright rather than prompted. The only writes a
// plan-mode run can make are tools that declare themselves tools.PlanSafe
// (writePlan), which the executor lets through before consulting this table.
func Resolve(mode Mode, tier tools.ToolTier) Policy {
	if mode == FullAccess {
		return Allow
	}
	if tier == tools.TierRead {
		return Allow
	}
	return Deny
}
