package tts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// /tts/design sinh giọng TỪ MÔ TẢ: chỉ gửi `text` thì 3voices trả 400
// "Provide voice attributes". Đây là lỗi đã xảy ra thật, nên test giữ đúng bộ
// thuộc tính mặc định được gửi kèm.
func TestSynthesizeGuiVoiceAttributesChoDesign(t *testing.T) {
	var (
		gotPath string
		gotForm map[string]string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotForm = formFields(t, r)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFfake-audio"))
	}))
	defer srv.Close()

	audio, err := NewThreeVoices("sk-ov-test", srv.URL, "").
		Synthesize(context.Background(), domain.SpeechRequest{Text: "Bản tin sáng nay", Language: "vi"})
	if err != nil {
		t.Fatalf("Synthesize lỗi: %v", err)
	}
	if len(audio) == 0 {
		t.Fatal("không nhận được audio")
	}

	if gotPath != "/api/v1/tts/design" {
		t.Errorf("path = %q, muốn /api/v1/tts/design", gotPath)
	}
	for field, want := range map[string]string{
		"text":     "Bản tin sáng nay",
		"language": "vietnamese",
		"gender":   defaultVoiceGender,
		"age":      defaultVoiceAge,
		"pitch":    defaultVoicePitch,
	} {
		if gotForm[field] != want {
			t.Errorf("field %s = %q, muốn %q", field, gotForm[field], want)
		}
	}
}

// Có voice_id thì dùng giọng đã lưu, và KHÔNG gửi thuộc tính mô tả giọng —
// /tts/saved không nhận chúng.
func TestSynthesizeDungGiongDaLuu(t *testing.T) {
	var (
		gotPath string
		gotForm map[string]string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotForm = formFields(t, r)
		_, _ = w.Write([]byte("RIFFfake-audio"))
	}))
	defer srv.Close()

	if _, err := NewThreeVoices("sk-ov-test", srv.URL, "42").
		Synthesize(context.Background(), domain.SpeechRequest{Text: "xin chào", Language: "vi"}); err != nil {
		t.Fatalf("Synthesize lỗi: %v", err)
	}

	if gotPath != "/api/v1/tts/saved" {
		t.Errorf("path = %q, muốn /api/v1/tts/saved", gotPath)
	}
	if gotForm["voice_id"] != "42" {
		t.Errorf("voice_id = %q, muốn 42", gotForm["voice_id"])
	}
	if _, ok := gotForm["gender"]; ok {
		t.Error("giọng đã lưu thì không gửi kèm thuộc tính mô tả giọng")
	}
}

// Lỗi của nhà cung cấp phải nói được người dùng cần làm gì, và mã lỗi quyết
// định có retry hay không.
func TestSynthesizeDichLoiNhaCungCap(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantInMsg   string
		wantPermain bool
	}{
		{"key sai", http.StatusUnauthorized, `{"error":"Invalid API key"}`,
			"khai lại key ở mục AI Engine", true},
		{"hết credit", http.StatusPaymentRequired, `{"error":"Insufficient credit"}`,
			"hết credit", true},
		{"vượt rate limit thì còn retry", http.StatusTooManyRequests, `{"error":"Rate limit"}`,
			"tự thử lại", false},
		{"lỗi phía họ thì còn retry", http.StatusBadGateway, "bad gateway",
			"đang lỗi phía họ", false},
		{"400 khác: trả nguyên văn câu 3voices nói", http.StatusBadRequest,
			`{"error":"Provide voice attributes"}`, "Provide voice attributes", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			_, err := NewThreeVoices("sk-ov-test", srv.URL, "").
				Synthesize(context.Background(), domain.SpeechRequest{Text: "xin chào", Language: "vi"})
			if err == nil {
				t.Fatal("muốn lỗi, được nil")
			}

			msg := domain.UserMessage(err)
			if !strings.Contains(msg, tc.wantInMsg) {
				t.Errorf("thông báo = %q, muốn chứa %q", msg, tc.wantInMsg)
			}
			if strings.Contains(msg, "xem log") {
				t.Errorf("không được báo chung chung: %q", msg)
			}
			if got := domain.IsPermanent(err); got != tc.wantPermain {
				t.Errorf("IsPermanent = %v, muốn %v", got, tc.wantPermain)
			}
		})
	}
}

// Text rỗng chặn ngay tại chỗ: gọi ra 3voices chỉ để nhận 400 là tốn 1 lượt
// rate limit vô ích.
func TestSynthesizeChanTextRong(t *testing.T) {
	_, err := NewThreeVoices("sk-ov-test", "http://khong-goi-toi", "").
		Synthesize(context.Background(), domain.SpeechRequest{Text: "   ", Language: "vi"})
	if err == nil {
		t.Fatal("muốn lỗi, được nil")
	}
	if !domain.IsPermanent(err) {
		t.Error("text rỗng là lỗi vĩnh viễn, retry vô nghĩa")
	}
	var ue *domain.UserError
	if !errors.As(err, &ue) {
		t.Error("phải có câu giải thích cho người dùng")
	}
}

func formFields(t *testing.T, r *http.Request) map[string]string {
	t.Helper()
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("đọc multipart lỗi: %v", err)
	}
	out := make(map[string]string, len(r.MultipartForm.Value))
	for k, v := range r.MultipartForm.Value {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

// Người dùng chỉnh "Cấu hình giọng đọc" thì cấu hình đó THẮNG voice_id.
//
// Lý do phải có test: /tts/saved bỏ qua gender/age/pitch/accent, nên nếu vẫn đi
// đường saved thì họ chỉnh xong nghe lại thấy y hệt cũ và không có gì giải
// thích vì sao.
func TestSynthesizeCauHinhThangGiongDaLuu(t *testing.T) {
	var (
		gotPath string
		gotForm map[string]string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotForm = formFields(t, r)
		_, _ = w.Write([]byte("RIFFfake-audio"))
	}))
	defer srv.Close()

	speed := 1.25
	if _, err := NewThreeVoices("sk-ov-test", srv.URL, "42").
		Synthesize(context.Background(), domain.SpeechRequest{
			Text:     "xin chào",
			Language: "vi",
			Style: domain.VoiceStyle{
				Gender: "male",
				Age:    "elderly",
				Pitch:  "low pitch",
				Accent: "british accent",
				Speed:  &speed,
			},
		}); err != nil {
		t.Fatalf("Synthesize lỗi: %v", err)
	}

	if gotPath != "/api/v1/tts/design" {
		t.Errorf("path = %q, muốn /api/v1/tts/design — có cấu hình thì không dùng giọng đã lưu", gotPath)
	}
	if _, ok := gotForm["voice_id"]; ok {
		t.Error("đã chuyển sang design thì không gửi voice_id nữa")
	}
	for field, want := range map[string]string{
		"gender": "male",
		"age":    "elderly",
		"pitch":  "low pitch",
		"accent": "british accent",
		"speed":  "1.25",
	} {
		if gotForm[field] != want {
			t.Errorf("field %s = %q, muốn %q", field, gotForm[field], want)
		}
	}
}

// Trường bỏ trống rơi về mặc định, và accent KHÔNG có mặc định — không ép chất
// giọng vùng miền nào vào bản tin khi người dùng không chọn.
func TestSynthesizeCauHinhMotPhanRoiVeMacDinh(t *testing.T) {
	var gotForm map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotForm = formFields(t, r)
		_, _ = w.Write([]byte("RIFFfake-audio"))
	}))
	defer srv.Close()

	if _, err := NewThreeVoices("sk-ov-test", srv.URL, "").
		Synthesize(context.Background(), domain.SpeechRequest{
			Text:     "xin chào",
			Language: "vi",
			Style:    domain.VoiceStyle{Pitch: "whisper"},
		}); err != nil {
		t.Fatalf("Synthesize lỗi: %v", err)
	}

	if gotForm["pitch"] != "whisper" {
		t.Errorf("pitch = %q, muốn whisper", gotForm["pitch"])
	}
	if gotForm["gender"] != defaultVoiceGender || gotForm["age"] != defaultVoiceAge {
		t.Errorf("gender/age = %q/%q, muốn mặc định %q/%q",
			gotForm["gender"], gotForm["age"], defaultVoiceGender, defaultVoiceAge)
	}
	if gotForm["accent"] != "" {
		t.Errorf("accent = %q, muốn không gửi khi người dùng không chọn", gotForm["accent"])
	}
	if gotForm["speed"] != "1.00" {
		t.Errorf("speed = %q, muốn 1.00", gotForm["speed"])
	}
}
