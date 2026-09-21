// Shared helper functions for route handlers.
package routes

import (
	"github.com/gofiber/fiber/v2"
)

func fail400(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": err.Error()})
}
