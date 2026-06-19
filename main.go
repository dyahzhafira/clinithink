package main

import (
	"errors"
	"log"

	"clinithink/internal/config"
	"clinithink/internal/database"
	"clinithink/internal/routes"
	ws "clinithink/internal/ws"

	"github.com/gofiber/fiber/v2"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	app := fiber.New(fiber.Config{
		ErrorHandler: errorHandler,
	})

	db, err := database.NewPostgres(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	defer db.Close()

	rdb, err := database.NewRedis(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis error: %v", err)
	}
	defer rdb.Close()

	hub := ws.NewHub()
	routes.Setup(app, cfg, db, rdb, hub)

	log.Printf("server starting on :%s", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    "INTERNAL_ERROR",
			"message": "Terjadi kesalahan pada server",
		},
	})
}