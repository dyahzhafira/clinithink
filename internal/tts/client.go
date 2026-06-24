package tts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const gcpTTSURL = "https://texttospeech.googleapis.com/v1/text:synthesize"

const DefaultVoice = "id-ID-Standard-A"

type synthesizeRequest struct {
	Input       inputConfig `json:"input"`
	Voice       voiceConfig `json:"voice"`
	AudioConfig audioConfig `json:"audioConfig"`
}

type inputConfig struct {
	Text string `json:"text"`
}

type voiceConfig struct {
	LanguageCode string `json:"languageCode"`
	Name         string `json:"name"`
}

type audioConfig struct {
	AudioEncoding string `json:"audioEncoding"`
}

type synthesizeResponse struct {
	AudioContent string `json:"audioContent"`
}

// Synthesize calls Google Cloud TTS and returns base64-encoded MP3 audio.
func Synthesize(apiKey, text, voice string) (string, error) {
	if voice == "" {
		voice = DefaultVoice
	}
	payload := synthesizeRequest{
		Input:       inputConfig{Text: text},
		Voice:       voiceConfig{LanguageCode: "id-ID", Name: voice},
		AudioConfig: audioConfig{AudioEncoding: "MP3"},
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(gcpTTSURL+"?key="+apiKey, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("tts request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tts API error %d: %s", resp.StatusCode, string(respBody))
	}
	var result synthesizeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("tts response decode: %w", err)
	}
	return result.AudioContent, nil
}
