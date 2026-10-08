// Desktop assistant support routes (/api/assist/*): slash-command
// autocomplete (init + discovered skills) and @-mention file search scoped
// to the session cwd.
package routes

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const (
	initCommandName        = "init"
	initCommandDescription = "Generate a console.toml with project run scripts for the Run tab"
	// computerUseCommandName is the chat command that activates computer use:
	// listed so the composer suggests it, while the model itself learns
	// nothing until the user actually invokes it (see SplitInvocation).
	computerUseCommandName        = "computer-use"
	computerUseCommandDescription = "Drive the computer: read app windows and operate them with keyboard and mouse"
)

func registerAssistRoutes(app *fiber.App, sessions *services.SessionService, fs *services.FsService, skills *services.SkillsService) {
	handleCommands := func(c *fiber.Ctx) error {
		cwd := resolveSessionCwd(c, sessions)
		commands := []*consolev1.SlashCommandInfo{
			{Name: initCommandName, Description: initCommandDescription, Builtin: true},
			{Name: computerUseCommandName, Description: computerUseCommandDescription, Builtin: true},
		}
		for _, skill := range skills.Discover(cwd) {
			commands = append(commands, &consolev1.SlashCommandInfo{
				Name: skill.Name, Description: skill.Description, Builtin: false,
			})
		}
		data, err := marshalProtoList(commands)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
	}

	handleSearch := func(c *fiber.Ctx) error {
		query := strings.TrimSpace(c.Query("q"))
		if query == "" {
			query = strings.TrimSpace(c.Query("query"))
		}
		root := c.Query("root")
		if root == "" {
			root = resolveSessionCwd(c, sessions)
		}
		includeDirs := c.Query("includeDirs") != "false" && c.Query("includeDirs") != "0"
		searchQuery := query
		if searchQuery == "" {
			// Empty query (user just typed "@") falls back to a broad scan.
			searchQuery = "."
		}
		items, err := fs.SearchFiles(root, searchQuery, 50, includeDirs)
		if err != nil {
			return fail400(c, err)
		}
		// Ensure we always return a valid response even if no items found
		if items == nil {
			items = []types.FileSearchResult{}
		}
		// query echoes the *raw* query, not the "." an empty one searched
		// with — clients show it back to the user verbatim.
		protoItems := make([]*consolev1.FileSearchResult, 0, len(items))
		for _, it := range items {
			protoItems = append(protoItems, &consolev1.FileSearchResult{
				RelativePath: it.RelativePath, AbsolutePath: it.AbsolutePath,
				IsDir: it.IsDir, Score: it.Score,
			})
		}
		raw, err := protoMarshal.Marshal(&consolev1.AssistFileSearchResponse{
			Root:  root,
			Query: query,
			Items: protoItems,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	}

	app.Get("/api/assist/:sessionId/commands", handleCommands)
	app.Get("/api/assist/commands", handleCommands)
	app.Get("/api/assist/:sessionId/search", handleSearch)
	app.Get("/api/assist/search", handleSearch)
	app.Get("/api/assist/files", handleSearch)
}

// RegisterAssistRoutes exposes the assist routes for tests, following the
// same pattern as RegisterMCPRoutes.
func RegisterAssistRoutes(app *fiber.App, sessions *services.SessionService, fs *services.FsService, skills *services.SkillsService) {
	registerAssistRoutes(app, sessions, fs, skills)
}

// resolveSessionCwd returns the cwd of the session named by the sessionId
// param or query, else the server cwd.
func resolveSessionCwd(c *fiber.Ctx, sessions *services.SessionService) string {
	sessionID := c.Params("sessionId")
	if sessionID == "" {
		sessionID = c.Query("sessionId")
	}
	if sessionID != "" {
		if loaded, err := sessions.Load(sessionID, 0, 0); err == nil && loaded != nil && loaded.Header.Cwd != "" {
			if _, err := os.Stat(loaded.Header.Cwd); err == nil {
				return loaded.Header.Cwd
			}
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
