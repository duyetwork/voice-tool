package platform

import (
	"errors"
	"strings"
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// Lỗi lưu vào last_error phải là 1 câu đọc được, không phải nguyên văn stderr
// của yt-dlp.
func TestExplainYtDlp(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantMsg   string
		permanent bool
	}{
		{
			name: "bài yêu cầu đăng nhập",
			raw: "yt-dlp: exit status 1: ERROR: [facebook] pfbid0uGgkjVMgKty: This video is " +
				"only available for registered users. Use --cookies-from-browser or --cookies " +
				"for the authentication.",
			wantMsg:   "Bài này bị nền tảng chặn, phải đăng nhập mới xem được — cần cấu hình cookies cho yt-dlp",
			permanent: true,
		},
		{
			name:      "link không có video",
			raw:       "yt-dlp: exit status 1: ERROR: [facebook] 123: There's no video in this post",
			wantMsg:   "Link này không có video/audio để tách — dùng hình thức B hoặc C để đọc phần text",
			permanent: true,
		},
		{
			name:      "bài đã bị xoá",
			raw:       "yt-dlp: exit status 1: ERROR: [youtube] abc: Video unavailable",
			wantMsg:   "Bài đăng không còn tồn tại trên nền tảng",
			permanent: true,
		},
		{
			// yt-dlp viết "This video is unavailable" (có chữ "is") — bản
			// trước không khớp nên bị retry 3 lần vô nghĩa.
			name:      "video is unavailable",
			raw:       "yt-dlp: exit status 1: ERROR: [youtube] aqzXKE-bpKQ: This video is unavailable",
			wantMsg:   "Bài đăng không còn tồn tại trên nền tảng",
			permanent: true,
		},
		{
			name:      "ID video sai định dạng",
			raw:       "ERROR: [youtube:truncated_id] abc: Incomplete YouTube ID abc.",
			wantMsg:   "Bài đăng không còn tồn tại trên nền tảng",
			permanent: true,
		},
		{
			name:      "youtube chặn IP datacenter",
			raw:       "ERROR: [youtube] abc: Sign in to confirm you're not a bot.",
			wantMsg:   "Nền tảng đang chặn IP máy chủ — cần cấu hình cookies hoặc proxy cho yt-dlp",
			permanent: false,
		},
		{
			name:      "rate limit thì retry được",
			raw:       "ERROR: unable to download: HTTP Error 429: Too Many Requests",
			wantMsg:   "Nền tảng đang giới hạn truy cập — thử lại sau",
			permanent: false,
		},
		{
			name:      "lỗi lạ vẫn có câu chung",
			raw:       "yt-dlp: exit status 2: something nobody predicted",
			wantMsg:   "Không tải được nội dung từ nền tảng",
			permanent: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := explainYtDlp(errors.New(tc.raw))
			if got := domain.UserMessage(err); got != tc.wantMsg {
				t.Errorf("UserMessage = %q,\n muốn %q", got, tc.wantMsg)
			}
			if domain.IsPermanent(err) != tc.permanent {
				t.Errorf("IsPermanent = %v, muốn %v", domain.IsPermanent(err), tc.permanent)
			}
			// Nguyên văn lỗi phải còn trong chuỗi để log/debug được.
			if !strings.Contains(err.Error(), tc.raw) {
				t.Errorf("lỗi gốc bị mất: %v", err)
			}
		})
	}
}
