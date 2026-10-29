package console

import (
	"context"
	"fmt"
	"strings"

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

// PromptFunc delivers a follow-up prompt to an existing session: it starts a
// run when the session is idle, or stages the prompt to run when the current
// run settles (queued=true). It lives behind a callback because the run
// service imports this package.
type PromptFunc func(sessionID, text, model, provider, approval, thinking string) (queued bool, err error)

// sessionsPrompt sends a follow-up prompt to an existing session.
//
// The approval mode for the turn is capped at this run's own mode. A target
// session often stores a higher mode than the caller holds; without the cap a
// restricted run could act with the target's authority by prompting it. The cap
// applies to this turn only and never rewrites the session's stored mode.
func (r *Runtime) sessionsPrompt(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	if err := r.requireSessions(); err != nil {
		return nil, err
	}
	if r.prompt == nil {
		return nil, fmt.Errorf("prompting sessions is unavailable")
	}
	id, err := stringArg(args, "session_id")
	if err != nil {
		return nil, err
	}
	text, err := stringArg(args, "prompt")
	if err != nil {
		return nil, err
	}
	if id == r.sessionID {
		return nil, fmt.Errorf("cannot prompt the session you are running in; finish this turn instead")
	}
	header, err := r.sessions.Header(id)
	if err != nil {
		return nil, err
	}
	if header == nil {
		return nil, fmt.Errorf("session %q not found", id)
	}
	model, _ := args["model"].(string)
	provider, _ := args["provider"].(string)
	thinking, _ := args["thinking_level"].(string)
	requested, _ := args["approval_mode"].(string)

	if thinking != "" {
		checkModel, checkProvider := model, provider
		if checkModel == "" {
			checkModel = header.ModelID
		}
		if checkProvider == "" {
			checkProvider = header.Provider
		}
		if err := r.validateThinking(ctx, checkModel, checkProvider, thinking); err != nil {
			return nil, err
		}
	}

	// An omitted mode means "the session's own"; either way the result is
	// capped at this run's authority.
	if requested == "" {
		requested = header.ApprovalMode
	}
	approval, clamped, note, err := clampApproval(r.ceiling, requested)
	if err != nil {
		return nil, err
	}

	queued, err := r.prompt(id, text, model, provider, approval, thinking)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"session_id": id, "queued": queued, "approval_mode": approval}
	if queued {
		out["note"] = "the session is busy; the prompt will run when its current run finishes"
	}
	if clamped {
		out["clamped"] = note
	}
	return marshalResult(out)
}

// QueueControl reads and clears a session's staged prompt. It is a callback
// pair because the run service owns the queue and imports this package.
type QueueControl struct {
	Get   func(sessionID string) (prompt string, createdAt string, ok bool, err error)
	Clear func(sessionID string) (bool, error)
}

// sessionsQueue shows the prompt waiting to run on a session, if any.
func (r *Runtime) sessionsQueue(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if err := onlyArgs(args, "session_id"); err != nil {
		return nil, err
	}
	id, err := stringArg(args, "session_id")
	if err != nil {
		return nil, err
	}
	if r.queue.Get == nil {
		return nil, fmt.Errorf("queue control is unavailable")
	}
	prompt, createdAt, ok, err := r.queue.Get(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return marshalResult(map[string]any{"session_id": id, "queued": false})
	}
	return marshalResult(map[string]any{"session_id": id, "queued": true, "prompt": prompt, "created_at": createdAt})
}

// sessionsClearQueue discards the waiting prompt, which is what a refused
// sessions.prompt asks the caller to do.
func (r *Runtime) sessionsClearQueue(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if err := onlyArgs(args, "session_id"); err != nil {
		return nil, err
	}
	id, err := stringArg(args, "session_id")
	if err != nil {
		return nil, err
	}
	if r.queue.Clear == nil {
		return nil, fmt.Errorf("queue control is unavailable")
	}
	cleared, err := r.queue.Clear(id)
	if err != nil {
		return nil, err
	}
	return marshalResult(map[string]any{"session_id": id, "cleared": cleared})
}

// onlyArgs refuses arguments a method does not take. Silently ignoring an
// unknown key hides typos (`{projectId: ...}` instead of `{project_id: ...}`).
func onlyArgs(args map[string]any, allowed ...string) error {
	for key := range args {
		known := false
		for _, name := range allowed {
			if key == name {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("unknown argument %q; this method takes: %s", key, strings.Join(allowed, ", "))
		}
	}
	return nil
}

// optionalString reads a string argument. A value of any other type is an
// error rather than being treated as absent.
func optionalString(args map[string]any, name string) (string, error) {
	value, present := args[name]
	if !present {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string, got %v", name, value)
	}
	return text, nil
}
