package database

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

func SaveReasoningSubmission(ctx context.Context, sessionID uuid.UUID, rawInput string, modality string) error {
	query := `INSERT INTO simulation_reasoning_submissions (id, session_id, raw_input, input_modality) VALUES ($1, $2, $3, $4)`
	_, err := Pool.Exec(ctx, query, uuid.New(), sessionID, rawInput, modality)
	return err
}

func SaveSessionEvent(ctx context.Context, sessionID uuid.UUID, eventType string, data []byte, seq int) error {
	query := `INSERT INTO simulation_session_events (id, session_id, event_type, event_data, sequence_number) VALUES ($1, $2, $3, $4, $5)`
	_, err := Pool.Exec(ctx, query, uuid.New(), sessionID, eventType, data, seq)
	return err
}

type CaseData struct {
	Title            string
	PrimaryDiagnosis string
	ExpectedWorkup   []string
	AnamnesisItems   []string
}

func GetCaseBySessionID(ctx context.Context, sessionID uuid.UUID) (*CaseData, error) {
	// Query untuk mengambil data kasus berdasarkan relasi session ke kasus
	// Asumsi tabel kamu adalah: simulations (s), cases (c), illness_scripts (i)
	query := `
        SELECT c.title, i.primary_diagnosis, 
               c.osce_checklist->'anamnesis_items' as anamnesis,
               c.osce_checklist->'expected_workup' as workup
        FROM simulations s
        JOIN cases c ON s.case_id = c.id
        JOIN illness_scripts i ON i.case_id = c.id
        WHERE s.id = $1`

	var cd CaseData
	// Kita gunakan Pool untuk mengeksekusi query
	err := Pool.QueryRow(ctx, query, sessionID).Scan(
		&cd.Title,
		&cd.PrimaryDiagnosis,
		&cd.AnamnesisItems,
		&cd.ExpectedWorkup,
	)

	if err != nil {
		return nil, err
	}

	return &cd, nil
}
