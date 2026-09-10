// Package tts chứa các adapter Text-to-Speech. Hệ thống không tự xây TTS —
// chỉ tích hợp nhà cung cấp bên thứ 3 (specs 0, quyết định #12).
package tts

import (
	"fmt"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// New dựng provider TTS DỰ PHÒNG của hệ thống theo TTS_PROVIDER trong .env.
//
// Đường chính không đi qua đây: mỗi user khai API key riêng ở mục AI Engine và
// worker dựng provider từ key đó (xem Factory). Hàm này chỉ còn phục vụ 2 tình
// huống:
//
//   - dev: TTS_PROVIDER=mock để chạy đầu-cuối mà không tốn tiền, không cần key.
//   - key chung: đặt THREEVOICES_API_KEY cho cả hệ thống dùng khi user chưa khai.
//
// Trả về (nil, nil) khi không có key chung — hợp lệ, nghĩa là "không có dự
// phòng": user nào chưa khai key thì báo lỗi kèm hướng dẫn, thay vì lặng lẽ
// đọc bằng key của người khác hoặc sinh ra audio im lặng của mock.
func New(cfg *config.Config) (domain.TTSProvider, error) {
	switch cfg.TTSProvider {
	case domain.ProviderThreeVoices:
		if cfg.ThreeVoicesAPIKey == "" {
			return nil, nil
		}
		return NewThreeVoices(cfg.ThreeVoicesAPIKey, cfg.ThreeVoicesBaseURL, cfg.ThreeVoicesVoiceID), nil
	case "mock", "":
		return NewMock(), nil
	default:
		return nil, fmt.Errorf("TTS_PROVIDER %q chưa được tích hợp — chỉ còn 3voices (hoặc mock khi dev)", cfg.TTSProvider)
	}
}

// Factory dựng provider TTS từ API key của từng user (bảng ai_engine).
//
// Có nó vì API key không còn là hằng số trong .env: mỗi người khai key của
// mình, worker phải đọc bằng key của đúng người sở hữu voice. Cấu hình chung
// duy nhất còn lại là base URL (đổi khi 3voices đổi domain hoặc khi test).
type Factory struct {
	threeVoicesBaseURL string
}

var _ domain.TTSFactory = (*Factory)(nil)

func NewFactory(cfg *config.Config) *Factory {
	return &Factory{threeVoicesBaseURL: cfg.ThreeVoicesBaseURL}
}

func (f *Factory) For(cred domain.TTSCredential) (domain.TTSProvider, error) {
	switch cred.Provider {
	case domain.ProviderThreeVoices:
		if cred.APIKey == "" {
			return nil, fmt.Errorf("engine %s chưa có API key", cred.Provider)
		}
		return NewThreeVoices(cred.APIKey, f.threeVoicesBaseURL, cred.VoiceID), nil
	default:
		return nil, fmt.Errorf("provider TTS %q chưa được tích hợp", cred.Provider)
	}
}
