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
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func gitFileEntryToProto(f types.GitFileEntry) *consolev1.GitFileEntry {
	additions := uint32(f.Additions)
	deletions := uint32(f.Deletions)
	return &consolev1.GitFileEntry{
		Path: f.Path, Status: string(f.Status), Staged: f.Staged,
		Additions: &additions, Deletions: &deletions,
	}
}

func gitSummaryToProto(s types.GitStatusSummary) *consolev1.GitStatusSummary {
	out := &consolev1.GitStatusSummary{Branch: s.Branch, Clean: s.Clean}
	for _, f := range s.Files {
		out.Files = append(out.Files, gitFileEntryToProto(f))
	}
	return out
}

func RegisterGitRoutes(app *fiber.App, git *services.GitService, watch *services.FsWatchService) {
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
		return okProto(c, gitSummaryToProto(git.GetGitStatus(repoPath)))
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

			sendStatus := func() {
				summary, err := protoMarshal.Marshal(gitSummaryToProto(git.GetGitStatus(repoPath)))
				if err != nil {
					return
				}
				_ = sse.Send("gitStatus", string(summary))
			}
			sendStatus()

			debounce := time.NewTimer(400 * time.Millisecond)
			debounce.Stop()
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-events:
					debounce.Reset(400 * time.Millisecond)
				case <-debounce.C:
					sendStatus()
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
