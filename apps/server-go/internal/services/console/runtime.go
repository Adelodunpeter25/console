package console

import (
	"context"
	"errors"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/background"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
	terminalsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
)

// Runtime is one session's console kernel.
type Runtime struct {
	kernel    *jskernel.Runtime
	ports     *terminalsvc.PortRegistry
	scripts   *projectssvc.ProjectScriptsService
	sessions  *services.SessionService
	projects  *projectssvc.ProjectService
	pulls     *services.PullRequestService
	mcp       *mcp.Manager
	active    func(string) bool
	abort     func(string) bool
	prompt    PromptFunc
	queue     QueueControl
	ceiling   string
	start     func(string, string, string, string, string, string) error
	projectID string
	cwd       string
	sessionID string
	work      func() *background.Manager
}

// NewRuntime builds a console kernel with the console API methods installed.
func NewRuntime(ports *terminalsvc.PortRegistry, scripts *projectssvc.ProjectScriptsService, sessions *services.SessionService, projects *projectssvc.ProjectService, mcpManager *mcp.Manager, active func(string) bool, start func(string, string, string, string, string, string) error, abort func(string) bool, prompt PromptFunc, queue QueueControl, projectID, cwd string) (*Runtime, error) {
	r := &Runtime{ports: ports, scripts: scripts, sessions: sessions, projects: projects, mcp: mcpManager, active: active, abort: abort, prompt: prompt, queue: queue, start: start, projectID: projectID, cwd: cwd}
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
		{Name: "scripts.output", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.scriptsOutput(ctx, args)
		}},
		{Name: "scripts.stop", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.scriptsStop(ctx, args)
		}},
		{Name: "models", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.models(ctx, args)
		}},
		{Name: "approvalModes", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.approvalModes(ctx, args)
		}},
		{Name: "projects.list", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.projectsList(ctx, args)
		}},
		{Name: "projects.add", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.projectsAdd(ctx, args)
		}},
		{Name: "sessions.list", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsList(ctx, args)
		}},
		{Name: "sessions.read", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsRead(ctx, args)
		}},
		{Name: "sessions.create", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsCreate(ctx, args)
		}},
		{Name: "sessions.delete", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsDelete(ctx, args)
		}},
		{Name: "sessions.abort", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsAbort(ctx, args)
		}},
		{Name: "sessions.set_model", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsSetModel(ctx, args)
		}},
		{Name: "sessions.set_approval", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsSetApproval(ctx, args)
		}},
		{Name: "sessions.prompt", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsPrompt(ctx, args)
		}},
		{Name: "sessions.queue", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsQueue(ctx, args)
		}},
		{Name: "sessions.clear_queue", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsClearQueue(ctx, args)
		}},
		{Name: "sessions.wait", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsWait(ctx, args)
		}},
		{Name: "sessions.search", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsSearch(ctx, args)
		}},
		{Name: "sessions.fork", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.sessionsFork(ctx, args)
		}},
		{Name: "mcp.list", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.mcpListImpl(ctx, args)
		}},
		{Name: "mcp.tools", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.mcpToolsImpl(ctx, args)
		}},
		{Name: "mcp.add", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.mcpAddImpl(ctx, args)
		}},
		{Name: "mcp.update", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.mcpUpdateImpl(ctx, args)
		}},
		{Name: "mcp.remove", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.mcpRemoveImpl(ctx, args)
		}},
		{Name: "mcp.connect", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.mcpConnectImpl(ctx, args)
		}},
		{Name: "mcp.disconnect", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.mcpDisconnectImpl(ctx, args)
		}},
		{Name: "mcp.reset_auth", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.mcpResetAuthImpl(ctx, args)
		}},
		{Name: "mcp.call", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.mcpCallImpl(ctx, args)
		}},
		{Name: "pr.list", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.prList(ctx, args)
		}},
		{Name: "pr.read", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.prRead(ctx, args)
		}},
		{Name: "pr.create", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.prCreate(ctx, args)
		}},
		{Name: "pr.update", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.prUpdate(ctx, args)
		}},
		{Name: "pr.comment", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.prComment(ctx, args)
		}},
		{Name: "pr.merge", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.prMerge(ctx, args)
		}},
		{Name: "pr.close", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.prClose(ctx, args)
		}},
		{Name: "pr.reopen", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.prReopen(ctx, args)
		}},
		{Name: "pr.ready", Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			return r.prReady(ctx, args)
		}},
	}
	// Every method refuses keys it does not take, so a typo such as
	// {projectId: ...} fails with the accepted names instead of being ignored.
	for i := range methods {
		allowed, known := methodArgs[methods[i].Name]
		if !known {
			continue
		}
		call := methods[i].Call
		methods[i].Call = func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			if err := jskernel.OnlyArgs(args, allowed...); err != nil {
				return nil, err
			}
			return call(ctx, args)
		}
	}
	kernel, err := jskernel.New(jskernel.Options{
		Root:             "console",
		PlainResults:     true,
		InterruptMessage: "console stopped (timed out or cancelled); any call that was already in flight may have taken effect, but variables from earlier calls are kept. Check what happened before retrying: console.sessions.list() or console.sessions.read({session_id}) for sessions, console.scripts.status() for scripts, console.ports.list() for ports.",
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

func (r *Runtime) models(ctx context.Context, _ map[string]any) (*jskernel.Result, error) {
	return marshalResult(providers.ListProvidersWithContext(ctx))
}

// approvalMode is one valid approval mode, ordered from most restrictive to
// least. Rank is the clamp scale session control uses: a run may move a
// session down the scale, never up.
type approvalMode struct {
	Mode        string `json:"mode"`
	Rank        int    `json:"rank"`
	Description string `json:"description"`
}

// approvalModes lists the modes session control accepts, so the model never
// has to guess a valid value or the order a permission ceiling uses.
func (r *Runtime) approvalModes(context.Context, map[string]any) (*jskernel.Result, error) {
	return marshalResult(map[string]any{
		"ordering": []string{
			string(permissions.PlanMode),
			string(permissions.FullAccess),
		},
		"modes": []approvalMode{
			{Mode: string(permissions.PlanMode), Rank: 0, Description: "Read-only: writes and command execution are refused."},
			{Mode: string(permissions.FullAccess), Rank: 1, Description: "Everything runs without approval."},
		},
	})
}

// methodArgs lists the arguments each console method accepts. A method not
// listed here is not argument-checked.
var methodArgs = map[string][]string{
	"ports.list":            {},
	"ports.forward":         {"port"},
	"ports.unforward":       {"port"},
	"scripts.list":          {},
	"scripts.run":           {"id", "wait_ms"},
	"scripts.status":        {"run_id"},
	"scripts.output":        {"run_id", "stream", "cursor", "limit"},
	"scripts.stop":          {"run_id"},
	"models":                {},
	"approvalModes":         {},
	"projects.list":         {},
	"projects.add":          {"directory", "name"},
	"sessions.list":         {"project_id", "cwd"},
	"sessions.read":         {"session_id", "limit", "before", "include_deleted"},
	"sessions.create":       {"prompt", "project_id", "cwd", "model", "provider", "thinking_level", "approval_mode", "worktree"},
	"sessions.delete":       {"session_id"},
	"sessions.abort":        {"session_id"},
	"sessions.set_model":    {"session_id", "model", "provider"},
	"sessions.set_approval": {"session_id", "mode"},
	"sessions.prompt":       {"session_id", "prompt", "model", "provider", "thinking_level", "approval_mode"},
	"sessions.queue":        {"session_id"},
	"sessions.clear_queue":  {"session_id"},
	"sessions.fork":         {"session_id", "message_id"},
	"sessions.search":       {"query", "session_id", "project_id", "cwd", "all_projects", "roles", "limit", "per_session", "include_deleted"},
	"sessions.wait":         {"session_id", "session_ids", "mode", "timeout_ms", "background"},
	"mcp.list":              {},
	"mcp.tools":             {"id"},
	"mcp.add":               {"id", "transport", "url", "command", "args", "env", "label", "auth", "enabled"},
	"mcp.update":            {"id", "label", "url", "command", "args", "env", "auth", "tier_overrides", "enabled"},
	"mcp.remove":            {"id"},
	"mcp.connect":           {"id"},
	"mcp.disconnect":        {"id"},
	"mcp.reset_auth":        {"id"},
	"mcp.call":              {"id", "tool", "args"},
	"pr.list":               {"scope", "state", "project_id", "draft"},
	"pr.read":               {"owner", "repo", "number"},
	"pr.create":             {"owner", "repo", "project_id", "head", "base", "title", "body", "draft"},
	"pr.update":             {"owner", "repo", "number", "title", "body", "base", "state"},
	"pr.comment":            {"owner", "repo", "number", "body"},
	"pr.merge":              {"owner", "repo", "number", "method", "expected_head"},
	"pr.close":              {"owner", "repo", "number"},
	"pr.reopen":             {"owner", "repo", "number"},
	"pr.ready":              {"owner", "repo", "number"},
}
