// Pull-request routes (/api/pull-requests/*).
//
// Read-only in phase 1: list, detail, changed files, one file's diff. The
// response payloads are the canonical console.v1 pull-request types with
// the presentation fields the server computed (checks_state, merge_state,
// status_label); clients only render. Error status codes map the provider's
// error vocabulary so each failure says what to do about it.
package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/proto"

	prv "github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func RegisterPullRequestRoutes(app *fiber.App, svc *services.PullRequestService) {
	fail := func(c *fiber.Ctx, err error) error {
		return c.Status(prStatusFor(err)).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	ok := func(c *fiber.Ctx, msg proto.Message) error {
		raw, err := protoMarshal.Marshal(msg)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	}

	// GET /api/pull-requests?scope=mine|review|repo&state=&project_id=&draft=
	app.Get("/api/pull-requests", func(c *fiber.Ctx) error {
		list, err := svc.List(c.Context(),
			c.Query("scope", "mine"), c.Query("state", "open"),
			c.Query("project_id"), c.Query("draft"))
		if err != nil {
			return fail(c, err)
		}
		return ok(c, list)
	})

	// GET /api/pull-requests/:owner/:repo/:number
	app.Get("/api/pull-requests/:owner/:repo/:number", func(c *fiber.Ctx) error {
		number, err := prNumber(c)
		if err != nil {
			return fail(c, err)
		}
		detail, err := svc.Detail(c.Context(), c.Params("owner"), c.Params("repo"), number)
		if err != nil {
			return fail(c, err)
		}
		return ok(c, detail)
	})

	// GET /api/pull-requests/:owner/:repo/:number/files
	app.Get("/api/pull-requests/:owner/:repo/:number/files", func(c *fiber.Ctx) error {
		number, err := prNumber(c)
		if err != nil {
			return fail(c, err)
		}
		files, err := svc.Files(c.Context(), c.Params("owner"), c.Params("repo"), number)
		if err != nil {
			return fail(c, err)
		}
		return ok(c, files)
	})

	// GET /api/pull-requests/:owner/:repo/:number/files/*path — one file's
	// unified diff for the existing diff viewers.
	app.Get("/api/pull-requests/:owner/:repo/:number/files/*", func(c *fiber.Ctx) error {
		number, err := prNumber(c)
		if err != nil {
			return fail(c, err)
		}
		diff, err := svc.FileDiff(c.Context(), c.Params("owner"), c.Params("repo"), number, c.Params("*"))
		if err != nil {
			return fail(c, err)
		}
		return ok(c, diff)
	})
}

// prNumber parses the PR route parameter.
func prNumber(c *fiber.Ctx) (uint32, error) {
	n, err := strconv.ParseUint(c.Params("number"), 10, 32)
	if err != nil || n == 0 {
		return 0, errors.New("Pull request number must be a positive integer.")
	}
	return uint32(n), nil
}

// prStatusFor maps the provider's error vocabulary onto status codes so
// each failure says what to do about it.
func prStatusFor(err error) int {
	var rateLimited *prv.RateLimitError
	var validation *prv.ValidationError
	switch {
	case errors.As(err, &rateLimited):
		return http.StatusTooManyRequests
	case errors.As(err, &validation):
		return http.StatusUnprocessableEntity
	case errors.Is(err, prv.ErrTokenRejected):
		return http.StatusUnauthorized
	case errors.Is(err, prv.ErrMissingScope):
		return http.StatusForbidden
	case errors.Is(err, prv.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, prv.ErrNotConnected):
		return http.StatusBadRequest
	case errors.Is(err, prv.ErrUnreachable):
		return http.StatusBadGateway
	default:
		return http.StatusBadRequest
	}
}
