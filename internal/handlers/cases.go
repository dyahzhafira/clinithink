package handlers

import (
	"fmt"

	"clinithink/internal/response"

	"github.com/gofiber/fiber/v2"

	"clinithink/internal/grpc"
)

func (h *Handler) ListCases(c *fiber.Ctx) error {
	system := c.Query("system")
	difficulty := c.Query("difficulty")

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	query := `
		SELECT c.id, c.case_id, c.title, c.difficulty, c.station_duration_minutes,
		       s.system_code, s.system_name,
		       COUNT(*) OVER() AS total_count
		FROM cases c
		JOIN systems s ON s.id = c.system_id
		WHERE c.is_active = true`

	args := []interface{}{}
	argIdx := 1

	if system != "" {
		query += fmt.Sprintf(" AND s.system_code = $%d", argIdx)
		args = append(args, system)
		argIdx++
	}
	if difficulty != "" {
		query += fmt.Sprintf(" AND c.difficulty = $%d", argIdx)
		args = append(args, difficulty)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY s.system_code, c.case_id LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := h.db.Query(c.Context(), query, args...)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}
	defer rows.Close()

	type caseItem struct {
		ID                     string `json:"id"`
		CaseID                 string `json:"case_id"`
		Title                  string `json:"title"`
		Difficulty             string `json:"difficulty"`
		StationDurationMinutes int    `json:"station_duration_minutes"`
		SystemCode             string `json:"system_code"`
		SystemName             string `json:"system_name"`
	}

	var total int64
	cases := []caseItem{}
	for rows.Next() {
		var item caseItem
		if err := rows.Scan(
			&item.ID, &item.CaseID, &item.Title, &item.Difficulty,
			&item.StationDurationMinutes, &item.SystemCode, &item.SystemName,
			&total,
		); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}
		cases = append(cases, item)
	}
	if err := rows.Err(); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    cases,
		"meta":    fiber.Map{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) GetCase(c *fiber.Ctx) error {
	caseID := c.Params("id")

	type illnessScript struct {
		PrimaryDiagnosis     string    `json:"primary_diagnosis"`
		EnablingConditions   []string  `json:"enabling_conditions"`
		FaultPathophysiology string    `json:"fault_pathophysiology"`
		Consequences         fiber.Map `json:"consequences"`
	}

	type caseDetail struct {
		ID                     string        `json:"id"`
		CaseID                 string        `json:"case_id"`
		Title                  string        `json:"title"`
		Difficulty             string        `json:"difficulty"`
		StationDurationMinutes int           `json:"station_duration_minutes"`
		PatientPresentation    fiber.Map     `json:"patient_presentation"`
		SystemCode             string        `json:"system_code"`
		SystemName             string        `json:"system_name"`
		IllnessScript          illnessScript `json:"illness_script"`
		DifferentialDiagnoses  []fiber.Map   `json:"differential_diagnoses"`
		OSCEChecklist          fiber.Map     `json:"osce_checklist"`
		SCTItems               []fiber.Map   `json:"sct_items"`
	}

	var detail caseDetail
	var presentationRaw, consequencesRaw []byte

	err := h.db.QueryRow(c.Context(), `
		SELECT c.id, c.case_id, c.title, c.difficulty, c.station_duration_minutes,
		       c.patient_presentation, s.system_code, s.system_name,
		       i.primary_diagnosis, i.enabling_conditions, i.fault_pathophysiology, i.consequences
		FROM cases c
		JOIN systems s ON s.id = c.system_id
		JOIN illness_scripts i ON i.case_id = c.id
		WHERE c.id = $1 AND c.is_active = true`, caseID,
	).Scan(
		&detail.ID, &detail.CaseID, &detail.Title, &detail.Difficulty,
		&detail.StationDurationMinutes, &presentationRaw,
		&detail.SystemCode, &detail.SystemName,
		&detail.IllnessScript.PrimaryDiagnosis, &detail.IllnessScript.EnablingConditions,
		&detail.IllnessScript.FaultPathophysiology, &consequencesRaw,
	)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Kasus tidak ditemukan")
	}

	if err := parseJSON(presentationRaw, &detail.PatientPresentation); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}
	if err := parseJSON(consequencesRaw, &detail.IllnessScript.Consequences); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	// Differential diagnoses
	ddRows, err := h.db.Query(c.Context(), `
		SELECT diagnosis, distinguishing_features, relevance_note
		FROM differential_diagnoses WHERE case_id = $1`, detail.ID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}
	defer ddRows.Close()

	detail.DifferentialDiagnoses = []fiber.Map{}
	for ddRows.Next() {
		var diagnosis, features, note string
		if err := ddRows.Scan(&diagnosis, &features, &note); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}
		detail.DifferentialDiagnoses = append(detail.DifferentialDiagnoses, fiber.Map{
			"diagnosis":               diagnosis,
			"distinguishing_features": features,
			"relevance_note":          note,
		})
	}
	if err := ddRows.Err(); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	// OSCE checklist
	checkRows, err := h.db.Query(c.Context(), `
		SELECT item_type, item_text FROM osce_checklist_items
		WHERE case_id = $1 ORDER BY item_type, display_order`, detail.ID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}
	defer checkRows.Close()

	anamnesis, physicalExam, workup := []string{}, []string{}, []string{}
	for checkRows.Next() {
		var itemType, itemText string
		if err := checkRows.Scan(&itemType, &itemText); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}
		switch itemType {
		case "anamnesis":
			anamnesis = append(anamnesis, itemText)
		case "physical_exam":
			physicalExam = append(physicalExam, itemText)
		case "workup":
			workup = append(workup, itemText)
		}
	}
	if err := checkRows.Err(); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}
	detail.OSCEChecklist = fiber.Map{
		"anamnesis_items":     anamnesis,
		"physical_exam_items": physicalExam,
		"expected_workup":     workup,
	}

	// expert panel modal response excluded
	sctRows, err := h.db.Query(c.Context(), `
		SELECT id, item_id, scenario_addition, hypothesis_tested, rationale
		FROM sct_items WHERE case_id = $1 ORDER BY item_id`, detail.ID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}
	defer sctRows.Close()

	detail.SCTItems = []fiber.Map{}
	for sctRows.Next() {
		var id, itemID, scenario, hypothesis, rationale string
		if err := sctRows.Scan(&id, &itemID, &scenario, &hypothesis, &rationale); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
		}
		detail.SCTItems = append(detail.SCTItems, fiber.Map{
			"id":                id,
			"item_id":           itemID,
			"scenario_addition": scenario,
			"hypothesis_tested": hypothesis,
			"rationale":         rationale,
		})
	}
	if err := sctRows.Err(); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
	}

	anamnesisItems := detail.OSCEChecklist["anamnesis_items"].([]string)
	workupItems := detail.OSCEChecklist["expected_workup"].([]string)
	go grpc.SendAnalysisTrigger(
		caseID,
		"CASE_LOADED",
		detail.Title,
		detail.IllnessScript.PrimaryDiagnosis,
		workupItems,
		anamnesisItems,
	)

	return response.OK(c, detail)
}
