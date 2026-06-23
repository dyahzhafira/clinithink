package handlers

import (
	"errors"

	"clinithink/internal/response"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) GetMe(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	type profile struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Email       string  `json:"email"`
		Institution *string `json:"institution"`
		CohortYear  *int    `json:"cohort_year"`
		CreatedAt   string  `json:"created_at"`
	}

	var p profile
	err := h.db.QueryRow(c.Context(),
		`SELECT id, name, email, institution, cohort_year, created_at::text FROM students WHERE id = $1`,
		studentID,
	).Scan(&p.ID, &p.Name, &p.Email, &p.Institution, &p.CohortYear, &p.CreatedAt)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Profil tidak ditemukan")
	}

	return response.OK(c, p)
}

func (h *Handler) GetSummary(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	var total, submitted int
	if err := h.db.QueryRow(c.Context(),
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'submitted')
		 FROM sessions WHERE student_id = $1`,
		studentID,
	).Scan(&total, &submitted); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	var biasCount int
	if err := h.db.QueryRow(c.Context(),
		`SELECT COUNT(*) FROM bias_detections bd
		 JOIN sessions s ON s.id = bd.session_id
		 WHERE s.student_id = $1`,
		studentID,
	).Scan(&biasCount); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	var topBias *string
	if err := h.db.QueryRow(c.Context(),
		`SELECT bd.bias_type FROM bias_detections bd
		 JOIN sessions s ON s.id = bd.session_id
		 WHERE s.student_id = $1
		 GROUP BY bd.bias_type ORDER BY COUNT(*) DESC LIMIT 1`,
		studentID,
	).Scan(&topBias); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	return response.OK(c, fiber.Map{
		"total_sessions":     total,
		"submitted_sessions": submitted,
		"bias_detections":    biasCount,
		"top_bias_type":      topBias,
	})
}

func (h *Handler) UpdateMe(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	var body struct {
		Name        *string `json:"name"`
		Institution *string `json:"institution"`
		CohortYear  *int    `json:"cohort_year"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Body tidak valid")
	}

	if body.Name != nil && len(*body.Name) < 2 {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Nama minimal 2 karakter")
	}
	if body.CohortYear != nil && (*body.CohortYear < 2000 || *body.CohortYear > 2100) {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Tahun angkatan tidak valid")
	}

	_, err := h.db.Exec(c.Context(), `
		UPDATE students
		SET
			name        = COALESCE($2, name),
			institution = COALESCE($3, institution),
			cohort_year = COALESCE($4, cohort_year)
		WHERE id = $1`,
		studentID, body.Name, body.Institution, body.CohortYear,
	)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	type profile struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Email       string  `json:"email"`
		Institution *string `json:"institution"`
		CohortYear  *int    `json:"cohort_year"`
		CreatedAt   string  `json:"created_at"`
	}
	var p profile
	h.db.QueryRow(c.Context(),
		`SELECT id, name, email, institution, cohort_year, created_at::text FROM students WHERE id = $1`,
		studentID,
	).Scan(&p.ID, &p.Name, &p.Email, &p.Institution, &p.CohortYear, &p.CreatedAt)

	return response.OK(c, p)
}
