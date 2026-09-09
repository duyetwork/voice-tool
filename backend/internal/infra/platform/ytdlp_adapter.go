package platform

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// YtDlpAdapter là adapter chung cho các nền tảng mà yt-dlp tải trực tiếp từ URL
// gốc: Facebook, TikTok, Instagram, X. Khác YouTube ở chỗ KHÔNG dựng lại được
// URL từ ID (TikTok cần cả @username, Facebook cần cả tên page...), nên luôn
// fetch bằng đúng URL người dùng đưa vào.
type YtDlpAdapter struct {
	name     domain.Platform
	hostRe   *regexp.Regexp
	patterns []idPattern
	core     ytdlpCore
	// channelSuffix nối thêm khi quét kênh (ví dụ TikTok không cần, Facebook
	// dùng thẳng URL page).
	channelSuffix string
}

type idPattern struct {
	contentType string
	re          *regexp.Regexp
}

var _ domain.PlatformAdapter = (*YtDlpAdapter)(nil)

func (a *YtDlpAdapter) Name() domain.Platform { return a.name }

func (a *YtDlpAdapter) DetectPlatform(rawURL string) bool {
	host, _, ok := splitHostPath(rawURL)
	if !ok {
		return false
	}
	return a.hostRe.MatchString(host)
}

// ExtractID parse loại nội dung + ID bài. Các nền tảng này đổi định dạng URL
// khá thường xuyên, nên không khớp pattern nào thì vẫn cho qua bằng ID suy ra
// từ chính URL — yt-dlp mới là bên quyết định tải được hay không.
func (a *YtDlpAdapter) ExtractID(rawURL string) (string, string, error) {
	for _, p := range a.patterns {
		if m := p.re.FindStringSubmatch(rawURL); len(m) == 2 {
			return p.contentType, m[1], nil
		}
	}
	id := fallbackID(rawURL)
	if id == "" {
		return "", "", fmt.Errorf("%w: %s", domain.ErrUnsupportedType, rawURL)
	}
	return domain.ContentVideo, id, nil
}

func (a *YtDlpAdapter) FetchContent(
	ctx context.Context,
	ref domain.PostRef,
	mode domain.CollectMode,
) (domain.FetchedContent, error) {
	if strings.TrimSpace(ref.URL) == "" {
		return domain.FetchedContent{}, domain.Permanent(fmt.Errorf(
			"%w: nền tảng %s cần URL gốc của bài, không dựng lại được từ ID",
			domain.ErrInvalidInput, a.name))
	}
	contentType, _, err := a.ExtractID(ref.URL)
	if err != nil {
		contentType = domain.ContentVideo
	}
	return a.core.fetch(ctx, ref.URL, ref.PostID, contentType, mode)
}

func (a *YtDlpAdapter) FetchLatestPosts(ctx context.Context, channelURL string, limit int) ([]domain.RemotePost, error) {
	target := strings.TrimRight(strings.TrimSpace(channelURL), "/") + a.channelSuffix
	posts, err := a.core.latestPosts(ctx, target, limit)
	if err != nil {
		return nil, err
	}
	for i := range posts {
		if posts[i].URL == "" {
			// Không có URL thì bài này không xử lý lại được -> bỏ ID để scan
			// coi như không hợp lệ.
			posts[i].PostID = ""
		}
		posts[i].ContentType = domain.ContentVideo
	}
	return posts, nil
}

// fallbackID lấy ID từ path khi URL không khớp pattern nào: ưu tiên đoạn path
// cuối cùng, cuối cùng mới băm URL để vẫn có khoá chống trùng ổn định.
func fallbackID(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if v := u.Query().Get("v"); v != "" {
		return v
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if p := parts[i]; p != "" && !strings.Contains(p, ".") {
			return p
		}
	}
	if u.Host == "" {
		return ""
	}
	sum := sha1.Sum([]byte(u.Host + u.Path))
	return hex.EncodeToString(sum[:8])
}

// ---------------------------------------------------------------------------
// Các nền tảng cụ thể
// ---------------------------------------------------------------------------

func NewFacebook(runner CommandRunner, tempDir string) *YtDlpAdapter {
	return &YtDlpAdapter{
		name:   domain.PlatformFacebook,
		hostRe: regexp.MustCompile(`(?i)^(www\.|m\.|web\.|mbasic\.)?(facebook\.com|fb\.com|fb\.watch)$`),
		patterns: []idPattern{
			{domain.ContentReel, regexp.MustCompile(`(?i)/reel/(\d+)`)},
			{domain.ContentVideo, regexp.MustCompile(`(?i)/videos/(?:[^/]+/)?(\d+)`)},
			{domain.ContentVideo, regexp.MustCompile(`(?i)[?&]v=(\d+)`)},
			{domain.ContentPost, regexp.MustCompile(`(?i)[?&]story_fbid=(\d+)`)},
			{domain.ContentVideo, regexp.MustCompile(`(?i)fb\.watch/([\w-]+)`)},
			{domain.ContentVideo, regexp.MustCompile(`(?i)/share/[rv]/([\w-]+)`)},
		},
		core:          newCore(runner, tempDir),
		channelSuffix: "/videos",
	}
}

func NewTikTok(runner CommandRunner, tempDir string) *YtDlpAdapter {
	return &YtDlpAdapter{
		name:   domain.PlatformTikTok,
		hostRe: regexp.MustCompile(`(?i)^(www\.|m\.|vm\.|vt\.)?tiktok\.com$`),
		patterns: []idPattern{
			{domain.ContentVideo, regexp.MustCompile(`(?i)/video/(\d+)`)},
			{domain.ContentPost, regexp.MustCompile(`(?i)/photo/(\d+)`)},
			// Link rút gọn vm.tiktok.com/XXXX — yt-dlp tự resolve redirect.
			{domain.ContentVideo, regexp.MustCompile(`(?i)tiktok\.com/(?:t/)?([A-Za-z0-9]{6,})/?$`)},
		},
		core: newCore(runner, tempDir),
	}
}

func NewInstagram(runner CommandRunner, tempDir string) *YtDlpAdapter {
	return &YtDlpAdapter{
		name:   domain.PlatformInstagram,
		hostRe: regexp.MustCompile(`(?i)^(www\.)?instagram\.com$`),
		patterns: []idPattern{
			{domain.ContentReel, regexp.MustCompile(`(?i)/reels?/([\w-]+)`)},
			{domain.ContentPost, regexp.MustCompile(`(?i)/p/([\w-]+)`)},
			{domain.ContentVideo, regexp.MustCompile(`(?i)/tv/([\w-]+)`)},
			{domain.ContentStory, regexp.MustCompile(`(?i)/stories/[^/]+/(\d+)`)},
		},
		core:          newCore(runner, tempDir),
		channelSuffix: "/reels",
	}
}

func NewX(runner CommandRunner, tempDir string) *YtDlpAdapter {
	return &YtDlpAdapter{
		name:   domain.PlatformX,
		hostRe: regexp.MustCompile(`(?i)^(www\.|mobile\.)?(x\.com|twitter\.com)$`),
		patterns: []idPattern{
			{domain.ContentTweet, regexp.MustCompile(`(?i)/status(?:es)?/(\d+)`)},
		},
		core: newCore(runner, tempDir),
	}
}
