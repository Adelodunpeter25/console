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
	projectID string
}

// NewRuntime builds a console kernel with the console API methods installed.
func NewRuntime(ports *services.PortRegistry, projectID string) (*Runtime, error) {
	r := &Runtime{ports: ports, projectID: projectID}
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
