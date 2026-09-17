package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// Adapter quét TÀI KHOẢN Instagram
// ---------------------------------------------------------------------------
//
// Cùng hình dạng với FacebookScrapeAdapter và vì cùng một lý do: yt-dlp lấy
// được từng bài Instagram nhưng extractor `instagram:user` đã bị chính yt-dlp
// đánh dấu CURRENTLY BROKEN, nên phần LIỆT KÊ tài khoản phải tự làm.
//
// Khác Facebook ở đúng một điểm, và đó là điểm đáng mừng: Instagram có một
// endpoint JSON thật (`web_profile_info`) mà chính trang web của họ gọi, nên ở
// đây không phải moi dữ liệu ra khỏi HTML. Hình dạng JSON vẫn là nội bộ và
// không có cam kết tương thích, nhưng nó ổn định hơn hẳn việc dò regex trong
// một khối script.
//
// ⚠️ CHƯA ĐƯỢC KIỂM CHỨNG TRÊN DỮ LIỆU THẬT, đúng như bộ phân tích Facebook.
// Quy trình bắt buộc trước khi mở khoá cho người dùng: thêm via Instagram thật,
// chạy thử vài tài khoản thật, đối chiếu số bài lấy về với thực tế, rồi mới bật
// INSTAGRAM_CHANNEL_SCAN.

// instagramAppID — giá trị `X-IG-App-ID` mà web client của Instagram gửi kèm
// mọi lời gọi tới `/api/v1/`.
//
// Thiếu header này thì endpoint trả 403 kể cả khi cookies hoàn toàn hợp lệ, và
// bộ phân loại sẽ đọc 403 đó thành "IP bị chặn" — tức là giết nhầm proxy vì
// một header thiếu. Đó là lý do nó là hằng số ở đây chứ không phải tuỳ chọn.
const instagramAppID = "936619743392459"

// InstagramScrapeAdapter = adapter yt-dlp + khả năng liệt kê tài khoản bằng via.
type InstagramScrapeAdapter struct {
	*YtDlpAdapter

	pool    domain.ScrapePool
	fetcher *ScrapeFetcher
}

var _ domain.PlatformAdapter = (*InstagramScrapeAdapter)(nil)

// NewInstagramScrape dựng adapter Instagram có khả năng quét cả tài khoản.
//
// pool nil = chưa có hạ tầng via: adapter hành xử y hệt bản yt-dlp thuần và vẫn
// chặn việc thêm kênh. Xem NewFacebookScrape.
func NewInstagramScrape(
	base *YtDlpAdapter, pool domain.ScrapePool, fetcher *ScrapeFetcher,
) *InstagramScrapeAdapter {
	return &InstagramScrapeAdapter{YtDlpAdapter: base, pool: pool, fetcher: fetcher}
}

func (a *InstagramScrapeAdapter) CheckChannelScan() error {
	if a.pool == nil {
		return a.YtDlpAdapter.CheckChannelScan()
	}
	return nil
}

// FetchLatestPosts liệt kê bài mới nhất của một tài khoản Instagram.
//
// Mỗi lần gọi = 1 đơn vị hạn mức của via.
func (a *InstagramScrapeAdapter) FetchLatestPosts(
	ctx context.Context, channelURL string, limit int,
) ([]domain.RemotePost, error) {
	if a.pool == nil {
		return nil, a.YtDlpAdapter.CheckChannelScan()
	}
	if limit <= 0 {
		limit = 10
	}

	// Kiểm URL TRƯỚC khi xin via: một URL sai không đáng lấy mất một suất hạn
	// mức, và lỗi của nó cũng không nói gì về sức khoẻ của via.
	username, err := instagramUsername(channelURL)
	if err != nil {
		return nil, err
	}

	session, err := a.pool.Acquire(ctx, domain.PlatformInstagram)
	if err != nil {
		// Hết via KHÔNG phải sự cố — xem FacebookScrapeAdapter.FetchLatestPosts.
		return nil, err
	}

	var posts []domain.RemotePost
	defer func() {
		a.pool.Release(ctx, session, domain.ScrapeOwner{Platform: domain.PlatformInstagram},
			len(posts), err)
	}()

	target := "https://www.instagram.com/api/v1/users/web_profile_info/?username=" +
		url.QueryEscape(username)

	body, err := a.fetcher.Get(ctx, session, target, map[string]string{
		"X-IG-App-ID":      instagramAppID,
		"X-Requested-With": "XMLHttpRequest",
		// Xin JSON chứ không xin HTML: cùng một đường dẫn trả về hai thứ khác
		// nhau tuỳ header, và bản HTML là trang đăng nhập.
		"Accept": "application/json, text/plain, */*",
		// Referer trỏ đúng trang hồ sơ đang xem — thiếu nó là dấu hiệu rõ nhất
		// của một công cụ gọi thẳng vào endpoint nội bộ.
		"Referer": "https://www.instagram.com/" + username + "/",
	})
	if err != nil {
		return nil, err
	}

	posts, err = parseInstagramProfile(body, username, limit)
	if err != nil {
		return nil, err
	}
	return posts, nil
}

// instagramUsername tách tên tài khoản khỏi URL hồ sơ.
//
// Từ chối các đường dẫn KHÔNG phải hồ sơ (/p/, /reel/, /explore/...): dán link
// một bài lẻ vào ô "kênh" là nhầm lẫn thường gặp nhất ở màn Thêm kênh, và nhận
// bừa nó thì kênh được tạo ra rồi lặng lẽ không bao giờ ra bài.
func instagramUsername(raw string) (string, error) {
	bad := func() error {
		return domain.Permanent(domain.Explain(
			"URL phải là trang tài khoản Instagram, ví dụ https://www.instagram.com/tenkenh",
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
	name := parts[0]
	switch name {
	case "p", "reel", "reels", "tv", "stories", "share", "explore", "accounts", "directory":
		return "", bad()
	}
	return name, nil
}

// ---------------------------------------------------------------------------
// Bộ phân tích
// ---------------------------------------------------------------------------

// instagramProfile — đúng những nhánh của `web_profile_info` mà ta dùng.
//
// Khai tường minh thay vì map[string]any: cây JSON này sâu và nhiều tên gần
// giống nhau (`edge_owner_to_timeline_media` cạnh `edge_felix_video_timeline`),
// nên lần theo nó bằng ép kiểu từng tầng là cách chắc chắn đọc nhầm nhánh.
type instagramProfile struct {
	Data struct {
		User *struct {
			Timeline struct {
				Edges []struct {
					Node struct {
						ID          string `json:"id"`
						Shortcode   string `json:"shortcode"`
						TakenAt     int64  `json:"taken_at_timestamp"`
						IsVideo     bool   `json:"is_video"`
						ProductType string `json:"product_type"`
						Caption     struct {
							Edges []struct {
								Node struct {
									Text string `json:"text"`
								} `json:"node"`
							} `json:"edges"`
						} `json:"edge_media_to_caption"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"edge_owner_to_timeline_media"`
		} `json:"user"`
	} `json:"data"`
}

// parseInstagramProfile đổi phản hồi JSON thành danh sách bài, mới nhất trước.
//
// SẮP LẠI theo `taken_at_timestamp` giảm dần trước khi cắt `limit`, không tin
// thứ tự mảng.
//
// Bài học lấy từ X, cùng một họ lỗi: ở đó mảng `entries` trông như đã xếp sẵn
// nhưng thực ra trộn lẫn tweet từ 2016 đến 2025, và lấy N phần tử đầu ra một
// nhúm bài ngẫu nhiên. Ở Instagram vấn đề nhỏ hơn nhưng cùng bản chất — BÀI
// GHIM nằm đầu danh sách mà không có cờ nào phân biệt, nên với hạn mức 12
// bài/lần gọi thì tối đa 3 bài ghim cũ có thể chiếm 1/4 số suất của một vòng
// quét tin nóng.
//
// Hệ quả phải chấp nhận: danh sách ở đây sẽ KHÁC thứ tự người vận hành nhìn
// thấy trên trang thật (bài ghim bị đẩy xuống). Đổi lại "N bài mới nhất" đúng
// nghĩa là N bài mới nhất — mà đó mới là hợp đồng cả hệ thống dựa vào.
func parseInstagramProfile(body []byte, username string, limit int) ([]domain.RemotePost, error) {
	var profile instagramProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, domain.Explain(
			"Instagram trả về dữ liệu không đọc được — có thể họ đã đổi cấu trúc API",
			fmt.Errorf("instagram: giải mã web_profile_info: %w", err))
	}
	// `user: null` = tài khoản không tồn tại, đã đổi tên, hoặc bị khoá. Vĩnh
	// viễn theo nghĩa thử lại không giúp gì — người vận hành phải sửa URL.
	if profile.Data.User == nil {
		return nil, domain.Permanent(domain.FetchBlocked(domain.FetchBlockUnavailable, domain.Explain(
			"Không tìm thấy tài khoản Instagram này — có thể đã đổi tên hoặc bị khoá",
			fmt.Errorf("instagram: user null cho @%s", username))))
	}

	edges := profile.Data.User.Timeline.Edges
	if len(edges) == 0 {
		// Đọc được hồ sơ nhưng không có bài nào: tài khoản riêng tư (via chưa
		// theo dõi) hoặc thật sự chưa đăng gì. KHÔNG gắn nhãn chặn — gắn
		// `bot_block` ở đây là giết proxy vì một tài khoản trống.
		return nil, domain.Explain(
			"Đọc được hồ sơ nhưng không có bài nào — tài khoản riêng tư (via chưa theo dõi) hoặc chưa đăng bài",
			fmt.Errorf("instagram: 0 bài từ @%s", username))
	}

	// SliceStable: hai bài cùng mốc thời gian (hoặc cùng thiếu) giữ nguyên vị
	// trí tương đối thay vì hoán đổi ngẫu nhiên giữa các lần chạy.
	sort.SliceStable(edges, func(i, j int) bool {
		return edges[i].Node.TakenAt > edges[j].Node.TakenAt
	})

	out := make([]domain.RemotePost, 0, min(limit, len(edges)))
	for _, e := range edges {
		if len(out) >= limit {
			break
		}
		n := e.Node
		// Không có shortcode thì không dựng được URL bài, và một bài không mở
		// được là một bài vô dụng ở mọi bước sau.
		if n.Shortcode == "" {
			continue
		}

		var text string
		if len(n.Caption.Edges) > 0 {
			text = strings.TrimSpace(n.Caption.Edges[0].Node.Text)
		}

		// `product_type = clips` là reel; `is_video` không phân biệt được hai
		// thứ đó, và chúng cần hai dạng URL khác nhau (xem instagramPostURL).
		kind := domain.ContentPost
		switch {
		case n.ProductType == "clips":
			kind = domain.ContentReel
		case n.IsVideo:
			kind = domain.ContentVideo
		}

		// PostID lấy `id` số của Instagram chứ không lấy shortcode: đó là khoá
		// chống trùng ổn định, còn shortcode chỉ là cách mã hoá nó thành URL.
		postID := n.ID
		if postID == "" {
			postID = n.Shortcode
		}

		post := domain.RemotePost{
			PostID:      postID,
			URL:         instagramPostURL(kind, n.Shortcode),
			ContentType: kind,
			Text:        text,
		}
		post.Meta.Title = text
		post.Meta.Hashtags = domain.ExtractHashtags(text, nil)
		if n.TakenAt > 0 {
			t := time.Unix(n.TakenAt, 0).UTC()
			post.Meta.PostedAt = &t
		}
		out = append(out, post)
	}
	if len(out) == 0 {
		return nil, domain.Explain(
			"Đọc được hồ sơ nhưng không tách được bài nào — Instagram có thể đã đổi cấu trúc API",
			fmt.Errorf("instagram: %d mục nhưng không mục nào dùng được, @%s", len(edges), username))
	}
	return out, nil
}

// instagramPostURL dựng link bài từ shortcode.
//
// Reel phải đi đường `/reel/`: `/p/<shortcode>` của một reel vẫn mở được trên
// trình duyệt, nhưng regex nhận diện của adapter yt-dlp sẽ xếp nó vào nhóm bài
// ảnh và bước tải về sau đó lấy sai thứ.
func instagramPostURL(kind, shortcode string) string {
	if kind == domain.ContentReel {
		return "https://www.instagram.com/reel/" + shortcode + "/"
	}
	return "https://www.instagram.com/p/" + shortcode + "/"
}
