package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"clinithink/internal/bias"
	"clinithink/internal/config"
	"clinithink/internal/grpc"
	ws "clinithink/internal/ws"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	livekit_proto "github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	cfg        *config.Config
	db         *pgxpool.Pool
	redis      *redis.Client
	hub        *ws.Hub
	ttsKey     string
	roomClient *lksdk.RoomServiceClient
}

func New(cfg *config.Config, db *pgxpool.Pool, redis *redis.Client, hub *ws.Hub) *Handler {
	roomClient := lksdk.NewRoomServiceClient(
		os.Getenv("LIVEKIT_HOST"),
		os.Getenv("LIVEKIT_API_KEY"),
		os.Getenv("LIVEKIT_API_SECRET"),
	)
	return &Handler{
		cfg:        cfg,
		db:         db,
		redis:      redis,
		hub:        hub,
		ttsKey:     cfg.GCPTTSKey,
		roomClient: roomClient,
	}
}

func parseJSON(raw []byte, dest interface{}) error {
	return json.Unmarshal(raw, dest)
}

func (h *Handler) runBiasDetection(sessionID string) {
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

	results := bias.Detect(events)

	for _, r := range results {
		h.db.Exec(context.Background(), `
            INSERT INTO bias_detections (session_id, bias_type, detected_at_sequence, confidence_note)
            VALUES ($1, $2, $3, $4)
            ON CONFLICT DO NOTHING`,
			sessionID, r.BiasType, r.DetectedAtSequence, r.ConfidenceNote)
	}
}

// Chat menangani permintaan chat dari frontend, mengirimkan ke AI via gRPC,
// lalu meneruskan respons kembali ke room LiveKit via SendData.
func (h *Handler) Chat(c *fiber.Ctx) error {
	type Request struct {
		Message string `json:"message"`
	}
	req := new(Request)
	if err := c.BodyParser(req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if req.Message == "" {
		return c.Status(400).JSON(fiber.Map{"error": "message wajib diisi"})
	}

	sessionID := c.Params("id")
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	sID, err := uuid.Parse(sessionID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid session ID"})
	}

	// 1. Validasi sesi masih berjalan dan miliki student yang benar
	var status string
	err = h.db.QueryRow(c.Context(),
		`SELECT status FROM sessions WHERE id = $1 AND student_id = $2`,
		sessionID, studentID,
	).Scan(&status)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Sesi tidak ditemukan"})
	}
	if status != "in_progress" {
		return c.Status(409).JSON(fiber.Map{"error": "Sesi sudah selesai"})
	}

	// 2. Ambil data kasus dari DB melalui relasi session → cases → illness_scripts + osce_checklist
	type caseContext struct {
		Title            string
		PrimaryDiagnosis string
		AnamnesisItems   []string
		ExpectedWorkup   []string
	}

	var cc caseContext
	err = h.db.QueryRow(c.Context(), `
		SELECT c.title, i.primary_diagnosis
		FROM sessions s
		JOIN cases c ON c.id = s.case_id
		JOIN illness_scripts i ON i.case_id = c.id
		WHERE s.id = $1`, sessionID,
	).Scan(&cc.Title, &cc.PrimaryDiagnosis)
	if err != nil {
		log.Printf("[Chat] Gagal ambil data kasus untuk sesi %s: %v", sessionID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Gagal memuat konteks kasus"})
	}

	// Ambil anamnesis & workup dari osce_checklist_items
	checkRows, err := h.db.Query(c.Context(), `
		SELECT item_type, item_text
		FROM osce_checklist_items oci
		JOIN cases c ON c.id = oci.case_id
		JOIN sessions s ON s.case_id = c.id
		WHERE s.id = $1 ORDER BY oci.item_type, oci.display_order`, sessionID)
	if err == nil {
		defer checkRows.Close()
		for checkRows.Next() {
			var itemType, itemText string
			if checkRows.Scan(&itemType, &itemText) == nil {
				switch itemType {
				case "anamnesis":
					cc.AnamnesisItems = append(cc.AnamnesisItems, itemText)
				case "workup":
					cc.ExpectedWorkup = append(cc.ExpectedWorkup, itemText)
				}
			}
		}
	}

	// 3. Simpan input user ke reasoning_submissions
	_, err = h.db.Exec(c.Context(),
		`INSERT INTO reasoning_submissions (session_id, raw_input, input_modality) VALUES ($1, $2, $3)`,
		sID, req.Message, "text")
	if err != nil {
		log.Printf("[Chat] Gagal simpan reasoning submission: %v", err)
		// Tidak fatal, lanjut
	}

	// 4. Panggil AI Agent (Python) via gRPC
	log.Printf("[Chat] Mengirim ke AI gRPC — session=%s title=%q turns_anamnesis=%d", sessionID, cc.Title, len(cc.AnamnesisItems))
	aiRes, err := grpc.SendAnalysisTrigger(
		sessionID,
		req.Message,
		cc.Title,
		cc.PrimaryDiagnosis,
		cc.ExpectedWorkup,
		cc.AnamnesisItems,
	)
	if err != nil {
		log.Printf("[Chat] AI gRPC error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "AI service unavailable"})
	}

	log.Printf("[Chat] Respon AI diterima: %q", aiRes.Status)

	// 5. Kirim respons teks AI ke room LiveKit via SendData
	chatPayload, _ := json.Marshal(map[string]interface{}{
		"type":    "chat_message",
		"role":    "ai",
		"content": aiRes.Status,
	})
	_, sendErr := h.roomClient.SendData(c.Context(), &livekit_proto.SendDataRequest{
		Room: sessionID,
		Data: chatPayload,
		Kind: livekit_proto.DataPacket_RELIABLE,
	})
	if sendErr != nil {
		log.Printf("[Chat] Gagal kirim chat_message ke LiveKit room %s: %v", sessionID, sendErr)
	}

	// 6. Log AI response ke session_events untuk replay whiteboard
	var seqNum int
	h.db.QueryRow(c.Context(),
		`SELECT COALESCE(MAX(sequence_number), 0) + 1 FROM session_events WHERE session_id = $1`,
		sessionID,
	).Scan(&seqNum)

	eventData, _ := json.Marshal(map[string]interface{}{
		"type":    "chat_message",
		"role":    "ai",
		"content": aiRes.Status,
	})
	_, logErr := h.db.Exec(c.Context(),
		`INSERT INTO session_events (session_id, event_type, event_data, sequence_number)
		 VALUES ($1, $2, $3::jsonb, $4)`,
		sessionID, "ai_response", string(eventData), seqNum)
	if logErr != nil {
		log.Printf("[Chat] Gagal log ai_response ke session_events: %v", logErr)
	}

	// 7. Jalankan bias detection di background
	go h.runBiasDetection(sessionID)

	return c.JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"answer": aiRes.Status},
	})
}

// sendWhiteboardAction mengirim whiteboard action ke room LiveKit dan mencatatnya ke session_events.
func (h *Handler) sendWhiteboardAction(ctx context.Context, sessionID string, action interface{}, seqNum int) {
	payload, err := json.Marshal(map[string]interface{}{
		"type": "whiteboard_action",
		"data": action,
	})
	if err != nil {
		log.Printf("[sendWhiteboardAction] Marshal error: %v", err)
		return
	}

	_, err = h.roomClient.SendData(ctx, &livekit_proto.SendDataRequest{
		Room: sessionID,
		Data: payload,
		Kind: livekit_proto.DataPacket_RELIABLE,
	})
	if err != nil {
		log.Printf("[sendWhiteboardAction] LiveKit SendData error untuk room %s: %v", sessionID, err)
		return
	}

	// Log ke session_events untuk replay
	actionData, _ := json.Marshal(action)
	_, dbErr := h.db.Exec(ctx,
		`INSERT INTO session_events (session_id, event_type, event_data, sequence_number)
		 VALUES ($1, $2, $3::jsonb, $4)`,
		sessionID, "ai_action", string(actionData), seqNum)
	if dbErr != nil {
		log.Printf("[sendWhiteboardAction] Gagal log ai_action: %v", dbErr)
	}

	fmt.Printf("[sendWhiteboardAction] Action dikirim ke room %s (seq %d)\n", sessionID, seqNum)
}
