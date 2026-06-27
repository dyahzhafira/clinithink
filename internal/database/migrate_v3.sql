-- ============================================================
-- MIGRATION v3 — Generative Case System
-- Strategi: PURELY ADDITIVE. Tidak ada DROP, ALTER, atau RENAME
-- terhadap tabel existing (cases, illness_scripts,
-- differential_diagnoses, sessions).
--
-- Jalankan ke DB yang sudah ada schema v1/v2:
--   psql $DATABASE_URL -f internal/database/migrate_v3.sql
-- ============================================================

-- ------------------------------------------------------------
-- 1. SCT RUBRIC ITEMS
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS  sct_rubric_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id UUID NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
  finding_category VARCHAR(100) NOT NULL,
  hypothesis_template VARCHAR(255) NOT NULL,
  expert_panel_modal_response SMALLINT NOT NULL CHECK (expert_panel_modal_response BETWEEN -2 AND 2),
  scoring_rationale TEXT,
  created_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE(case_id, finding_category)
);

COMMENT ON TABLE sct_rubric_items IS 'Rubrik SCT per case, dipakai untuk scoring sesi dengan skenario yang di-generate AI. Menggantikan sct_items lama untuk flow generative.';

-- ------------------------------------------------------------
-- 2. BIAS RUBRIC ITEMS
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS bias_rubric_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id UUID NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
  bias_type VARCHAR(30) NOT NULL CHECK (bias_type IN ('premature_closure', 'anchoring_bias')),
  required_finding_category VARCHAR(100) NOT NULL,
  trigger_pattern TEXT,
  risk_level VARCHAR(20) NOT NULL CHECK (risk_level IN ('rendah', 'sedang', 'tinggi')),
  created_at TIMESTAMPTZ DEFAULT now()
);

COMMENT ON TABLE bias_rubric_items IS 'Rubrik deteksi bias per case, dipakai rule engine Go untuk menilai pola reasoning mahasiswa terhadap skenario yang di-generate AI.';

-- ------------------------------------------------------------
-- 3. SIMULATION SESSIONS
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS  simulation_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  student_id UUID NOT NULL REFERENCES students(id),
  case_id UUID NOT NULL REFERENCES cases(id),

  generated_scenario JSONB NOT NULL,
  embedded_finding_categories TEXT[] NOT NULL DEFAULT '{}',
  is_validated BOOLEAN NOT NULL DEFAULT false,

  status VARCHAR(20) NOT NULL DEFAULT 'pending_validation'
    CHECK (status IN ('pending_validation', 'active', 'completed', 'timeout', 'rejected')),

  started_at TIMESTAMPTZ,
  submitted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_simulation_sessions_student ON simulation_sessions(student_id);
CREATE INDEX IF NOT EXISTS idx_simulation_sessions_case ON simulation_sessions(case_id);
CREATE INDEX IF NOT EXISTS idx_simulation_sessions_status ON simulation_sessions(status);

COMMENT ON TABLE simulation_sessions IS 'Sesi simulasi dengan skenario generative. Menggantikan sessions lama untuk flow baru. sessions lama dipertahankan untuk kompatibilitas data existing, tidak menerima sesi baru setelah migration ini.';
COMMENT ON COLUMN simulation_sessions.embedded_finding_categories IS 'Daftar finding_category yang dilaporkan AI Orchestrator tersisip di generated_scenario. WAJIB divalidasi terhadap sct_rubric_items dan bias_rubric_items untuk case_id terkait SEBELUM is_validated diset true. Lihat larangan #13.';

-- ------------------------------------------------------------
-- 4. TABEL ANAK SIMULATION SESSIONS
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS simulation_reasoning_submissions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id UUID NOT NULL REFERENCES simulation_sessions(id) ON DELETE CASCADE,
  raw_input TEXT NOT NULL,
  input_modality VARCHAR(20) NOT NULL DEFAULT 'text'
    CHECK (input_modality IN ('text', 'voice')),
  parsed_finding_categories TEXT[] DEFAULT '{}',
  submitted_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sim_reasoning_session ON simulation_reasoning_submissions(session_id);

CREATE TABLE IF NOT EXISTS  simulation_session_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id UUID NOT NULL REFERENCES simulation_sessions(id) ON DELETE CASCADE,
  event_type VARCHAR(50) NOT NULL,
  event_data JSONB,
  sequence_number INT NOT NULL,
  occurred_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sim_events_session ON simulation_session_events(session_id);
CREATE INDEX IF NOT EXISTS idx_sim_events_sequence ON simulation_session_events(session_id, sequence_number);

CREATE TABLE IF NOT EXISTS simulation_sct_scores (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id UUID NOT NULL REFERENCES simulation_sessions(id) ON DELETE CASCADE,
  rubric_item_id UUID NOT NULL REFERENCES sct_rubric_items(id),
  student_response SMALLINT NOT NULL CHECK (student_response BETWEEN -2 AND 2),
  deviation SMALLINT NOT NULL,
  credit_score NUMERIC(3,2) NOT NULL,
  scored_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE(session_id, rubric_item_id)
);

CREATE INDEX IF NOT EXISTS idx_sim_sct_scores_session ON simulation_sct_scores(session_id);

CREATE TABLE IF NOT EXISTS  simulation_bias_detections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id UUID NOT NULL REFERENCES simulation_sessions(id) ON DELETE CASCADE,
  bias_type VARCHAR(30) NOT NULL,
  detected BOOLEAN NOT NULL,
  detail TEXT,
  detected_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE(session_id, bias_type)
);

CREATE INDEX IF NOT EXISTS idx_sim_bias_session ON simulation_bias_detections(session_id);

-- ------------------------------------------------------------
-- 5. DEPRECATION MARKERS
-- ------------------------------------------------------------
COMMENT ON TABLE sessions IS 'DEPRECATED sejak migration v3. Tidak menerima sesi baru. Gunakan simulation_sessions untuk seluruh flow baru. Dipertahankan karena masih di-FK oleh reasoning_submissions, session_events, sct_scores, bias_detections lama.';
COMMENT ON TABLE sct_items IS 'DEPRECATED untuk flow utama sejak migration v3. Digantikan sct_rubric_items. Konfirmasi dengan partner whiteboard sebelum menghapus.';

-- ============================================================
-- CATATAN
-- 1. Tidak ada kolom embedding VECTOR di migration ini.
--    Vector store = Pinecone (Go + Python). Kalau perlu simpan
--    referensi ke Pinecone, tambah pinecone_vector_id VARCHAR(255)
--    di cases lewat migration TERPISAH.
-- 2. embedded_finding_categories memakai TEXT[]. Jika AI Orchestrator
--    butuh metadata per finding (confidence, posisi), ubah ke JSONB
--    SEBELUM migration dijalankan, bukan sesudah.
-- 3. is_validated dan status sengaja dipisah agar logic Larangan #13
--    jelas: status bisa 'pending_validation' sementara is_validated
--    masih false, baru pindah ke 'active' setelah is_validated = true.
-- 4. Tidak ada tabel illness_script_master — master yang sebenarnya
--    adalah cases + illness_scripts + differential_diagnoses yang
--    sudah ada. Jangan buat tabel illness_script_master tanpa diskusi
--    ulang, karena akan jadi sumber kebenaran ganda.
-- ============================================================
