// Worktree launch routes (POST /api/sessions/with-worktree).
//
// The instant-launch path behind the "New worktree" composer option:
// create the session row immediately and return its header, then provision
// the worktree in the background and auto-run the stored first prompt when
// it lands. Clients render launchState ("provisioning"/"failed") from the
// session header instead of blocking on the create call, and stream the run
// through the normal GET /api/sessions/:id/run/stream attach.
//
// Distinct from POST /api/sessions with a worktree spec, which stays fully
// synchronous (provision before responding) for existing callers.
package routes

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	gitsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/git"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

type worktreeLaunchBody struct {
	Cwd           string  `json:"cwd"`
	ProjectID     *string `json:"projectId"`
	Title         string  `json:"title"`
	ModelID       string  `json:"modelId"`
	Provider      string  `json:"provider"`
	ApprovalMode  string  `json:"approvalMode"`
	ThinkingLevel *string `json:"thinkingLevel"`
	Worktree      *struct {
		Branch     string `json:"branch"`
		BaseBranch string `json:"baseBranch"`
	} `json:"worktree"`
	Prompt struct {
		Prompt       string   `json:"prompt"`
		ContextFiles []string `json:"contextFiles"`
		Attachments  []struct {
			Data     string `json:"data"`
			MimeType string `json:"mimeType"`
		} `json:"attachments"`
		ModelID      string `json:"modelId"`
		Provider     string `json:"provider"`
		ApprovalMode string `json:"approvalMode"`
		Thinking     string `json:"thinkingLevel"`
	} `json:"prompt"`
}

// runWorktreeLaunch provisions the worktree for a registered launch and
// starts its run, moving the registry entry through provisioning → cleared
// (ready) or failed. Retry skips the attach when the session already owns
// the worktree: a failed run-start after a successful attach must not die
// on ErrWorktreeAlreadyOwned.
func runWorktreeLaunch(sessions *services.SessionService, runs *run.Service, launch services.SessionLaunch) {
	header, err := sessions.Header(launch.SessionID)
	if err != nil || header == nil {
		services.WorktreeLaunches.MarkFailed(launch.SessionID, "session not found")
		return
	}
	if header.Worktree == nil {
		spec := launch.Worktree
		if _, err := sessions.AttachWorktree(launch.SessionID, &spec); err != nil {
			services.WorktreeLaunches.MarkFailed(launch.SessionID, err.Error())
			return
		}
	}
	prompt := run.Prompt{
		Text: strings.TrimSpace(launch.Prompt.Text), ContextFiles: launch.Prompt.ContextFiles,
		ModelID: launch.Prompt.ModelID, Provider: launch.Prompt.Provider,
		ApprovalMode: launch.Prompt.ApprovalMode, Thinking: launch.Prompt.Thinking,
	}
	for _, a := range launch.Prompt.Attachments {
		prompt.Attachments = append(prompt.Attachments, run.Attachment{Data: a.Data, MimeType: a.MimeType})
	}
	if _, err := runs.StartRun(launch.SessionID, prompt); err != nil {
		services.WorktreeLaunches.MarkFailed(launch.SessionID, err.Error())
		return
	}
	services.WorktreeLaunches.Clear(launch.SessionID)
}

func RegisterWorktreeLaunchRoutes(app *fiber.App, sessions *services.SessionService, runs *run.Service) {
	// POST /api/sessions/with-worktree — create the session now, provision
	// + run in the background. The response header carries
	// launchState=provisioning until the worktree exists and the prompt is
	// running.
	app.Post("/api/sessions/with-worktree", func(c *fiber.Ctx) error {
		var body worktreeLaunchBody
		if err := json.Unmarshal(c.Body(), &body); err != nil {
			return sessionError(c, fiber.StatusBadRequest, "Invalid request body.")
		}
		if body.Worktree == nil {
			return sessionError(c, fiber.StatusBadRequest, "Field 'worktree' is required.")
		}
		if strings.TrimSpace(body.Prompt.Prompt) == "" {
			return sessionError(c, fiber.StatusBadRequest, "Field 'prompt.prompt' is required.")
		}
		if body.Cwd == "" {
			return sessionError(c, fiber.StatusBadRequest, "Field 'cwd' is required.")
		}

		header, err := sessions.Create(types.CreateSessionOptions{
			Title: body.Title, Cwd: body.Cwd, ProjectID: body.ProjectID,
			ModelID: body.ModelID, Provider: body.Provider,
			ApprovalMode: body.ApprovalMode, ThinkingLevel: body.ThinkingLevel,
		})
		if err != nil {
			if errors.Is(err, services.ErrWorktreeScratchpad) ||
				errors.Is(err, services.ErrWorktreeNeedsCwd) ||
				errors.Is(err, gitsvc.ErrNotGitRepo) ||
				errors.Is(err, gitsvc.ErrUnknownBaseBranch) ||
				errors.Is(err, gitsvc.ErrUnbornHEAD) {
				return sessionError(c, fiber.StatusBadRequest, err.Error())
			}
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}

		prompt := services.LaunchPrompt{
			Text: body.Prompt.Prompt, ContextFiles: body.Prompt.ContextFiles,
			ModelID: body.Prompt.ModelID, Provider: body.Prompt.Provider,
			ApprovalMode: body.Prompt.ApprovalMode, Thinking: body.Prompt.Thinking,
		}
		for _, a := range body.Prompt.Attachments {
			prompt.Attachments = append(prompt.Attachments, services.LaunchAttachment{Data: a.Data, MimeType: a.MimeType})
		}
		spec := types.CreateWorktreeSpec{Branch: body.Worktree.Branch, BaseBranch: body.Worktree.BaseBranch}
		services.WorktreeLaunches.Start(header.ID, spec, prompt)
		go runWorktreeLaunch(sessions, runs, services.SessionLaunch{
			SessionID: header.ID, Worktree: spec, Prompt: prompt,
			State: services.LaunchProvisioning,
		})

		raw, err := protoMarshal.Marshal(sessionHeaderToProto(header))
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	// POST /api/sessions/:id/with-worktree/retry — re-run a failed launch
	// from its stored payload. 404 when nothing is registered, 409 while
	// still provisioning.
	app.Post("/api/sessions/:id/with-worktree/retry", func(c *fiber.Ctx) error {
		id := c.Params("id")
		launch, ok := services.WorktreeLaunches.TakeFailed(id)
		if !ok {
			if existing := services.WorktreeLaunches.Get(id); existing != nil {
				return sessionError(c, fiber.StatusConflict, "Worktree launch is already in progress.")
			}
			return sessionError(c, fiber.StatusNotFound, "No failed worktree launch for session '"+id+"'.")
		}
		go runWorktreeLaunch(sessions, runs, *launch)
		return c.JSON(fiber.Map{"success": true})
	})
}
