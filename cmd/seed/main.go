package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type CaseBank struct {
	Systems []SystemEntry `json:"systems"`
}

type SystemEntry struct {
	System     string `json:"system"`
	SystemCode string `json:"system_code"`
	Cases      []Case `json:"cases"`
}

type Case struct {
	CaseID                 string                 `json:"case_id"`
	Title                  string                 `json:"title"`
	Difficulty             string                 `json:"difficulty"`
	StationDurationMinutes int                    `json:"station_duration_minutes"`
	PatientPresentation    map[string]interface{} `json:"patient_presentation"`
	IllnessScript          IllnessScript          `json:"illness_script"`
	DifferentialDiagnoses  []DifferentialDiagnosis `json:"differential_diagnoses"`
	OSCEChecklist          OSCEChecklist          `json:"osce_checklist"`
	SCTItems               []SCTItem              `json:"sct_items"`
	CommonBiasTriggers     CommonBiasTriggers     `json:"common_bias_triggers"`
}

type IllnessScript struct {
	PrimaryDiagnosis     string                 `json:"primary_diagnosis"`
	EnablingConditions   []string               `json:"enabling_conditions"`
	FaultPathophysiology string                 `json:"fault_pathophysiology"`
	Consequences         map[string]interface{} `json:"consequences"`
}

type DifferentialDiagnosis struct {
	Diagnosis              string `json:"diagnosis"`
	DistinguishingFeatures string `json:"distinguishing_features"`
	RelevanceNote          string `json:"relevance_note"`
}

type OSCEChecklist struct {
	AnamnesisItems    []string `json:"anamnesis_items"`
	PhysicalExamItems []string `json:"physical_exam_items"`
	ExpectedWorkup    []string `json:"expected_workup"`
}

type SCTItem struct {
	ItemID                   string `json:"item_id"`
	ScenarioAddition         string `json:"scenario_addition"`
	HypothesisTested         string `json:"hypothesis_tested"`
	ExpertPanelModalResponse string `json:"expert_panel_modal_response"`
	Rationale                string `json:"rationale"`
}

type CommonBiasTriggers struct {
	PrematureClosureRisk string `json:"premature_closure_risk"`
	AnchoringRisk        string `json:"anchoring_risk"`
}

func main() {
	_ = godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer pool.Close()

	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM cases").Scan(&count); err != nil {
		log.Fatalf("failed to check existing data: %v", err)
	}
	if count > 0 {
		log.Fatalf("cases table already has %d rows — drop and recreate schema first to re-seed", count)
	}

	data, err := os.ReadFile("casebank.json")
	if err != nil {
		log.Fatalf("failed to read casebank.json: %v", err)
	}

	var cb CaseBank
	if err := json.Unmarshal(data, &cb); err != nil {
		log.Fatalf("failed to parse casebank.json: %v", err)
	}

	total := 0
	for _, sys := range cb.Systems {
		var systemID string
		if err := pool.QueryRow(ctx,
			`INSERT INTO systems (system_code, system_name) VALUES ($1, $2) RETURNING id`,
			sys.SystemCode, sys.System,
		).Scan(&systemID); err != nil {
			log.Fatalf("failed to insert system %s: %v", sys.SystemCode, err)
		}

		for _, c := range sys.Cases {
			if err := importCase(ctx, pool, systemID, c); err != nil {
				log.Fatalf("failed to import case %s: %v", c.CaseID, err)
			}
			log.Printf("imported: %s — %s", c.CaseID, c.Title)
			total++
		}
	}

	log.Printf("done: %d cases imported", total)
}

func importCase(ctx context.Context, pool *pgxpool.Pool, systemID string, c Case) error {
	presentationJSON, _ := json.Marshal(c.PatientPresentation)

	var caseID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cases (case_id, system_id, title, difficulty, station_duration_minutes, patient_presentation)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		c.CaseID, systemID, c.Title, c.Difficulty, c.StationDurationMinutes, presentationJSON,
	).Scan(&caseID); err != nil {
		return fmt.Errorf("cases: %w", err)
	}

	consequencesJSON, _ := json.Marshal(c.IllnessScript.Consequences)
	if _, err := pool.Exec(ctx, `
		INSERT INTO illness_scripts (case_id, primary_diagnosis, enabling_conditions, fault_pathophysiology, consequences)
		VALUES ($1, $2, $3, $4, $5)`,
		caseID, c.IllnessScript.PrimaryDiagnosis,
		c.IllnessScript.EnablingConditions,
		c.IllnessScript.FaultPathophysiology,
		consequencesJSON,
	); err != nil {
		return fmt.Errorf("illness_scripts: %w", err)
	}

	for _, dd := range c.DifferentialDiagnoses {
		if _, err := pool.Exec(ctx, `
			INSERT INTO differential_diagnoses (case_id, diagnosis, distinguishing_features, relevance_note)
			VALUES ($1, $2, $3, $4)`,
			caseID, dd.Diagnosis, dd.DistinguishingFeatures, dd.RelevanceNote,
		); err != nil {
			return fmt.Errorf("differential_diagnoses: %w", err)
		}
	}

	insertChecklist := func(items []string, itemType string) error {
		for i, item := range items {
			if _, err := pool.Exec(ctx, `
				INSERT INTO osce_checklist_items (case_id, item_type, item_text, display_order)
				VALUES ($1, $2, $3, $4)`,
				caseID, itemType, item, i,
			); err != nil {
				return err
			}
		}
		return nil
	}
	if err := insertChecklist(c.OSCEChecklist.AnamnesisItems, "anamnesis"); err != nil {
		return fmt.Errorf("checklist anamnesis: %w", err)
	}
	if err := insertChecklist(c.OSCEChecklist.PhysicalExamItems, "physical_exam"); err != nil {
		return fmt.Errorf("checklist physical_exam: %w", err)
	}
	if err := insertChecklist(c.OSCEChecklist.ExpectedWorkup, "workup"); err != nil {
		return fmt.Errorf("checklist workup: %w", err)
	}

	for _, sct := range c.SCTItems {
		if _, err := pool.Exec(ctx, `
			INSERT INTO sct_items (item_id, case_id, scenario_addition, hypothesis_tested, expert_panel_modal_response, rationale)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			sct.ItemID, caseID, sct.ScenarioAddition, sct.HypothesisTested,
			sct.ExpertPanelModalResponse, sct.Rationale,
		); err != nil {
			return fmt.Errorf("sct_items %s: %w", sct.ItemID, err)
		}
	}

	pcRisk, pcNote := parseBiasRisk(c.CommonBiasTriggers.PrematureClosureRisk)
	anchorRisk, anchorNote := parseBiasRisk(c.CommonBiasTriggers.AnchoringRisk)
	if _, err := pool.Exec(ctx, `
		INSERT INTO case_bias_metadata (case_id, premature_closure_risk, premature_closure_note, anchoring_risk, anchoring_note)
		VALUES ($1, $2, $3, $4, $5)`,
		caseID, pcRisk, pcNote, anchorRisk, anchorNote,
	); err != nil {
		return fmt.Errorf("case_bias_metadata: %w", err)
	}

	return nil
}

//parse bias risk splits
func parseBiasRisk(s string) (risk, note string) {
	parts := strings.SplitN(s, " - ", 2)
	risk = strings.ToLower(strings.TrimSpace(parts[0]))
	if len(parts) == 2 {
		note = strings.TrimSpace(parts[1])
	}
	return
}
