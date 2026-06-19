package handlers

import (
	"encoding/json"

	"clinithink/internal/bias"
	"clinithink/internal/response"

	"github.com/gofiber/fiber/v2"
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
	err := h.db.QueryRow(c.Context(),
		`SELECT status FROM sessions WHERE id = $1 AND student_id = $2`,
		sessionID, studentID,
	).Scan(&status)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Sesi tidak ditemukan")
	}
	if status != "in_progress" {
		return response.Error(c, fiber.StatusConflict, "SESSION_CLOSED", "Sesi sudah disubmit atau ditutup")
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

	results := bias.Detect(events)
	for _, r := range results {
		h.db.Exec(c.Context(), `
			INSERT INTO bias_detections (session_id, bias_type, detected_at_sequence, confidence_note)
			VALUES ($1, $2, $3, $4)`,
			sessionID, r.BiasType, r.DetectedAtSequence, r.ConfidenceNote)
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
		"symptom_mentioned": true, "hypothesis_proposed": true, "question_asked": true,
		"differential_explored": true, "hypothesis_committed": true,
		"new_info_received": true, "hypothesis_revised": true,
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
	h.db.QueryRow(c.Context(),
		`SELECT COALESCE(MAX(sequence_number), 0) + 1 FROM session_events WHERE session_id = $1`,
		sessionID,
	).Scan(&seqNum)

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

// SubmitAnalysis
func (h *Handler) SubmitAnalysis(c *fiber.Ctx) error {
	return response.NotImplemented(c)
}

// GetAnalysis
func (h *Handler) GetAnalysis(c *fiber.Ctx) error {
	return response.NotImplemented(c)
}

// SubmitExpertResponse
func (h *Handler) SubmitExpertResponse(c *fiber.Ctx) error {
	return response.NotImplemented(c)
}
