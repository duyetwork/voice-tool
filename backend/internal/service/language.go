package service

import (
	"fmt"
	"slices"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// ModeGate quyết định hình thức thu thập nào đang dùng được, và VÌ SAO cái
// còn lại thì không.
//
// Có 2 nguồn khoá khác nhau nên phải kèm lý do, không thể chỉ trả "chưa hỗ
// trợ": người vận hành tự tắt trong ENABLED_COLLECT_MODES, hoặc hệ thống tự
// tắt vì thiếu provider thật (mode C mà LLM còn là mock thì không có gì viết
// lại nội dung — xem app.disablePromptMode).
type ModeGate struct {
	// Enabled: nil = chưa cấu hình gì, cho qua tất cả (zero value dùng trong
	// test). Slice RỖNG khác nil: nó nghĩa là "không mode nào dùng được" —
	// xảy ra khi người vận hành chỉ bật đúng mode C mà LLM lại là mock.
	Enabled []domain.CollectMode
	// Reason: mode -> câu giải thích vì sao bị tắt. Không có trong map nghĩa
	// là do cấu hình ENABLED_COLLECT_MODES.
	Reason map[domain.CollectMode]string
}

// Allows cho biết mode có đang bật không.
func (g ModeGate) Allows(mode domain.CollectMode) bool {
	return g.Enabled == nil || slices.Contains(g.Enabled, mode)
}

// Why trả lý do mode bị tắt; rỗng nghĩa là mode đang bật.
func (g ModeGate) Why(mode domain.CollectMode) string {
	if g.Allows(mode) {
		return ""
	}
	if r := g.Reason[mode]; r != "" {
		return r
	}
	return "chưa được bật trong cấu hình ENABLED_COLLECT_MODES của server"
}

// Check chặn hình thức chưa dùng được, kèm lý do cụ thể — API từ chối sớm thay
// vì để job chạy rồi mới fail trong worker.
func (g ModeGate) Check(mode domain.CollectMode) error {
	if g.Allows(mode) {
		return nil
	}
	return fmt.Errorf("%w: hình thức thu thập %s %s",
		domain.ErrInvalidInput, mode, g.Why(mode))
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
