package console

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

func (r *Runtime) scriptsList(_ context.Context, _ map[string]any) (*jskernel.Result, error) {
	if r.scripts == nil {
		return nil, fmt.Errorf("project scripts service is unavailable")
	}
	result, err := r.scripts.ListIn(r.projectID, r.cwd)
	if err != nil {
		return nil, err
	}
	return marshalResult(result)
}

func (r *Runtime) scriptsRun(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.scripts == nil {
		return nil, fmt.Errorf("project scripts service is unavailable")
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	run, err := r.scripts.RunIn(r.projectID, id, r.cwd)
	if err != nil {
		return nil, err
	}
	wait, hasWait, err := optionalInt(args, "wait_ms")
	if err != nil {
		return nil, err
	}
	if hasWait && wait > 0 {
		if wait > 10000 {
			wait = 10000
		}
		timer := time.NewTimer(time.Duration(wait) * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
		if updated := r.scripts.GetRun(r.projectID, run.RunID); updated != nil {
			run = *updated
		}
	}
	return marshalResult(run)
}

func (r *Runtime) scriptsStatus(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.scripts == nil {
		return nil, fmt.Errorf("project scripts service is unavailable")
	}
	if runID, ok := args["run_id"].(string); ok && runID != "" {
		run := r.scripts.GetRun(r.projectID, runID)
		if run == nil {
			return nil, fmt.Errorf("run %q not found", runID)
		}
		return marshalResult(*run)
	}
	return marshalResult(r.scripts.ListRunsIn(r.projectID, r.cwd))
}

func (r *Runtime) scriptsStop(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.scripts == nil {
		return nil, fmt.Errorf("project scripts service is unavailable")
	}
	runID, err := stringArg(args, "run_id")
	if err != nil {
		return nil, err
	}
	if !r.scripts.Stop(r.projectID, runID) {
		return nil, fmt.Errorf("run %q is not running", runID)
	}
	return &jskernel.Result{StructuredContent: json.RawMessage(`{"success":true}`)}, nil
}

// scriptsOutput pages through a run's output. Pass back `next` as `cursor` to
// read only what was written since the last call.
func (r *Runtime) scriptsOutput(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.scripts == nil {
		return nil, fmt.Errorf("project scripts service is unavailable")
	}
	runID, err := stringArg(args, "run_id")
	if err != nil {
		return nil, err
	}
	stream, _ := args["stream"].(string)
	if stream == "" {
		stream = "stdout"
	}
	cursor, _, err := optionalInt(args, "cursor")
	if err != nil {
		return nil, err
	}
	limit, _, err := optionalInt(args, "limit")
	if err != nil {
		return nil, err
	}
	chunk, err := r.scripts.ReadOutput(r.projectID, runID, stream, int64(cursor), limit)
	if err != nil {
		return nil, err
	}
	return marshalResult(chunk)
}
