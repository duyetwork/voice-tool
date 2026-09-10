package platform

import (
	"slices"
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

func TestOtherPlatformsDetectAndExtract(t *testing.T) {
	cases := []struct {
		name        string
		adapter     *YtDlpAdapter
		url         string
		contentType string
		postID      string
	}{
		{"facebook reel", NewFacebook(nil, ""),
			"https://www.facebook.com/reel/1234567890", domain.ContentReel, "1234567890"},
		{"facebook watch", NewFacebook(nil, ""),
			"https://www.facebook.com/watch?v=987654321", domain.ContentVideo, "987654321"},
		{"facebook videos", NewFacebook(nil, ""),
			"https://www.facebook.com/vtv24/videos/555000111", domain.ContentVideo, "555000111"},
		{"tiktok video", NewTikTok(nil, ""),
			"https://www.tiktok.com/@user/video/7300000000000000000", domain.ContentVideo, "7300000000000000000"},
		{"tiktok rút gọn", NewTikTok(nil, ""),
			"https://vm.tiktok.com/ZSabcdef/", domain.ContentVideo, "ZSabcdef"},
		{"instagram reel", NewInstagram(nil, ""),
			"https://www.instagram.com/reel/CxYzAbCdEfG/", domain.ContentReel, "CxYzAbCdEfG"},
		{"instagram post", NewInstagram(nil, ""),
			"https://www.instagram.com/p/CxYzAbCdEfG/", domain.ContentPost, "CxYzAbCdEfG"},
		{"x status", NewX(nil, ""),
			"https://x.com/user/status/1700000000000000000", domain.ContentTweet, "1700000000000000000"},
		{"twitter.com vẫn nhận", NewX(nil, ""),
			"https://twitter.com/user/status/1700000000000000000", domain.ContentTweet, "1700000000000000000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.adapter.DetectPlatform(tc.url) {
				t.Fatalf("DetectPlatform(%q) = false, muốn true", tc.url)
			}
			ct, id, err := tc.adapter.ExtractID(tc.url)
			if err != nil {
				t.Fatalf("ExtractID(%q) lỗi: %v", tc.url, err)
			}
			if ct != tc.contentType || id != tc.postID {
				t.Errorf("ExtractID(%q) = (%q, %q), muốn (%q, %q)",
					tc.url, ct, id, tc.contentType, tc.postID)
			}
		})
	}
}

// Registry phải phân nền tảng đúng, không để adapter này nuốt URL của adapter kia.
func TestRegistryResolvesEachPlatform(t *testing.T) {
	r := NewRegistry(
		NewYouTube(nil, ""),
		NewFacebook(nil, ""),
		NewTikTok(nil, ""),
		NewInstagram(nil, ""),
		NewX(nil, ""),
	)

	cases := map[string]domain.Platform{
		"https://youtu.be/dQw4w9WgXcQ":                        domain.PlatformYouTube,
		"https://fb.watch/abc123/":                            domain.PlatformFacebook,
		"https://www.tiktok.com/@user/video/7300000000000000": domain.PlatformTikTok,
		"https://www.instagram.com/reel/CxYzAbCdEfG/":         domain.PlatformInstagram,
		"https://x.com/user/status/1700000000000000000":       domain.PlatformX,
	}

	for url, want := range cases {
		adapter, err := r.Resolve(url)
		if err != nil {
			t.Errorf("Resolve(%q) lỗi: %v", url, err)
			continue
		}
		if adapter.Name() != want {
			t.Errorf("Resolve(%q) = %q, muốn %q", url, adapter.Name(), want)
		}
	}

	if _, err := r.Resolve("https://vimeo.com/12345"); err == nil {
		t.Error("URL không hỗ trợ phải trả lỗi, không được đoán mò")
	}
}

// URL lạ của chính nền tảng đó vẫn phải qua được: yt-dlp mới là bên quyết định
// tải được hay không, không phải regex của mình.
func TestExtractIDFallback(t *testing.T) {
	fb := NewFacebook(nil, "")
	_, id, err := fb.ExtractID("https://www.facebook.com/permalink/some-new-format")
	if err != nil {
		t.Fatalf("ExtractID lỗi: %v", err)
	}
	if id != "some-new-format" {
		t.Errorf("ExtractID fallback = %q, muốn %q", id, "some-new-format")
	}
}

func TestExtractHashtags(t *testing.T) {
	got := domain.ExtractHashtags("Tin nóng hôm nay #tinnong #ViệtNam #tinnong", nil)
	want := []string{"#tinnong", "#ViệtNam"}
	if !slices.Equal(got, want) {
		t.Errorf("ExtractHashtags() = %v, muốn %v (bỏ trùng, giữ thứ tự)", got, want)
	}

	// Không có hashtag trong text -> fallback tags của nền tảng.
	got = domain.ExtractHashtags("Video không có hashtag", []string{"tin tuc", "thoi su"})
	want = []string{"#tintuc", "#thoisu"}
	if !slices.Equal(got, want) {
		t.Errorf("ExtractHashtags(fallback tags) = %v, muốn %v", got, want)
	}

	if got := domain.ExtractHashtags("", nil); len(got) != 0 {
		t.Errorf("ExtractHashtags(rỗng) = %v, muốn rỗng", got)
	}
}

func TestStripHashtags(t *testing.T) {
	in := "Tin nóng hôm nay\n\nXem thêm tại đây\n#tinnong #vietnam #24h"
	want := "Tin nóng hôm nay\nXem thêm tại đây"
	if got := domain.StripHashtags(in); got != want {
		t.Errorf("StripHashtags() = %q, muốn %q", got, want)
	}

	// Hashtag nằm giữa câu cũng bị bỏ, phần chữ còn lại giữ nguyên.
	if got := domain.StripHashtags("Bản tin #thoisu buổi sáng"); got != "Bản tin buổi sáng" {
		t.Errorf("StripHashtags(giữa câu) = %q", got)
	}

	// Không có hashtag thì giữ nguyên nội dung.
	if got := domain.StripHashtags("Mô tả bình thường"); got != "Mô tả bình thường" {
		t.Errorf("StripHashtags(không có thẻ) = %q", got)
	}
}

// Link share và link rút gọn của từng nền tảng phải nhận đúng nền tảng + loại
// nội dung — đây là dạng người dùng dán vào nhiều nhất (copy từ app mobile).
func TestShareAndShortLinks(t *testing.T) {
	cases := []struct {
		name        string
		adapter     *YtDlpAdapter
		url         string
		contentType string
		postID      string
	}{
		{"facebook share bài viết", NewFacebook(nil, ""),
			"https://www.facebook.com/share/p/1GoKsA89zx/", domain.ContentPost, "1GoKsA89zx"},
		{"facebook share reel", NewFacebook(nil, ""),
			"https://www.facebook.com/share/r/1M2uz5WSvK/", domain.ContentReel, "1M2uz5WSvK"},
		{"facebook share video", NewFacebook(nil, ""),
			"https://www.facebook.com/share/v/abcDEF123/", domain.ContentVideo, "abcDEF123"},
		{"facebook bài viết /posts/", NewFacebook(nil, ""),
			"https://www.facebook.com/vtv24/posts/pfbid0uGgkjVMgKty", domain.ContentPost, "0uGgkjVMgKty"},
		{"facebook permalink.php", NewFacebook(nil, ""),
			"https://www.facebook.com/permalink.php?story_fbid=123456&id=999", domain.ContentPost, "123456"},
		{"facebook ảnh fbid", NewFacebook(nil, ""),
			"https://www.facebook.com/photo/?fbid=987654321", domain.ContentPost, "987654321"},
		{"facebook post trong group", NewFacebook(nil, ""),
			"https://www.facebook.com/groups/123456/posts/789012/", domain.ContentPost, "789012"},
		{"facebook /watch/?v=", NewFacebook(nil, ""),
			"https://www.facebook.com/watch/?v=555000111", domain.ContentVideo, "555000111"},
		{"fb.watch rút gọn", NewFacebook(nil, ""),
			"https://fb.watch/abcXYZ-9/", domain.ContentVideo, "abcXYZ-9"},
		{"tiktok /t/ rút gọn", NewTikTok(nil, ""),
			"https://www.tiktok.com/t/ZSabcdef/", domain.ContentVideo, "ZSabcdef"},
		{"tiktok photo", NewTikTok(nil, ""),
			"https://www.tiktok.com/@user/photo/7300000000000000001", domain.ContentPost, "7300000000000000001"},
		{"instagram share reel", NewInstagram(nil, ""),
			"https://www.instagram.com/share/reel/CxYzAbCdEfG/", domain.ContentReel, "CxYzAbCdEfG"},
		{"instagram share post", NewInstagram(nil, ""),
			"https://www.instagram.com/share/p/CxYzAbCdEfG/", domain.ContentPost, "CxYzAbCdEfG"},
		{"x /i/web/status/", NewX(nil, ""),
			"https://x.com/i/web/status/1700000000000000000", domain.ContentTweet, "1700000000000000000"},
		{"t.co rút gọn", NewX(nil, ""),
			"https://t.co/aBcDeF1234", domain.ContentTweet, "aBcDeF1234"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.adapter.DetectPlatform(tc.url) {
				t.Fatalf("DetectPlatform(%q) = false, muốn true", tc.url)
			}
			ct, id, err := tc.adapter.ExtractID(tc.url)
			if err != nil {
				t.Fatalf("ExtractID(%q) lỗi: %v", tc.url, err)
			}
			if ct != tc.contentType || id != tc.postID {
				t.Errorf("ExtractID(%q) = (%q, %q), muốn (%q, %q)",
					tc.url, ct, id, tc.contentType, tc.postID)
			}
		})
	}
}
