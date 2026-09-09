// Package tts chứa các adapter Text-to-Speech. Hệ thống không tự xây TTS —
// chỉ tích hợp nhà cung cấp bên thứ 3 (specs 0, quyết định #12).
package tts

import (
	"fmt"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// New chọn provider theo TTS_PROVIDER trong .env.
func New(cfg *config.Config) (domain.TTSProvider, error) {
	switch cfg.TTSProvider {
	case "3voices":
		if cfg.ThreeVoicesAPIKey == "" {
			return nil, fmt.Errorf("THREEVOICES_API_KEY là bắt buộc khi TTS_PROVIDER=3voices")
		}
		return NewThreeVoices(cfg.ThreeVoicesAPIKey, cfg.ThreeVoicesBaseURL, cfg.ThreeVoicesVoiceID), nil
	case "elevenlabs":
		if cfg.ElevenLabsAPIKey == "" {
			return nil, fmt.Errorf("ELEVENLABS_API_KEY là bắt buộc khi TTS_PROVIDER=elevenlabs")
		}
		return NewElevenLabs(cfg.ElevenLabsAPIKey, cfg.ElevenLabsVoice), nil
	case "mock", "":
		return NewMock(), nil
	default:
		return nil, fmt.Errorf("TTS_PROVIDER %q chưa được tích hợp", cfg.TTSProvider)
	}
}
