package domain

import "strings"

// MaxTTSTextRunes chặn text nhập tay quá dài trước khi gửi sang TTS.
//
// Không phải giới hạn do nhà cung cấp công bố — 3voices không nói con số — mà
// là hàng rào của hệ thống: text càng dài thì audio càng dài, càng dễ timeout
// và càng tốn tiền cho một lần bấm nhầm. 20.000 ký tự ~ 25 phút đọc, dư cho
// mọi bài tin.
const MaxTTSTextRunes = 20000

// TextPostMetadata dựng metadata cho Bài Post nhập tay bằng text.
//
// Cùng luật với bài lấy từ nền tảng (xem platform.PostContent): tiêu đề là
// TOÀN BỘ nội dung trừ hashtag, hashtag tách sang trường riêng — chỉ khác ở
// chỗ nội dung do người dùng gõ chứ không phải fetch về.
func TextPostMetadata(text string) PostMetadata {
	return PostMetadata{
		Title:    PostTitle(StripHashtags(text)),
		Hashtags: ExtractHashtags(text, nil),
	}
}

// NormalizeTTSText dọn text người dùng dán vào: bỏ khoảng trắng thừa 2 đầu và
// ký tự xuống dòng kiểu Windows (TTS đọc thừa dấu ngắt).
func NormalizeTTSText(text string) string {
	return strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
}
