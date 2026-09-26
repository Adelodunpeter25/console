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
	// OnBeforeExecute, when set, runs after permission is granted and
	// immediately before the tool executes — the last point where the
	// tool's input files are still untouched. Used by the run service to
	// capture pre-write content for the session file-change diff. Optional.
	OnBeforeExecute func(call tools.ToolCall, tool tools.Tool)
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
		return e.errResult(call, err.Error()), nil
	}

	decision := permissions.Resolve(e.mode, tool.Tier())
	if decision == permissions.Prompt {
		if e.approver == nil {
			return e.errResult(call, tools.NewToolError("Tool '%s' requires approval but no approver is connected (mode %s).", call.Name, e.mode).Error()), nil
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
			// Timeout/abort/steer while waiting fails this call only (TS
			// parity); the loop sees ctx and stops on its own when aborted.
			return e.errResult(call, e.failureMessage(ctx, err)), nil
		}
		if !ok {
			return tools.ToolResult{
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    tools.NewToolError("User denied permission for tool '%s'.", call.Name).Error(),
				IsError:    true,
				Args:       call.Arguments,
			}, nil
		}
	} else if decision == permissions.Deny {
		return tools.ToolResult{
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Content:    tools.NewToolError("Tool '%s' is denied in %s mode.", call.Name, e.mode).Error(),
			IsError:    true,
			Args:       call.Arguments,
		}, nil
	}

	if ctx.Err() != nil {
		return e.errResult(call, abortedMessage), nil
	}
	if e.OnBeforeExecute != nil {
		e.OnBeforeExecute(call, tool)
	}
	out, err := executeToolCall(ctx, tool, call)
	if err != nil {
		// Every tool failure (including ask timeouts and "Run ended"
		// rejections) becomes an isError result the model can react to,
		// mirroring the TS executor; it never tears down the whole run.
		return e.errResult(call, e.failureMessage(ctx, err)), nil
	}
	slog.Debug("tool executed", "tool", call.Name, "call", call.ID)
	content, isError := any(out), false
	if envelope, ok := out.(tools.Envelope); ok {
		content, isError = envelope.Content, envelope.IsError
	}
	return tools.ToolResult{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Content:    content,
		IsError:    isError,
		Args:       call.Arguments,
	}, nil
}

// executeToolCall prefers CallAwareTool.ExecuteCall (full call incl. id)
// over plain argument execution.
func executeToolCall(ctx context.Context, tool tools.Tool, call tools.ToolCall) (any, error) {
	if aware, ok := tool.(tools.CallAwareTool); ok {
		return aware.ExecuteCall(ctx, call)
	}
	return tool.Execute(ctx, call.Arguments)
}

const abortedMessage = "Tool execution cancelled by user abort."

// failureMessage prefers the abort message when the run was cancelled so
// the model (and transcript) sees why, not a raw "context canceled".
func (e *Executor) failureMessage(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		return abortedMessage
	}
	return err.Error()
}

func (e *Executor) errResult(call tools.ToolCall, errMsg string) tools.ToolResult {
	return tools.ToolResult{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Content:    errMsg,
		IsError:    true,
		Args:       call.Arguments,
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
