package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

const whisperURL = "https://api.openai.com/v1/audio/transcriptions"

// Whisper — STT qua OpenAI Whisper API.
type Whisper struct {
	apiKey string
	http   *http.Client
}

var _ domain.STTProvider = (*Whisper)(nil)

func NewWhisper(apiKey string) *Whisper {
	return &Whisper{apiKey: apiKey, http: &http.Client{Timeout: 5 * time.Minute}}
}

func (w *Whisper) Name() string { return "openai-whisper" }

func (w *Whisper) Transcribe(ctx context.Context, audio []byte, language string) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	part, err := mw.CreateFormFile("file", "audio.mp3")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	_ = mw.WriteField("model", "whisper-1")
	if language != "" {
		_ = mw.WriteField("language", language)
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, whisperURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+w.apiKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := w.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("gọi whisper: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		err := fmt.Errorf("whisper trả về %d: %s", resp.StatusCode, string(raw))
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return "", domain.Permanent(err)
		}
		return "", err
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("parse response whisper: %w", err)
	}
	return out.Text, nil
}
