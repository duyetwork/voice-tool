package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// Adapter quét TÀI KHOẢN X (Twitter)
// ---------------------------------------------------------------------------
//
// yt-dlp có twitter, twitter:card, twitter:spaces, twitter:broadcast — không có
// cái nào đọc được dòng thời gian của một tài khoản. Phần LIỆT KÊ vì thế phải
// tự làm, giống Facebook và Instagram.
//
// VÌ SAO DÙNG syndication.twitter.com CHỨ KHÔNG PHẢI GraphQL CỦA x.com:
//
// Đường mà trang x.com tự dùng là `/i/api/graphql/<queryId>/UserTweets`, và nó
// đòi ba thứ rotate độc lập nhau — mã truy vấn trong chính URL, một khối
// `features` dài mà thiếu một khoá là 400, và bearer của web client. Cả ba đổi
// mà không báo trước, nên một adapter bám vào chúng sẽ hỏng vài tuần một lần và
// mỗi lần hỏng đều cần người sửa code, không phải sửa cấu hình.
//
// Endpoint syndication là đường X dựng cho widget nhúng tweet: một URL cố định,
// trả HTML có sẵn khối JSON của Next.js, không mã truy vấn, không features,
// không bearer. Đổi lại nó trả về MỘT LẦN khoảng 100 tweet và KHÔNG phân trang
// — không có tham số cursor nào, `count` bị bỏ qua (đã đo trực tiếp). Đó là
// trần cứng của cách này: xin nhiều hơn chừng đó bài cũ thì không có đường lấy.
//
// Chấp nhận được vì kênh nguồn theo thiết kế là trang công khai của người khác
// và mỗi vòng quét chỉ cần vài bài mới nhất; chỉ lần quét ĐẦU (lấy bài cũ) mới
// chạm trần, và FetchLatestPosts nói thẳng ra khi điều đó xảy ra.
//
// VẪN ĐI QUA VIA/PROXY dù endpoint này không đòi đăng nhập. Hai lý do: X chặn
// theo IP rất nhanh nên proxy là thứ thật sự cần, và ScrapePool là chỗ duy nhất
// đếm hạn mức/ghi via_usage_log — bỏ qua nó thì lượt quét X biến mất khỏi mọi
// bảng theo dõi. Hệ quả phải chấp nhận: người vận hành vẫn phải khai một via X
// để mở khoá, dù cookies của nó không được dùng để đọc danh sách.
//
// ⚠️ CHƯA ĐƯỢC KIỂM CHỨNG TRÊN DỮ LIỆU THẬT, như bộ phân tích Facebook. Đối
// chiếu với một tài khoản thật trước khi bật X_CHANNEL_SCAN.

// XScrapeAdapter = adapter yt-dlp + khả năng liệt kê dòng thời gian.
type XScrapeAdapter struct {
	*YtDlpAdapter

	pool    domain.ScrapePool
	fetcher *ScrapeFetcher
}

var _ domain.PlatformAdapter = (*XScrapeAdapter)(nil)

// NewXScrape dựng adapter X có khả năng quét cả tài khoản. pool nil = chưa có
// hạ tầng via, adapter vẫn chặn việc thêm kênh (xem NewFacebookScrape).
func NewXScrape(
	base *YtDlpAdapter, pool domain.ScrapePool, fetcher *ScrapeFetcher,
) *XScrapeAdapter {
	return &XScrapeAdapter{YtDlpAdapter: base, pool: pool, fetcher: fetcher}
}

func (a *XScrapeAdapter) CheckChannelScan() error {
	if a.pool == nil {
		return a.YtDlpAdapter.CheckChannelScan()
	}
	return nil
}

// FetchLatestPosts liệt kê tweet mới nhất của một tài khoản.
//
// Mỗi lần gọi = 1 đơn vị hạn mức của via.
func (a *XScrapeAdapter) FetchLatestPosts(
	ctx context.Context, channelURL string, limit int,
) ([]domain.RemotePost, error) {
	if a.pool == nil {
		return nil, a.YtDlpAdapter.CheckChannelScan()
	}
	if limit <= 0 {
		limit = 10
	}

	// Kiểm URL trước khi xin via — xem InstagramScrapeAdapter.FetchLatestPosts.
	handle, err := xScreenName(channelURL)
	if err != nil {
		return nil, err
	}

	session, err := a.pool.Acquire(ctx, domain.PlatformX)
	if err != nil {
		return nil, err
	}

	var posts []domain.RemotePost
	defer func() {
		a.pool.Release(ctx, session, domain.ScrapeOwner{Platform: domain.PlatformX},
			len(posts), err)
	}()

	target := "https://syndication.twitter.com/srv/timeline-profile/screen-name/" +
		url.PathEscape(handle) + "?showReplies=false"

	body, err := a.fetcher.Get(ctx, session, target, map[string]string{
		// Widget nhúng luôn đi kèm referer của trang nhúng nó; x.com là lựa
		// chọn vô hại và nhất quán nhất.
		"Referer": "https://x.com/",
	})
	if err != nil {
		return nil, err
	}

	posts, err = parseXTimeline(body, handle, limit)
	if err != nil {
		return nil, err
	}
	return posts, nil
}

// xScreenName tách @handle khỏi URL hồ sơ.
//
// Từ chối link tweet lẻ và các đường dẫn hệ thống (/i/, /home, /search...):
// dán link một tweet vào ô "kênh" là nhầm lẫn thường gặp, và nhận bừa nó thì
// kênh được tạo rồi lặng lẽ không ra bài nào.
func xScreenName(raw string) (string, error) {
	bad := func() error {
		return domain.Permanent(domain.Explain(
			"URL phải là trang tài khoản X, ví dụ https://x.com/tenkenh",
			fmt.Errorf("%w: %s", domain.ErrUnsupportedURL, raw)))
	}

	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", bad()
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", bad()
	}
	name := strings.TrimPrefix(parts[0], "@")
	switch strings.ToLower(name) {
	case "i", "home", "search", "explore", "notifications", "messages", "settings", "intent":
		return "", bad()
	}
	// Có phần thứ hai nghĩa là link tweet (/status/...) hoặc tab (/media),
	// không phải trang tài khoản.
	if len(parts) > 1 && parts[1] == "status" {
		return "", bad()
	}
	if !xHandleRe.MatchString(name) {
		return "", bad()
	}
	return name, nil
}

// xHandleRe — luật đặt tên tài khoản của X: chữ, số, gạch dưới, tối đa 15 ký tự.
var xHandleRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)

// ---------------------------------------------------------------------------
// Bộ phân tích
// ---------------------------------------------------------------------------

// xNextData bắt khối JSON mà Next.js nhúng vào trang syndication.
var xNextData = regexp.MustCompile(
	`(?s)<script id="__NEXT_DATA__" type="application/json"[^>]*>(.*?)</script>`)

// xTweet — đúng những trường của một tweet mà ta dùng.
//
// Kiểu có tên (không lồng ẩn danh) vì bộ so sánh lúc sắp xếp cần nhận nó làm
// tham số; xem xPostedAt.
type xTweet struct {
	IDStr     string `json:"id_str"`
	FullText  string `json:"full_text"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
	Permalink string `json:"permalink"`
	User      struct {
		ScreenName string `json:"screen_name"`
	} `json:"user"`
}

// xTimelinePage — đúng những nhánh của __NEXT_DATA__ mà ta dùng.
type xTimelinePage struct {
	Props struct {
		PageProps struct {
			Timeline struct {
				Entries []struct {
					Type    string `json:"type"`
					EntryID string `json:"entry_id"`
					Content struct {
						Tweet *xTweet `json:"tweet"`
					} `json:"content"`
				} `json:"entries"`
			} `json:"timeline"`
		} `json:"pageProps"`
	} `json:"props"`
}

// xCreatedAt — định dạng ngày của X, giữ nguyên từ thời Twitter API v1.1.
const xCreatedAt = "Mon Jan 02 15:04:05 -0700 2006"

// parseXTimeline moi danh sách tweet ra khỏi trang syndication.
//
// SẮP LẠI THEO THỜI GIAN ĐĂNG, mới nhất trước, TRƯỚC khi cắt `limit`. Mảng
// `entries` không bảo đảm theo thứ tự thời gian: đo trên @TF1Info ngày
// 17/09/2026, một biến thể phản hồi trả 100 tweet mà 5 phần tử đầu lần lượt từ
// 2022, 2020, 2025, 2025, 2022. Lấy N phần tử đầu của mảng khi gặp biến thể đó
// nghĩa là lấy một nhúm bài ngẫu nhiên rải suốt 10 năm, trong khi cả hệ thống
// được xây trên giả định "N bài MỚI NHẤT".
//
// KHÔNG dùng `sort_index` dù nó có mặt và trông như một khoá xếp hạng: giá trị
// của nó là snowflake của CHÍNH THỜI ĐIỂM TRẢ LỜI, giảm đúng 1 đơn vị cho mỗi
// phần tử (đã giải mã để kiểm). Nó chỉ đánh số vị trí trong mảng, không nói gì
// về tweet — sắp theo nó là một phép toán không làm gì cả.
//
// Khoá dùng được chỉ có hai: `created_at`, và `id_str` (snowflake của X, giải
// ra được thời điểm đăng) làm dự phòng khi created_at rỗng hoặc sai định dạng.
func parseXTimeline(body []byte, handle string, limit int) ([]domain.RemotePost, error) {
	block := xNextData.FindSubmatch(body)
	if block == nil {
		// Không có khối JSON = trang trả về không phải trang widget: tài khoản
		// bị khoá/không tồn tại, hoặc X đã đổi cách dựng trang. Không gắn nhãn
		// chặn — hai nguyên nhân đó đều không phải lỗi của proxy.
		return nil, domain.Explain(
			"Không đọc được dòng thời gian của @"+handle+
				" — tài khoản có thể ở chế độ riêng tư, đã bị khoá, hoặc X đã đổi cấu trúc trang",
			fmt.Errorf("x: không tìm thấy __NEXT_DATA__ cho @%s", handle))
	}

	var page xTimelinePage
	if err := json.Unmarshal(block[1], &page); err != nil {
		return nil, domain.Explain(
			"X trả về dữ liệu không đọc được — có thể họ đã đổi cấu trúc trang",
			fmt.Errorf("x: giải mã __NEXT_DATA__ của @%s: %w", handle, err))
	}

	entries := page.Props.PageProps.Timeline.Entries

	// Sắp TRƯỚC khi cắt. Cắt trước rồi sắp thì vẫn là cắt nhầm nhúm bài.
	//
	// SliceStable: hai tweet không đọc được thời gian thì giữ nguyên vị trí
	// tương đối của chúng, thay vì bị hoán đổi ngẫu nhiên giữa các lần chạy.
	sort.SliceStable(entries, func(i, j int) bool {
		return xPostedAt(entries[i].Content.Tweet) > xPostedAt(entries[j].Content.Tweet)
	})

	out := make([]domain.RemotePost, 0, min(limit, len(entries)))
	for _, e := range entries {
		if len(out) >= limit {
			break
		}
		// Dòng thời gian còn có entry quảng cáo và gợi ý theo dõi; chỉ lấy
		// đúng tweet thật.
		t := e.Content.Tweet
		if t == nil || t.IDStr == "" {
			continue
		}

		text := strings.TrimSpace(t.FullText)
		if text == "" {
			text = strings.TrimSpace(t.Text)
		}

		// Permalink của X là đường dẫn tương đối; tự ghép từ handle khi thiếu
		// để mọi bài đều có URL tuyệt đối mở được.
		link := "https://x.com/" + handle + "/status/" + t.IDStr
		if p := strings.TrimSpace(t.Permalink); strings.HasPrefix(p, "/") {
			link = "https://x.com" + p
		}

		post := domain.RemotePost{
			PostID:      t.IDStr,
			URL:         link,
			ContentType: domain.ContentTweet,
			Text:        text,
		}
		post.Meta.Title = text
		post.Meta.Hashtags = domain.ExtractHashtags(text, nil)
		if at, err := time.Parse(xCreatedAt, t.CreatedAt); err == nil {
			utc := at.UTC()
			post.Meta.PostedAt = &utc
		}
		out = append(out, post)
	}

	if len(out) == 0 {
		return nil, domain.Explain(
			"Đọc được trang nhưng không có tweet nào — tài khoản chưa đăng bài, hoặc X đã đổi cấu trúc",
			fmt.Errorf("x: 0 tweet từ %d entry của @%s", len(entries), handle))
	}
	return out, nil
}

// xPostedAt trả thời điểm đăng dưới dạng epoch ms, dùng làm khoá xếp.
//
// Ưu tiên `created_at`; rỗng hoặc sai định dạng thì giải từ `id_str` — ID của X
// là snowflake, 41 bit cao là mốc thời gian tính từ 2010-11-04. Không có cả
// hai thì 0, và 0 xếp cuối: đúng chỗ cho một phần tử ta không biết gì về thời
// điểm của nó, chứ không phải đầu danh sách "mới nhất".
func xPostedAt(t *xTweet) int64 {
	if t == nil {
		return 0
	}
	if at, err := time.Parse(xCreatedAt, t.CreatedAt); err == nil {
		return at.UnixMilli()
	}
	return xSnowflakeMillis(t.IDStr)
}

// xTwitterEpoch — mốc 0 của snowflake Twitter (2010-11-04T01:42:54.657Z).
const xTwitterEpoch = 1288834974657

func xSnowflakeMillis(id string) int64 {
	n, err := strconv.ParseUint(strings.TrimSpace(id), 10, 64)
	if err != nil || n == 0 {
		return 0
	}
	return int64(n>>22) + xTwitterEpoch
}
