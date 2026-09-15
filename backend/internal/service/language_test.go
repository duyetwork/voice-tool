package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

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

// speechLanguage phân biệt "người dùng chọn" với "hệ thống đoán".
//
// Bài YouTube tiếng Nga từng làm hỏng cả luồng B: hệ thống tự nhận diện ra
// `ru`, 3voices không đọc được, và job chết với câu "AI engine không hỗ trợ
// ngôn ngữ này" — trong khi người dùng chỉ bấm "đọc bài này" chứ chưa từng
// chọn tiếng Nga.
func TestSpeechLanguage(t *testing.T) {
	e := &Engine{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	tts := &fakeTTS{langs: []string{"vi", "en"}}

	cases := []struct {
		name         string
		language     string
		autoDetected bool
		want         string
		wantErr      bool
	}{
		{"tiếng được hỗ trợ thì giữ nguyên", "vi", false, "vi", false},
		{"auto thì để provider tự xử", "auto", true, "", false},
		{"hệ thống đoán ra tiếng lạ -> vẫn đọc, provider tự nhận diện", "ru", true, "", false},
		{"người dùng CHỌN tiếng lạ -> từ chối", "ru", false, "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := e.speechLanguage(context.Background(), tts, tc.language, tc.autoDetected)
			if tc.wantErr {
				if !errors.Is(err, domain.ErrLangUnsupported) {
					t.Fatalf("muốn ErrLangUnsupported, được %v", err)
				}
				// Câu báo phải nói rõ tiếng gì và đọc được những tiếng nào.
				msg := domain.UserMessage(err)
				if !strings.Contains(msg, "ru") || !strings.Contains(msg, "vi") {
					t.Errorf("thông báo thiếu thông tin: %q", msg)
				}
				return
			}
			if err != nil {
				t.Fatalf("không mong lỗi, được %v", err)
			}
			if got != tc.want {
				t.Errorf("speechLanguage() = %q, muốn %q", got, tc.want)
			}
		})
	}
}

type fakeTTS struct{ langs []string }

func (f *fakeTTS) Name() string                 { return "fake" }
func (f *fakeTTS) SupportedLanguages() []string { return f.langs }
func (f *fakeTTS) Synthesize(context.Context, domain.SpeechRequest) ([]byte, error) {
	return []byte("audio"), nil
}
