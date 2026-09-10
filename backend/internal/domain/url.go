package domain

import (
	"net/url"
	"strings"
)

// redirectParams là tên tham số chứa URL thật trong các link bọc redirect.
var redirectParams = []string{"u", "url", "q", "target"}

// NormalizeSourceURL dọn URL người dùng dán trước khi nhận diện nền tảng.
//
// Hai việc:
//
//  1. Bóc link bọc redirect. Facebook bọc mọi link ngoài thành
//     `l.facebook.com/l.php?u=<url thật>`, Google/Zalo cũng có dạng tương tự.
//     Không bóc thì hệ thống nhận diện sai nền tảng (thành Facebook) rồi đưa
//     yt-dlp một URL nó không tải được.
//  2. Bỏ tham số tracking (`fbclid`, `utm_*`, `igsh`...). Chúng không đổi nội
//     dung nhưng làm cùng 1 bài trông như 2 URL khác nhau, phá dedup.
//
// URL không parse được thì trả về nguyên trạng — việc báo lỗi là của bước
// nhận diện nền tảng, không phải của hàm này.
func NormalizeSourceURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// Bóc tối đa 3 lớp: link share bọc link share là có thật, nhưng sâu hơn
	// thì gần như chắc chắn là vòng lặp.
	for range 3 {
		unwrapped, ok := unwrapRedirect(raw)
		if !ok {
			break
		}
		raw = unwrapped
	}
	return stripTracking(raw)
}

// unwrapRedirect lấy URL thật ra khỏi link bọc; ok=false nếu đây không phải
// link bọc.
func unwrapRedirect(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw, false
	}
	// Chỉ bóc khi path đúng là endpoint redirect — `?u=` trên URL bài đăng
	// bình thường không phải link bọc.
	if !isRedirectPath(u) {
		return raw, false
	}
	for _, name := range redirectParams {
		target := u.Query().Get(name)
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			return target, true
		}
	}
	return raw, false
}

func isRedirectPath(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	path := strings.ToLower(u.Path)
	switch {
	case strings.HasSuffix(host, "facebook.com") && (path == "/l.php" || path == "/flx/warn/"):
		return true
	case host == "l.messenger.com" || host == "lm.facebook.com":
		return true
	case host == "www.google.com" && path == "/url":
		return true
	case strings.HasSuffix(host, "zalo.me") && path == "/redirect":
		return true
	}
	return false
}

// trackingParams là các tham số chỉ dùng để theo dõi nguồn truy cập.
var trackingParams = []string{
	"fbclid", "igsh", "igshid", "mibextid", "rdid", "share_url", "sfnsn",
	"gclid", "si", "feature", "app", "_rdr", "checkpoint_src",
}

func stripTracking(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	q := u.Query()
	if len(q) == 0 {
		return raw
	}
	for key := range q {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") {
			q.Del(key)
			continue
		}
		for _, t := range trackingParams {
			if lower == t {
				q.Del(key)
				break
			}
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
