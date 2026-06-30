package handlers

import (
	"encoding/json"
	"time"

	"clinithink/internal/bias"
	"clinithink/internal/response"
	ws "clinithink/internal/ws"

	"github.com/gofiber/fiber/v2"

	"fmt"
	"os"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

func (h *Handler) CreateSession(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	var body struct {
		CaseID string `json:"case_id"`
	}
	if err := c.BodyParser(&body); err != nil || body.CaseID == "" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "case_id wajib diisi")
	}

	var sessionID string
	err := h.db.QueryRow(c.Context(), `
		INSERT INTO sessions (student_id, case_id)
		VALUES ($1, $2) RETURNING id`,
		studentID, body.CaseID,
	).Scan(&sessionID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	var caseCode string
	err = h.db.QueryRow(c.Context(), `
		SELECT case_id FROM cases WHERE id = $1`,
		body.CaseID,
	).Scan(&caseCode)

	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Kasus tidak ditemukan")
	}

	metadata := map[string]string{
		"case_id": caseCode, // Contoh: "NEURO-001"
	}

	metadataBytes, _ := json.Marshal(metadata)

	// 2. Gunakan lksdk.NewRoomServiceClient
	roomClient := lksdk.NewRoomServiceClient(
		os.Getenv("LIVEKIT_HOST"),
		os.Getenv("LIVEKIT_API_KEY"),
		os.Getenv("LIVEKIT_API_SECRET"),
	)

	// 3. Panggil CreateRoom
	_, err = roomClient.CreateRoom(c.Context(), &livekit.CreateRoomRequest{
		Name:     sessionID,
		Metadata: string(metadataBytes),
	})

	if err != nil {
		// TAMBAHKAN LOG INI
		fmt.Printf("ERROR LIVEKIT: %v\n", err)
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Gagal membuat ruang")
	}

	return response.OK(c, fiber.Map{"id": sessionID, "status": "in_progress"})
}

func (h *Handler) ListSessions(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	rows, err := h.db.Query(c.Context(), `
		SELECT s.id, s.status, s.started_at::text, s.submitted_at::text,
		       c.case_id, c.title, c.difficulty,
		       COUNT(*) OVER() AS total_count
		FROM sessions s
		JOIN cases c ON c.id = s.case_id
		WHERE s.student_id = $1
		ORDER BY s.started_at DESC
		LIMIT $2 OFFSET $3`, studentID, limit, offset)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}
	defer rows.Close()

	type sessionItem struct {
		ID          string  `json:"id"`
		Status      string  `json:"status"`
		StartedAt   string  `json:"started_at"`
		SubmittedAt *string `json:"submitted_at"`
		CaseID      string  `json:"case_id"`
		CaseTitle   string  `json:"case_title"`
		Difficulty  string  `json:"difficulty"`
	}

	var total int64
	sessions := []sessionItem{}
	for rows.Next() {
		var item sessionItem
		if err := rows.Scan(
			&item.ID, &item.Status, &item.StartedAt, &item.SubmittedAt,
			&item.CaseID, &item.CaseTitle, &item.Difficulty,
			&total,
		); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}
		sessions = append(sessions, item)
	}
	if err := rows.Err(); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    sessions,
		"meta":    fiber.Map{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) GetSession(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	sessionID := c.Params("id")

	type sessionDetail struct {
		ID          string  `json:"id"`
		Status      string  `json:"status"`
		StartedAt   string  `json:"started_at"`
		SubmittedAt *string `json:"submitted_at"`
		CaseID      string  `json:"case_id"`
		CaseTitle   string  `json:"case_title"`
	}

	var detail sessionDetail
	err := h.db.QueryRow(c.Context(), `
		SELECT s.id, s.status, s.started_at::text, s.submitted_at::text,
		       c.case_id, c.title
		FROM sessions s
		JOIN cases c ON c.id = s.case_id
		WHERE s.id = $1 AND s.student_id = $2`,
		sessionID, studentID,
	).Scan(&detail.ID, &detail.Status, &detail.StartedAt, &detail.SubmittedAt,
		&detail.CaseID, &detail.CaseTitle)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Sesi tidak ditemukan")
	}

	return response.OK(c, detail)
}

func (h *Handler) SubmitReasoning(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	sessionID := c.Params("id")

	var body struct {
		RawInput      string `json:"raw_input"`
		InputModality string `json:"input_modality"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Format request tidak valid")
	}
	if body.RawInput == "" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "raw_input wajib diisi")
	}
	if body.InputModality != "text" && body.InputModality != "voice" {
		body.InputModality = "text"
	}

	var status string
	var startedAt time.Time
	var durationMin int
	err := h.db.QueryRow(c.Context(), `
		SELECT s.status, s.started_at, c.station_duration_minutes
		FROM sessions s JOIN cases c ON c.id = s.case_id
		WHERE s.id = $1 AND s.student_id = $2`,
		sessionID, studentID,
	).Scan(&status, &startedAt, &durationMin)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Sesi tidak ditemukan")
	}
	if status != "in_progress" {
		return response.Error(c, fiber.StatusConflict, "SESSION_CLOSED", "Sesi sudah disubmit atau ditutup")
	}
	if time.Since(startedAt) > time.Duration(durationMin)*time.Minute {
		h.db.Exec(c.Context(),
			`UPDATE sessions SET status = 'abandoned', submitted_at = now() WHERE id = $1 AND status = 'in_progress'`,
			sessionID,
		)
		h.hub.StopTimer(sessionID)
		return response.Error(c, fiber.StatusConflict, "SESSION_EXPIRED", "Waktu sesi sudah habis")
	}

	var submissionID string
	err = h.db.QueryRow(c.Context(), `
		INSERT INTO reasoning_submissions (session_id, raw_input, input_modality)
		VALUES ($1, $2, $3) RETURNING id`,
		sessionID, body.RawInput, body.InputModality,
	).Scan(&submissionID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	_, err = h.db.Exec(c.Context(), `
		UPDATE sessions SET status = 'submitted', submitted_at = now()
		WHERE id = $1`, sessionID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	h.hub.StopTimer(sessionID)
	h.hub.Send(sessionID, ws.Event{
		Type:    "session_ended",
		Payload: map[string]string{"reason": "submitted"},
	})

	return response.OK(c, fiber.Map{
		"submission_id": submissionID,
		"session_id":    sessionID,
		"status":        "submitted",
	})
}

func (h *Handler) BiasCheck(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	sessionID := c.Params("id")

	var exists bool
	h.db.QueryRow(c.Context(),
		`SELECT EXISTS(SELECT 1 FROM sessions WHERE id = $1 AND student_id = $2)`,
		sessionID, studentID,
	).Scan(&exists)
	if !exists {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Sesi tidak ditemukan")
	}

	// Return cached detections if already computed for this session
	cachedRows, err := h.db.Query(c.Context(),
		`SELECT bias_type, detected_at_sequence, confidence_note
		 FROM bias_detections WHERE session_id = $1 ORDER BY detected_at_sequence`, sessionID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}
	defer cachedRows.Close()

	cached := []bias.DetectionResult{}
	for cachedRows.Next() {
		var r bias.DetectionResult
		if err := cachedRows.Scan(&r.BiasType, &r.DetectedAtSequence, &r.ConfidenceNote); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}
		cached = append(cached, r)
	}
	cachedRows.Close()

	if len(cached) > 0 {
		return response.OK(c, fiber.Map{
			"session_id": sessionID,
			"detections": cached,
			"cached":     true,
		})
	}

	rows, err := h.db.Query(c.Context(),
		`SELECT event_type, sequence_number FROM session_events
		 WHERE session_id = $1 ORDER BY sequence_number`, sessionID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}
	defer rows.Close()

	var events []bias.Event
	for rows.Next() {
		var e bias.Event
		if err := rows.Scan(&e.EventType, &e.SequenceNumber); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	results := bias.Detect(events)
	for _, r := range results {
		if _, err := h.db.Exec(c.Context(), `
			INSERT INTO bias_detections (session_id, bias_type, detected_at_sequence, confidence_note)
			VALUES ($1, $2, $3, $4)`,
			sessionID, r.BiasType, r.DetectedAtSequence, r.ConfidenceNote); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}
	}

	if results == nil {
		results = []bias.DetectionResult{}
	}

	return response.OK(c, fiber.Map{
		"session_id":  sessionID,
		"event_count": len(events),
		"detections":  results,
	})
}

func (h *Handler) LogEvent(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	sessionID := c.Params("id")

	var body struct {
		EventType string          `json:"event_type"`
		EventData json.RawMessage `json:"event_data"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Format request tidak valid")
	}

	validTypes := map[string]bool{
		// Event yang dikirim oleh frontend (aktivitas mahasiswa)
		"symptom_mentioned":     true,
		"hypothesis_proposed":   true,
		"question_asked":        true,
		"differential_explored": true,
		"hypothesis_committed":  true,
		"new_info_received":     true,
		"hypothesis_revised":    true,
		// Event yang dikirim oleh backend (respons AI, untuk replay whiteboard)
		"ai_action":             true,
		"ai_response":           true,
	}
	if !validTypes[body.EventType] {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "event_type tidak valid")
	}

	var status string
	if err := h.db.QueryRow(c.Context(),
		`SELECT status FROM sessions WHERE id = $1 AND student_id = $2`,
		sessionID, studentID,
	).Scan(&status); err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Sesi tidak ditemukan")
	}
	if status != "in_progress" {
		return response.Error(c, fiber.StatusConflict, "SESSION_CLOSED", "Sesi sudah disubmit atau ditutup")
	}

	var seqNum int
	if err := h.db.QueryRow(c.Context(),
		`SELECT COALESCE(MAX(sequence_number), 0) + 1 FROM session_events WHERE session_id = $1`,
		sessionID,
	).Scan(&seqNum); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	var eventData *string
	if len(body.EventData) > 0 && string(body.EventData) != "null" {
		s := string(body.EventData)
		eventData = &s
	}

	var eventID string
	if err := h.db.QueryRow(c.Context(), `
		INSERT INTO session_events (session_id, event_type, event_data, sequence_number)
		VALUES ($1, $2, $3::jsonb, $4) RETURNING id`,
		sessionID, body.EventType, eventData, seqNum,
	).Scan(&eventID); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	return response.OK(c, fiber.Map{
		"id":              eventID,
		"session_id":      sessionID,
		"event_type":      body.EventType,
		"sequence_number": seqNum,
	})
}

// GetEvents mengambil riwayat event sesi untuk keperluan replay whiteboard.
// Hanya student pemilik sesi yang dapat mengakses.
func (h *Handler) GetEvents(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	sessionID := c.Params("id")

	// Validasi kepemilikan sesi (Bug 12 fix)
	var exists bool
	if err := h.db.QueryRow(c.Context(),
		`SELECT EXISTS(SELECT 1 FROM sessions WHERE id = $1 AND student_id = $2)`,
		sessionID, studentID,
	).Scan(&exists); err != nil || !exists {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Sesi tidak ditemukan")
	}

	rows, err := h.db.Query(c.Context(),
		`SELECT id, event_type, event_data, sequence_number 
         FROM session_events WHERE session_id = $1 ORDER BY sequence_number ASC`,
		sessionID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil data")
	}
	defer rows.Close()

	type eventItem struct {
		ID             string          `json:"id"`
		EventType      string          `json:"event_type"`
		EventData      json.RawMessage `json:"event_data"`
		SequenceNumber int             `json:"sequence_number"`
	}

	events := []eventItem{}
	for rows.Next() {
		var e eventItem
		if err := rows.Scan(&e.ID, &e.EventType, &e.EventData, &e.SequenceNumber); err == nil {
			events = append(events, e)
		}
	}

	return response.OK(c, fiber.Map{"events": events})
}

// SubmitAnalysis
func (h *Handler) SubmitAnalysis(c *fiber.Ctx) error {
	return response.NotImplemented(c)
}

// GetAnalysis returns aggregated session results: reasoning text, SCT scores, bias detections.
func (h *Handler) GetAnalysis(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	sessionID := c.Params("id")

	var status string
	if err := h.db.QueryRow(c.Context(),
		`SELECT status FROM sessions WHERE id = $1 AND student_id = $2`,
		sessionID, studentID,
	).Scan(&status); err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Sesi tidak ditemukan")
	}
	if status != "submitted" {
		return response.Error(c, fiber.StatusConflict, "SESSION_NOT_SUBMITTED", "Analisis hanya tersedia untuk sesi yang sudah selesai")
	}

	// Reasoning text
	var reasoningRaw string
	h.db.QueryRow(c.Context(),
		`SELECT raw_input FROM reasoning_submissions WHERE session_id = $1 ORDER BY submitted_at DESC LIMIT 1`,
		sessionID,
	).Scan(&reasoningRaw)

	// SCT per-item scores
	type sctItem struct {
		SCTItemID           string  `json:"sct_item_id"`
		StudentResponse     string  `json:"student_response"`
		ExpertModalResponse string  `json:"expert_modal_response"`
		Score               float64 `json:"score"`
	}
	sctItems := []sctItem{}
	var totalScore float64

	sctRows, err := h.db.Query(c.Context(), `
		SELECT ss.sct_item_id, ss.student_response, ss.expert_modal_response, ss.score_obtained
		FROM sct_scores ss
		JOIN reasoning_submissions rs ON rs.id = ss.submission_id
		WHERE rs.session_id = $1
		ORDER BY ss.created_at`, sessionID)
	if err == nil {
		for sctRows.Next() {
			var item sctItem
			if err := sctRows.Scan(&item.SCTItemID, &item.StudentResponse, &item.ExpertModalResponse, &item.Score); err == nil {
				totalScore += item.Score
				sctItems = append(sctItems, item)
			}
		}
		sctRows.Close()
	}

	var sctNormalized *float64
	if len(sctItems) > 0 {
		n := totalScore / float64(len(sctItems))
		sctNormalized = &n
	}

	// Bias detections from cache
	type biasDetection struct {
		BiasType           string `json:"bias_type"`
		DetectedAtSequence int    `json:"detected_at_sequence"`
		ConfidenceNote     string `json:"confidence_note"`
	}
	biasDetections := []biasDetection{}

	biasRows, err := h.db.Query(c.Context(),
		`SELECT bias_type, detected_at_sequence, confidence_note
		 FROM bias_detections WHERE session_id = $1 ORDER BY detected_at_sequence`, sessionID)
	if err == nil {
		for biasRows.Next() {
			var b biasDetection
			if err := biasRows.Scan(&b.BiasType, &b.DetectedAtSequence, &b.ConfidenceNote); err == nil {
				biasDetections = append(biasDetections, b)
			}
		}
		biasRows.Close()
	}

	var topBias *string
	if len(biasDetections) > 0 {
		h.db.QueryRow(c.Context(),
			`SELECT bias_type FROM bias_detections WHERE session_id = $1
			 GROUP BY bias_type ORDER BY COUNT(*) DESC LIMIT 1`, sessionID,
		).Scan(&topBias)
	}

	return response.OK(c, fiber.Map{
		"session_id":           sessionID,
		"reasoning_raw":        reasoningRaw,
		"sct_normalized_score": sctNormalized,
		"sct_total_items":      len(sctItems),
		"sct_items":            sctItems,
		"bias_count":           len(biasDetections),
		"top_bias_type":        topBias,
		"bias_detections":      biasDetections,
	})
}
