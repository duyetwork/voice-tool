// Package stt chứa các adapter Speech-to-Text, dùng khi mode B/C không lấy
// được caption/transcript sẵn có và phải nghe audio để ra text.
package stt

import (
	"context"
	"fmt"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
)

func New(cfg *config.Config) (domain.STTProvider, error) {
	switch cfg.STTProvider {
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY là bắt buộc khi STT_PROVIDER=openai")
		}
		return NewWhisper(cfg.OpenAIAPIKey), nil
	case "mock", "":
		return NewMock(), nil
	default:
		return nil, fmt.Errorf("STT_PROVIDER %q chưa được tích hợp", cfg.STTProvider)
	}
}

// Mock trả về text cố định — đủ để chạy luồng end-to-end khi dev.
type Mock struct{}

var _ domain.STTProvider = (*Mock)(nil)

func NewMock() *Mock { return &Mock{} }

func (m *Mock) Name() string { return "mock" }

func (m *Mock) Transcribe(_ context.Context, audio []byte, _ string) (string, error) {
	return fmt.Sprintf("[mock transcript của %d bytes audio]", len(audio)), nil
}
