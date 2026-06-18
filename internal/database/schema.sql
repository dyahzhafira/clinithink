CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS vector;

-- CASE BANK
CREATE TABLE systems (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    system_code VARCHAR(20) UNIQUE NOT NULL,  -- e.g. 'RESP', 'CARDIO'
    system_name VARCHAR(100) NOT NULL, -- e.g. 'Respirasi'
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE cases (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_id VARCHAR(20) UNIQUE NOT NULL,      -- e.g. 'RESP-001'
    system_id UUID REFERENCES systems(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    difficulty VARCHAR(20) CHECK (difficulty IN ('basic', 'intermediate', 'advanced')),
    station_duration_minutes INT DEFAULT 10,

    -- Patient presentation (JSONB untuk fleksibilitas)
    patient_presentation JSONB NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_by UUID,  -- FK ke tabel dosen/admin yang membuat
    validated_by UUID, -- FK ke dosen yang validasi (expert validation)
    validated_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_cases_system ON cases(system_id);
CREATE INDEX idx_cases_difficulty ON cases(difficulty);

-- ILLNESS SCRIPT (komponen inti: enabling conditions, fault, consequences)
CREATE TABLE illness_scripts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_id UUID REFERENCES cases(id) ON DELETE CASCADE,
    primary_diagnosis VARCHAR(255) NOT NULL,

    enabling_conditions TEXT[] NOT NULL,       -- array faktor risiko
    fault_pathophysiology TEXT NOT NULL,       -- penjelasan patofisiologi

    -- consequences sebagai JSONB karena ada substruktur (symptoms, signs, findings)
    consequences JSONB NOT NULL,

    -- Embedding untuk RAG similarity search
    embedding VECTOR(768),

    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_illness_scripts_case ON illness_scripts(case_id);
CREATE INDEX idx_illness_scripts_embedding ON illness_scripts
    USING hnsw (embedding vector_cosine_ops);

-- DIFFERENTIAL DIAGNOSES (untuk missing hypothesis detection)

CREATE TABLE differential_diagnoses (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_id UUID REFERENCES cases(id) ON DELETE CASCADE,
    diagnosis VARCHAR(255) NOT NULL,
    distinguishing_features TEXT NOT NULL,
    relevance_note TEXT,
    embedding VECTOR(768), -- untuk similarity matching terhadap input mahasiswa

    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_diff_dx_case ON differential_diagnoses(case_id);
CREATE INDEX idx_diff_dx_embedding ON differential_diagnoses
    USING hnsw (embedding vector_cosine_ops);

-- OSCE CHECKLIST
CREATE TABLE osce_checklist_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_id UUID REFERENCES cases(id) ON DELETE CASCADE,
    item_type VARCHAR(30) CHECK (item_type IN ('anamnesis', 'physical_exam', 'workup')),
    item_text TEXT NOT NULL,
    display_order INT DEFAULT 0,

    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_checklist_case ON osce_checklist_items(case_id);
CREATE INDEX idx_checklist_type ON osce_checklist_items(item_type);

-- SCT ITEMS (basis scoring AI)
CREATE TABLE sct_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    item_id VARCHAR(30) UNIQUE NOT NULL, -- e.g. 'RESP-001-SCT1'
    case_id UUID REFERENCES cases(id) ON DELETE CASCADE,

    scenario_addition TEXT NOT NULL, -- informasi baru yang diberikan
    hypothesis_tested VARCHAR(255) NOT NULL,

    -- Expert panel responses disimpan terpisah di tabel sct_expert_responses
    -- modal_response dihitung dari situ, tapi disimpan di sini sebagai cache
    expert_panel_modal_response VARCHAR(5), -- '-2', '-1', '0', '+1', '+2'
    rationale TEXT NOT NULL,

    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_sct_case ON sct_items(case_id);

-- Tabel jawaban tiap dosen di expert panel (sebelum diagregat jadi modal response)
CREATE TABLE sct_expert_responses (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sct_item_id UUID REFERENCES sct_items(id) ON DELETE CASCADE,
    expert_id UUID NOT NULL,  -- FK ke tabel dosen panel
    response VARCHAR(5) CHECK (response IN ('-2', '-1', '0', '+1', '+2')),
    submitted_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_sct_responses_item ON sct_expert_responses(sct_item_id);

-- BIAS TRIGGERS (metadata untuk kalibrasi sistem deteksi bias)
CREATE TABLE case_bias_metadata (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_id UUID REFERENCES cases(id) ON DELETE CASCADE,
    premature_closure_risk VARCHAR(20) CHECK (premature_closure_risk IN ('rendah', 'sedang', 'tinggi', 'sangat tinggi')),
    premature_closure_note TEXT,
    anchoring_risk VARCHAR(20) CHECK (anchoring_risk IN ('rendah', 'sedang', 'tinggi', 'sangat tinggi')),
    anchoring_note TEXT
);

-- STUDENT INTERACTION & EVENT LOG (untuk bias detection realtime)
CREATE TABLE students (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    institution VARCHAR(255),
    cohort_year INT,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    student_id UUID REFERENCES students(id) ON DELETE CASCADE,
    case_id UUID REFERENCES cases(id),
    started_at TIMESTAMPTZ DEFAULT now(),
    submitted_at TIMESTAMPTZ,
    status VARCHAR(20) DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'submitted', 'abandoned'))
);

CREATE INDEX idx_sessions_student ON sessions(student_id);
CREATE INDEX idx_sessions_case ON sessions(case_id);

-- Event log granular untuk sequence analysis (bias detection)
CREATE TABLE session_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,
    event_type VARCHAR(30) NOT NULL CHECK (event_type IN (
        'symptom_mentioned', 'hypothesis_proposed', 'question_asked',
        'differential_explored', 'hypothesis_committed', 'new_info_received',
        'hypothesis_revised'
    )),
    event_data JSONB, -- detail spesifik event (entitas yang disebut, dll)
    sequence_number INT NOT NULL, -- urutan event dalam sesi (utk analisis pola)
    timestamp TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_events_session ON session_events(session_id);
CREATE INDEX idx_events_sequence ON session_events(session_id, sequence_number);

-- REASONING SUBMISSION & PARSING RESULT

CREATE TABLE reasoning_submissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,

    raw_input TEXT NOT NULL, -- teks mahasiswa (free text)
    input_modality VARCHAR(10) CHECK (input_modality IN ('text', 'voice')),

    -- Hasil parsing NLP (entitas terstruktur)
    parsed_symptoms TEXT[],
    parsed_hypotheses TEXT[],
    parsed_reasoning TEXT,

    submitted_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_submissions_session ON reasoning_submissions(session_id);

-- ANALYSIS RESULTS (output dari Analysis Engine)

CREATE TABLE missing_hypotheses_detected (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    submission_id UUID REFERENCES reasoning_submissions(id) ON DELETE CASCADE,
    differential_diagnosis_id UUID REFERENCES differential_diagnoses(id),
    similarity_score FLOAT,  -- skor dari pgvector similarity search

    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE bias_detections (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,
    bias_type VARCHAR(30) CHECK (bias_type IN ('premature_closure', 'anchoring_bias')),
    detected_at_sequence INT,  -- sequence_number tempat bias terdeteksi
    confidence_note TEXT,

    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE sct_scores (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    submission_id UUID REFERENCES reasoning_submissions(id) ON DELETE CASCADE,
    sct_item_id UUID REFERENCES sct_items(id),
    student_response VARCHAR(5),
    expert_modal_response VARCHAR(5),
    score_obtained FLOAT,  -- partial credit berdasarkan concordance

    created_at TIMESTAMPTZ DEFAULT now()
);

-- DTI (Diagnostic Thinking Inventory) PRE/POST TEST

CREATE TABLE dti_responses (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    student_id UUID REFERENCES students(id) ON DELETE CASCADE,
    test_phase VARCHAR(10) CHECK (test_phase IN ('pre', 'post')),

    -- 41 item DTI, disimpan sebagai JSONB {"item_1": 4, "item_2": 5, ...}
    item_responses JSONB NOT NULL,

    flexibility_in_thinking_score FLOAT, -- hasil kalkulasi subscale FT
    structure_of_knowledge_score FLOAT, -- hasil kalkulasi subscale SK

    submitted_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_dti_student ON dti_responses(student_id);