package console

import (
	"context"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/roles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// SetCeiling records the approval mode of the run currently driving this
// kernel. It is the upper bound for anything the run creates or reconfigures.
func (r *Runtime) SetCeiling(mode string) { r.ceiling = mode }

// ceilingOrDefault is the mode a new session inherits when none is requested.
func (r *Runtime) ceilingOrDefault() string {
	if _, ok := approvalRank[permissionsMode(r.ceiling)]; ok {
		return r.ceiling
	}
	return "plan-mode"
}

// catalogModels lists the models the server can run. It is a variable so
// tests can supply a catalog: the real one is filled by live provider
// discovery, which needs a logged-in account.
var catalogModels = func(ctx context.Context) []types.ProviderEntry {
	return providers.ListProvidersWithContext(ctx)
}

// validateThinking checks a thinking level against the model it will run on,
// so a bad value fails here with the supported list instead of mid-run. A
// model that is not in the catalog is refused rather than waved through: an
// unverifiable level would only fail later, inside the run.
func (r *Runtime) validateThinking(ctx context.Context, modelID, provider, level string) error {
	if modelID == "" {
		return fmt.Errorf("thinking_level needs an explicit model so it can be checked; call console.models() for valid ids")
	}
	for _, entry := range catalogModels(ctx) {
		if provider != "" && entry.Name != provider {
			continue
		}
		for _, m := range entry.Models {
			if m.ID == modelID {
				return roles.ValidateLevel(m, level)
			}
		}
	}
	return fmt.Errorf("unknown model %q; call console.models() for valid ids", modelID)
}

func (r *Runtime) requireSessions() error {
	if r.sessions == nil {
		return fmt.Errorf("session service is unavailable")
	}
	return nil
}

// sessionsAbort stops a session's active run. Aborting the run that is
// calling is refused: it would kill the very call that asked for it.
func (r *Runtime) sessionsAbort(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	id, err := stringArg(args, "session_id")
	if err != nil {
		return nil, err
	}
	if id == r.sessionID {
		return nil, fmt.Errorf("cannot abort the current session %q from inside it", id)
	}
	if r.abort == nil {
		return nil, fmt.Errorf("run control is unavailable")
	}
	if !r.abort(id) {
		return nil, fmt.Errorf("session %q has no active run", id)
	}
	return marshalResult(map[string]any{"aborted": true, "session_id": id})
}

func (r *Runtime) sessionsSetModel(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	if err := r.requireSessions(); err != nil {
		return nil, err
	}
	id, err := stringArg(args, "session_id")
	if err != nil {
		return nil, err
	}
	model, err := stringArg(args, "model")
	if err != nil {
		return nil, err
	}
	provider, _ := args["provider"].(string)
	header, err := r.sessions.Header(id)
	if err != nil {
		return nil, err
	}
	if header == nil {
		return nil, fmt.Errorf("session %q not found", id)
	}
	if provider == "" {
		provider = header.Provider
	}
	if !r.modelExists(ctx, provider, model) {
		return nil, fmt.Errorf("unknown model %q for provider %q; call console.models() for valid ids", model, provider)
	}
	if err := r.sessions.UpdateModel(id, model, provider); err != nil {
		return nil, err
	}
	return marshalResult(map[string]any{"session_id": id, "model": model, "provider": provider})
}

func (r *Runtime) modelExists(ctx context.Context, provider, model string) bool {
	for _, entry := range catalogModels(ctx) {
		if entry.Name != provider {
			continue
		}
		for _, m := range entry.Models {
			if m.ID == model {
				return true
			}
		}
	}
	return false
}

// sessionsSetApproval moves a session's approval mode, never above the
// calling run's own mode.
func (r *Runtime) sessionsSetApproval(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if err := r.requireSessions(); err != nil {
		return nil, err
	}
	id, err := stringArg(args, "session_id")
	if err != nil {
		return nil, err
	}
	mode, err := stringArg(args, "mode")
	if err != nil {
		return nil, err
	}
	if id == r.sessionID {
		return nil, fmt.Errorf("a run cannot change its own approval mode")
	}
	effective, clamped, note, err := clampApproval(r.ceiling, mode)
	if err != nil {
		return nil, err
	}
	header, err := r.sessions.Header(id)
	if err != nil {
		return nil, err
	}
	if header == nil {
		return nil, fmt.Errorf("session %q not found", id)
	}
	if err := r.sessions.UpdateApprovalMode(id, effective); err != nil {
		return nil, err
	}
	out := map[string]any{"session_id": id, "approval_mode": effective}
	if clamped {
		out["clamped"] = note
	}
	return marshalResult(out)
}

func (r *Runtime) sessionsFork(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if err := r.requireSessions(); err != nil {
		return nil, err
	}
	id, err := stringArg(args, "session_id")
	if err != nil {
		return nil, err
	}
	messageID, err := stringArg(args, "message_id")
	if err != nil {
		return nil, err
	}
	header, err := r.sessions.Fork(id, messageID)
	if err != nil {
		return nil, err
	}
	// A fork copies the source's stored approval mode, which can exceed this
	// run's authority; bring it back under the ceiling.
	if effective, clamped, note, cerr := clampApproval(r.ceiling, header.ApprovalMode); cerr == nil && clamped {
		if err := r.sessions.UpdateApprovalMode(header.ID, effective); err != nil {
			return nil, err
		}
		header.ApprovalMode = effective
		return marshalResult(map[string]any{"session": header, "clamped": note})
	}
	return marshalResult(map[string]any{"session": header})
}
