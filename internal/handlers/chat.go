package handlers

import (
	"clinithink/internal/database"
	"clinithink/internal/grpc"
	"encoding/json"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func ChatHandler(c *fiber.Ctx) error {
	type ChatRequest struct {
		SessionID string `json:"sessionId"`
		Message   string `json:"message"`
	}

	req := new(ChatRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request body"})
	}

	sID, err := uuid.Parse(req.SessionID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid Session ID"})
	}

	caseData, err := database.GetCaseBySessionID(c.Context(), sID)
	if err != nil {
		log.Printf("Gagal ambil context kasus: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Gagal memuat konteks kasus"})
	}

	err = database.SaveReasoningSubmission(c.Context(), sID, req.Message, "voice")
	if err != nil {
		log.Printf("Gagal simpan input: %v", err)
	}

	aiRes, err := grpc.SendAnalysisTrigger(
		req.SessionID,
		req.Message,
		caseData.Title,
		caseData.PrimaryDiagnosis,
		caseData.ExpectedWorkup,
		caseData.AnamnesisItems,
	)
	if err != nil {
		log.Printf("AI Service Error: %v", err)
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "AI service unavailable"})
	}

	eventData, _ := json.Marshal(map[string]string{"answer": aiRes.Status})
	_ = database.SaveSessionEvent(c.Context(), sID, "ai_response", eventData, 1)

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"answer": aiRes.Status,
		},
	})
}
