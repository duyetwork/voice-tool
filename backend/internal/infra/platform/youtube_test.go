package platform

import (
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

func TestYouTubeDetectAndExtract(t *testing.T) {
	yt := NewYouTube(nil, "")

	cases := []struct {
		url         string
		contentType string
		postID      string
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", domain.ContentVideo, "dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ", domain.ContentVideo, "dQw4w9WgXcQ"},
		{"https://www.youtube.com/shorts/abcdefghijk", domain.ContentShort, "abcdefghijk"},
		{"https://m.youtube.com/watch?v=dQw4w9WgXcQ&t=30s", domain.ContentVideo, "dQw4w9WgXcQ"},
		{"https://www.youtube.com/live/abcdefghijk", domain.ContentVideo, "abcdefghijk"},
	}

	for _, tc := range cases {
		if !yt.DetectPlatform(tc.url) {
			t.Errorf("DetectPlatform(%q) = false, muốn true", tc.url)
			continue
		}
		ct, id, err := yt.ExtractID(tc.url)
		if err != nil {
			t.Errorf("ExtractID(%q) lỗi: %v", tc.url, err)
			continue
		}
		if ct != tc.contentType || id != tc.postID {
			t.Errorf("ExtractID(%q) = (%q, %q), muốn (%q, %q)", tc.url, ct, id, tc.contentType, tc.postID)
		}
	}
}

func TestYouTubeRejectsOtherPlatforms(t *testing.T) {
	yt := NewYouTube(nil, "")
	for _, url := range []string{
		"https://www.facebook.com/reel/123",
		"https://www.tiktok.com/@user/video/123",
		"không-phải-url",
	} {
		if yt.DetectPlatform(url) {
			t.Errorf("DetectPlatform(%q) = true, muốn false", url)
		}
	}
}

func TestRegistryResolveUnsupported(t *testing.T) {
	r := NewRegistry(NewYouTube(nil, ""))

	if _, err := r.Resolve("https://vimeo.com/12345"); err == nil {
		t.Error("URL không hỗ trợ phải trả lỗi, không được đoán mò")
	}
	if _, err := r.Resolve("https://youtu.be/dQw4w9WgXcQ"); err != nil {
		t.Errorf("Resolve URL youtube lỗi: %v", err)
	}
}

func TestParseVTT(t *testing.T) {
	raw := "WEBVTT\nKind: captions\nLanguage: vi\n\n" +
		"00:00:01.000 --> 00:00:03.000\nXin chào các bạn\n\n" +
		"00:00:03.000 --> 00:00:05.000\nXin chào các bạn\nhôm nay trời <c.colorE5E5E5>đẹp</c>\n"

	got := ParseVTT(raw)
	want := "Xin chào các bạn hôm nay trời đẹp"
	if got != want {
		t.Errorf("ParseVTT() = %q, muốn %q", got, want)
	}
}
