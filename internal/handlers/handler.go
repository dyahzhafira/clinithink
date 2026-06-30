package handlers

import (
	"context"
	"encoding/json"

	"clinithink/internal/bias"

	"clinithink/internal/config"
	ws "clinithink/internal/ws"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"clinithink/internal/grpc"

	"github.com/google/uuid"
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

func (h *Handler) Chat(c *fiber.Ctx) error {
	type Request struct {
		SessionID string `json:"sessionId"`
		Message   string `json:"message"`
	}
	req := new(Request)
	if err := c.BodyParser(req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	sID, err := uuid.Parse(req.SessionID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid Session ID"})
	}

	// 1. Simpan input user ke database (tabel simulation_reasoning_submissions)
	_, err = h.db.Exec(c.Context(),
		"INSERT INTO simulation_reasoning_submissions (id, session_id, raw_input, input_modality) VALUES ($1, $2, $3, $4)",
		uuid.New(), sID, req.Message, "voice")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save submission"})
	}

	// 2. Panggil AI Agent (Python) via gRPC
	aiRes, err := grpc.SendAnalysisTrigger(req.SessionID, req.Message, "", "", []string{}, []string{})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "AI service unavailable"})
	}

	// 3. Simpan respon AI ke database (tabel simulation_session_events)
	eventData, _ := json.Marshal(map[string]string{"answer": aiRes.Status})
	_, err = h.db.Exec(c.Context(),
		"INSERT INTO simulation_session_events (id, session_id, event_type, event_data, sequence_number) VALUES ($1, $2, $3, $4, $5)",
		uuid.New(), sID, "ai_response", eventData, 1)

	// 4. Kirim respon ke Frontend
	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"answer": aiRes.Status,
		},
	})
}
