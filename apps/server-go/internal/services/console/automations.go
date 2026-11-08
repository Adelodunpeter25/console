// The automation methods: console.automation.* — schedules an agent creates
// for its own session. They act on the calling session only; the scanner in
// internal/run does the firing.
package console

import (
	"context"
	"fmt"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/automations"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

func requireAutomations(r *Runtime) (*automations.Service, error) {
	if r.automations == nil {
		return nil, fmt.Errorf("automations are unavailable")
	}
	return r.automations, nil
}

func automationPayload(a *automations.Automation) map[string]any {
	return map[string]any{
		"id": a.ID, "session_id": a.SessionID, "prompt": a.Prompt, "kind": a.Kind,
		"interval_seconds": a.IntervalSeconds, "run_at": a.RunAt,
		"next_run_at": a.NextRunAt, "status": a.Status,
		"last_fired_at": a.LastFiredAt, "last_error": a.LastError, "created_at": a.CreatedAt,
	}
}

// automationTiming reads the optional whole-number timing arguments. An absent
// or zero value means "not given"; the store validates the combination.
func automationTiming(args map[string]any) (interval, runAt int, err error) {
	interval, hasInterval, err := optionalInt(args, "interval_seconds")
	if err != nil {
		return 0, 0, err
	}
	if !hasInterval {
		interval = 0
	}
	runAt, hasRunAt, err := optionalInt(args, "run_at")
	if err != nil {
		return 0, 0, err
	}
	if !hasRunAt {
		runAt = 0
	}
	return interval, runAt, nil
}

// automationCreate schedules future work on this session. The prompt runs in
// this same thread with full context when the schedule fires.
func (r *Runtime) automationCreate(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireAutomations(r)
	if err != nil {
		return nil, err
	}
	if err := jskernel.OnlyArgs(args, "prompt", "kind", "interval_seconds", "run_at"); err != nil {
		return nil, err
	}
	prompt, err := stringArg(args, "prompt")
	if err != nil {
		return nil, err
	}
	kind, err := stringArg(args, "kind")
	if err != nil {
		return nil, err
	}
	interval, runAt, err := automationTiming(args)
	if err != nil {
		return nil, err
	}
	a, err := store.Create(r.sessionID, prompt, kind, int64(interval), int64(runAt), time.Now())
	if err != nil {
		return nil, err
	}
	return marshalResult(automationPayload(a))
}

// automationGet reads one automation by id, or lists this session's
// automations when no id is given.
func (r *Runtime) automationGet(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireAutomations(r)
	if err != nil {
		return nil, err
	}
	if err := jskernel.OnlyArgs(args, "id"); err != nil {
		return nil, err
	}
	id, present := args["id"]
	if !present || id == nil || id == "" {
		list, err := store.ListForSession(r.sessionID)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(list))
		for i := range list {
			out = append(out, automationPayload(&list[i]))
		}
		return marshalResult(out)
	}
	text, ok := id.(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("id must be a string, got %v", id)
	}
	a, err := store.Get(text)
	if err != nil {
		return nil, err
	}
	if a == nil || a.SessionID != r.sessionID {
		return nil, fmt.Errorf("no automation %q on this session", text)
	}
	return marshalResult(automationPayload(a))
}

// automationUpdate rewrites the mutable fields of an active automation and
// re-arms its next run from now.
func (r *Runtime) automationUpdate(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireAutomations(r)
	if err != nil {
		return nil, err
	}
	if err := jskernel.OnlyArgs(args, "id", "prompt", "kind", "interval_seconds", "run_at"); err != nil {
		return nil, err
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	existing, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	if existing == nil || existing.SessionID != r.sessionID {
		return nil, fmt.Errorf("no automation %q on this session", id)
	}
	prompt, kind := existing.Prompt, existing.Kind
	if value, err := optionalString(args, "prompt"); err != nil {
		return nil, err
	} else if value != "" {
		prompt = value
	}
	if value, err := optionalString(args, "kind"); err != nil {
		return nil, err
	} else if value != "" {
		kind = value
	}
	interval, runAt, err := automationTiming(args)
	if err != nil {
		return nil, err
	}
	if interval == 0 {
		interval = int(existing.IntervalSeconds)
	}
	if runAt == 0 {
		runAt = int(existing.RunAt)
	}
	a, err := store.Update(id, prompt, kind, int64(interval), int64(runAt), time.Now())
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("automation %q is %s and cannot be updated", id, existing.Status)
	}
	return marshalResult(automationPayload(a))
}

// automationDelete removes an automation. Call this when a monitoring task is
// complete — the harness does not know when the task is done.
func (r *Runtime) automationDelete(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireAutomations(r)
	if err != nil {
		return nil, err
	}
	if err := jskernel.OnlyArgs(args, "id"); err != nil {
		return nil, err
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	existing, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	if existing == nil || existing.SessionID != r.sessionID {
		return nil, fmt.Errorf("no automation %q on this session", id)
	}
	if _, err := store.Delete(id); err != nil {
		return nil, err
	}
	return marshalResult(map[string]any{"id": id, "deleted": true})
}
