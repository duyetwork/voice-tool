package platform

import (
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// ChannelScanSupport là thứ form Thêm kênh đọc để KHOÁ ô nhập. Thiếu `max_posts`
// ở đây thì ô nhập không có trần, người dùng lại khai 50, và ta quay về đúng
// chỗ cũ — chỉ khác là lần này không còn cả dòng cảnh báo.
func TestChannelScanSupportCarriesCap(t *testing.T) {
	runner := &fakeRunner{}
	r := NewRegistry(
		NewYouTube(runner, ""),
		NewFacebook(runner, ""),
		NewInstagram(runner, ""),
		NewX(runner, ""),
		NewTikTok(runner, ""),
	)

	got := map[domain.Platform]domain.ChannelScan{}
	for _, item := range r.ChannelScanSupport() {
		got[item.Platform] = item
	}

	for _, p := range domain.ScrapePlatforms {
		item, ok := got[p]
		if !ok {
			t.Fatalf("thiếu %s trong ChannelScanSupport", p)
		}
		want, reason := domain.MaxChannelPosts(p)
		if item.MaxPosts != want {
			t.Errorf("%s: max_posts = %d, muốn %d", p, item.MaxPosts, want)
		}
		if item.MaxPostsReason != reason {
			t.Errorf("%s: thiếu lý do của trần", p)
		}
	}

	// Nền tảng yt-dlp phân trang được thì KHÔNG được có trần — đặt nhầm một con
	// số ở đây là tự cắt ngắn thứ đang chạy tốt.
	for _, p := range []domain.Platform{domain.PlatformYouTube, domain.PlatformTikTok} {
		if item := got[p]; item.MaxPosts != 0 {
			t.Errorf("%s không được có trần, nhận %d", p, item.MaxPosts)
		}
	}
}
