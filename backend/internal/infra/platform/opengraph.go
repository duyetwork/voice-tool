package platform

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// OpenGraph đọc metadata của 1 bài đăng bằng cách tải trang và parse thẻ
// <meta property="og:...">.
//
// Vì sao cần: yt-dlp chỉ hiểu bài CÓ media. Bài viết dạng text, bài chỉ có ảnh
// (rất phổ biến trên Facebook) làm nó thoát ngay với "no video", nên không lấy
// được cả tiêu đề. Thẻ Open Graph thì mọi nền tảng đều phát ra để link preview
// hoạt động, nên đây là đường lấy nội dung/ảnh bìa cho loại bài đó.
//
// Giới hạn cần biết: nền tảng dựng tường đăng nhập (Facebook với bài chỉ dành
// cho người đã đăng nhập, Instagram) sẽ trả về trang login và không có thẻ og —
// lúc đó Fetch trả lỗi và lỗi gốc của yt-dlp mới là lỗi được báo.
type OpenGraph struct {
	client *http.Client
}

// ogMaxBytes: thẻ og nằm trong <head>, không cần đọc hết trang. 512KB là dư
// cho phần head của mọi nền tảng và chặn được trang khổng lồ.
const ogMaxBytes = 512 << 10

// ogUserAgent: dùng UA của trình duyệt thật. Nhiều nền tảng trả trang rỗng
// hoặc redirect sang login khi thấy UA lạ.
const ogUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/124.0 Safari/537.36"

func NewOpenGraph(timeout time.Duration) *OpenGraph {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &OpenGraph{client: &http.Client{Timeout: timeout}}
}

// ogTagRe khớp thẻ meta theo cả 2 thứ tự thuộc tính (property trước content và
// ngược lại) vì không nền tảng nào thống nhất.
var ogTagRe = regexp.MustCompile(
	`(?is)<meta[^>]+(?:property|name)\s*=\s*["']([^"']+)["'][^>]+content\s*=\s*["']([^"']*)["']` +
		`|<meta[^>]+content\s*=\s*["']([^"']*)["'][^>]+(?:property|name)\s*=\s*["']([^"']+)["']`)

var titleTagRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// Fetch tải trang và rút metadata. Không có thẻ nào dùng được thì trả lỗi để
// caller giữ nguyên lỗi gốc thay vì lưu 1 bản ghi rỗng.
func (o *OpenGraph) Fetch(ctx context.Context, rawURL string) (domain.PostMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return domain.PostMetadata{}, fmt.Errorf("tạo request %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", ogUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "vi,en;q=0.9")

	res, err := o.client.Do(req)
	if err != nil {
		return domain.PostMetadata{}, fmt.Errorf("tải trang %s: %w", rawURL, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return domain.PostMetadata{}, fmt.Errorf("tải trang %s: HTTP %d", rawURL, res.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(res.Body, ogMaxBytes))
	if err != nil {
		return domain.PostMetadata{}, fmt.Errorf("đọc trang %s: %w", rawURL, err)
	}

	meta := parseOpenGraph(string(body))
	if meta.Title == "" && meta.ThumbnailURL == "" {
		return domain.PostMetadata{}, fmt.Errorf("trang %s không có thẻ Open Graph dùng được", rawURL)
	}
	return meta, nil
}

// parseOpenGraph tách phần parse ra khỏi phần tải để test được không cần mạng.
func parseOpenGraph(page string) domain.PostMetadata {
	tags := map[string]string{}
	for _, m := range ogTagRe.FindAllStringSubmatch(page, -1) {
		key, value := m[1], m[2]
		if key == "" {
			// Nhánh thứ 2 của regex: content đứng trước property.
			key, value = m[4], m[3]
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if _, seen := tags[key]; !seen {
			tags[key] = strings.TrimSpace(html.UnescapeString(value))
		}
	}

	pick := func(keys ...string) string {
		for _, k := range keys {
			if v := tags[k]; v != "" {
				return v
			}
		}
		return ""
	}

	title := pick("og:title", "twitter:title")
	if title == "" {
		if m := titleTagRe.FindStringSubmatch(page); len(m) == 2 {
			title = strings.TrimSpace(html.UnescapeString(m[1]))
		}
	}
	description := pick("og:description", "twitter:description", "description")

	// Tiêu đề của Facebook/Instagram cũng là <title> của trang nên dính số liệu
	// tương tác, và og:description mới là caption đầy đủ — PostContent lo cả hai
	// việc đó để ra đúng nội dung bài (xem title.go).
	content := PostContent(title, description)

	return domain.PostMetadata{
		Title:        content,
		Hashtags:     domain.ExtractHashtags(title+"\n"+description, nil),
		ThumbnailURL: pick("og:image", "og:image:secure_url", "twitter:image"),
		AuthorName:   pick("og:site_name", "author", "article:author"),
		PostedAt:     parseOGTime(pick("article:published_time", "og:updated_time")),
	}
}

func parseOGTime(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return &t
		}
	}
	return nil
}
