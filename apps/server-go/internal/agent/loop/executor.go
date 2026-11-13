// Tool executor: runs model-requested tool calls through the registry with
// permission resolution, emitting events.
package loop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type Executor struct {
	registry *tools.Registry
	modeMu   sync.RWMutex
	mode     permissions.Mode
	// OnBeforeExecute, when set, runs after permission is granted and
	// immediately before the tool executes — the last point where the
	// tool's input files are still untouched. Used by the run service to
	// capture pre-write content for the session file-change diff. Optional.
	OnBeforeExecute func(call tools.ToolCall, tool tools.Tool)
	// Guard, when set, confines write-capable tool calls to the session's
	// project root and fills in glob/grep roots that would otherwise resolve
	// against the server's launch directory. It rewrites the arguments it
	// approves, so the tool acts on the exact path that was checked. Optional.
	Guard *permissions.Guard
}

func NewExecutor(registry *tools.Registry, mode permissions.Mode) *Executor {
	return &Executor{registry: registry, mode: mode}
}

// Mode is the executor's current permission mode.
func (e *Executor) Mode() permissions.Mode {
	e.modeMu.RLock()
	defer e.modeMu.RUnlock()
	return e.mode
}

// SetMode changes the mode mid-run; calls resolved afterwards use it.
func (e *Executor) SetMode(mode permissions.Mode) {
	e.modeMu.Lock()
	defer e.modeMu.Unlock()
	e.mode = mode
}

// Execute resolves permissions, runs the tool, and always returns a
// ToolResult (tool failures become isError results; only abort failures
// surface as errors).
func (e *Executor) Execute(ctx context.Context, call tools.ToolCall) (tools.ToolResult, error) {
	tool, err := e.registry.Get(call.Name)
	if err != nil {
		return e.errResult(call, err.Error()), nil
	}

	mode := e.Mode()
	decision := permissions.Resolve(mode, tool.Tier())
	// A plan-safe tool (writePlan) is the one write plan mode permits: it is
	// confined to the plan folder, so it needs no approval and is not denied.
	if mode == permissions.PlanMode {
		if safe, ok := tool.(tools.PlanSafe); ok && safe.PlanSafe() {
			decision = permissions.Allow
		}
	}
	if decision == permissions.Deny {
		return tools.ToolResult{
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Content:    deniedMessage(call.Name, mode),
			IsError:    true,
			Args:       call.Arguments,
		}, nil
	}

	if ctx.Err() != nil {
		return e.errResult(call, abortedMessage), nil
	}
	// The guard runs before OnBeforeExecute so a rejected call never triggers
	// a snapshot of files it was not going to touch.
	if e.Guard != nil {
		args, err := e.Guard.Check(call)
		if err != nil {
			return e.errResult(call, err.Error()), nil
		}
		call.Arguments = args
	}
	if e.OnBeforeExecute != nil {
		e.OnBeforeExecute(call, tool)
	}
	out, err := executeToolCall(ctx, tool, call)
	if err != nil {
		// Every tool failure (including ask timeouts and "Run ended"
		// rejections) becomes an isError result the model can react to,
		// it never tears down the whole run.
		return e.errResult(call, e.failureMessage(ctx, err)), nil
	}
	slog.Debug("tool executed", "tool", call.Name, "call", call.ID)
	content, isError := any(out), false
	var visual *tools.VisualArtifact
	if envelope, ok := out.(tools.Envelope); ok {
		content, isError, visual = envelope.Content, envelope.IsError, envelope.Visual
	}
	return tools.ToolResult{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Content:    content,
		IsError:    isError,
		Args:       call.Arguments,
		Visual:     visual,
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

// newRequestID mints a unique id per permission prompt, so concurrent
// sessions can never collide on one map key.
func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "perm_" + hex.EncodeToString(b)
}

// deniedMessage explains a deny so the model does not retry. In plan mode it
// says what to do instead: the model is otherwise left guessing why a write
// that worked a turn ago now fails.
func deniedMessage(tool string, mode permissions.Mode) string {
	if mode == permissions.PlanMode {
		return tools.NewToolError("Tool '%s' is denied: plan mode is read-only. Describe the change in your plan; to save the plan as a file use writePlan. To make changes, call requestFullAccess if the user asked you to implement; otherwise ask the user to switch out of plan mode.", tool).Error()
	}
	return tools.NewToolError("Tool '%s' is denied in %s mode.", tool, mode).Error()
}
