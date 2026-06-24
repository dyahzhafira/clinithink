package response

import "github.com/gofiber/fiber/v2"

func OK(c *fiber.Ctx, data interface{}) error {
	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

func Error(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    code,
			"message": message,
		},
	})
}

func NotImplemented(c *fiber.Ctx) error {
	return Error(c, fiber.StatusNotImplemented, "NOT_IMPLEMENTED",
		"Endpoint belum diimplementasi")
}
