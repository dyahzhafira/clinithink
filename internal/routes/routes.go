package routes

import (
	"clinithink/internal/config"
	"clinithink/internal/handlers"
	"clinithink/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func Setup(app *fiber.App, cfg *config.Config) {
	h := handlers.New(cfg)
	authMW := middleware.JWT(cfg.JWTSecret)

	api := app.Group("/api")

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"status": "ok"}})
	})

	auth := api.Group("/auth")
	auth.Post("/register", h.Register)
	auth.Post("/login", h.Login)

	p := api.Group("", authMW)

	p.Get("/cases", h.ListCases)
	p.Get("/cases/:id", h.GetCase)

	p.Post("/sessions", h.CreateSession)
	p.Get("/sessions", h.ListSessions)
	p.Get("/sessions/:id", h.GetSession)
	p.Post("/sessions/:id/submit", h.SubmitReasoning)
	p.Get("/sessions/:id/bias-check", h.BiasCheck)

	p.Post("/sessions/:id/analysis", h.SubmitAnalysis)
	p.Get("/sessions/:id/analysis", h.GetAnalysis)
	p.Post("/sct-items/:id/expert-response", h.SubmitExpertResponse)
}
