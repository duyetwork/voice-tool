package service

import (
	"fmt"
	"slices"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// ModeEnabled chặn hình thức thu thập chưa được bật (ENABLED_COLLECT_MODES).
// Mode B/C cần TTS/LLM thật nên mặc định tắt — API từ chối sớm thay vì để job
// chạy rồi mới fail trong worker.
func ModeEnabled(mode domain.CollectMode, enabled []domain.CollectMode) error {
	if len(enabled) == 0 || slices.Contains(enabled, mode) {
		return nil
	}
	return fmt.Errorf("%w: hình thức thu thập %s chưa được hỗ trợ", domain.ErrInvalidInput, mode)
}

// resolveLanguage cài đặt cascade ngôn ngữ theo tầng (business rule #9):
//
//	SourcePost.language (override) > List.language_default > mặc định hệ thống
//
// Mode A không dùng ngôn ngữ để xử lý voice, chỉ dùng để gắn nhãn — nên vẫn
// resolve bình thường, chỉ khác là không truyền vào TTS.
func resolveLanguage(postLevel, listLevel, systemDefault string) string {
	for _, candidate := range []string{postLevel, listLevel, systemDefault} {
		if v := strings.TrimSpace(candidate); v != "" {
			return strings.ToLower(v)
		}
	}
	// Không ai chốt ngôn ngữ -> để nền tảng/multime tự nhận diện.
	return domain.LanguageAuto
}

// languageSupported kiểm tra engine có hỗ trợ ngôn ngữ đã chọn (specs 3.3).
// Danh sách rỗng nghĩa là engine không khai báo giới hạn -> coi như hỗ trợ.
func languageSupported(lang string, supported []string) bool {
	if len(supported) == 0 {
		return true
	}
	base := baseLanguage(lang)
	for _, s := range supported {
		if baseLanguage(s) == base {
			return true
		}
	}
	return false
}

// baseLanguage bỏ phần region: "vi-VN" -> "vi".
func baseLanguage(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		return lang[:i]
	}
	return lang
}
