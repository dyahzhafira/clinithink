package handlers

import (
	"encoding/json"

	"clinithink/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	cfg   *config.Config
	db    *pgxpool.Pool
	redis *redis.Client
}

func New(cfg *config.Config, db *pgxpool.Pool, redis *redis.Client) *Handler {
	return &Handler{cfg: cfg, db: db, redis: redis}
}

func parseJSON(raw []byte, dest interface{}) error {
	return json.Unmarshal(raw, dest)
}
