package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// Adapter quét TRANG Facebook
// ---------------------------------------------------------------------------
//
// yt-dlp lấy được từng bài Facebook nhưng KHÔNG có extractor nào liệt kê bài của
// một trang. Adapter này bọc quanh adapter yt-dlp sẵn có và chỉ thay đúng một
// việc: FetchLatestPosts. Mọi thứ còn lại (nhận diện URL, tách ID, tải nội dung,
// lấy metadata) vẫn đi qua yt-dlp vì phần đó đang chạy tốt.
//
// Bọc chứ không viết lại: một adapter Facebook thứ hai nghĩa là hai bộ regex
// nhận diện URL, và chúng sẽ lệch nhau ngay lần đầu Facebook đổi dạng link.

// FacebookScrapeAdapter = adapter yt-dlp + khả năng liệt kê trang bằng via.
type FacebookScrapeAdapter struct {
	// YtDlpAdapter nhúng trực tiếp: mọi phương thức không ghi đè ở dưới đều
	// dùng nguyên bản của nó.
	*YtDlpAdapter

	pool    domain.ScrapePool
	fetcher *ScrapeFetcher
}

var _ domain.PlatformAdapter = (*FacebookScrapeAdapter)(nil)

// NewFacebookScrape dựng adapter Facebook có khả năng quét cả trang.
//
// pool nil = không có hạ tầng via (dev, hoặc chưa cấu hình): lúc đó adapter
// hành xử y hệt bản yt-dlp thuần, tức là vẫn chặn quét kênh. Không tự bịa ra
// một đường chạy không có via, vì nó chắc chắn hỏng và hỏng một cách khó hiểu.
func NewFacebookScrape(
	base *YtDlpAdapter, pool domain.ScrapePool, fetcher *ScrapeFetcher,
) *FacebookScrapeAdapter {
	return &FacebookScrapeAdapter{YtDlpAdapter: base, pool: pool, fetcher: fetcher}
}

// CheckChannelScan mở khoá form Thêm kênh cho Facebook KHI VÀ CHỈ KHI có hạ
// tầng via.
//
// Không có via mà vẫn cho thêm kênh thì mỗi vòng quét là một job chắc chắn
// hỏng, retry ba lần, và trên giao diện kênh vẫn "Đang bật" — đúng thứ cờ
// noChannelScan sinh ra để tránh.
func (a *FacebookScrapeAdapter) CheckChannelScan() error {
	if a.pool == nil {
		return a.YtDlpAdapter.CheckChannelScan()
	}
	return nil
}

// FetchLatestPosts liệt kê bài mới nhất của một trang Facebook.
//
// Mỗi lần gọi = 1 đơn vị hạn mức của via, kể cả khi bên trong phải tải nhiều
// trang nối tiếp.
func (a *FacebookScrapeAdapter) FetchLatestPosts(
	ctx context.Context, channelURL string, limit int,
) ([]domain.RemotePost, error) {
	if a.pool == nil {
		return nil, a.YtDlpAdapter.CheckChannelScan()
	}
	if limit <= 0 {
		limit = 10
	}

	session, err := a.pool.Acquire(ctx, domain.PlatformFacebook)
	if err != nil {
		// Hết via KHÔNG phải sự cố — vòng quét bỏ qua kênh này và thử lại lượt
		// sau. Lỗi đã mang sẵn domain.ErrNoViaAvailable để chỗ gọi nhận ra.
		return nil, err
	}

	var posts []domain.RemotePost
	defer func() {
		a.pool.Release(ctx, session, domain.ScrapeOwner{Platform: domain.PlatformFacebook},
			len(posts), err)
	}()

	target, err := facebookPageURL(channelURL)
	if err != nil {
		return nil, err
	}

	body, err := a.fetcher.Get(ctx, session, target, map[string]string{
		// Referer của chính Facebook: request tới trang mà không có referer là
		// dấu hiệu của một công cụ, không phải người đang duyệt.
		"Referer": "https://www.facebook.com/",
	})
	if err != nil {
		return nil, err
	}

	posts = parseFacebookPage(string(body), limit)
	if len(posts) == 0 {
		// Không gắn nhãn chặn: trang đọc được, chỉ là không moi ra bài nào.
		// Gắn `bot_block` ở đây sẽ giết proxy vì một lần Facebook đổi giao diện.
		err = domain.Explain(
			"Đọc được trang nhưng không tách được bài nào — Facebook có thể đã đổi cấu trúc trang",
			fmt.Errorf("facebook: 0 bài từ %s", target))
		return nil, err
	}
	return posts, nil
}

// facebookPageURL chuẩn hoá URL trang về dạng mà Facebook trả HTML đầy đủ.
//
// Dùng `www.` chứ không `m.`/`mbasic.`: bản mobile nhẹ hơn và dễ phân tích hơn
// nhiều, nhưng Facebook đã bỏ mbasic và bản m. trả về nội dung rút gọn khác hẳn
// — chọn nó là tự chuốc lấy một bộ phân tích thứ hai.
func facebookPageURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", domain.Permanent(domain.Explain(
			"URL trang Facebook không hợp lệ",
			fmt.Errorf("%w: %s", domain.ErrUnsupportedURL, raw)))
	}
	u.Scheme, u.Host = "https", "www.facebook.com"
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

// ---------------------------------------------------------------------------
// Bộ phân tích trang
// ---------------------------------------------------------------------------
//
// ⚠️ ĐÂY LÀ PHẦN DUY NHẤT CHƯA ĐƯỢC KIỂM CHỨNG TRÊN DỮ LIỆU THẬT.
//
// Facebook nhúng nội dung trang trong các khối <script type="application/json">
// của bộ khung Relay, và hình dạng khối đó đổi theo từng bản triển khai — không
// có tài liệu, không có cam kết tương thích. Những gì dưới đây bám vào các khoá
// ổn định nhất quan sát được (`post_id`, `creation_time`, `message.text`), và
// vẫn phải đối chiếu với HTML thật của một trang thật trước khi tin.
//
// Vì lẽ đó, quy trình bắt buộc trước khi mở khoá Facebook cho người dùng:
//
//  1. Thêm via thật ở màn Cài đặt.
//  2. Chạy thử với vài trang thật, xem tab "Lịch sử quét" của kênh.
//  3. Chỉ khi số bài lấy về khớp với thực tế mới truyền pool vào adapter
//     (xem NewFacebookScrape) — chính việc đó mới gỡ cờ chặn quét kênh.
//
// Mở khoá trước khi đối chiếu thì người dùng tạo ra hàng loạt kênh im lặng
// không ra bài, và không có gì trên giao diện nói vì sao.

// fbJSONBlock bắt các khối JSON Relay nhúng trong trang.
var fbJSONBlock = regexp.MustCompile(`(?s)<script type="application/json"[^>]*>(.*?)</script>`)

// fbPostFields bắt một bài trong khối JSON: id bài + thời điểm đăng.
//
// Bám vào cặp (post_id, creation_time) vì đó là hai khoá đi cùng nhau bền nhất
// qua các bản triển khai — tên khối bao quanh chúng thì đổi liên tục.
var fbPostFields = regexp.MustCompile(
	`"post_id"\s*:\s*"(\d+)"|"creation_time"\s*:\s*(\d{9,})|"text"\s*:\s*"((?:[^"\\]|\\.)*)"`)

// parseFacebookPage moi danh sách bài ra khỏi HTML của trang.
//
// Trả về mới-nhất-trước, đúng hợp đồng mà service.Scan trông đợi.
func parseFacebookPage(body string, limit int) []domain.RemotePost {
	type draft struct {
		id   string
		text string
		at   int64
	}
	byID := map[string]*draft{}

	for _, block := range fbJSONBlock.FindAllStringSubmatch(body, -1) {
		raw := block[1]
		var cur *draft
		var pendingText string

		for _, m := range fbPostFields.FindAllStringSubmatch(raw, -1) {
			switch {
			case m[1] != "":
				cur = byID[m[1]]
				if cur == nil {
					cur = &draft{id: m[1]}
					byID[m[1]] = cur
				}
				// Đoạn text gặp NGAY TRƯỚC post_id thường là nội dung của chính
				// bài đó: Relay xếp `message` trước `post_id` trong cùng một nút.
				if cur.text == "" && pendingText != "" {
					cur.text = pendingText
				}
				pendingText = ""
			case m[2] != "" && cur != nil && cur.at == 0:
				if ts, err := strconv.ParseInt(m[2], 10, 64); err == nil {
					cur.at = ts
				}
			case m[3] != "":
				if decoded := decodeJSONString(m[3]); len(decoded) > len(pendingText) {
					pendingText = decoded
				}
			}
		}
	}

	drafts := make([]*draft, 0, len(byID))
	for _, d := range byID {
		drafts = append(drafts, d)
	}
	// Mới nhất trước. Bài không có thời điểm xếp cuối chứ không bị loại: thiếu
	// ngày không có nghĩa là không phải bài.
	sort.SliceStable(drafts, func(i, j int) bool { return drafts[i].at > drafts[j].at })

	out := make([]domain.RemotePost, 0, min(limit, len(drafts)))
	for _, d := range drafts {
		if len(out) >= limit {
			break
		}
		post := domain.RemotePost{
			PostID:      d.id,
			URL:         "https://www.facebook.com/" + d.id,
			ContentType: domain.ContentPost,
			Text:        d.text,
		}
		post.Meta.Title = d.text
		post.Meta.Hashtags = domain.ExtractHashtags(d.text, nil)
		if d.at > 0 {
			t := time.Unix(d.at, 0).UTC()
			post.Meta.PostedAt = &t
		}
		out = append(out, post)
	}
	return out
}

// decodeJSONString giải chuỗi đã escape trong JSON (\n, \uXXXX, ...).
//
// Đi qua encoding/json thay vì tự thay thế: chuỗi Facebook đầy emoji và ký tự
// unicode escape, và một bộ giải tự viết sẽ sai ở đúng những chỗ đó.
func decodeJSONString(raw string) string {
	var out string
	if err := json.Unmarshal([]byte(`"`+raw+`"`), &out); err != nil {
		return html.UnescapeString(raw)
	}
	return strings.TrimSpace(out)
}
