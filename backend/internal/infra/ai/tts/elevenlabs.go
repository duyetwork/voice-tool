package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

const elevenLabsBaseURL = "https://api.elevenlabs.io/v1"

// ElevenLabs — TTS provider mặc định cho production (đa ngôn ngữ, có tiếng Việt).
type ElevenLabs struct {
	apiKey  string
	voiceID string
	http    *http.Client
}

var _ domain.TTSProvider = (*ElevenLabs)(nil)

func NewElevenLabs(apiKey, voiceID string) *ElevenLabs {
	if voiceID == "" {
		voiceID = "21m00Tcm4TlvDq8ikWAM" // Rachel — voice mặc định của ElevenLabs
	}
	return &ElevenLabs{
		apiKey:  apiKey,
		voiceID: voiceID,
		http:    &http.Client{Timeout: 2 * time.Minute},
	}
}

func (e *ElevenLabs) Name() string { return "elevenlabs" }

// eleven_multilingual_v2 hỗ trợ 29 ngôn ngữ, gồm vi.
func (e *ElevenLabs) SupportedLanguages() []string {
	return []string{"vi", "en", "ja", "ko", "zh", "fr", "de", "es", "it", "pt", "id", "th"}
}

type elevenRequest struct {
	Text          string         `json:"text"`
	ModelID       string         `json:"model_id"`
	LanguageCode  string         `json:"language_code,omitempty"`
	VoiceSettings map[string]any `json:"voice_settings,omitempty"`
}

func (e *ElevenLabs) Synthesize(ctx context.Context, text, language string) ([]byte, error) {
	payload, err := json.Marshal(elevenRequest{
		Text:         text,
		ModelID:      "eleven_multilingual_v2",
		LanguageCode: language,
		VoiceSettings: map[string]any{
			"stability":        0.5,
			"similarity_boost": 0.75,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request elevenlabs: %w", err)
	}

	url := fmt.Sprintf("%s/text-to-speech/%s", elevenLabsBaseURL, e.voiceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gọi elevenlabs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		err := fmt.Errorf("elevenlabs trả về %d: %s", resp.StatusCode, string(body))
		// 4xx (trừ 429) là lỗi cấu hình/input -> không retry.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return nil, domain.Permanent(err)
		}
		return nil, err
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("đọc audio elevenlabs: %w", err)
	}
	return audio, nil
}
