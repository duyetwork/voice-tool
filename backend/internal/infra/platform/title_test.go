package platform

import (
	"strings"
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// Bài kiểu caption (Facebook, TikTok, Instagram, X, Open Graph): nội dung Bài
// Post là TOÀN BỘ caption, trừ hashtag.
func TestPostContent(t *testing.T) {
	cases := []struct {
		name        string
		title       string
		description string
		want        string
	}{
		{
			name:  "facebook bỏ số liệu tương tác và tên nền tảng",
			title: "42K views · 824 reactions | Tin nóng: cháy lớn ở Hà Nội | Kênh ABC | Facebook",
			want:  "Tin nóng: cháy lớn ở Hà Nội",
		},
		{
			name:  "facebook nhiều loại số liệu",
			title: "1.2M views · 3.4K reactions · 512 comments · 89 shares | Caption thật",
			want:  "Caption thật",
		},
		{
			name:        "tiêu đề chỉ có số liệu thì lấy nguyên caption",
			title:       "42K views · 824 reactions",
			description: "#tinnong\nCháy lớn tại khu công nghiệp sáng nay.\nChi tiết bên dưới.",
			want:        "Cháy lớn tại khu công nghiệp sáng nay.\nChi tiết bên dưới.",
		},
		{
			// Đây là điểm khác cốt lõi so với trước: KHÔNG cắt còn 1 dòng.
			// StripHashtags gộp các dòng trống liên tiếp nên đoạn cách nhau
			// bằng dòng trắng về còn 1 lần xuống dòng — nội dung vẫn đủ.
			name:        "giữ trọn caption nhiều dòng",
			title:       "Bản tin sáng nay",
			description: "Bản tin sáng nay\n\nCháy lớn tại KCN.\nCảnh sát đang phong toả.",
			want:        "Bản tin sáng nay\nCháy lớn tại KCN.\nCảnh sát đang phong toả.",
		},
		{
			name:        "tiêu đề bị cắt cụt của caption thì lấy caption",
			title:       "Cháy lớn tại khu công nghiệp sáng nay, hàng…",
			description: "Cháy lớn tại khu công nghiệp sáng nay, hàng trăm người phải sơ tán.",
			want:        "Cháy lớn tại khu công nghiệp sáng nay, hàng trăm người phải sơ tán.",
		},
		{
			name:        "tiêu đề và caption là 2 mảnh khác nhau thì giữ cả hai",
			title:       "Tin nóng hôm nay",
			description: "Chi tiết bản tin sáng nay.",
			want:        "Tin nóng hôm nay\n\nChi tiết bản tin sáng nay.",
		},
		{
			// multime nhận hashtag qua trường `hashtags` riêng — để lại trong
			// nội dung là lặp và ăn hết giới hạn ký tự của tiêu đề Voice.
			name:  "bỏ hashtag khỏi nội dung",
			title: "HIẾU SẠCH GÀU VỚI CLEAR PRO #ClearScalpceuticalspro #ClearPro",
			want:  "HIẾU SẠCH GÀU VỚI CLEAR PRO",
		},
		{
			name:        "tiêu đề toàn hashtag thì lấy caption",
			title:       "#tinnong #khancap",
			description: "Bản tin sáng nay.",
			want:        "Bản tin sáng nay.",
		},
		{
			name:  "gộp khoảng trắng thừa",
			title: "  Tin   nóng  ",
			want:  "Tin nóng",
		},
		{
			name:        "không có tiêu đề thì lấy caption",
			title:       "",
			description: "Bản tin sáng nay.",
			want:        "Bản tin sáng nay.",
		},
		{
			name:  "số liệu tiếng Việt",
			title: "42 N lượt xem · 824 bình luận | Caption tiếng Việt",
			want:  "Caption tiếng Việt",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PostContent(tc.title, tc.description); got != tc.want {
				t.Errorf("PostContent(%q, %q) = %q, muốn %q", tc.title, tc.description, got, tc.want)
			}
		})
	}
}

// YouTube: tiêu đề video là nội dung bài, phần mô tả bên dưới (link, timestamp)
// không được đăng lại.
func TestPostContentTitled(t *testing.T) {
	const desc = "Đăng ký kênh: https://youtube.com/@kenh\n00:00 Mở đầu"

	if got := PostContentTitled("Tin nóng 24h | VTV24", desc); got != "Tin nóng 24h | VTV24" {
		t.Errorf("tiêu đề YouTube phải giữ nguyên, được %q", got)
	}
	if got := PostContentTitled("", "Nội dung duy nhất còn lại."); got != "Nội dung duy nhất còn lại." {
		t.Errorf("không có tiêu đề thì phải rơi về mô tả, được %q", got)
	}
}

// Nội dung dài bị cắt ở ranh giới từ, không cắt giữa chữ.
func TestPostContentCắtTheoGiớiHạn(t *testing.T) {
	long := strings.Repeat("Tin nong hom nay ", 400) // ~6800 ký tự
	got := PostContent(long, "")

	if n := len([]rune(got)); n > domain.MaxPostTitleRunes+1 {
		t.Errorf("nội dung dài %d rune, muốn <= %d", n, domain.MaxPostTitleRunes+1)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("nội dung bị cắt phải có dấu …, được %q", got[len(got)-10:])
	}
	if strings.HasSuffix(strings.TrimSuffix(got, "…"), " ") {
		t.Errorf("không được cắt để lại khoảng trắng cuối: %q", got)
	}
}

// Tiêu đề Voice là nội dung Bài Post gộp về 1 dòng, cắt đúng giới hạn multime.
func TestVoiceTitleTừNộiDungBàiPost(t *testing.T) {
	content := PostContent("Bản tin sáng", "Bản tin sáng\n\n"+strings.Repeat("chi tiết ", 60))

	got := domain.VoiceTitle(content)
	if n := len([]rune(got)); n > domain.MaxVoiceTitleRunes+1 {
		t.Errorf("tiêu đề voice dài %d rune, muốn <= %d", n, domain.MaxVoiceTitleRunes+1)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("tiêu đề voice phải gộp về 1 dòng, được %q", got)
	}
	if !strings.HasPrefix(got, "Bản tin sáng") {
		t.Errorf("tiêu đề voice phải bắt đầu từ đầu nội dung, được %q", got)
	}
}
