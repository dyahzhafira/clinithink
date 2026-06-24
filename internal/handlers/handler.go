package handlers

import (
	"encoding/json"

	"clinithink/internal/config"
	ws "clinithink/internal/ws"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	cfg    *config.Config
	db     *pgxpool.Pool
	redis  *redis.Client
	hub    *ws.Hub
	ttsKey string
}

func New(cfg *config.Config, db *pgxpool.Pool, redis *redis.Client, hub *ws.Hub) *Handler {
	return &Handler{cfg: cfg, db: db, redis: redis, hub: hub, ttsKey: cfg.GCPTTSKey}
}

func parseJSON(raw []byte, dest interface{}) error {
	return json.Unmarshal(raw, dest)
}
