package handlers

import (
	"context"
	"encoding/json"
	"time"

	ws "clinithink/internal/ws"

	"github.com/gofiber/fiber/v2"
	gws "github.com/gofiber/websocket/v2"
	"github.com/golang-jwt/jwt/v5"
)

func (h *Handler) WebSocketAuth(c *fiber.Ctx) error {
	if !gws.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}
	token := c.Query("token")
	if token == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "token wajib disertakan sebagai query param ?token=")
	}
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fiber.ErrUnauthorized
		}
		return []byte(h.cfg.JWTSecret), nil
	})
	if err != nil || !parsed.Valid {
		return fiber.NewError(fiber.StatusUnauthorized, "token tidak valid atau sudah kadaluarsa")
	}
	userID, _ := claims["sub"].(string)
	c.Locals("user_id", userID)
	return c.Next()
}

// HandleSession menangani koneksi WebSocket untuk sesi simulasi.
// Bertanggung jawab untuk memulai timer dan meneruskan event ke frontend via LiveKit.
func (h *Handler) HandleSession(c *gws.Conn) {
	sessionID := c.Params("id")
	studentID, _ := c.Locals("user_id").(string)

	var startedAt time.Time
	var durationMin int
	var status string
	err := h.db.QueryRow(context.Background(), `
		SELECT s.started_at, s.status, c.station_duration_minutes
		FROM sessions s
		JOIN cases c ON c.id = s.case_id
		WHERE s.id = $1 AND s.student_id = $2`,
		sessionID, studentID,
	).Scan(&startedAt, &status, &durationMin)
	if err != nil || status != "in_progress" {
		b, _ := json.Marshal(ws.Event{Type: "error", Payload: map[string]string{"message": "sesi tidak ditemukan atau sudah selesai"}})
		c.WriteMessage(gws.TextMessage, b)
		return
	}

	remaining := time.Duration(durationMin)*time.Minute - time.Since(startedAt)
	if remaining <= 0 {
		h.db.Exec(context.Background(),
			`UPDATE sessions SET status = 'abandoned', submitted_at = now() WHERE id = $1 AND status = 'in_progress'`,
			sessionID,
		)
		b, _ := json.Marshal(ws.Event{Type: "session_ended", Payload: map[string]string{"reason": "timeout"}})
		c.WriteMessage(gws.TextMessage, b)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	h.hub.Register(sessionID, c, cancel)
	defer h.hub.Unregister(sessionID)

	// StartTimer sekarang menggunakan h.roomClient (LiveKit) untuk mengirim timer_tick
	go ws.StartTimer(ctx, sessionID, int(remaining.Seconds()), h.roomClient)

	// Read loop — tetap terbuka selama koneksi aktif
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			break
		}
	}
}
