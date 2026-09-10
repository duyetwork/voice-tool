package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// ThreeVoices — TTS provider 3voices.win.
//
// Hợp đồng API (https://3voices.win/docs/api):
//   - Auth:  Authorization: Bearer sk-ov-...
//   - Body:  multipart/form-data
//   - POST /api/v1/tts/saved  (khi có voice_id)  — dùng giọng đã lưu
//   - POST /api/v1/tts/design (khi chưa có)      — sinh giọng theo mô tả
//   - Sync mode trả về audio/wav binary; async trả job_id (không dùng ở đây
//     vì worker đã chạy bất đồng bộ sẵn).
//   - Rate limit: 10 req/phút, 2 job đồng thời — worker cần điều tiết nếu chạy
//     nhiều process (xem WORKER_CONCURRENCY).
type ThreeVoices struct {
	apiKey  string
	baseURL string
	voiceID string
	http    *http.Client
}

var _ domain.TTSProvider = (*ThreeVoices)(nil)

func NewThreeVoices(apiKey, baseURL, voiceID string) *ThreeVoices {
	if baseURL == "" {
		baseURL = "https://3voices.win"
	}
	return &ThreeVoices{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		voiceID: strings.TrimSpace(voiceID),
		http:    &http.Client{Timeout: 5 * time.Minute, Transport: threeVoicesTransport()},
	}
}

// threeVoicesTransport nới các mốc thời gian của transport mặc định.
//
// Mặc định của Go cho TLS handshake là 10 giây — quá ngắn khi server chạy ở
// Việt Nam đi qua Cloudflare của 3voices: bắt tay TLS thỉnh thoảng chạm mốc đó
// và job chết với "TLS handshake timeout" dù đường mạng vẫn thông. Tổng thời
// gian cả request vẫn bị chặn bởi Client.Timeout (5 phút) nên nới ở đây không
// làm worker treo lâu hơn.
func threeVoicesTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSHandshakeTimeout = 30 * time.Second
	t.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	t.ResponseHeaderTimeout = 5 * time.Minute
	return t
}

func (t *ThreeVoices) Name() string { return "3voices" }

func (t *ThreeVoices) SupportedLanguages() []string {
	return []string{"vi", "en", "zh", "ja", "ko", "fr", "de", "es", "th"}
}

// languageNames: API nhận tên ngôn ngữ dạng chữ, không phải mã ISO.
var languageNames = map[string]string{
	"vi": "vietnamese",
	"en": "english",
	"zh": "chinese",
	"ja": "japanese",
	"ko": "korean",
	"fr": "french",
	"de": "german",
	"es": "spanish",
	"th": "thai",
}

// Giọng mặc định cho /tts/design.
//
// BẮT BUỘC gửi: endpoint design sinh giọng TỪ MÔ TẢ, nên chỉ có `text` thì nó
// từ chối với 400 `{"error":"Provide voice attributes"}` — không có mô tả thì
// nó không biết phải sinh giọng như thế nào.
//
// Giá trị phải nằm trong ĐÚNG bộ từ khoá 3voices công bố, sai một chữ là hỏng
// cả request (họ trả 500 kèm danh sách hợp lệ). Bộ tiếng Anh:
//
//	giới tính : female, male
//	tuổi      : child, teenager, young adult, middle-aged, elderly
//	cao độ    : very low pitch, low pitch, moderate pitch, high pitch,
//	            very high pitch, whisper
//	giọng vùng: american/australian/british/canadian/chinese/indian/japanese/
//	            korean/portuguese/russian accent
//
// KHÔNG có khái niệm "style" (normal/cheerful/…) như tài liệu của họ gợi ý —
// gửi `style=normal` bị từ chối. Đừng thêm lại nếu chưa thấy nó trong danh
// sách trên.
//
// Chọn bộ trung tính (nữ, trẻ, cao độ vừa) vì đây là giọng đọc bản tin. Ai cần
// giọng cố định giữa các voice thì lưu giọng bên 3voices rồi khai
// THREEVOICES_VOICE_ID — lúc đó hệ thống gọi /tts/saved và bộ mặc định này
// không được dùng tới.
const (
	defaultVoiceGender = "female"
	defaultVoiceAge    = "young adult"
	defaultVoicePitch  = "moderate pitch"
)

func (t *ThreeVoices) Synthesize(ctx context.Context, text, language string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, domain.Permanent(domain.Explain(
			"Không có nội dung nào để đọc", fmt.Errorf("3voices: text rỗng")))
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	if err := mw.WriteField("text", text); err != nil {
		return nil, err
	}
	if name := languageNames[baseLang(language)]; name != "" {
		_ = mw.WriteField("language", name)
	}
	_ = mw.WriteField("speed", strconv.FormatFloat(1.0, 'f', 1, 64))

	path := "/api/v1/tts/design"
	if t.voiceID != "" {
		path = "/api/v1/tts/saved"
		_ = mw.WriteField("voice_id", t.voiceID)
	} else {
		_ = mw.WriteField("gender", defaultVoiceGender)
		_ = mw.WriteField("age", defaultVoiceAge)
		_ = mw.WriteField("pitch", defaultVoicePitch)
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+path, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "audio/wav")

	resp, err := t.http.Do(req)
	if err != nil {
		// Lỗi mạng là lỗi TẠM THỜI: không bọc Permanent để Asynq còn retry.
		// Nói rõ là không gọi được tới 3voices, không phải key sai.
		return nil, domain.Explain(
			"Không kết nối được tới 3voices (mạng chập chờn hoặc bắt tay TLS quá lâu) — "+
				"hệ thống sẽ tự thử lại",
			fmt.Errorf("gọi 3voices %s: %w", path, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, t.httpError(resp.StatusCode, path, raw)
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("đọc audio 3voices: %w", err)
	}
	if len(audio) == 0 {
		return nil, domain.Explain(
			"3voices trả về file rỗng — thử lại, nếu vẫn vậy thì báo nhà cung cấp",
			fmt.Errorf("3voices %s trả về audio rỗng", path))
	}
	return audio, nil
}

// httpError dịch mã lỗi HTTP + body của 3voices sang câu người dùng hiểu được
// và làm được gì tiếp theo.
//
// Đây là chỗ DUY NHẤT biết lỗi của nhà cung cấp nghĩa là gì, nên phải giải
// thích ngay tại đây: lên tới UI thì chỉ còn một dòng chữ, không ai đọc được
// "3voices trả về 400" mà biết phải sửa gì.
func (t *ThreeVoices) httpError(status int, path string, raw []byte) error {
	detail := providerMessage(raw)
	technical := fmt.Errorf("3voices %s trả về %d: %s", path, status, strings.TrimSpace(string(raw)))

	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return domain.Permanent(domain.Explain(
			"API key 3voices không hợp lệ hoặc đã bị thu hồi — khai lại key ở mục AI Engine"+
				suffix(detail), technical))
	case status == http.StatusPaymentRequired:
		return domain.Permanent(domain.Explain(
			"Tài khoản 3voices hết credit — nạp thêm rồi chạy lại"+suffix(detail), technical))
	case status == http.StatusTooManyRequests:
		// 429 = vượt rate limit (10 req/phút) -> để Asynq retry với backoff.
		return domain.Explain(
			"3voices báo vượt giới hạn số request — hệ thống sẽ tự thử lại sau"+suffix(detail),
			technical)
	case status == http.StatusRequestEntityTooLarge:
		return domain.Permanent(domain.Explain(
			"Nội dung quá dài so với giới hạn của 3voices — rút ngắn text rồi chạy lại"+suffix(detail),
			technical))
	case status >= 500:
		return domain.Explain(
			"3voices đang lỗi phía họ — hệ thống sẽ tự thử lại sau"+suffix(detail), technical)
	default:
		// 4xx còn lại: lỗi ở request của mình, retry cũng vô ích. Trả nguyên
		// văn câu 3voices nói để còn biết đường sửa.
		return domain.Permanent(domain.Explain(
			fmt.Sprintf("3voices từ chối yêu cầu (lỗi %d)%s", status, suffix(detail)), technical))
	}
}

// providerMessage bóc câu lỗi 3voices trả về. Body của họ là JSON dạng
// {"error": "..."} hoặc {"message": "..."}; không phải JSON thì lấy nguyên văn.
func providerMessage(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil {
		for _, m := range []string{payload.Error, payload.Message, payload.Detail} {
			if m = strings.TrimSpace(m); m != "" {
				return m
			}
		}
	}
	return truncate(trimmed, 200)
}

func suffix(detail string) string {
	if detail == "" {
		return ""
	}
	return " (3voices: " + detail + ")"
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// baseLang bỏ phần region: "vi-VN" -> "vi".
func baseLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		return lang[:i]
	}
	return lang
}
