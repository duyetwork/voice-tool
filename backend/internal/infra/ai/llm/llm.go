// Package llm chứa các adapter LLM, dùng cho Mode C: viết lại text nguồn theo
// Prompt mẫu trước khi đưa qua TTS.
//
// Mỗi adapter ở đây là MỘT cặp (nhà cung cấp, model) đã gắn sẵn 1 API key. Việc
// chọn key nào, model nào, và chuyển dự phòng khi hết quota thuộc về LLMRouter
// (service/llmrouter.go) đứng trên chúng — cùng kiểu phân tầng với
// TTSProvider / TTSFactory.
package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// New dựng provider dự phòng cấu hình trong .env.
//
// Chỉ còn dùng cho DEV: đường chính là bộ API key trong DB, qua Factory. Giữ
// lại vì `make api` không có bộ nào trong DB vẫn phải chạy được, và vì ModeGate
// cần biết "có LLM thật hay không" ngay lúc khởi động.
func New(cfg *config.Config) (domain.LLMProvider, error) {
	switch cfg.LLMProvider {
	case "anthropic":
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY là bắt buộc khi LLM_PROVIDER=anthropic")
		}
		return NewAnthropic(cfg.AnthropicAPIKey, cfg.AnthropicModel), nil
	case "gemini":
		if cfg.GeminiAPIKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY là bắt buộc khi LLM_PROVIDER=gemini")
		}
		return NewGemini(cfg.GeminiAPIKey, cfg.GeminiModel), nil
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY là bắt buộc khi LLM_PROVIDER=openai")
		}
		return NewOpenAI(cfg.OpenAIAPIKey, cfg.OpenAIModel), nil
	case "mock", "":
		return NewMock(), nil
	default:
		return nil, fmt.Errorf("LLM_PROVIDER %q chưa được tích hợp", cfg.LLMProvider)
	}
}

// Factory dựng provider từ credential đọc ra từ bảng llm_api_key.
//
// Tồn tại vì cùng lý do với TTSFactory: key không còn là hằng số trong .env mà
// là dữ liệu, và model thì do chuỗi dự phòng quyết định tại thời điểm chạy.
type Factory struct{}

var _ domain.LLMFactory = (*Factory)(nil)

func NewFactory() *Factory { return &Factory{} }

func (f *Factory) For(cred domain.LLMCredential) (domain.LLMProvider, error) {
	key := strings.TrimSpace(cred.APIKey)
	if key == "" {
		return nil, fmt.Errorf("%w: thiếu API key cho %s", domain.ErrInvalidInput, cred.Provider)
	}
	if strings.TrimSpace(cred.Model) == "" {
		return nil, fmt.Errorf("%w: thiếu model cho %s", domain.ErrInvalidInput, cred.Provider)
	}

	switch cred.Provider {
	case domain.LLMGemini:
		return NewGemini(key, cred.Model), nil
	case domain.LLMOpenAI:
		return NewOpenAI(key, cred.Model), nil
	case domain.LLMAnthropic:
		return NewAnthropic(key, cred.Model), nil
	default:
		return nil, fmt.Errorf("%w: nhà cung cấp LLM %q chưa được tích hợp",
			domain.ErrInvalidInput, cred.Provider)
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

func (m *Mock) GenerateBatch(_ context.Context, _ string, items []string) ([]string, error) {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = strings.TrimSpace(item)
	}
	return out, nil
}
