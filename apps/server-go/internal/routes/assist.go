// Desktop assistant support routes (/api/assist/*). Port of
// apps/server/api/src/routes/assist.ts: slash-command autocomplete (init +
// discovered skills) and fff-backed @-mention file search scoped to the
// session cwd.
package routes

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const (
	initCommandName        = "init"
	initCommandDescription = "Generate a console.toml with project run scripts for the Run tab"
)

func registerAssistRoutes(app *fiber.App, sessions *services.SessionService, fs *services.FsService, skills *services.SkillsService) {
	handleCommands := func(c *fiber.Ctx) error {
		cwd := resolveSessionCwd(c, sessions)
		commands := []types.SlashCommandInfo{
			{Name: initCommandName, Description: initCommandDescription, Builtin: true},
		}
		for _, skill := range skills.Discover(cwd) {
			commands = append(commands, types.SlashCommandInfo{
				Name: skill.Name, Description: skill.Description, Builtin: false,
			})
		}
		return c.JSON(fiber.Map{"success": true, "data": commands})
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
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
			"root": root, "query": query, "items": items,
		}})
	}

	app.Get("/api/assist/:sessionId/commands", handleCommands)
	app.Get("/api/assist/commands", handleCommands)
	app.Get("/api/assist/:sessionId/search", handleSearch)
	app.Get("/api/assist/search", handleSearch)
	app.Get("/api/assist/files", handleSearch)
}

// resolveSessionCwd mirrors the TS handler: explicit session id param or
// query, else the server cwd.
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
