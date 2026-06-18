package routes

import (
	"time"

	"clinithink/internal/config"
	"clinithink/internal/handlers"
	"clinithink/internal/middleware"
	"clinithink/internal/response"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func Setup(app *fiber.App, cfg *config.Config, db *pgxpool.Pool, rdb *redis.Client) {
	h := handlers.New(cfg, db, rdb)
	authMW := middleware.JWT(cfg.JWTSecret)

	api := app.Group("/api")

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"status": "ok"}})
	})

	auth := api.Group("/auth", limiter.New(limiter.Config{
		Max:        10,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return response.Error(c, fiber.StatusTooManyRequests, "RATE_LIMITED",
				"Terlalu banyak percobaan, coba lagi dalam 1 menit")
		},
	}))
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
