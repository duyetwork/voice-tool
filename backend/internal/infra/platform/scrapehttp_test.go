package platform

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// resp dựng một phản hồi giả để đem đi phân loại.
func resp(status int) *http.Response {
	u, _ := url.Parse("https://www.facebook.com/somepage")
	return &http.Response{
		StatusCode: status,
		Request:    &http.Request{URL: u},
	}
}

// TestClassifyScrapeResponse là test quan trọng nhất của tầng này.
//
// Nhãn lỗi ở đây là thứ chạy CẢ HAI máy trạng thái: gắn nhầm `bot_block` thành
// `login_required` sẽ giết dần đàn via trong khi thứ hỏng thật là địa chỉ IP —
// và không có gì trên giao diện nói ra điều đó, chỉ có via cứ chết.
func TestClassifyScrapeResponse(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr bool
		// Nhãn mong đợi; rỗng = lỗi KHÔNG được gắn nhãn nào (không trách ai).
		wantKind domain.FetchBlockKind
	}{
		{
			name: "trang đọc được", status: 200,
			body: `<html><div>Nội dung trang</div></html>`,
		},
		{
			name:   "429 là rate-limit, không phải lỗi của via hay proxy",
			status: 429, wantErr: true, wantKind: domain.FetchBlockRateLimit,
		},
		{
			// 403 của Facebook gần như luôn là chặn theo IP: phiên sai thì họ
			// trả 200 kèm trang đăng nhập, không trả 403.
			name: "403 là chặn IP", status: 403, wantErr: true, wantKind: domain.FetchBlockBot,
		},
		{
			name:   "404 là trang không còn tồn tại",
			status: 404, wantErr: true, wantKind: domain.FetchBlockUnavailable,
		},
		{
			// Đây là cái bẫy chính: HTTP 200 nhưng nội dung là trang đăng nhập.
			// Chỉ nhìn mã trạng thái thì mọi via hỏng đều trông như thành công.
			name:   "200 kèm trang đăng nhập = via hỏng",
			status: 200, body: `<title>Log into Facebook</title>`,
			wantErr: true, wantKind: domain.FetchBlockLogin,
		},
		{
			name:   "200 kèm form đăng nhập = via hỏng",
			status: 200, body: `<input type="hidden" name="login_source" value="comet">`,
			wantErr: true, wantKind: domain.FetchBlockLogin,
		},
		{
			name:   "200 kèm checkpoint = lỗi của IP",
			status: 200, body: `window.location.href="/checkpoint/?next=x"`,
			wantErr: true, wantKind: domain.FetchBlockBot,
		},
		{
			name:   "200 kèm captcha = lỗi của IP",
			status: 200, body: `<div id="captcha">Please verify you are a human</div>`,
			wantErr: true, wantKind: domain.FetchBlockBot,
		},
		{
			// Trang checkpoint thường chứa CẢ HAI dấu hiệu. Đọc nhầm nó thành
			// "đòi đăng nhập" là giết via vì lỗi của proxy — đây là lý do thứ
			// tự kiểm tra trong classifyScrapeResponse là có chủ ý.
			name:   "checkpoint kèm cả dấu hiệu đăng nhập vẫn phải là lỗi IP",
			status: 200,
			body: `<title>Log into Facebook</title>` +
				`<meta content="/checkpoint/?next=https://facebook.com">`,
			wantErr: true, wantKind: domain.FetchBlockBot,
		},
		{
			// 5xx tự khỏi: không được trách via lẫn proxy.
			name: "5xx không gắn nhãn", status: 503, wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyScrapeResponse(resp(tc.status), []byte(tc.body))
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("muốn không lỗi, được %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("muốn lỗi, được nil")
			}
			kind, ok := domain.FetchBlockKindOf(err)
			if tc.wantKind == "" {
				if ok {
					t.Errorf("lỗi này không được gắn nhãn, nhưng có nhãn %q", kind)
				}
				return
			}
			if !ok || kind != tc.wantKind {
				t.Errorf("nhãn = %q (có nhãn: %v), muốn %q", kind, ok, tc.wantKind)
			}
		})
	}
}

func TestFacebookPageURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://www.facebook.com/vnexpress", "https://www.facebook.com/vnexpress"},
		// Mọi biến thể host đều về www: bản m./mbasic trả nội dung khác hẳn, và
		// chấp nhận chúng là tự chuốc lấy một bộ phân tích thứ hai.
		{"https://m.facebook.com/vnexpress", "https://www.facebook.com/vnexpress"},
		{"https://mbasic.facebook.com/vnexpress/", "https://www.facebook.com/vnexpress"},
		// Query string bị bỏ: tham số theo dõi không thuộc về danh tính trang.
		{"https://www.facebook.com/vnexpress?ref=xyz", "https://www.facebook.com/vnexpress"},
	}
	for _, tc := range tests {
		got, err := facebookPageURL(tc.in)
		if err != nil {
			t.Errorf("facebookPageURL(%q) lỗi: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("facebookPageURL(%q) = %q, muốn %q", tc.in, got, tc.want)
		}
	}

	if _, err := facebookPageURL("không phải url"); err == nil {
		t.Error("muốn lỗi với URL không hợp lệ")
	}
}

// TestParseFacebookPage kiểm bộ phân tích trên dữ liệu TỰ DỰNG.
//
// Nó chứng minh hình dạng dữ liệu vào -> ra, KHÔNG chứng minh rằng Facebook
// thật sự trả về hình dạng đó. Việc đó chỉ đối chiếu được với HTML thật của một
// trang thật — xem ghi chú ở đầu phần Bộ phân tích trang.
func TestParseFacebookPage(t *testing.T) {
	body := `<html>
<script type="application/json" data-sjs>
{"data":{"node":{"feed":{"edges":[
 {"node":{"message":{"text":"Bài mới nhất #tin"},"post_id":"111","creation_time":1700000200}},
 {"node":{"message":{"text":"Bai cu hon"},"post_id":"222","creation_time":1700000100}}
]}}}}
</script>
</html>`

	posts := parseFacebookPage(body, 10)
	if len(posts) != 2 {
		t.Fatalf("lấy được %d bài, muốn 2", len(posts))
	}
	// Mới nhất trước — hợp đồng mà service.Scan trông đợi.
	if posts[0].PostID != "111" || posts[1].PostID != "222" {
		t.Errorf("thứ tự sai: %s, %s", posts[0].PostID, posts[1].PostID)
	}
	if posts[0].Text != "Bài mới nhất #tin" {
		t.Errorf("text = %q, muốn đã giải escape unicode", posts[0].Text)
	}
	if posts[0].Meta.PostedAt == nil || posts[0].Meta.PostedAt.Unix() != 1700000200 {
		t.Errorf("posted_at = %v", posts[0].Meta.PostedAt)
	}
	if len(posts[0].Meta.Hashtags) == 0 {
		t.Error("hashtag phải được tách ra khỏi nội dung")
	}
	if posts[0].URL == "" {
		t.Error("bài không có URL thì vòng quét không xử lý lại được")
	}

	t.Run("limit cắt đúng", func(t *testing.T) {
		if got := parseFacebookPage(body, 1); len(got) != 1 {
			t.Errorf("lấy %d bài, muốn 1", len(got))
		}
	})

	t.Run("trang không có khối JSON nào", func(t *testing.T) {
		// Không panic, không đoán bừa — trả rỗng để tầng trên báo đúng rằng
		// không tách được bài nào.
		if got := parseFacebookPage("<html><body>trống</body></html>", 10); len(got) != 0 {
			t.Errorf("muốn rỗng, được %d bài", len(got))
		}
	})
}

func TestDecodeJSONString(t *testing.T) {
	tests := []struct{ in, want string }{
		{`xin chào`, "xin chào"},
		{`dòng 1\ndòng 2`, "dòng 1\ndòng 2"},
		{`  có khoảng trắng thừa  `, "có khoảng trắng thừa"},
	}
	for _, tc := range tests {
		if got := decodeJSONString(tc.in); got != tc.want {
			t.Errorf("decodeJSONString(%q) = %q, muốn %q", tc.in, got, tc.want)
		}
	}
}
