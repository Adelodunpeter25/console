package console

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// Runtime is one session's console kernel.
type Runtime struct {
	kernel  *jskernel.Runtime
	manager *Manager
}

// NewRuntime builds a console kernel with the console API methods installed.
func NewRuntime() (*Runtime, error) {
	r := &Runtime{}
	// Phase 1: ports methods
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

// SetCancel installs the stop hook polled while a method call is in flight.
func (r *Runtime) SetCancel(fn func() bool) {
	r.kernel.SetCancel(fn)
}

// Eval runs code and converts the outcome to harness content parts.
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

// portsList returns the list of forwarded ports.
func (r *Runtime) portsList(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	// TODO: Implement by calling port_registry_service.List
	return &jskernel.Result{
		StructuredContent: json.RawMessage(`{"ports":[]}`),
	}, nil
}

// portsForward forwards a local port.
func (r *Runtime) portsForward(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	// TODO: Implement by calling port_registry_service.Forward
	return &jskernel.Result{
		StructuredContent: json.RawMessage(`{"success":true}`),
	}, nil
}

// portsUnforward removes a port forward.
func (r *Runtime) portsUnforward(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	// TODO: Implement by calling port_registry_service.RemoveOwner
	return &jskernel.Result{
		StructuredContent: json.RawMessage(`{"success":true}`),
	}, nil
}
