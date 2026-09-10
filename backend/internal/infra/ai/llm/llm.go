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

// Mock cho phép chạy luồng end-to-end khi dev mà không tốn tiền API.
//
// Trả về ĐÚNG text nguồn, không viết lại gì và tuyệt đối không ghép prompt vào
// kết quả: đầu ra của bước này đi thẳng vào TTS, nên nhét prompt vào đây nghĩa
// là voice đọc to cả câu lệnh cho AI. Hình thức C thật sự cần LLM thật —
// app.go tắt hẳn mode C khi provider còn là mock (xem ModeGate).
type Mock struct{}

var _ domain.LLMProvider = (*Mock)(nil)

func NewMock() *Mock { return &Mock{} }

func (m *Mock) Name() string { return "mock" }

func (m *Mock) Generate(_ context.Context, _, sourceText string) (string, error) {
	return strings.TrimSpace(sourceText), nil
}
