// Chrome import routes: expose the local Chrome data pass-through.
package routes

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	chromeimport "github.com/Adelodunpeter25/console/apps/server-go/internal/services/chrome_import"
)

// registerChromeImportRoutes wires the two import endpoints. Both are
// registered after the device-token middleware, like the session routes:
// no public surface here.
func registerChromeImportRoutes(app *fiber.App) {
	// GET /api/chrome/profiles — the profiles with data on this machine.
	app.Get("/api/chrome/profiles", func(c *fiber.Ctx) error {
		profiles := chromeimport.DetectProfiles()
		return c.JSON(fiber.Map{"success": true, "data": profiles})
	})

	// POST /api/chrome/import — {profileId, wantHistory, wantCookies}.
	app.Post("/api/chrome/import", func(c *fiber.Ctx) error {
		var body struct {
			ProfileID   string `json:"profileId"`
			WantHistory bool   `json:"wantHistory"`
			WantCookies bool   `json:"wantCookies"`
		}
		if err := c.BodyParser(&body); err != nil {
			return chromeImportError(c, fiber.StatusBadRequest, "invalid request body")
		}
		if body.ProfileID == "" {
			return chromeImportError(c, fiber.StatusBadRequest, "profileId is required")
		}

		resp := fiber.Map{"history": []chromeimport.ChromeHistoryEntry{}, "cookies": []chromeimport.ChromeCookie{}}
		if body.WantHistory {
			history, err := chromeimport.ReadHistory(body.ProfileID, 1000)
			if err != nil {
				return chromeImportError(c, fiber.StatusInternalServerError, err.Error())
			}
			resp["history"] = history
		}
		if body.WantCookies {
			cookies, err := chromeimport.ReadCookies(body.ProfileID)
			if err != nil {
				return chromeImportStatusCode(c, err)
			}
			resp["cookies"] = cookies
		}
		return c.JSON(fiber.Map{"success": true, "data": resp})
	})
}

// chromeImportStatusCode maps keychain failures to their real status: the
// secret could not be read, so the request is refused, not failed mid-way.
func chromeImportStatusCode(c *fiber.Ctx, err error) error {
	if strings.Contains(err.Error(), chromeimport.ErrNoKeychain.Error()) || errors.Is(err, chromeimport.ErrNoKeychain) {
		return chromeImportError(c, fiber.StatusConflict, err.Error())
	}
	return chromeImportError(c, fiber.StatusInternalServerError, err.Error())
}

func chromeImportError(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(fiber.Map{"success": false, "error": message})
}
