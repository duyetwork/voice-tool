// Package llm chứa các adapter LLM, dùng cho Mode C: viết lại text nguồn theo
// Prompt mẫu trước khi đưa qua TTS.
package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
)

func New(cfg *config.Config) (domain.LLMProvider, error) {
	switch cfg.LLMProvider {
	case "anthropic":
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY là bắt buộc khi LLM_PROVIDER=anthropic")
		}
		return NewAnthropic(cfg.AnthropicAPIKey, cfg.AnthropicModel), nil
	case "mock", "":
		return NewMock(), nil
	default:
		return nil, fmt.Errorf("LLM_PROVIDER %q chưa được tích hợp", cfg.LLMProvider)
	}
}

// Mock ghép prompt + text để chạy luồng end-to-end khi dev, không tốn API.
type Mock struct{}

var _ domain.LLMProvider = (*Mock)(nil)

func NewMock() *Mock { return &Mock{} }

func (m *Mock) Name() string { return "mock" }

func (m *Mock) Generate(_ context.Context, promptContent, sourceText string) (string, error) {
	var b strings.Builder
	b.WriteString("[mock LLM] prompt: ")
	b.WriteString(promptContent)
	b.WriteString("\n---\n")
	b.WriteString(sourceText)
	return b.String(), nil
}
