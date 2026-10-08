// OAuth auth routes (codex slice): status, login URL generation, and the
// code-exchange callback. Other providers answer 501 until implemented.
package routes

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

// RegisterAuthRoutes wires the /api/auth routes onto an app. Exported so
// route tests can build a Fiber app with only these routes, mirroring
// RegisterMCPRoutes.
func RegisterAuthRoutes(app *fiber.App, authSvc *auth.AuthService) {
	h := app.Group("/api/auth")

	h.Get("/status", func(c *fiber.Ctx) error {
		raw, err := protoMarshal.Marshal(authStatusToProto(authSvc.GetStatus()))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	h.Post("/login/url", func(c *fiber.Ctx) error {
		var req struct {
			Provider string `json:"provider"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if req.Provider == "" {
			req.Provider = "codex"
		}
		switch req.Provider {
		case "codex", "claude", "antigravity":
			result, err := authSvc.GetLoginURLFor(req.Provider)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
			}
			raw, err := protoMarshal.Marshal(loginURLToProto(result))
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
			}
			return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
		case "devin":
			return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"success": false, "error": "OAuth login for '" + req.Provider + "' is not supported by the Go server yet."})
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid OAuth provider."})
		}
	})

	h.Post("/login/callback", func(c *fiber.Ctx) error {
		var req struct {
			Provider string `json:"provider"`
			Code     string `json:"code"`
			State    string `json:"state"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if req.Provider == "" {
			req.Provider = "codex"
		}
		switch req.Provider {
		case "codex", "claude", "antigravity":
			if req.Code == "" {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Authorization 'code' is required."})
			}
			result, err := authSvc.HandleCallbackFor(req.Provider, req.Code, req.State)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
			}
			raw, err := protoMarshal.Marshal(callbackResultToProto(result))
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
			}
			return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
		case "devin":
			return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"success": false, "error": "OAuth login for '" + req.Provider + "' is not supported by the Go server yet."})
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid OAuth provider."})
		}
	})

	h.Get("/project-id/:provider", func(c *fiber.Ctx) error {
		provider := c.Params("provider")
		if provider != "antigravity" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid provider."})
		}
		projectID, err := authSvc.GetProjectID(provider)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		raw, err := protoMarshal.Marshal(&consolev1.ProjectIDResponse{ProjectId: optionalNonEmpty(projectID)})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	h.Post("/project-id", func(c *fiber.Ctx) error {
		var req struct {
			Provider  string  `json:"provider"`
			ProjectID *string `json:"projectId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if req.Provider != "antigravity" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid provider."})
		}
		var id string
		if req.ProjectID != nil {
			id = *req.ProjectID
		}
		if err := authSvc.SetProjectID(req.Provider, id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		trimmed := strings.TrimSpace(id)
		raw, err := protoMarshal.Marshal(&consolev1.SetProjectIDResponse{
			Provider:  req.Provider,
			ProjectId: optionalNonEmpty(trimmed),
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	registerGitHubAuthRoutes(h, authSvc)
}

// registerGitHubAuthRoutes wires the GitHub git-credential endpoints. These
// are separate from the OAuth provider routes above: a personal access token
// is not a chat session, and no GitHub OAuth App is involved.
//
// The token itself never crosses this boundary — the response carries only
// the cached username and scopes.
func registerGitHubAuthRoutes(h fiber.Router, authSvc *auth.AuthService) {
	h.Post("/github/pat", func(c *fiber.Ctx) error {
		var req struct {
			Token string `json:"token"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if strings.TrimSpace(req.Token) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "A GitHub token is required."})
		}
		status, err := authSvc.ValidateGitHubPAT(c.Context(), req.Token)
		if err != nil {
			// Distinguish "your token is wrong" (400, fix the input) from
			// "GitHub is unreachable" (502, retry later). ErrInvalidToken is
			// checked with errors.Is; the unreachable case is matched on the
			// sentinel error rather than its message so the wording can change
			// without silently flipping the status code.
			switch {
			case errors.Is(err, github.ErrInvalidToken):
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": err.Error()})
			case errors.Is(err, github.ErrUnreachable):
				return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"success": false, "error": err.Error()})
			default:
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": err.Error()})
			}
		}
		raw, err := protoMarshal.Marshal(githubStatusToProto(status))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	h.Get("/github/status", func(c *fiber.Ctx) error {
		raw, err := protoMarshal.Marshal(githubStatusToProto(auth.GitHubStatus()))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	h.Post("/github/logout", func(c *fiber.Ctx) error {
		if err := authSvc.DisconnectGitHub(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		raw, err := protoMarshal.Marshal(&consolev1.GitHubLogoutResponse{Disconnected: true})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})
}

// optionalNonEmpty maps "" to an absent optional field. Both project-id
// routes used to answer null for "unset", and protojson drops an unset
// optional, so the key simply disappears — clients already read it as a
// defaulting Option, so this is the same "no project id" they saw before.
func optionalNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// authStatusToProto converts the aggregated login state to the canonical wire
// type. Every provider is always emitted so a client never has to tell
// "logged out" apart from "not reported".
func authStatusToProto(s auth.AuthStatus) *consolev1.AuthStatusResponse {
	return &consolev1.AuthStatusResponse{
		Antigravity: providerAuthToProto(s.Antigravity),
		Codex:       providerAuthToProto(s.Codex),
		Devin:       providerAuthToProto(s.Devin),
		Claude:      providerAuthToProto(s.Claude),
		Github:      githubStatusToProto(s.GitHub),
	}
}

func providerAuthToProto(s auth.ProviderAuthStatus) *consolev1.ProviderAuthStatus {
	out := &consolev1.ProviderAuthStatus{LoggedIn: s.LoggedIn}
	// Email is omitempty on the Go side, so an absent email must stay absent
	// rather than becoming an empty string.
	if s.Email != "" {
		out.Email = &s.Email
	}
	return out
}

func githubStatusToProto(s auth.GitHubAuthStatus) *consolev1.GitHubAuthStatus {
	out := &consolev1.GitHubAuthStatus{Connected: s.Connected}
	if s.Username != "" {
		out.Username = &s.Username
	}
	out.Scopes = s.Scopes
	return out
}

func loginURLToProto(u auth.LoginURL) *consolev1.OAuthLoginUrlResponse {
	return &consolev1.OAuthLoginUrlResponse{
		Provider:    u.Provider,
		AuthUrl:     u.AuthURL,
		State:       u.State,
		RedirectUri: u.RedirectURI,
	}
}

func callbackResultToProto(r auth.CallbackResult) *consolev1.OAuthCallbackResponse {
	out := &consolev1.OAuthCallbackResponse{Provider: r.Provider}
	if r.UserEmail != "" {
		out.UserEmail = &r.UserEmail
	}
	return out
}

// AuthStatusToProtoForTest exposes the status converter to the api test
// package, which lives outside internal/routes.
func AuthStatusToProtoForTest(s auth.AuthStatus) *consolev1.AuthStatusResponse {
	return authStatusToProto(s)
}
