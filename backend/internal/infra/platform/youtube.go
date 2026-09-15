package platform

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// YouTube adapter — dùng yt-dlp làm backend fetch (không cần API key cho
// caption/audio; YOUTUBE_API_KEY chỉ cần khi muốn quét kênh qua Data API).
type YouTube struct {
	core ytdlpCore
}

var _ domain.PlatformAdapter = (*YouTube)(nil)

func NewYouTube(runner CommandRunner, tempDir string, opts ...Option) *YouTube {
	return &YouTube{core: newCore(runner, tempDir, true).apply(opts)}
}

func (y *YouTube) Name() domain.Platform { return domain.PlatformYouTube }

var ytHostRe = regexp.MustCompile(
	`(?i)^(www\.|m\.|music\.)?(youtube\.com|youtube-nocookie\.com|youtu\.be)$`)

func (y *YouTube) DetectPlatform(rawURL string) bool {
	host, _, ok := splitHostPath(rawURL)
	if !ok {
		return false
	}
	return ytHostRe.MatchString(host)
}

// Các dạng URL YouTube cần nhận diện (specs 1.3): video, short, live, embed.
var ytPatterns = []struct {
	contentType string
	re          *regexp.Regexp
}{
	{domain.ContentVideo, regexp.MustCompile(`(?i)youtu\.be/([\w-]{11})`)},
	{domain.ContentVideo, regexp.MustCompile(`(?i)[?&]v=([\w-]{11})`)},
	{domain.ContentShort, regexp.MustCompile(`(?i)/shorts/([\w-]{11})`)},
	{domain.ContentVideo, regexp.MustCompile(`(?i)/live/([\w-]{11})`)},
	{domain.ContentVideo, regexp.MustCompile(`(?i)/embed/([\w-]{11})`)},
	{domain.ContentVideo, regexp.MustCompile(`(?i)/v/([\w-]{11})`)},
	{domain.ContentShort, regexp.MustCompile(`(?i)/short/([\w-]{11})`)},
}

func (y *YouTube) ExtractID(rawURL string) (string, string, error) {
	for _, p := range ytPatterns {
		if m := p.re.FindStringSubmatch(rawURL); len(m) == 2 {
			return p.contentType, m[1], nil
		}
	}
	return "", "", fmt.Errorf("%w: %s", domain.ErrUnsupportedType, rawURL)
}

func (y *YouTube) FetchContent(
	ctx context.Context,
	ref domain.PostRef,
	mode domain.CollectMode,
) (domain.FetchedContent, error) {
	// YouTube dựng lại được URL chuẩn từ ID, nên ID là nguồn tin cậy hơn URL
	// người dùng dán (có thể kèm playlist, timestamp...).
	return y.core.fetch(ctx, y.videoURL(ref), ref.PostID, domain.ContentVideo, mode)
}

func (y *YouTube) FetchMetadata(ctx context.Context, ref domain.PostRef) (domain.PostMetadata, error) {
	return y.core.metadata(ctx, y.videoURL(ref))
}

// videoURL dựng URL chuẩn từ ID: URL người dùng dán có thể kèm playlist,
// timestamp, tham số tracking.
func (y *YouTube) videoURL(ref domain.PostRef) string {
	if ref.PostID != "" {
		return "https://www.youtube.com/watch?v=" + ref.PostID
	}
	return ref.URL
}

func (y *YouTube) CheckChannelScan() error { return nil }

func (y *YouTube) FetchLatestPosts(ctx context.Context, channelURL string, limit int) ([]domain.RemotePost, error) {
	posts, err := y.core.latestPosts(ctx, normalizeChannelURL(channelURL), limit)
	if err != nil {
		return nil, err
	}
	// --flat-playlist không luôn trả webpage_url -> dựng lại từ ID.
	for i := range posts {
		if posts[i].URL == "" {
			posts[i].URL = "https://www.youtube.com/watch?v=" + posts[i].PostID
		}
		posts[i].ContentType = domain.ContentVideo
	}
	return posts, nil
}

// normalizeChannelURL: chấp nhận cả URL kênh và URL tab /videos.
func normalizeChannelURL(u string) string {
	trimmed := strings.TrimRight(u, "/")
	if strings.Contains(trimmed, "/videos") || strings.Contains(trimmed, "/playlist") {
		return trimmed
	}
	return trimmed + "/videos"
}
