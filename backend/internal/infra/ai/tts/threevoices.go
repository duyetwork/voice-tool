package tts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
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
		http:    &http.Client{Timeout: 5 * time.Minute},
	}
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

func (t *ThreeVoices) Synthesize(ctx context.Context, text, language string) ([]byte, error) {
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
		return nil, fmt.Errorf("gọi 3voices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		e := fmt.Errorf("3voices trả về %d: %s", resp.StatusCode, string(raw))
		// 429 = vượt rate limit -> để Asynq retry với backoff.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return nil, domain.Permanent(e)
		}
		return nil, e
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("đọc audio 3voices: %w", err)
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("3voices trả về audio rỗng")
	}
	return audio, nil
}

// baseLang bỏ phần region: "vi-VN" -> "vi".
func baseLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		return lang[:i]
	}
	return lang
}
