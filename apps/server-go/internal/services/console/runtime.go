package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// Runtime is one session's console kernel.
type Runtime struct {
	kernel    *jskernel.Runtime
	ports     *services.PortRegistry
	scripts   *services.ProjectScriptsService
	projectID string
	cwd       string
}

// NewRuntime builds a console kernel with the console API methods installed.
func NewRuntime(ports *services.PortRegistry, scripts *services.ProjectScriptsService, projectID, cwd string) (*Runtime, error) {
	r := &Runtime{ports: ports, scripts: scripts, projectID: projectID, cwd: cwd}
	methods := []jskernel.Method{
		{Name: "ports.list", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.portsList(ctx, args)
		}},
		{Name: "ports.forward", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.portsForward(ctx, args)
		}},
		{Name: "ports.unforward", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.portsUnforward(ctx, args)
		}},
		{Name: "scripts.list", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.scriptsList(ctx, args)
		}},
		{Name: "scripts.run", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.scriptsRun(ctx, args)
		}},
		{Name: "scripts.status", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.scriptsStatus(ctx, args)
		}},
		{Name: "scripts.stop", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.scriptsStop(ctx, args)
		}},
	}
	kernel, err := jskernel.New(jskernel.Options{
		Root:             "console",
		InterruptMessage: "console stopped; action completion is unknown. Inspect fresh state before retrying.",
		AsyncMessage:     "async code is not supported: call console methods directly, they already wait for the result",
	}, methods)
	if err != nil {
		return nil, err
	}
	r.kernel = kernel
	return r, nil
}

func (r *Runtime) SetCancel(fn func() bool) { r.kernel.SetCancel(fn) }

func (r *Runtime) Eval(ctx context.Context, code string, timeout time.Duration) (any, error) {
	out, err := r.kernel.Eval(ctx, code, timeout)
	if err != nil {
		var kernelErr *jskernel.Error
		if errors.As(err, &kernelErr) {
			return nil, tools.NewToolError("%s", kernelErr.Msg)
		}
		return nil, err
	}
	return out, nil
}

func (r *Runtime) portsList(_ context.Context, _ map[string]any) (*jskernel.Result, error) {
	if r.ports == nil {
		return nil, fmt.Errorf("ports service is unavailable")
	}
	ports := r.ports.List("localhost", r.projectID)
	data, err := json.Marshal(ports)
	if err != nil {
		return nil, err
	}
	return &jskernel.Result{StructuredContent: data}, nil
}

func (r *Runtime) portsForward(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.ports == nil {
		return nil, fmt.Errorf("ports service is unavailable")
	}
	port, err := portArg(args)
	if err != nil {
		return nil, err
	}
	forwarded, err := r.ports.Forward(port, r.projectID)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(forwarded)
	if err != nil {
		return nil, err
	}
	return &jskernel.Result{StructuredContent: data}, nil
}

func (r *Runtime) portsUnforward(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.ports == nil {
		return nil, fmt.Errorf("ports service is unavailable")
	}
	port, err := portArg(args)
	if err != nil {
		return nil, err
	}
	if !r.ports.Unforward(port, r.projectID) {
		return nil, fmt.Errorf("port %d is not currently forwarded", port)
	}
	return &jskernel.Result{StructuredContent: json.RawMessage(`{"success":true}`)}, nil
}

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
	if wait, ok := optionalInt(args, "wait_ms"); ok && wait > 0 {
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

func marshalResult(value any) (*jskernel.Result, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &jskernel.Result{StructuredContent: data}, nil
}

func stringArg(args map[string]any, name string) (string, error) {
	value, ok := args[name].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func optionalInt(args map[string]any, name string) (int, bool) {
	value, ok := args[name]
	if !ok {
		return 0, false
	}
	if n, ok := value.(int); ok {
		return n, true
	}
	if f, ok := value.(float64); ok && f == float64(int(f)) {
		return int(f), true
	}
	return 0, false
}

func portArg(args map[string]any) (int, error) {
	value, ok := args["port"]
	if !ok {
		return 0, fmt.Errorf("port is required")
	}
	n, ok := value.(int)
	if !ok {
		if f, ok := value.(float64); ok && f == float64(int(f)) {
			n = int(f)
		} else {
			return 0, fmt.Errorf("port must be an integer")
		}
	}
	if n <= 0 {
		return 0, fmt.Errorf("port must be greater than zero")
	}
	return n, nil
}
