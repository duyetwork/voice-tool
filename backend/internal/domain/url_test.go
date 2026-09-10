package domain

import "testing"

func TestNormalizeSourceURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bóc link bọc l.facebook.com",
			in:   "https://l.facebook.com/l.php?u=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3DdQw4w9WgXcQ&h=AT1",
			want: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name: "bóc link bọc google",
			in:   "https://www.google.com/url?q=https://www.tiktok.com/@user/video/7300000000000000000",
			want: "https://www.tiktok.com/@user/video/7300000000000000000",
		},
		{
			name: "bỏ tham số tracking",
			in:   "https://www.facebook.com/reel/1234567890?fbclid=abc&mibextid=xyz",
			want: "https://www.facebook.com/reel/1234567890",
		},
		{
			name: "bỏ utm_*",
			in:   "https://www.youtube.com/watch?v=dQw4w9WgXcQ&utm_source=fb&utm_medium=post",
			want: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name: "giữ nguyên URL bài đăng bình thường",
			in:   "https://www.facebook.com/share/p/1GoKsA89zx/",
			want: "https://www.facebook.com/share/p/1GoKsA89zx/",
		},
		{
			name: "?u= trên URL bài đăng KHÔNG bị coi là link bọc",
			in:   "https://www.facebook.com/watch?v=123&u=456",
			want: "https://www.facebook.com/watch?u=456&v=123",
		},
		{
			name: "URL rác trả về nguyên trạng",
			in:   "khong-phai-url",
			want: "khong-phai-url",
		},
		{
			name: "chuỗi rỗng",
			in:   "   ",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeSourceURL(tc.in); got != tc.want {
				t.Errorf("NormalizeSourceURL(%q) = %q, muốn %q", tc.in, got, tc.want)
			}
		})
	}
}
