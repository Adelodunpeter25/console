// Tool executor: runs model-requested tool calls through the registry with
// permission resolution, emitting events. Port of
// apps/server/agent/src/service/tool-executor.ts (initial slice).
package loop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Approver answers permission prompts (CLI/stdin, API approval route...).
type Approver interface {
	// Approve blocks until the user answers. ctx cancellation aborts.
	Approve(ctx context.Context, req permissions.Request) (bool, error)
}

type Executor struct {
	registry *tools.Registry
	mode     permissions.Mode
	approver Approver
}

func NewExecutor(registry *tools.Registry, mode permissions.Mode, approver Approver) *Executor {
	return &Executor{registry: registry, mode: mode, approver: approver}
}

// Execute resolves permissions, runs the tool, and always returns a
// ToolResult (tool failures become isError results; only approval/abort
// failures surface as errors).
func (e *Executor) Execute(ctx context.Context, call tools.ToolCall) (tools.ToolResult, error) {
	tool, err := e.registry.Get(call.Name)
	if err != nil {
		return e.errResult(call, err), nil
	}

	decision := permissions.Resolve(e.mode, tool.Tier())
	if decision == permissions.Prompt {
		if e.approver == nil {
			return e.errResult(call, tools.NewToolError("Tool '%s' requires approval but no approver is connected (mode %s).", call.Name, e.mode)), nil
		}
		req := permissions.Request{
			RequestID:  newRequestID(),
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Args:       jsonAny(call.Arguments),
			Tier:       tool.Tier(),
		}
		ok, err := e.approver.Approve(ctx, req)
		if err != nil {
			return tools.ToolResult{}, err
		}
		if !ok {
			return e.errResult(call, tools.NewToolError("User denied permission for tool '%s'.", call.Name)), nil
		}
	} else if decision == permissions.Deny {
		return e.errResult(call, tools.NewToolError("Tool '%s' is denied in %s mode.", call.Name, e.mode)), nil
	}

	out, err := executeToolCall(ctx, tool, call)
	if err != nil {
		var toolErr *tools.ToolError
		if asToolError(err, &toolErr) {
			return e.errResult(call, err), nil
		}
		// Unknown (non-tool) errors abort the turn.
		return tools.ToolResult{}, err
	}
	slog.Debug("tool executed", "tool", call.Name, "call", call.ID)
	return tools.ToolResult{ToolCallID: call.ID, ToolName: call.Name, Content: out}, nil
}

// executeToolCall prefers CallAwareTool.ExecuteCall (full call incl. id)
// over plain argument execution.
func executeToolCall(ctx context.Context, tool tools.Tool, call tools.ToolCall) (any, error) {
	if aware, ok := tool.(tools.CallAwareTool); ok {
		return aware.ExecuteCall(ctx, call)
	}
	return tool.Execute(ctx, call.Arguments)
}

func (e *Executor) errResult(call tools.ToolCall, err error) tools.ToolResult {
	return tools.ToolResult{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Content:    err.Error(),
		IsError:    true,
	}
}

// newRequestID mints a unique id per permission prompt (mirroring the TS
// randomUUID), so concurrent sessions can never collide on one map key.
func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "perm_" + hex.EncodeToString(b)
}
