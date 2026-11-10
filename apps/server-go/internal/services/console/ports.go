package console

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

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
	n, ok := intValue(value)
	if !ok {
		return 0, fmt.Errorf("port must be a whole number such as 3000, got %v", value)
	}
	if n <= 0 || n > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535, got %d", n)
	}
	return n, nil
}
