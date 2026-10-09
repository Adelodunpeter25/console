package run

import (
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// sessionScopedScripts runs the project_scripts tool against the session's own
// folder, so a worktree chat's agent reads console.toml from and runs scripts
// in its worktree rather than the project's main checkout.
type sessionScopedScripts struct {
	svc *services.ProjectScriptsService
	cwd string
}

func (p sessionScopedScripts) List(projectID string) (types.ProjectScriptsResult, error) {
	return p.svc.ListIn(projectID, p.cwd)
}

func (p sessionScopedScripts) Run(projectID, scriptID string) (types.ScriptRun, error) {
	return p.svc.RunIn(projectID, scriptID, p.cwd)
}

func (p sessionScopedScripts) Stop(projectID, runID string) bool {
	return p.svc.Stop(projectID, runID)
}

func (p sessionScopedScripts) ListRuns(projectID string) []types.ScriptRun {
	return p.svc.ListRuns(projectID)
}

func (p sessionScopedScripts) GetRun(projectID, runID string) *types.ScriptRun {
	return p.svc.GetRun(projectID, runID)
}
