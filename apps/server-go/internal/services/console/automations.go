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

func automationPayload(a *automations.Automation, now time.Time) map[string]any {
	return map[string]any{
		"id": a.ID, "session_id": a.SessionID, "prompt": a.Prompt, "kind": a.Kind,
		"interval_seconds": a.IntervalSeconds, "run_at": a.RunAt,
		"next_run_at": a.NextRunAt, "status": a.Status,
		"last_fired_at": a.LastFiredAt, "last_error": a.LastError, "created_at": a.CreatedAt,
		"now": now.Unix(),
	}
}

// automationTiming reads the optional whole-number timing arguments. An absent
// or zero value means "not given"; the store validates the combination.
func automationTiming(args map[string]any, now time.Time) (interval, runAt int, hasInterval, hasRunAt bool, err error) {
	interval, hasInterval, err = optionalInt(args, "interval_seconds")
	if err != nil {
		return 0, 0, false, false, err
	}
	runAt, hasRunAt, err = optionalInt(args, "run_at")
	if err != nil {
		return 0, 0, false, false, err
	}
	inSeconds, hasIn, err := optionalInt(args, "in_seconds")
	if err != nil {
		return 0, 0, false, false, err
	}
	if hasIn {
		if hasRunAt {
			return 0, 0, false, false, fmt.Errorf("pass either run_at or in_seconds, not both")
		}
		if inSeconds <= 0 {
			return 0, 0, false, false, fmt.Errorf("in_seconds must be positive")
		}
		runAt = int(now.Unix()) + inSeconds
		hasRunAt = true
	}
	return interval, runAt, hasInterval, hasRunAt, nil
}

// automationCreate schedules future work on this session. The prompt runs in
// this same thread with full context when the schedule fires.
func (r *Runtime) automationCreate(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireAutomations(r)
	if err != nil {
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
	now := time.Now()
	interval, runAt, _, _, err := automationTiming(args, now)
	if err != nil {
		return nil, err
	}
	a, err := store.Create(r.sessionID, prompt, kind, int64(interval), int64(runAt), now)
	if err != nil {
		return nil, err
	}
	return marshalResult(automationPayload(a, now))
}

// automationGet reads one automation by id, or lists this session's
// automations when no id is given.
func (r *Runtime) automationGet(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireAutomations(r)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	id, present := args["id"]
	if !present || id == nil || id == "" {
		list, err := store.ListForSession(r.sessionID)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(list))
		for i := range list {
			out = append(out, automationPayload(&list[i], now))
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
	return marshalResult(automationPayload(a, now))
}

// automationUpdate rewrites the mutable fields of an active, paused, or errored
// automation and re-arms its next run from now.
func (r *Runtime) automationUpdate(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireAutomations(r)
	if err != nil {
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
	now := time.Now()
	interval, runAt, hasInterval, hasRunAt, err := automationTiming(args, now)
	if err != nil {
		return nil, err
	}
	if kind == existing.Kind {
		if !hasInterval {
			interval = int(existing.IntervalSeconds)
		}
		if !hasRunAt {
			runAt = int(existing.RunAt)
		}
	}
	a, err := store.Update(id, prompt, kind, int64(interval), int64(runAt), now)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("automation %q is %s and cannot be updated", id, existing.Status)
	}
	return marshalResult(automationPayload(a, now))
}

// automationPause pauses an active automation on this session.
func (r *Runtime) automationPause(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireAutomations(r)
	if err != nil {
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
	a, err := store.Pause(id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("automation %q is %s and cannot be paused", id, existing.Status)
	}
	return marshalResult(automationPayload(a, time.Now()))
}

// automationResume resumes a paused or errored automation on this session.
func (r *Runtime) automationResume(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireAutomations(r)
	if err != nil {
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
	now := time.Now()
	a, err := store.Resume(id, now)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("automation %q is %s and cannot be resumed", id, existing.Status)
	}
	return marshalResult(automationPayload(a, now))
}

// automationDelete removes an automation. Call this when a monitoring task is
// complete — the harness does not know when the task is done.
func (r *Runtime) automationDelete(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	store, err := requireAutomations(r)
	if err != nil {
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
