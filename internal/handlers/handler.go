package handlers

import (
	"context"
	"encoding/json"

	"clinithink/internal/bias"

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

func (h *Handler) runBiasDetection(sessionID string) {
	// ambil data event dari database
	rows, err := h.db.Query(context.Background(),
		"SELECT event_type, sequence_number FROM session_events WHERE session_id = $1 ORDER BY sequence_number",
		sessionID)
	if err != nil {
		return
	}
	defer rows.Close()

	var events []bias.Event
	for rows.Next() {
		var e bias.Event
		rows.Scan(&e.EventType, &e.SequenceNumber)
		events = append(events, e)
	}

	// deteksi bias
	results := bias.Detect(events)

	// simpan hasil di database python bagian ai untuk pembuatan node
	for _, r := range results {
		h.db.Exec(context.Background(), `
            INSERT INTO bias_detections (session_id, bias_type, detected_at_sequence, confidence_note)
            VALUES ($1, $2, $3, $4)
            ON CONFLICT DO NOTHING`,
			sessionID, r.BiasType, r.DetectedAtSequence, r.ConfidenceNote)
	}
}
