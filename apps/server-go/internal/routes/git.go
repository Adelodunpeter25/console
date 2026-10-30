// Git routes (/api/git/*).
//
// Eighth domain on the shared protobuf schema. Status, diff, branches, and
// checkout payloads are built from console.v1 generated types; the watch
// stream keeps its snapshot-per-frame behavior with proto bytes. Status
// codes stay plain strings and numstat counts narrow to uint32, so numbers
// stay numbers. Canonical omissions apply (false staged/clean, empty lists,
// empty diff path) where the old code emitted zeros.
package routes

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/fs"
	gitsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/git"
)

// gitStatusDebounce is the minimum gap between status recomputations pushed
// to a watch stream.
const gitStatusDebounce = 1500 * time.Millisecond

func RegisterGitRoutes(app *fiber.App, git *gitsvc.GitService, watch *fs.FsWatchService) {
	h := app.Group("/api/git")

	okProto := func(c *fiber.Ctx, msg proto.Message) error {
		raw, err := protoMarshal.Marshal(msg)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	}
	fail := func(c *fiber.Ctx, err error) error {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": err.Error()})
	}

	// GET /api/git/status
	h.Get("/status", func(c *fiber.Ctx) error {
		repoPath := c.Query("path")
		if repoPath == "" {
			return fail(c, fmt.Errorf("Query parameter 'path' is required."))
		}
		return okProto(c, services.GitSummaryToProto(git.GetGitStatus(repoPath)))
	})

	// GET /api/git/status/watch — SSE snapshots on subscribe + debounced fs changes.
	h.Get("/status/watch", func(c *fiber.Ctx) error {
		repoPath := c.Query("path")
		if repoPath == "" {
			return fail(c, fmt.Errorf("Query parameter 'path' is required."))
		}
		watch.Watch(repoPath)
		return streamSSE(c, func(sse *sseStream) {
			events := watch.Subscribe(repoPath)
			defer watch.Unsubscribe(events)

			// A send error means the client is gone; stop instead of running
			// git for a dead stream until the next ping notices.
			sendStatus := func() error {
				summary, err := protoMarshal.Marshal(services.GitSummaryToProto(git.GetGitStatus(repoPath)))
				if err != nil {
					return nil
				}
				return sse.Send("gitStatus", string(summary))
			}
			if err := sendStatus(); err != nil {
				return
			}

			// Leading-edge throttle: the first change schedules one refresh
			// gitStatusDebounce later and further changes in that window fold
			// into it. Builds and pushes emit events continuously, so a trailing
			// debounce that resets on each one would never fire, and a short one
			// would spawn git constantly.
			debounce := time.NewTimer(gitStatusDebounce)
			debounce.Stop()
			pending := false
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-events:
					if !pending {
						pending = true
						debounce.Reset(gitStatusDebounce)
					}
				case <-debounce.C:
					pending = false
					if err := sendStatus(); err != nil {
						return
					}
				case <-ticker.C:
					if err := sse.Send("ping", ""); err != nil {
						return
					}
				}
			}
		})
	})

	// GET /api/git/diff
	h.Get("/diff", func(c *fiber.Ctx) error {
		repoPath := c.Query("repoPath")
		if repoPath == "" {
			repoPath = c.Query("cwd")
		}
		filePath := c.Query("path")
		if repoPath == "" && filePath == "" {
			return fail(c, fmt.Errorf("Query parameter 'repoPath' (or 'path') is required."))
		}
		diff, err := git.GetDiff(repoPath, filePath)
		if err != nil {
			return fail(c, err)
		}
		return okProto(c, &consolev1.GitDiffResponse{Path: filePath, Diff: diff})
	})

	// GET /api/git/branches
	h.Get("/branches", func(c *fiber.Ctx) error {
		repoPath := c.Query("path")
		if repoPath == "" {
			return fail(c, fmt.Errorf("Query parameter 'path' is required."))
		}
		summary := git.ListBranches(repoPath)
		out := &consolev1.GitBranchesResponse{IsGitRepository: summary.IsGitRepository}
		for _, b := range summary.Branches {
			out.Branches = append(out.Branches, &consolev1.GitBranchInfo{Name: b.Name, Current: b.Current})
		}
		return okProto(c, out)
	})

	// POST /api/git/checkout
	h.Post("/checkout", func(c *fiber.Ctx) error {
		var body consolev1.GitCheckoutRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &body); err != nil {
			return fail(c, fmt.Errorf("Invalid body."))
		}
		if body.Branch == "" {
			return fail(c, fmt.Errorf("Field 'branch' is required."))
		}
		if body.Path == "" {
			return fail(c, fmt.Errorf("Field 'path' is required."))
		}
		if err := git.CheckoutBranch(body.Path, body.Branch); err != nil {
			return fail(c, err)
		}
		return okProto(c, &consolev1.GitCheckoutResponse{Branch: body.Branch})
	})
}
