package handlers

import (
	"clinithink/internal/response"

	"github.com/gofiber/fiber/v2"
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
	h.db.QueryRow(c.Context(),
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'submitted')
		 FROM sessions WHERE student_id = $1`,
		studentID,
	).Scan(&total, &submitted)

	var biasCount int
	h.db.QueryRow(c.Context(),
		`SELECT COUNT(*) FROM bias_detections bd
		 JOIN sessions s ON s.id = bd.session_id
		 WHERE s.student_id = $1`,
		studentID,
	).Scan(&biasCount)

	var topBias *string
	h.db.QueryRow(c.Context(),
		`SELECT bd.bias_type FROM bias_detections bd
		 JOIN sessions s ON s.id = bd.session_id
		 WHERE s.student_id = $1
		 GROUP BY bd.bias_type ORDER BY COUNT(*) DESC LIMIT 1`,
		studentID,
	).Scan(&topBias)

	return response.OK(c, fiber.Map{
		"total_sessions":     total,
		"submitted_sessions": submitted,
		"bias_detections":    biasCount,
		"top_bias_type":      topBias,
	})
}
