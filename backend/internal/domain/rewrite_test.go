package domain

import "testing"

// Đường chính: model tuân thủ hợp đồng JSON.
func TestParseRewriteDocDungKhuon(t *testing.T) {
	got := ParseRewrite(`{"title":"Bản tin sáng","content":"Xin chào quý vị.","hashtags":["thoisu","tintuc"]}`)
	if got.Content != "Xin chào quý vị." {
		t.Fatalf("content = %q", got.Content)
	}
	if got.Title != "Bản tin sáng" {
		t.Fatalf("title = %q", got.Title)
	}
	if len(got.Hashtags) != 2 || got.Hashtags[0] != "#thoisu" {
		t.Fatalf("hashtags = %v", got.Hashtags)
	}
}

// Model bọc JSON trong rào code dù đã được dặn là không — vẫn phải đọc ra.
func TestParseRewriteGoRaoCode(t *testing.T) {
	got := ParseRewrite("```json\n{\"content\":\"Nội dung đọc\",\"title\":\"Tiêu đề\"}\n```")
	if got.Content != "Nội dung đọc" || got.Title != "Tiêu đề" {
		t.Fatalf("got %+v", got)
	}
}

// Quan trọng nhất: model phớt lờ khuôn JSON và trả thẳng đoạn văn. Prompt mẫu
// do người dùng viết nên chuyện này sẽ xảy ra, và nó KHÔNG được làm hỏng voice —
// cả chuỗi trở thành lời đọc, đúng hành vi trước khi có hợp đồng JSON.
func TestParseRewriteVanThuanCoiLaLoiDoc(t *testing.T) {
	raw := "Hôm nay thị trường chứng khoán tăng điểm."
	got := ParseRewrite(raw)
	if got.Content != raw {
		t.Fatalf("content = %q, muốn nguyên văn", got.Content)
	}
	if got.Title != "" || len(got.Hashtags) != 0 {
		t.Fatalf("không được bịa tiêu đề/hashtag: %+v", got)
	}
}

// JSON hợp lệ nhưng thiếu lời đọc thì cũng vô dụng như không parse được: quay
// về coi cả chuỗi là lời đọc thay vì tạo ra một voice rỗng.
func TestParseRewriteJSONThieuContent(t *testing.T) {
	raw := `{"title":"Chỉ có tiêu đề"}`
	if got := ParseRewrite(raw); got.Content != raw {
		t.Fatalf("content = %q, muốn nguyên văn chuỗi vào", got.Content)
	}
}

func TestParseRewriteRong(t *testing.T) {
	if got := ParseRewrite("   "); got.Content != "" {
		t.Fatalf("content = %q, muốn rỗng", got.Content)
	}
}

// Hashtag model trả về đi qua đúng bộ luật của hashtag lấy từ bài gốc: thêm '#',
// bỏ ký tự lạ, khử trùng không phân biệt hoa thường.
func TestParseRewriteChuanHoaHashtag(t *testing.T) {
	got := ParseRewrite(`{"content":"x","hashtags":["#ThoiSu","thoisu","tin tuc",""]}`)
	if len(got.Hashtags) != 2 {
		t.Fatalf("hashtags = %v, muốn 2 thẻ sau khi khử trùng", got.Hashtags)
	}
	if got.Hashtags[0] != "#ThoiSu" || got.Hashtags[1] != "#tintuc" {
		t.Fatalf("hashtags = %v", got.Hashtags)
	}
}
