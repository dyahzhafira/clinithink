package handlers

import (
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

	rows, err := h.db.Query(c.Context(), `
		SELECT s.id, s.status, s.started_at, s.submitted_at,
		       c.case_id, c.title, c.difficulty
		FROM sessions s
		JOIN cases c ON c.id = s.case_id
		WHERE s.student_id = $1
		ORDER BY s.started_at DESC`, studentID)
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

	sessions := []sessionItem{}
	for rows.Next() {
		var item sessionItem
		if err := rows.Scan(
			&item.ID, &item.Status, &item.StartedAt, &item.SubmittedAt,
			&item.CaseID, &item.CaseTitle, &item.Difficulty,
		); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}
		sessions = append(sessions, item)
	}

	return response.OK(c, sessions)
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
		SELECT s.id, s.status, s.started_at, s.submitted_at,
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

// BiasCheck — Sprint 1 Task #10
func (h *Handler) BiasCheck(c *fiber.Ctx) error {
	return response.NotImplemented(c)
}

// SubmitAnalysis — skeleton 501 permanen, full implementation Sprint 3
func (h *Handler) SubmitAnalysis(c *fiber.Ctx) error {
	return response.NotImplemented(c)
}

// GetAnalysis — skeleton 501 permanen, full implementation Sprint 3
func (h *Handler) GetAnalysis(c *fiber.Ctx) error {
	return response.NotImplemented(c)
}

// SubmitExpertResponse — skeleton 501 permanen, full implementation Sprint 4
func (h *Handler) SubmitExpertResponse(c *fiber.Ctx) error {
	return response.NotImplemented(c)
}
