// The computer-use JavaScript runtime: one persistent interpreter per chat
// session with a preinstalled `cua` object.
//
// Instead of registering every driver tool as a harness tool (56 schemas,
// ~28K tokens re-sent on every turn), the model gets two tools — `computer`
// to run code and `computer_reset` to clear state — and the whole driver API
// as callable methods documented once in the /computer-use skill. The
// per-turn schema cost stays flat no matter how many methods the driver
// grows; API knowledge is read once, not re-sent as definitions.
//
// Evaluation, print collection, completion rendering, timeouts and interrupts
// live in internal/services/jskernel, shared with the console control-plane
// kernel. This file owns only what is computer-use specific: resolving
// element ids, waiting for a launched window, and adapting driver results to
// the kernel's result shape.
package cua

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// JSCaller is what the runtime drives: one tool call by name. Satisfied by
// *Driver; fakes satisfy it in tests so no native library is needed.
type JSCaller interface {
	Call(ctx context.Context, name string, args map[string]any, cancel func() bool) (*ToolResult, error)
	ListTools() ([]ToolDef, error)
}

// JSRuntime is one persistent JavaScript interpreter with a preinstalled
// `cua` object: one session's kernel, where variables and bindings survive
// between evaluations until reset. Exported so tests and the CLI can drive it
// without a chat session; the agent path goes through Manager, which owns
// one per session.
type JSRuntime struct {
	kernel *jskernel.Runtime
	caller JSCaller
	cancel func() bool
}

// NewJSRuntime builds a runtime and installs print plus the cua object from
// the live inventory, so new driver methods appear with no Console release.
func NewJSRuntime(caller JSCaller) (*JSRuntime, error) {
	r := &JSRuntime{caller: caller}
	defs, err := caller.ListTools()
	if err != nil {
		return nil, err
	}
	methods := make([]jskernel.Method, 0, len(defs))
	for _, def := range defs {
		name := def.Name
		if name == "" {
			continue
		}
		methods = append(methods, jskernel.Method{
			Name: name,
			Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
				return r.callDriver(ctx, name, args)
			},
		})
	}
	kernel, err := jskernel.New(jskernel.Options{
		Root:             "cua",
		InterruptMessage: "computer stopped; action completion is unknown. Inspect fresh state before retrying.",
		AsyncMessage:     "async code is not supported: call cua methods directly, they already wait for the result",
	}, methods)
	if err != nil {
		return nil, err
	}
	r.kernel = kernel
	return r, nil
}

// SetCancel installs the stop hook polled while a driver call is in flight, so
// a Manager Stop interrupts admitted work.
func (r *JSRuntime) SetCancel(fn func() bool) {
	r.cancel = fn
	r.kernel.SetCancel(fn)
}

// Eval runs code and converts the outcome to harness content parts.
func (r *JSRuntime) Eval(ctx context.Context, code string, timeout time.Duration) (any, error) {
	out, err := r.kernel.Eval(ctx, code, timeout)
	if err != nil {
		// The kernel reports model-visible failures through its own error
		// type so it can stay independent of the tool framework. Convert at
		// this boundary, where the tool contract lives.
		var kernelErr *jskernel.Error
		if errors.As(err, &kernelErr) {
			return nil, tools.NewToolError("%s", kernelErr.Msg)
		}
		return nil, err
	}
	return out, nil
}

// callDriver is every cua.* method: fix up the arguments, call the driver, and
// translate the result. Driver failures throw as JavaScript errors so the model
// sees the driver's own message and can react, rather than getting a harness
// abort.
func (r *JSRuntime) callDriver(ctx context.Context, name string, args map[string]any) (*jskernel.Result, error) {
	// element_id is ours, not the driver's: resolve it to a fresh token now
	// (the wire inputs deny unknown fields, so it must never be forwarded).
	var resolved *resolvedElement
	if _, ok := args["element_id"]; ok {
		var err error
		resolved, err = r.resolveElementArg(ctx, name, args)
		if err != nil {
			return nil, err
		}
	}
	// wait_timeout_ms tunes only the launch wait below; strip it for the same
	// reason.
	waitTimeoutMs := 0
	if rawTimeout, ok := args["wait_timeout_ms"]; ok {
		delete(args, "wait_timeout_ms")
		if timeout, ok := toInt64(rawTimeout); ok && timeout > 0 {
			waitTimeoutMs = int(timeout)
		}
	}
	result, err := r.caller.Call(ctx, name, args, r.cancel)
	if err != nil {
		return nil, fmt.Errorf("cua.%s failed: %w", name, err)
	}
	out := kernelResult(result)
	if resolved != nil && resolved.note != "" && !result.IsError {
		// Confirm what was actually touched: the id requested, the row it
		// landed on, and the role and label the tree displayed. Only for
		// successful acts — a failed call has nothing to confirm — and only
		// for the element_id path, which is the only one that resolved
		// anything.
		out.Content = append(out.Content, jskernel.Part{Type: "text", Text: resolved.note})
	}
	if name == "launch_app" {
		if note := r.waitLaunchWindow(ctx, args, result, waitTimeoutMs); note != "" {
			out.Content = append(out.Content, jskernel.Part{Type: "text", Text: note})
		}
	}
	return out, nil
}

// kernelResult adapts a driver result to the kernel's content-part shape.
func kernelResult(result *ToolResult) *jskernel.Result {
	if result == nil {
		return &jskernel.Result{}
	}
	out := &jskernel.Result{StructuredContent: result.StructuredContent, IsError: result.IsError}
	for _, part := range result.Content {
		out.Content = append(out.Content, jskernel.Part{
			Type: part.Type, Text: part.Text, Data: part.Data, MIMEType: part.MIMEType,
		})
	}
	return out
}
