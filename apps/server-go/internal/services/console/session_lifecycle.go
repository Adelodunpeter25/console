package console

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func (r *Runtime) sessionsList(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.sessions == nil {
		return nil, fmt.Errorf("session service is unavailable")
	}
	if err := jskernel.OnlyArgs(args, "project_id", "cwd"); err != nil {
		return nil, err
	}
	filter := session.ListFilter{}
	var err error
	if filter.ProjectID, err = optionalString(args, "project_id"); err != nil {
		return nil, err
	}
	if filter.Cwd, err = optionalString(args, "cwd"); err != nil {
		return nil, err
	}
	sessions, err := r.sessions.ListFiltered(filter)
	if err != nil {
		return nil, err
	}
	return marshalResult(sessions)
}

func (r *Runtime) sessionsRead(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.sessions == nil {
		return nil, fmt.Errorf("session service is unavailable")
	}
	id, err := stringArg(args, "session_id")
	if err != nil {
		return nil, err
	}
	// limit returns only the most recent N messages; pass nextCursor back as
	// `before` to page further into history. Omitted, the whole transcript.
	limit, _, err := optionalInt(args, "limit")
	if err != nil {
		return nil, err
	}
	before, _, err := optionalInt(args, "before")
	if err != nil {
		return nil, err
	}
	if limit < 0 || before < 0 {
		return nil, fmt.Errorf("limit and before must not be negative")
	}
	includeDeleted := false
	if raw, present := args["include_deleted"]; present {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("include_deleted must be true or false, got %v", raw)
		}
		includeDeleted = b
	}
	var loaded *types.LoadedSession
	if includeDeleted {
		loaded, err = r.sessions.LoadIncludingDeleted(id, int64(limit), int64(before))
	} else {
		loaded, err = r.sessions.Load(id, int64(limit), int64(before))
	}
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		if h, herr := r.sessions.HeaderIncludingDeleted(id); herr == nil && h != nil && h.DeletedAt != nil {
			if deleted, _ := args["include_deleted"].(bool); !deleted {
				return nil, fmt.Errorf("session %q is deleted; pass include_deleted: true to read it", id)
			}
		}
		return nil, fmt.Errorf("session %q not found", id)
	}
	return marshalResult(loaded)
}

func (r *Runtime) sessionsCreate(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.sessions == nil {
		return nil, fmt.Errorf("session service is unavailable")
	}
	prompt, _ := args["prompt"].(string)
	model, _ := args["model"].(string)
	provider, _ := args["provider"].(string)
	approval, _ := args["approval_mode"].(string)
	thinking, _ := args["thinking_level"].(string)
	projectID, _ := args["project_id"].(string)
	cwd, _ := args["cwd"].(string)

	var callingHeader *types.SessionHeader
	if r.sessionID != "" {
		if h, err := r.sessions.Header(r.sessionID); err == nil && h != nil {
			callingHeader = h
		}
	}

	// When model is omitted, inherit the calling session's model, provider,
	// and thinking level.
	if model == "" && callingHeader != nil && callingHeader.ModelID != "" {
		model = callingHeader.ModelID
		if provider == "" {
			provider = callingHeader.Provider
		}
		if thinking == "" && callingHeader.ThinkingLevel != nil {
			thinking = *callingHeader.ThinkingLevel
		}
	}
	if cwd == "" && projectID != "" && r.projects != nil {
		project, err := r.projects.Get(projectID)
		if err != nil {
			return nil, err
		}
		cwd = project.Path
	}
	// A new session may never hold more authority than the run creating it.
	// An omitted mode inherits the ceiling itself.
	var clampNote string
	if approval == "" {
		approval = r.ceilingOrDefault()
	} else {
		effective, _, note, err := clampApproval(r.ceiling, approval)
		if err != nil {
			return nil, err
		}
		approval, clampNote = effective, note
	}
	var thinkingPtr *string
	if thinking != "" {
		if err := r.validateThinking(ctx, model, provider, thinking); err != nil {
			return nil, err
		}
		thinkingPtr = &thinking
	}
	var projectPtr *string
	if projectID != "" {
		projectPtr = &projectID
	}
	var worktree *types.CreateWorktreeSpec
	if raw, ok := args["worktree"].(map[string]any); ok {
		worktree = &types.CreateWorktreeSpec{}
		worktree.Branch, _ = raw["branch"].(string)
		worktree.BaseBranch, _ = raw["base_branch"].(string)
	}
	header, err := r.sessions.Create(types.CreateSessionOptions{Title: "", Cwd: cwd, ProjectID: projectPtr, ModelID: model, Provider: provider, ApprovalMode: approval, ThinkingLevel: thinkingPtr, Worktree: worktree})
	if err != nil {
		return nil, err
	}
	if prompt != "" && r.start != nil {
		if err := r.start(header.ID, prompt, model, provider, approval, thinking); err != nil {
			return nil, err
		}
	}
	// The header was read before the run started, so it still says idle. When a
	// prompt was started the session is running; say so rather than leaving the
	// caller to poll.
	started := prompt != "" && r.start != nil
	if started {
		header.Status = "working"
	}
	out := map[string]any{"session": header, "prompt": prompt, "approval_mode": approval, "started": started}
	if clampNote != "" {
		out["clamped"] = clampNote
	}
	return marshalResult(out)
}

func (r *Runtime) sessionsDelete(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.sessions == nil {
		return nil, fmt.Errorf("session service is unavailable")
	}
	id, err := stringArg(args, "session_id")
	if err != nil {
		return nil, err
	}
	if r.active != nil && r.active(id) {
		return nil, fmt.Errorf("cannot delete active session %q; abort it first", id)
	}
	deleted, err := r.sessions.SoftDelete(id)
	if err != nil {
		return nil, err
	}
	if !deleted {
		return nil, fmt.Errorf("session %q not found", id)
	}
	return &jskernel.Result{StructuredContent: json.RawMessage(`{"deleted":true}`)}, nil
}
