package routes

import (
	"time"

	"clinithink/internal/config"
	"clinithink/internal/handlers"
	"clinithink/internal/middleware"
	"clinithink/internal/response"
	ws "clinithink/internal/ws"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	gws "github.com/gofiber/websocket/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func Setup(app *fiber.App, cfg *config.Config, db *pgxpool.Pool, rdb *redis.Client, hub *ws.Hub) {
	h := handlers.New(cfg, db, rdb, hub)
	authMW := middleware.JWT(cfg.JWTSecret)

	api := app.Group("/api")

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"status": "ok"}})
	})

	authMiddlewares := []fiber.Handler{}
	if cfg.AppEnv != "test" {
		authMiddlewares = append(authMiddlewares, limiter.New(limiter.Config{
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
	}
	auth := api.Group("/auth", authMiddlewares...)
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
	p.Post("/sessions/:id/events", h.LogEvent)
	p.Post("/sessions/:id/sct", h.SubmitSCT)
	p.Get("/sessions/:id/sct", h.GetSCTScores)
	p.Post("/sessions/:id/analysis", h.SubmitAnalysis)
	p.Get("/sessions/:id/analysis", h.GetAnalysis)
	p.Post("/sct-items/:id/expert-response", h.SubmitExpertResponse)

	p.Get("/students/me", h.GetMe)
	p.Get("/students/me/summary", h.GetSummary)

	// WebSocket — auth via ?token= query param (separate from JWT middleware group)
	app.Use("/ws/sessions", h.WebSocketAuth)
	app.Get("/ws/sessions/:id", gws.New(h.HandleSession))
}
