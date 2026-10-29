package console

import (
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
)

// approvalRank orders the approval modes from most restrictive to least. It
// is the scale the permission ceiling clamps on.
var approvalRank = map[permissions.Mode]int{
	permissions.PlanMode:    0,
	permissions.AlwaysAsk:   1,
	permissions.AcceptEdits: 2,
	permissions.FullAccess:  3,
}

// clampApproval applies the permission ceiling: the effective mode is the
// lower of requested and ceiling, so a run can never hand out more authority
// than it holds. clamped reports whether the request was lowered, and note is
// a message for the model so it does not retry the same call.
//
// An empty ceiling means the caller has no run behind it (a kernel used
// outside a run), which is treated as the most restrictive known mode rather
// than as unlimited: failing closed is the safe default for a permission check.
func clampApproval(ceiling, requested string) (effective string, clamped bool, note string, err error) {
	req := permissions.Mode(requested)
	reqRank, ok := approvalRank[req]
	if !ok {
		return "", false, "", fmt.Errorf("unknown approval mode %q; valid modes: plan-mode, always-ask, accept-edits, full-access (see console.approvalModes())", requested)
	}
	ceil := permissions.Mode(ceiling)
	ceilRank, ok := approvalRank[ceil]
	if !ok {
		ceil, ceilRank = permissions.PlanMode, approvalRank[permissions.PlanMode]
	}
	if reqRank <= ceilRank {
		return requested, false, "", nil
	}
	return string(ceil), true, fmt.Sprintf("requested %s, clamped to %s (this run's approval ceiling)", requested, ceil), nil
}

func permissionsMode(s string) permissions.Mode { return permissions.Mode(s) }
