package platform

import (
	"regexp"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// yt-dlp in lỗi ra stderr dưới dạng dài dòng, ví dụ:
//
//	ERROR: [facebook] pfbid0uGgk...: This video is only available for registered
//	users. Use --cookies-from-browser or --cookies for the authentication. See
//	https://github.com/yt-dlp/yt-dlp/wiki/FAQ#how-do-i-pass-cookies-to-yt-dlp ...
//
// Người dùng cuối không cần biết cờ dòng lệnh của yt-dlp; họ cần biết "bài này
// bị chặn" hay "link này không có video". Bảng dưới đây đổi từng loại lỗi sang
// 1 câu, và lỗi vĩnh viễn được đánh dấu để Asynq không retry vô nghĩa.
var ytdlpErrors = []struct {
	re        *regexp.Regexp
	msg       string
	permanent bool
}{
	{
		regexp.MustCompile(`(?i)only available for registered users|login required|requires authentication|Sign in to confirm your age|You must be logged in`),
		"Bài này bị nền tảng chặn, phải đăng nhập mới xem được — cần cấu hình cookies cho yt-dlp",
		true,
	},
	{
		regexp.MustCompile(`(?i)Sign in to confirm you're not a bot|confirm you.re not a bot`),
		"Nền tảng đang chặn IP máy chủ — cần cấu hình cookies hoặc proxy cho yt-dlp",
		false,
	},
	{
		regexp.MustCompile(`(?i)private video|this video is private|account is private`),
		"Bài đăng ở chế độ riêng tư",
		true,
	},
	{
		regexp.MustCompile(`(?i)video (?:is )?unavailable|content isn.t available|has been removed` +
			`|no longer available|does not exist|not found|deleted|incomplete youtube id`),
		"Bài đăng không còn tồn tại trên nền tảng",
		true,
	},
	{
		regexp.MustCompile(`(?i)not available in your country|geo.?restricted|blocked it in your country`),
		"Bài đăng bị chặn ở khu vực của máy chủ",
		true,
	},
	{
		regexp.MustCompile(`(?i)no video|there.s no video|unable to extract video|no media found|cannot parse data`),
		"Link này không có video/audio để tách — dùng hình thức B hoặc C để đọc phần text",
		true,
	},
	{
		regexp.MustCompile(`(?i)unsupported url`),
		"yt-dlp không hỗ trợ dạng link này",
		true,
	},
	{
		regexp.MustCompile(`(?i)HTTP Error 429|too many requests|rate.?limit`),
		"Nền tảng đang giới hạn truy cập — thử lại sau",
		false,
	},
	{
		regexp.MustCompile(`(?i)HTTP Error 4\d\d`),
		"Nền tảng từ chối yêu cầu tải bài này",
		true,
	},
	{
		regexp.MustCompile(`(?i)ffmpeg|ffprobe`),
		"Máy chủ thiếu ffmpeg để xử lý audio",
		false,
	},
	{
		regexp.MustCompile(`(?i)executable file not found|no such file or directory.*yt-dlp`),
		"Máy chủ chưa cài yt-dlp",
		false,
	},
	{
		regexp.MustCompile(`(?i)timeout|context deadline exceeded|timed out`),
		"Tải bài quá lâu và bị huỷ — thử lại sau",
		false,
	},
}

// explainYtDlp gắn câu giải thích ngắn vào lỗi của yt-dlp, và đánh dấu
// PermanentError với những loại lỗi retry cũng vô nghĩa (bài bị xoá, riêng tư,
// link không có video).
func explainYtDlp(err error) error {
	if err == nil {
		return nil
	}
	raw := err.Error()
	for _, e := range ytdlpErrors {
		if e.re.MatchString(raw) {
			explained := domain.Explain(e.msg, err)
			if e.permanent {
				return domain.Permanent(explained)
			}
			return explained
		}
	}
	return domain.Explain("Không tải được nội dung từ nền tảng", err)
}

// hasNoMedia cho biết lỗi là "link không có video/audio" — dùng để quyết định
// fallback sang đọc metadata/text của trang thay vì bỏ hẳn.
func hasNoMedia(err error) bool {
	if err == nil {
		return false
	}
	raw := strings.ToLower(err.Error())
	return strings.Contains(raw, "no video") ||
		strings.Contains(raw, "there's no video") ||
		strings.Contains(raw, "unable to extract video") ||
		strings.Contains(raw, "no media found")
}
