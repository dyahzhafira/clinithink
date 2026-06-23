package handlers

import (
	"strconv"

	"clinithink/internal/response"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) SubmitSCT(c *fiber.Ctx) error {
	studentID, ok := c.Locals("user_id").(string)
	if !ok || studentID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}

	sessionID := c.Params("id")

	var body struct {
		Answers []struct {
			SCTItemID string `json:"sct_item_id"`
			Response  string `json:"response"`
		} `json:"answers"`
	}
	if err := c.BodyParser(&body); err != nil || len(body.Answers) == 0 {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "answers wajib diisi dan tidak boleh kosong")
	}

	validResponses := map[string]bool{"-2": true, "-1": true, "0": true, "+1": true, "+2": true}
	for _, a := range body.Answers {
		if !validResponses[a.Response] {
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "response harus salah satu dari: -2, -1, 0, +1, +2")
		}
	}

	// validate session from student, get case id
	var status, caseID string
	if err := h.db.QueryRow(c.Context(),
		`SELECT status, case_id FROM sessions WHERE id = $1 AND student_id = $2`,
		sessionID, studentID,
	).Scan(&status, &caseID); err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Sesi tidak ditemukan")
	}
	if status == "abandoned" {
		return response.Error(c, fiber.StatusConflict, "SESSION_CLOSED", "Sesi sudah ditutup")
	}

	//get newest submission_id this session
	var submissionID string
	if err := h.db.QueryRow(c.Context(),
		`SELECT id FROM reasoning_submissions WHERE session_id = $1 ORDER BY submitted_at DESC LIMIT 1`,
		sessionID,
	).Scan(&submissionID); err != nil {
		return response.Error(c, fiber.StatusConflict, "NO_SUBMISSION", "Submit reasoning terlebih dahulu sebelum mengisi SCT")
	}

	// check submitted sct
	var alreadyScored bool
	h.db.QueryRow(c.Context(),
		`SELECT EXISTS(SELECT 1 FROM sct_scores WHERE submission_id = $1)`,
		submissionID,
	).Scan(&alreadyScored)
	if alreadyScored {
		return response.Error(c, fiber.StatusConflict, "SCT_ALREADY_SUBMITTED", "SCT sudah disubmit untuk sesi ini")
	}

	type itemResult struct {
		SCTItemID           string  `json:"sct_item_id"`
		StudentResponse     string  `json:"student_response"`
		ExpertModalResponse string  `json:"expert_modal_response"`
		Score               float64 `json:"score"`
	}

	var totalScore float64
	results := []itemResult{}

	for _, answer := range body.Answers {
		// validate item with the same case
		var modalResponse string
		if err := h.db.QueryRow(c.Context(),
			`SELECT expert_panel_modal_response FROM sct_items WHERE id = $1 AND case_id = $2`,
			answer.SCTItemID, caseID,
		).Scan(&modalResponse); err != nil {
			return response.Error(c, fiber.StatusBadRequest, "INVALID_ITEM",
				"SCT item tidak ditemukan atau tidak termasuk kasus ini")
		}

		score := scoreSCT(answer.Response, modalResponse)
		totalScore += score

		if _, err := h.db.Exec(c.Context(), `
			INSERT INTO sct_scores (submission_id, sct_item_id, student_response, expert_modal_response, score_obtained)
			VALUES ($1, $2, $3, $4, $5)`,
			submissionID, answer.SCTItemID, answer.Response, modalResponse, score,
		); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}

		results = append(results, itemResult{
			SCTItemID:           answer.SCTItemID,
			StudentResponse:     answer.Response,
			ExpertModalResponse: modalResponse,
			Score:               score,
		})
	}

	normalizedScore := totalScore / float64(len(results))

	return response.OK(c, fiber.Map{
		"session_id":       sessionID,
		"submission_id":    submissionID,
		"items":            results,
		"total_score":      totalScore,
		"normalized_score": normalizedScore,
		"total_items":      len(results),
	})
}

func (h *Handler) GetSCTScores(c *fiber.Ctx) error {
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

	rows, err := h.db.Query(c.Context(), `
		SELECT ss.sct_item_id, ss.student_response, ss.expert_modal_response, ss.score_obtained
		FROM sct_scores ss
		JOIN reasoning_submissions rs ON rs.id = ss.submission_id
		WHERE rs.session_id = $1
		ORDER BY ss.created_at`, sessionID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}
	defer rows.Close()

	type itemResult struct {
		SCTItemID           string  `json:"sct_item_id"`
		StudentResponse     string  `json:"student_response"`
		ExpertModalResponse string  `json:"expert_modal_response"`
		Score               float64 `json:"score"`
	}

	var totalScore float64
	items := []itemResult{}
	for rows.Next() {
		var item itemResult
		if err := rows.Scan(&item.SCTItemID, &item.StudentResponse, &item.ExpertModalResponse, &item.Score); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}
		totalScore += item.Score
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	normalizedScore := 0.0
	if len(items) > 0 {
		normalizedScore = totalScore / float64(len(items))
	}

	return response.OK(c, fiber.Map{
		"session_id":       sessionID,
		"items":            items,
		"total_score":      totalScore,
		"normalized_score": normalizedScore,
		"total_items":      len(items),
	})
}

func (h *Handler) SubmitExpertResponse(c *fiber.Ctx) error {
	expertID, ok := c.Locals("user_id").(string)
	if !ok || expertID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
	}
	role, _ := c.Locals("user_role").(string)
	if role != "expert" {
		return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Hanya expert yang dapat mengisi respons panel")
	}

	sctItemID := c.Params("id")

	var body struct {
		Response  string `json:"response"`
		Rationale string `json:"rationale"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Body tidak valid")
	}

	validResponses := map[string]bool{"-2": true, "-1": true, "0": true, "+1": true, "+2": true}
	if !validResponses[body.Response] {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR",
			"response harus salah satu dari: -2, -1, 0, +1, +2")
	}

	// Verify item exists
	var exists bool
	h.db.QueryRow(c.Context(),
		`SELECT EXISTS(SELECT 1 FROM sct_items WHERE id = $1)`, sctItemID,
	).Scan(&exists)
	if !exists {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "SCT item tidak ditemukan")
	}

	// upsert, remove previous response from this expert, then insert
	h.db.Exec(c.Context(),
		`DELETE FROM sct_expert_responses WHERE sct_item_id = $1 AND expert_id = $2`,
		sctItemID, expertID,
	)
	if _, err := h.db.Exec(c.Context(),
		`INSERT INTO sct_expert_responses (sct_item_id, expert_id, response) VALUES ($1, $2, $3)`,
		sctItemID, expertID, body.Response,
	); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	// Recalculate modal (most frequent tie-break by lexicographic order for determinism)
	var modalResponse string
	h.db.QueryRow(c.Context(), `
		SELECT response FROM sct_expert_responses
		WHERE sct_item_id = $1
		GROUP BY response
		ORDER BY COUNT(*) DESC, response ASC
		LIMIT 1`, sctItemID,
	).Scan(&modalResponse)

	h.db.Exec(c.Context(),
		`UPDATE sct_items SET expert_panel_modal_response = $1 WHERE id = $2`,
		modalResponse, sctItemID,
	)

	if body.Rationale != "" {
		h.db.Exec(c.Context(),
			`UPDATE sct_items SET rationale = $1 WHERE id = $2`,
			body.Rationale, sctItemID,
		)
	}

	return response.OK(c, fiber.Map{
		"sct_item_id":    sctItemID,
		"response":       body.Response,
		"modal_response": modalResponse,
	})
}

//scale -2 to +2, distance 0 =1.0, each step reduces by 0.25, minimum 0
func scoreSCT(studentResp, modalResp string) float64 {
	toInt := func(s string) int {
		if len(s) > 0 && s[0] == '+' {
			s = s[1:]
		}
		v, _ := strconv.Atoi(s)
		return v
	}
	dist := toInt(studentResp) - toInt(modalResp)
	if dist < 0 {
		dist = -dist
	}
	score := 1.0 - float64(dist)*0.25
	if score < 0 {
		score = 0
	}
	return score
}
