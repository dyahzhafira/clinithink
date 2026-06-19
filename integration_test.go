package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"clinithink/internal/config"
	"clinithink/internal/database"
	"clinithink/internal/routes"
	ws "clinithink/internal/ws"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
)

var testApp *fiber.App

func TestMain(m *testing.M) {
	_ = godotenv.Load()
	os.Setenv("APP_ENV", "test")

	cfg, err := config.Load()
	if err != nil {
		os.Exit(m.Run())
	}

	db, err := database.NewPostgres(cfg.DatabaseURL)
	if err != nil {
		os.Exit(m.Run())
	}

	rdb, err := database.NewRedis(cfg.RedisURL)
	if err != nil {
		os.Exit(m.Run())
	}

	//cleaning data
	db.Exec(context.Background(), "DELETE FROM students WHERE email LIKE '%@test.clinithink'")

	testApp = fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"error":   fiber.Map{"code": "INTERNAL_ERROR", "message": "error"},
			})
		},
	})
	hub := ws.NewHub()
	routes.Setup(testApp, cfg, db, rdb, hub)

	code := m.Run()

	db.Exec(context.Background(), "DELETE FROM students WHERE email LIKE '%@test.clinithink'")
	db.Close()
	rdb.Close()

	os.Exit(code)
}

func requireDB(t *testing.T) {
	t.Helper()
	if testApp == nil {
		t.Skip("database tidak tersedia, skip integration test")
	}
}

func doRequest(t *testing.T, method, path string, body interface{}, token string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := testApp.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func parseBody(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	b, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(b, &result); err != nil {
		t.Logf("parseBody decode error: %v | raw: %s", err, string(b))
	}
	return result
}

func getTestToken(t *testing.T) string {
	t.Helper()
	email := "flow@test.clinithink"
	doRequest(t, "POST", "/api/auth/register", map[string]interface{}{
		"name": "Flow Test", "email": email, "password": "password123",
	}, "")
	resp := doRequest(t, "POST", "/api/auth/login", map[string]interface{}{
		"email": email, "password": "password123",
	}, "")
	body := parseBody(t, resp)
	data, _ := body["data"].(map[string]interface{})
	token, _ := data["token"].(string)
	return token
}

//checking healt
func TestHealth(t *testing.T) {
	requireDB(t)
	resp := doRequest(t, "GET", "/api/health", nil, "")
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}
//auth
func TestRegister(t *testing.T) {
	requireDB(t)
	resp := doRequest(t, "POST", "/api/auth/register", map[string]interface{}{
		"name": "Test User", "email": "register@test.clinithink", "password": "password123",
	}, "")
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body := parseBody(t, resp)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	requireDB(t)
	payload := map[string]interface{}{
		"name": "Dup User", "email": "dup@test.clinithink", "password": "password123",
	}
	doRequest(t, "POST", "/api/auth/register", payload, "")
	resp := doRequest(t, "POST", "/api/auth/register", payload, "")
	if resp.StatusCode != 409 {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
}

func TestRegister_MissingFields(t *testing.T) {
	requireDB(t)
	resp := doRequest(t, "POST", "/api/auth/register", map[string]interface{}{
		"email": "missing@test.clinithink",
	}, "")
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestLogin(t *testing.T) {
	requireDB(t)
	doRequest(t, "POST", "/api/auth/register", map[string]interface{}{
		"name": "Login User", "email": "login@test.clinithink", "password": "password123",
	}, "")
	resp := doRequest(t, "POST", "/api/auth/login", map[string]interface{}{
		"email": "login@test.clinithink", "password": "password123",
	}, "")
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body := parseBody(t, resp)
	data, _ := body["data"].(map[string]interface{})
	if data["token"] == nil {
		t.Error("expected token in response")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	requireDB(t)
	resp := doRequest(t, "POST", "/api/auth/login", map[string]interface{}{
		"email": "login@test.clinithink", "password": "wrongpassword",
	}, "")
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

//cases
func TestListCases_Unauthorized(t *testing.T) {
	requireDB(t)
	resp := doRequest(t, "GET", "/api/cases", nil, "")
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestListCases(t *testing.T) {
	requireDB(t)
	token := getTestToken(t)
	resp := doRequest(t, "GET", "/api/cases", nil, token)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body := parseBody(t, resp)
	data, _ := body["data"].([]interface{})
	if len(data) != 16 {
		t.Errorf("expected 16 cases, got %d", len(data))
	}
}

func TestListCases_FilterBySystem(t *testing.T) {
	requireDB(t)
	token := getTestToken(t)
	resp := doRequest(t, "GET", "/api/cases?system=RESP", nil, token)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body := parseBody(t, resp)
	data, _ := body["data"].([]interface{})
	if len(data) == 0 {
		t.Error("expected at least 1 RESP case")
	}
	for _, item := range data {
		c, _ := item.(map[string]interface{})
		if c["system_code"] != "RESP" {
			t.Errorf("expected system_code=RESP, got %v", c["system_code"])
		}
	}
}

func TestGetCase_NotFound(t *testing.T) {
	requireDB(t)
	token := getTestToken(t)
	resp := doRequest(t, "GET", "/api/cases/00000000-0000-0000-0000-000000000000", nil, token)
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

//session flow
func TestSessionFlow(t *testing.T) {
	requireDB(t)
	token := getTestToken(t)

	// get case ID
	resp := doRequest(t, "GET", "/api/cases?system=RESP", nil, token)
	body := parseBody(t, resp)
	data, _ := body["data"].([]interface{})
	if len(data) == 0 {
		t.Fatal("no cases available")
	}
	firstCase, _ := data[0].(map[string]interface{})
	caseUUID, _ := firstCase["id"].(string)

	//create session
	resp = doRequest(t, "POST", "/api/sessions", map[string]interface{}{"case_id": caseUUID}, token)
	if resp.StatusCode != 200 {
		t.Fatalf("create session: expected 200, got %d", resp.StatusCode)
	}
	sessionBody := parseBody(t, resp)
	sessionData, _ := sessionBody["data"].(map[string]interface{})
	sessionID, _ := sessionData["id"].(string)
	t.Logf("sessionID=%q", sessionID)

	//get session
	resp = doRequest(t, "GET", "/api/sessions/"+sessionID, nil, token)
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("get session: expected 200, got %d | body: %s", resp.StatusCode, b)
	}

	//list sessions
	resp = doRequest(t, "GET", "/api/sessions", nil, token)
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("list sessions: expected 200, got %d | body: %s", resp.StatusCode, b)
	}

	//bias check (event log kosong)
	resp = doRequest(t, "GET", "/api/sessions/"+sessionID+"/bias-check", nil, token)
	if resp.StatusCode != 200 {
		t.Errorf("bias check: expected 200, got %d", resp.StatusCode)
	}

	//submit reasoning
	resp = doRequest(t, "POST", "/api/sessions/"+sessionID+"/submit", map[string]interface{}{
		"raw_input": "Pasien kemungkinan TB paru", "input_modality": "text",
	}, token)
	if resp.StatusCode != 200 {
		t.Fatalf("submit reasoning: expected 200, got %d", resp.StatusCode)
	}

	//submit again, must be 409
	resp = doRequest(t, "POST", "/api/sessions/"+sessionID+"/submit", map[string]interface{}{
		"raw_input": "second submit",
	}, token)
	if resp.StatusCode != 409 {
		t.Errorf("double submit: expected 409, got %d", resp.StatusCode)
	}
}

//events
func TestLogEvent(t *testing.T) {
	requireDB(t)
	token := getTestToken(t)

	//get session
	resp := doRequest(t, "GET", "/api/cases", nil, token)
	body := parseBody(t, resp)
	data, _ := body["data"].([]interface{})
	if len(data) == 0 {
		t.Fatal("no cases")
	}
	firstCase, _ := data[0].(map[string]interface{})
	caseUUID, _ := firstCase["id"].(string)

	resp = doRequest(t, "POST", "/api/sessions", map[string]interface{}{"case_id": caseUUID}, token)
	sessionBody := parseBody(t, resp)
	sessionData, _ := sessionBody["data"].(map[string]interface{})
	sessionID, _ := sessionData["id"].(string)

	//log event
	resp = doRequest(t, "POST", "/api/sessions/"+sessionID+"/events", map[string]interface{}{
		"event_type": "symptom_mentioned",
	}, token)
	if resp.StatusCode != 200 {
		t.Errorf("log event: expected 200, got %d", resp.StatusCode)
	}
	evBody := parseBody(t, resp)
	evData, _ := evBody["data"].(map[string]interface{})
	if evData["sequence_number"] == nil {
		t.Error("expected sequence_number in response")
	}

	//log event type not valid
	resp = doRequest(t, "POST", "/api/sessions/"+sessionID+"/events", map[string]interface{}{
		"event_type": "invalid_type",
	}, token)
	if resp.StatusCode != 400 {
		t.Errorf("invalid event_type: expected 400, got %d", resp.StatusCode)
	}

	//second event, sequence++
	resp = doRequest(t, "POST", "/api/sessions/"+sessionID+"/events", map[string]interface{}{
		"event_type": "differential_explored",
		"event_data": map[string]string{"entity": "TB Paru"},
	}, token)
	if resp.StatusCode != 200 {
		t.Errorf("second event: expected 200, got %d", resp.StatusCode)
	}
	ev2Body := parseBody(t, resp)
	ev2Data, _ := ev2Body["data"].(map[string]interface{})
	if ev2Data["sequence_number"].(float64) != 2 {
		t.Errorf("expected sequence_number=2, got %v", ev2Data["sequence_number"])
	}
}

//students
func TestGetMe(t *testing.T) {
	requireDB(t)
	token := getTestToken(t)
	resp := doRequest(t, "GET", "/api/students/me", nil, token)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body := parseBody(t, resp)
	data, _ := body["data"].(map[string]interface{})
	if data["email"] == nil {
		t.Error("expected email in profile")
	}
	if data["password_hash"] != nil {
		t.Error("password_hash should not be returned")
	}
}

func TestGetSummary(t *testing.T) {
	requireDB(t)
	token := getTestToken(t)
	resp := doRequest(t, "GET", "/api/students/me/summary", nil, token)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body := parseBody(t, resp)
	data, _ := body["data"].(map[string]interface{})
	if _, ok := data["total_sessions"]; !ok {
		t.Error("expected total_sessions in summary")
	}
	if _, ok := data["bias_detections"]; !ok {
		t.Error("expected bias_detections in summary")
	}
}

func TestSCTFlow(t *testing.T) {
	requireDB(t)
	token := getTestToken(t)

	//get a case with SCT items
	resp := doRequest(t, "GET", "/api/cases?system=RESP", nil, token)
	body := parseBody(t, resp)
	data, _ := body["data"].([]interface{})
	if len(data) == 0 {
		t.Fatal("no cases available")
	}
	firstCase, _ := data[0].(map[string]interface{})
	caseUUID, _ := firstCase["id"].(string)

	//get case detail to extract sct item UUIDs
	resp = doRequest(t, "GET", "/api/cases/"+caseUUID, nil, token)
	if resp.StatusCode != 200 {
		t.Fatalf("get case detail: expected 200, got %d", resp.StatusCode)
	}
	caseBody := parseBody(t, resp)
	caseData, _ := caseBody["data"].(map[string]interface{})
	sctItems, _ := caseData["sct_items"].([]interface{})
	if len(sctItems) == 0 {
		t.Fatal("no SCT items in case")
	}
	firstItem, _ := sctItems[0].(map[string]interface{})
	sctItemUUID, _ := firstItem["id"].(string)
	if sctItemUUID == "" {
		t.Fatal("sct_item id is empty")
	}

	//create session
	resp = doRequest(t, "POST", "/api/sessions", map[string]interface{}{"case_id": caseUUID}, token)
	if resp.StatusCode != 200 {
		t.Fatalf("create session: expected 200, got %d", resp.StatusCode)
	}
	sessionBody := parseBody(t, resp)
	sessionData, _ := sessionBody["data"].(map[string]interface{})
	sessionID, _ := sessionData["id"].(string)

	// submit reasoning first (required before SCT)
	resp = doRequest(t, "POST", "/api/sessions/"+sessionID+"/submit", map[string]interface{}{
		"raw_input": "Saya curiga TB paru berdasarkan gejala", "input_modality": "text",
	}, token)
	if resp.StatusCode != 200 {
		t.Fatalf("submit reasoning: expected 200, got %d", resp.StatusCode)
	}

	//submit SCT answers
	resp = doRequest(t, "POST", "/api/sessions/"+sessionID+"/sct", map[string]interface{}{
		"answers": []interface{}{
			map[string]interface{}{"sct_item_id": sctItemUUID, "response": "+1"},
		},
	}, token)
	if resp.StatusCode != 200 {
		t.Fatalf("submit SCT: expected 200, got %d", resp.StatusCode)
	}
	sctBody := parseBody(t, resp)
	sctData, _ := sctBody["data"].(map[string]interface{})
	normalizedScore, _ := sctData["normalized_score"].(float64)
	if normalizedScore < 0 || normalizedScore > 1 {
		t.Errorf("normalized_score out of range [0,1]: %v", normalizedScore)
	}

	// resubmit must be rejected
	resp = doRequest(t, "POST", "/api/sessions/"+sessionID+"/sct", map[string]interface{}{
		"answers": []interface{}{
			map[string]interface{}{"sct_item_id": sctItemUUID, "response": "0"},
		},
	}, token)
	if resp.StatusCode != 409 {
		t.Errorf("duplicate SCT submit: expected 409, got %d", resp.StatusCode)
	}

	//get SCT scores
	resp = doRequest(t, "GET", "/api/sessions/"+sessionID+"/sct", nil, token)
	if resp.StatusCode != 200 {
		t.Errorf("get SCT scores: expected 200, got %d", resp.StatusCode)
	}
	getBody := parseBody(t, resp)
	getData, _ := getBody["data"].(map[string]interface{})
	if getData["total_items"] == nil {
		t.Error("expected total_items in SCT scores response")
	}
}

func TestDTIFlow(t *testing.T) {
	requireDB(t)
	token := getTestToken(t)

	responses := make(map[string]interface{}, 41)
	for i := 1; i <= 41; i++ {
		responses[fmt.Sprintf("%d", i)] = 3
	}

	//submit pretest
	resp := doRequest(t, "POST", "/api/dti", map[string]interface{}{
		"test_phase": "pre",
		"responses":  responses,
	}, token)
	if resp.StatusCode != 200 {
		t.Fatalf("submit DTI pre: expected 200, got %d", resp.StatusCode)
	}
	body := parseBody(t, resp)
	data, _ := body["data"].(map[string]interface{})
	if data["flexibility_in_thinking_score"] == nil {
		t.Error("expected flexibility_in_thinking_score in response")
	}
	if data["structure_of_knowledge_score"] == nil {
		t.Error("expected structure_of_knowledge_score in response")
	}

	//duplicate pretest must be rejected
	resp = doRequest(t, "POST", "/api/dti", map[string]interface{}{
		"test_phase": "pre",
		"responses":  responses,
	}, token)
	if resp.StatusCode != 409 {
		t.Errorf("duplicate DTI pre: expected 409, got %d", resp.StatusCode)
	}

	//submit posttest
	resp = doRequest(t, "POST", "/api/dti", map[string]interface{}{
		"test_phase": "post",
		"responses":  responses,
	}, token)
	if resp.StatusCode != 200 {
		t.Fatalf("submit DTI post: expected 200, got %d", resp.StatusCode)
	}

	//get results, return both pre and post
	resp = doRequest(t, "GET", "/api/dti", nil, token)
	if resp.StatusCode != 200 {
		t.Errorf("get DTI: expected 200, got %d", resp.StatusCode)
	}
	getBody := parseBody(t, resp)
	results, _ := getBody["data"].([]interface{})
	if len(results) != 2 {
		t.Errorf("expected 2 DTI results (pre+post), got %d", len(results))
	}
}

//skeleton endpoints
func TestSkeletonEndpoints_Return501(t *testing.T) {
	requireDB(t)
	token := getTestToken(t)
	endpoints := []struct{ method, path string }{
		{"POST", "/api/sessions/00000000-0000-0000-0000-000000000000/analysis"},
		{"GET", "/api/sessions/00000000-0000-0000-0000-000000000000/analysis"},
	}
	for _, e := range endpoints {
		resp := doRequest(t, e.method, e.path, nil, token)
		if resp.StatusCode != 501 {
			t.Errorf("%s %s: expected 501, got %d", e.method, e.path, resp.StatusCode)
		}
	}
}
