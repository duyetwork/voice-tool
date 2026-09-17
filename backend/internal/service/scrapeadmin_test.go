package service

import (
	"strings"
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// TestNormalizeProxyEndpoint: dạng `ip:port:user:pass` là dạng các nhà bán
// proxy thật sự giao, nên nó phải dán thẳng vào được.
//
// Bắt người vận hành tự ghép tay thành URL cho từng dòng trong một danh sách
// vài chục proxy là vừa mất thời gian vừa dễ sai — và sai ở đây thì proxy lặng
// lẽ không dùng được, triệu chứng lại giống hệt proxy bị chặn.
func TestNormalizeProxyEndpoint(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{
			"dạng nhà bán proxy giao",
			"23.95.45.5:10356:u3qvdrto:9w6w0mc8c5",
			"http://u3qvdrto:9w6w0mc8c5@23.95.45.5:10356",
		},
		{"không cần đăng nhập", "23.95.45.5:10356", "http://23.95.45.5:10356"},
		{
			"URL đầy đủ giữ nguyên",
			"http://user:pass@gw.example.net:8000",
			"http://user:pass@gw.example.net:8000",
		},
		{
			// socks5 phải khai tường minh — không tự đoán giao thức, vì dùng
			// nhầm thì request hỏng theo kiểu rất khó đọc.
			"giữ nguyên scheme đã khai",
			"socks5://u:p@1.2.3.4:1080",
			"socks5://u:p@1.2.3.4:1080",
		},
		{"khoảng trắng thừa", "  23.95.45.5:10356  ", "http://23.95.45.5:10356"},
		{"rỗng", "", ""},
		{"chỉ có host", "23.95.45.5", ""},
		{"thừa phần", "a:b:c:d:e", ""},
		{"thiếu port", "23.95.45.5::user:pass", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.NormalizeProxyEndpoint(tc.in); got != tc.want {
				t.Errorf("= %q, muốn %q", got, tc.want)
			}
		})
	}

	t.Run("mật khẩu có ký tự đặc biệt vẫn ghép đúng", func(t *testing.T) {
		// Ghép chuỗi tay thì đúng những mật khẩu này hỏng: `@` cắt sai phần
		// host, `/` cắt sai phần path.
		got := domain.NormalizeProxyEndpoint("1.2.3.4:8000:user:p@ss/word")
		masked := domain.MaskProxyEndpoint(got)
		if masked != "http://1.2.3.4:8000" {
			t.Errorf("phần host bị hỏng: masked = %q (từ %q)", masked, got)
		}
		if strings.Contains(masked, "p@ss") {
			t.Error("mật khẩu lọt vào phần hiển thị")
		}
	})
}

// TestMissingCookies: dán cookies là thao tác tay, và cái sai hay gặp nhất là
// dán thiếu. Báo tại chỗ dán chứ không để vòng quét phát hiện sau 6 tiếng.
func TestMissingCookies(t *testing.T) {
	tests := []struct {
		name     string
		platform domain.Platform
		cookies  string
		want     []string
	}{
		{
			"facebook đủ", domain.PlatformFacebook,
			"datr=abc; c_user=100012345678901; xs=42%3Aabc; sb=xyz", nil,
		},
		{
			"facebook thiếu xs", domain.PlatformFacebook,
			"datr=abc; c_user=100012345678901", []string{"xs"},
		},
		{
			// Dán nhầm cookie của trang khác: không có cookie bắt buộc nào.
			"facebook dán nhầm hoàn toàn", domain.PlatformFacebook,
			"_ga=GA1.2.3; _gid=GA1.2.4", []string{"c_user", "xs"},
		},
		{
			"x đủ", domain.PlatformX,
			"guest_id=v1%3A123; auth_token=a1b2c3; ct0=9f8e7d", nil,
		},
		{
			// auth_token không thôi là chưa đủ: mọi request đọc dữ liệu của X
			// đều đòi thêm token CSRF.
			"x thiếu ct0", domain.PlatformX, "auth_token=a1b2c3", []string{"ct0"},
		},
		{
			"instagram đủ", domain.PlatformInstagram,
			"sessionid=123%3Aabc; ds_user_id=123; csrftoken=xyz", nil,
		},
		{
			"instagram thiếu cả hai", domain.PlatformInstagram,
			"csrftoken=xyz", []string{"sessionid", "ds_user_id"},
		},
		{
			// So khớp theo TÊN đứng trước `=`, không phải tìm chuỗi con: chuỗi
			// "xs" nằm trong giá trị của cookie khác không tính là có nó.
			"tên cookie nằm trong giá trị không tính", domain.PlatformFacebook,
			"c_user=100012345678901; datr=abcxs=def", []string{"xs"},
		},
		{
			// YouTube/TikTok không cần via, nên không có gì bắt buộc.
			"nền tảng không cần via", domain.PlatformYouTube, "", nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.MissingCookies(tc.platform, tc.cookies)
			if len(got) != len(tc.want) {
				t.Fatalf("thiếu = %v, muốn %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("thiếu = %v, muốn %v", got, tc.want)
					return
				}
			}
		})
	}
}

// TestViaCookieSpecs: mẫu hiện trên form phải chứa ĐÚNG những cookie mà server
// dùng để từ chối — nếu không, form hướng dẫn một đằng và API chặn một nẻo.
func TestViaCookieSpecs(t *testing.T) {
	specs := ViaCookieSpecs()
	if len(specs) != len(domain.ScrapePlatforms) {
		t.Fatalf("có %d nền tảng, muốn %d", len(specs), len(domain.ScrapePlatforms))
	}
	for _, spec := range specs {
		p := domain.Platform(spec.Platform)
		if len(spec.Required) == 0 {
			t.Errorf("%s: không khai cookie bắt buộc nào", p)
		}
		// Chính cái mẫu phải đi qua được bộ kiểm tra thật.
		if missing := domain.MissingCookies(p, spec.Example); len(missing) > 0 {
			t.Errorf("%s: mẫu trên form lại thiếu %v — form và API nói khác nhau",
				p, missing)
		}
		if spec.Hint == "" {
			t.Errorf("%s: không có hướng dẫn lấy cookie ở đâu", p)
		}
	}
}
