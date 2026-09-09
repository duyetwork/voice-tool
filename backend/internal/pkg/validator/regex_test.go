package validator

import (
	"regexp"
	"testing"
)

func TestNormalizePattern(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		matches    []string
		notMatches []string
	}{
		{
			name:       "hashtag",
			raw:        "#tinnong",
			matches:    []string{"Bản tin #tinnong hôm nay"},
			notMatches: []string{"tinnong không có dấu thăng"},
		},
		{
			name:       "từ khoá nhiều chữ, không phân biệt hoa thường",
			raw:        "tin nóng",
			matches:    []string{"TIN NÓNG: sự kiện lớn", "tin   nóng"},
			notMatches: []string{"tin tức"},
		},
		{
			name:       "danh sách từ khoá ghép OR",
			raw:        "bão, lũ",
			matches:    []string{"Cảnh báo bão số 5", "miền Trung có lũ"},
			notMatches: []string{"trời nắng"},
		},
		{
			name:       "regex có delimiter và cờ i",
			raw:        "/^BREAKING/i",
			matches:    []string{"breaking news"},
			notMatches: []string{"tin breaking đứng giữa"},
		},
		{
			name:       "regex thuần giữ nguyên",
			raw:        `\bCPI\b`,
			matches:    []string{"chỉ số CPI tháng 8"},
			notMatches: []string{"CPIX"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pattern, err := NormalizePattern(tc.raw)
			if err != nil {
				t.Fatalf("NormalizePattern(%q) lỗi: %v", tc.raw, err)
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				t.Fatalf("pattern sinh ra không compile được: %q: %v", pattern, err)
			}
			for _, s := range tc.matches {
				if !re.MatchString(s) {
					t.Errorf("pattern %q phải khớp %q", pattern, s)
				}
			}
			for _, s := range tc.notMatches {
				if re.MatchString(s) {
					t.Errorf("pattern %q không được khớp %q", pattern, s)
				}
			}
		})
	}
}

func TestNormalizePatternInvalid(t *testing.T) {
	for _, raw := range []string{"", "   ", "(chưa đóng ngoặc", "/pattern/x"} {
		if _, err := NormalizePattern(raw); err == nil {
			t.Errorf("NormalizePattern(%q) phải trả lỗi", raw)
		}
	}
}

func TestValidateTooLong(t *testing.T) {
	long := make([]byte, maxPatternLen+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := Validate(string(long)); err == nil {
		t.Error("pattern quá dài phải bị chặn")
	}
}
