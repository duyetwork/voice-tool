package platform

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Bài viết dạng text/ảnh: yt-dlp không đọc được, thẻ og là đường duy nhất lấy
// được nội dung + ảnh. og:title và og:description là 2 mảnh của cùng 1 bài nên
// tiêu đề Bài Post ghép cả hai.
func TestParseOpenGraph(t *testing.T) {
	page := `<html><head>
	<title>42K views · 824 reactions | Tin nong hom nay | Kenh ABC | Facebook</title>
	<meta property="og:title" content="42K views &middot; 824 reactions | Tin n&oacute;ng h&ocirc;m nay | Facebook" />
	<meta content="Chi ti&#7871;t b&#7843;n tin s&aacute;ng nay. #tinnong" property="og:description">
	<meta property="og:image" content="https://scontent.example/anh-bia.jpg">
	<meta property="article:published_time" content="2026-03-20T08:30:00Z">
	<meta property="og:site_name" content="Kenh ABC">
	</head><body>...</body></html>`

	meta := parseOpenGraph(page)

	wantTitle := "Tin nóng hôm nay\n\nChi tiết bản tin sáng nay."
	if meta.Title != wantTitle {
		t.Errorf("Title = %q, muốn %q", meta.Title, wantTitle)
	}
	if meta.ThumbnailURL != "https://scontent.example/anh-bia.jpg" {
		t.Errorf("ThumbnailURL = %q", meta.ThumbnailURL)
	}
	if meta.AuthorName != "Kenh ABC" {
		t.Errorf("AuthorName = %q", meta.AuthorName)
	}
	if len(meta.Hashtags) != 1 || meta.Hashtags[0] != "#tinnong" {
		t.Errorf("Hashtags = %v, muốn [#tinnong]", meta.Hashtags)
	}
	if meta.PostedAt == nil || meta.PostedAt.Format("2006-01-02") != "2026-03-20" {
		t.Errorf("PostedAt = %v", meta.PostedAt)
	}
}

// Không có thẻ og thì lấy <title>; không có gì cả thì rỗng để caller biết là
// thất bại (thường là bị tường đăng nhập).
func TestParseOpenGraphFallbackTitle(t *testing.T) {
	if got := parseOpenGraph(`<html><head><title>Chi co title</title></head></html>`); got.Title != "Chi co title" {
		t.Errorf("Title = %q, muốn %q", got.Title, "Chi co title")
	}
	empty := parseOpenGraph(`<html><body>Log in to continue</body></html>`)
	if empty.Title != "" || empty.ThumbnailURL != "" {
		t.Errorf("trang không có thẻ og phải trả rỗng, được %+v", empty)
	}
}

// Đường mạng của OpenGraph: gửi UA trình duyệt, đọc có giới hạn, và trả lỗi rõ
// ràng khi trang không dùng được (tường đăng nhập trả 200 kèm HTML rỗng thẻ og).
func TestOpenGraphFetch(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		switch r.URL.Path {
		case "/ok":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, `<html><head>
				<meta property="og:title" content="Bai viet chi co chu">
				<meta property="og:description" content="Noi dung bai viet.">
				<meta property="og:image" content="https://cdn.example/anh.jpg">
			</head></html>`)
		case "/login-wall":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, `<html><body>You must log in to continue</body></html>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	og := NewOpenGraph(5 * time.Second)

	meta, err := og.Fetch(context.Background(), srv.URL+"/ok")
	if err != nil {
		t.Fatalf("Fetch(/ok) lỗi: %v", err)
	}
	if meta.Title != "Bai viet chi co chu\n\nNoi dung bai viet." {
		t.Errorf("metadata sai: %+v", meta)
	}
	if meta.ThumbnailURL != "https://cdn.example/anh.jpg" {
		t.Errorf("ThumbnailURL = %q", meta.ThumbnailURL)
	}
	if !strings.Contains(gotUA, "Mozilla") {
		t.Errorf("User-Agent = %q, cần UA trình duyệt để nền tảng không trả trang rỗng", gotUA)
	}

	if _, err := og.Fetch(context.Background(), srv.URL+"/login-wall"); err == nil {
		t.Error("trang tường đăng nhập phải trả lỗi để caller giữ lỗi gốc của yt-dlp")
	}
	if _, err := og.Fetch(context.Background(), srv.URL+"/missing"); err == nil {
		t.Error("HTTP 404 phải trả lỗi")
	}
}
