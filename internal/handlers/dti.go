package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"

	"clinithink/internal/response"

	"github.com/gofiber/fiber/v2"
)

// FT: 2,3,4,5,6,11,15,16,23,24,26,27,28,30,32,34,35,36,38,40,41 (21 items)
// SK: 1,7,8,9,10,12,13,14,17,18,19,20,21,22,25,29,31,33,37,39 (20 items)
var dtiSubscale = buildDTISubscaleMap()

func buildDTISubscaleMap() map[int]string {
	m := make(map[int]string, 41)
	for _, n := range []int{2, 3, 4, 5, 6, 11, 15, 16, 23, 24, 26, 27, 28, 30, 32, 34, 35, 36, 38, 40, 41} {
		m[n] = "FT"
	}
	for _, n := range []int{1, 7, 8, 9, 10, 12, 13, 14, 17, 18, 19, 20, 21, 22, 25, 29, 31, 33, 37, 39} {
		m[n] = "SK"
	}
	return m
}

func (h *Handler) SubmitDTI(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	var body struct {
		TestPhase string         `json:"test_phase"`
		Responses map[string]int `json:"responses"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Body tidak valid")
	}
	if body.TestPhase != "pre" && body.TestPhase != "post" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "test_phase harus 'pre' atau 'post'")
	}
	if len(body.Responses) != 41 {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR",
			fmt.Sprintf("Semua 41 item harus dijawab, diterima %d item", len(body.Responses)))
	}
	for i := 1; i <= 41; i++ {
		key := strconv.Itoa(i)
		score, exists := body.Responses[key]
		if !exists {
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR",
				fmt.Sprintf("Item %d tidak ada dalam responses", i))
		}
		if score < 1 || score > 6 {
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR",
				fmt.Sprintf("Skor item %d harus antara 1-6, diterima %d", i, score))
		}
	}

	var alreadySubmitted bool
	h.db.QueryRow(c.Context(),
		`SELECT EXISTS(SELECT 1 FROM dti_responses WHERE student_id = $1 AND test_phase = $2)`,
		studentID, body.TestPhase,
	).Scan(&alreadySubmitted)
	if alreadySubmitted {
		return response.Error(c, fiber.StatusConflict, "DTI_ALREADY_SUBMITTED",
			fmt.Sprintf("DTI fase '%s' sudah pernah disubmit", body.TestPhase))
	}
	
	var ftScore, skScore float64
	for key, score := range body.Responses {
		itemNo, _ := strconv.Atoi(key)
		if dtiSubscale[itemNo] == "FT" {
			ftScore += float64(score)
		} else {
			skScore += float64(score)
		}
	}

	responsesJSON, _ := json.Marshal(body.Responses)

	var id string
	if err := h.db.QueryRow(c.Context(), `
		INSERT INTO dti_responses
			(student_id, test_phase, item_responses, flexibility_in_thinking_score, structure_of_knowledge_score)
		VALUES ($1, $2, $3::jsonb, $4, $5)
		RETURNING id`,
		studentID, body.TestPhase, string(responsesJSON), ftScore, skScore,
	).Scan(&id); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	return response.OK(c, fiber.Map{
		"id":                            id,
		"test_phase":                    body.TestPhase,
		"flexibility_in_thinking_score": ftScore,
		"structure_of_knowledge_score":  skScore,
		"score_note":                    "Skor sementara (raw sum). Reverse-coding diterapkan setelah verifikasi arah skala per item.",
	})
}

func (h *Handler) GetDTI(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	rows, err := h.db.Query(c.Context(), `
		SELECT id, test_phase, item_responses,
		       flexibility_in_thinking_score, structure_of_knowledge_score,
		       submitted_at::text
		FROM dti_responses
		WHERE student_id = $1
		ORDER BY submitted_at`, studentID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}
	defer rows.Close()

	type dtiResult struct {
		ID          string          `json:"id"`
		TestPhase   string          `json:"test_phase"`
		Responses   json.RawMessage `json:"responses"`
		FTScore     float64         `json:"flexibility_in_thinking_score"`
		SKScore     float64         `json:"structure_of_knowledge_score"`
		SubmittedAt string          `json:"submitted_at"`
	}

	results := []dtiResult{}
	for rows.Next() {
		var r dtiResult
		var respRaw []byte
		if err := rows.Scan(&r.ID, &r.TestPhase, &respRaw, &r.FTScore, &r.SKScore, &r.SubmittedAt); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}
		r.Responses = json.RawMessage(respRaw)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	return response.OK(c, results)
}
