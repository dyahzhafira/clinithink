package handlers

import (
	"clinithink/internal/tts"

	"github.com/gofiber/fiber/v2"
)

type synthesizeReq struct {
	Text  string `json:"text"`
	Voice string `json:"voice"`
}

func (h *Handler) SynthesizeSpeech(c *fiber.Ctx) error {
	if h.ttsKey == "" {
		return fiber.NewError(fiber.StatusServiceUnavailable,
			"TTS tidak tersedia: GCP_TTS_API_KEY belum dikonfigurasi")
	}
	var req synthesizeReq
	if err := c.BodyParser(&req); err != nil || req.Text == "" {
		return fiber.NewError(fiber.StatusBadRequest, "field 'text' wajib diisi")
	}
	audioContent, err := tts.Synthesize(h.ttsKey, req.Text, req.Voice)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway,
			"gagal menghubungi layanan TTS: "+err.Error())
	}
	voice := req.Voice
	if voice == "" {
		voice = tts.DefaultVoice
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"audio_content": audioContent,
			"format":        "mp3",
			"voice":         voice,
		},
	})
}
