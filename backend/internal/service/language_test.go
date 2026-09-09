package service

import "testing"

func TestResolveLanguageCascade(t *testing.T) {
	cases := []struct {
		name                            string
		post, list, systemDefault, want string
	}{
		{"post override thắng", "en", "vi", "vi", "en"},
		{"không có post thì lấy list", "", "ja", "vi", "ja"},
		{"không có cả hai thì lấy mặc định hệ thống", "", "", "vi", "vi"},
		{"chuẩn hoá về chữ thường", "EN", "vi", "vi", "en"},
		{"rỗng hết thì để auto-detect", "", "", "", "auto"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveLanguage(tc.post, tc.list, tc.systemDefault); got != tc.want {
				t.Errorf("resolveLanguage(%q,%q,%q) = %q, muốn %q",
					tc.post, tc.list, tc.systemDefault, got, tc.want)
			}
		})
	}
}

func TestLanguageSupported(t *testing.T) {
	engine := []string{"vi-VN", "en-US"}

	if !languageSupported("vi", engine) {
		t.Error("vi phải được coi là hỗ trợ khi engine khai báo vi-VN")
	}
	if languageSupported("ja", engine) {
		t.Error("ja không được coi là hỗ trợ")
	}
	if !languageSupported("ja", nil) {
		t.Error("engine không khai báo giới hạn thì coi như hỗ trợ mọi ngôn ngữ")
	}
}
