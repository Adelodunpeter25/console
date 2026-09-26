// project_scripts tool: list, run, stop, or check status of console.toml project scripts.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// ProjectScriptsProvider is the interface satisfied by *services.ProjectScriptsService.
type ProjectScriptsProvider interface {
	List(projectID string) (types.ProjectScriptsResult, error)
	Run(projectID, scriptID string) (types.ScriptRun, error)
	Stop(projectID, runID string) bool
	ListRuns(projectID string) []types.ScriptRun
	GetRun(projectID, runID string) *types.ScriptRun
}

type projectScriptsInput struct {
	Action   string `json:"action" jsonschema:"required,enum=list,enum=start,enum=stop,enum=status,description=Action to perform on project scripts."`
	ScriptID string `json:"script_id,omitempty" jsonschema:"description=Script ID from console.toml (e.g.\\, 'dev'\\, 'build'\\, 'test') for 'start' or 'status'."`
	RunID    string `json:"run_id,omitempty" jsonschema:"description=Specific run ID to stop or inspect (used with 'stop' or 'status')."`
}

func NewProjectScriptsTool(projectID string, provider ProjectScriptsProvider) Tool {
	return NewTool(
		"project_scripts",
		"List, start, stop, or check status of project run scripts defined in console.toml.",
		TierExec,
		func(ctx context.Context, input projectScriptsInput) (any, error) {
			if provider == nil {
				return nil, NewToolError("Project scripts service is unavailable.")
			}
			if projectID == "" {
				return nil, NewToolError("No active project context for project_scripts.")
			}

			switch input.Action {
			case "list":
				result, err := provider.List(projectID)
				if err != nil {
					return nil, NewToolError("Failed to list project scripts: %v", err)
				}
				data, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return nil, NewToolError("Failed to format project scripts: %v", err)
				}
				return textResult(string(data)), nil

			case "start":
				if input.ScriptID == "" {
					return nil, NewToolError("script_id is required for 'start' action.")
				}
				run, err := provider.Run(projectID, input.ScriptID)
				if err != nil {
					return nil, NewToolError("Failed to start script '%s': %v", input.ScriptID, err)
				}
				data, err := json.MarshalIndent(run, "", "  ")
				if err != nil {
					return nil, NewToolError("Failed to format script run: %v", err)
				}
				return textResult(fmt.Sprintf("Script '%s' started successfully (run_id: %s):\n%s", input.ScriptID, run.RunID, string(data))), nil

			case "stop":
				if input.RunID == "" && input.ScriptID == "" {
					return nil, NewToolError("run_id or script_id is required for 'stop' action.")
				}
				targetRunID := input.RunID
				if targetRunID == "" {
					runs := provider.ListRuns(projectID)
					for _, r := range runs {
						if r.ScriptID == input.ScriptID && r.Status == "running" {
							targetRunID = r.RunID
							break
						}
					}
					if targetRunID == "" {
						return nil, NewToolError("No running instance found for script '%s'.", input.ScriptID)
					}
				}

				stopped := provider.Stop(projectID, targetRunID)
				if !stopped {
					return nil, NewToolError("Could not stop run '%s' (might already be stopped or does not exist).", targetRunID)
				}
				return textResult(fmt.Sprintf("Run '%s' stopped successfully.", targetRunID)), nil

			case "status":
				if input.RunID != "" {
					run := provider.GetRun(projectID, input.RunID)
					if run == nil {
						return nil, NewToolError("Run '%s' not found.", input.RunID)
					}
					data, err := json.MarshalIndent(run, "", "  ")
					if err != nil {
						return nil, NewToolError("Failed to format run status: %v", err)
					}
					return textResult(string(data)), nil
				}

				runs := provider.ListRuns(projectID)
				if input.ScriptID != "" {
					filtered := make([]types.ScriptRun, 0)
					for _, r := range runs {
						if r.ScriptID == input.ScriptID {
							filtered = append(filtered, r)
						}
					}
					runs = filtered
				}

				if len(runs) == 0 {
					return textResult("No script runs found."), nil
				}
				data, err := json.MarshalIndent(runs, "", "  ")
				if err != nil {
					return nil, NewToolError("Failed to format runs: %v", err)
				}
				return textResult(string(data)), nil

			default:
				return nil, NewToolError("Unknown action: %s. Expected 'list', 'start', 'stop', or 'status'.", input.Action)
			}
		},
	)
}

// ProjectScripts is the unbound default instance used by DefaultTools() (nil provider).
var ProjectScripts = NewProjectScriptsTool("", nil)
