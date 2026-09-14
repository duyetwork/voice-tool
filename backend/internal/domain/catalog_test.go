package domain

import "testing"

// Strongbody trả TÊN ISO ĐẦY ĐỦ, không phải tên gọi thường ngày. Khớp theo tên
// vì thế là một cuộc đoán không bao giờ đủ; mã ISO mới là thứ chỉ có một dạng.
func TestCountrySortOrderTheoMaISO(t *testing.T) {
	cases := []struct {
		name string
		code string
		want int32
	}{
		{"Vietnam", "VN", 0},
		{"United States of America", "US", 1},
		{"United Kingdom of Great Britain and Northern Ireland", "GB", 2},
		{"Korea (Republic of)", "KR", 6},
		{"Bolivia (Plurinational State of)", "BO", 35},
		{"Venezuela (Bolivarian Republic of)", "VE", 44},
	}
	for _, tc := range cases {
		if got := CountrySortOrder(tc.name, tc.code); got != tc.want {
			t.Errorf("CountrySortOrder(%q, %q) = %d, muốn %d", tc.name, tc.code, got, tc.want)
		}
	}
}

// Cái bẫy đã gặp thật: "United States Minor Outlying Islands" bắt đầu đúng bằng
// "United States", nên khớp tiền tố đưa nó lên hạng 2 — đứng TRƯỚC chính nước
// Mỹ trong ô chọn.
func TestCountrySortOrderKhongKhopTienTo(t *testing.T) {
	bẫy := []struct{ name, code string }{
		{"United States Minor Outlying Islands", "UM"},
		{"Korea (Democratic People's Republic of)", "KP"},
		{"Virgin Islands (British)", "VG"},
	}
	for _, c := range bẫy {
		if got := CountrySortOrder(c.name, c.code); got != lowestPriority {
			t.Errorf("CountrySortOrder(%q, %q) = %d, muốn %d (không phải nước ưu tiên)",
				c.name, c.code, got, lowestPriority)
		}
	}
}

// Hàng không có mã thì mới so tên, và phải khớp chính xác.
func TestCountrySortOrderKhongCoMa(t *testing.T) {
	if got := CountrySortOrder("Vietnam", ""); got != 0 {
		t.Errorf("tên đúng mà không có mã: got %d, muốn 0", got)
	}
	if got := CountrySortOrder("United States Minor Outlying Islands", ""); got != lowestPriority {
		t.Errorf("tên chỉ TRÙNG TIỀN TỐ mà không có mã vẫn phải rơi xuống cuối, got %d", got)
	}
}

// Thứ tự ngôn ngữ bám theo thứ tự quốc gia, và ngôn ngữ trùng chỉ tính lần
// xuất hiện đầu: en lên hạng 2 nhờ United States, nên United Kingdom /
// Singapore / Canada không sinh thêm thứ hạng nào cho nó.
func TestLanguageOrder(t *testing.T) {
	want := []string{"vi", "en", "fr", "de", "ja", "ko", "zh-CN", "hi"}
	for i, code := range want {
		if LanguageOrder[i] != code {
			t.Fatalf("LanguageOrder[%d] = %q, muốn %q (đủ dãy: %v)",
				i, LanguageOrder[i], code, LanguageOrder[:len(want)])
		}
	}

	seen := map[string]bool{}
	for _, code := range LanguageOrder {
		if seen[code] {
			t.Errorf("ngôn ngữ %q xuất hiện 2 lần trong LanguageOrder", code)
		}
		seen[code] = true
	}
}
