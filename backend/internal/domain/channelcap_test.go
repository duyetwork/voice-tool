package domain

import "testing"

// Trần số bài mỗi lượt quét là con số CHẶN Ô NHẬP ở form Thêm kênh, nên nó phải
// là một nguồn sự thật duy nhất và đúng. Đặt sai thì người dùng khai một con số
// nền tảng không bao giờ trả về, rồi đi tìm lỗi ở chỗ khác.
func TestMaxChannelPosts(t *testing.T) {
	tests := []struct {
		platform Platform
		want     int
	}{
		// Ba nền tảng chạy bằng via — không phân trang được, xem channelPostCap.
		{PlatformFacebook, 10},
		{PlatformInstagram, 12},
		// 20 là số đo trên PHIÊN THẬT. Gọi không cookies thì endpoint trả một
		// bản đệm ~100 bài — đừng để con số đó lọt vào đây.
		{PlatformX, 20},
		// yt-dlp phân trang được nên không có trần.
		{PlatformYouTube, 0},
		{PlatformTikTok, 0},
	}

	for _, tc := range tests {
		t.Run(string(tc.platform), func(t *testing.T) {
			got, reason := MaxChannelPosts(tc.platform)
			if got != tc.want {
				t.Errorf("MaxChannelPosts(%s) = %d, muốn %d", tc.platform, got, tc.want)
			}
			// Có trần thì PHẢI có lý do: người dùng bị chặn ở một con số mà
			// không ai giải thích sẽ đi tìm lỗi ở chỗ khác.
			if tc.want > 0 && reason == "" {
				t.Errorf("%s có trần %d nhưng không kèm lý do", tc.platform, got)
			}
		})
	}
}

func TestClampChannelLimit(t *testing.T) {
	tests := []struct {
		name     string
		platform Platform
		limit    int
		want     int
	}{
		{"vượt trần thì bị ép xuống", PlatformInstagram, 50, 12},
		{"đúng trần thì giữ nguyên", PlatformInstagram, 12, 12},
		{"dưới trần thì giữ nguyên", PlatformInstagram, 5, 5},
		{"nền tảng không có trần thì không đụng vào", PlatformYouTube, 200, 200},
		{"0 không bị đẩy lên", PlatformX, 0, 0},
		{"nền tảng lạ coi như không có trần", Platform("threads"), 999, 999},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClampChannelLimit(tc.platform, tc.limit); got != tc.want {
				t.Errorf("ClampChannelLimit(%s, %d) = %d, muốn %d",
					tc.platform, tc.limit, got, tc.want)
			}
		})
	}
}
