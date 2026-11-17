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
	methods := kernelMethods(r)
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
