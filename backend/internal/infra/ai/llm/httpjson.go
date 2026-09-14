package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// httpTimeout — trần cho 1 lần gọi LLM. Batch 5 bài dài có thể mất vài chục
// giây; quá 3 phút thì gần như chắc chắn là treo chứ không phải chậm.
const httpTimeout = 3 * time.Minute

// sharedClient dùng chung cho mọi adapter HTTP để tái sử dụng connection pool.
// Router có thể thử 4 key trong một request — mỗi lần dựng client mới là một
// lần bắt tay TLS thừa.
var sharedClient = &http.Client{Timeout: httpTimeout}

// postJSON gửi 1 request JSON và giải mã response vào `out`.
//
// Lỗi HTTP được PHÂN LOẠI ngay tại đây bằng `classify` của từng nhà: chỉ ở tầng
// này mới còn cả status code lẫn body gốc, lên tới router thì chỉ còn chuỗi lỗi.
func postJSON(
	ctx context.Context,
	url string,
	headers map[string]string,
	body any,
	out any,
	classify func(status int, body []byte, retryAfter time.Duration) error,
) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return domain.LLMFail(domain.LLMFailTransient, fmt.Errorf("dựng request: %w", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return domain.LLMFail(domain.LLMFailTransient, fmt.Errorf("dựng request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := sharedClient.Do(req)
	if err != nil {
		// Đứt mạng / timeout: key không liên quan gì, đừng phạt nó.
		return domain.LLMFail(domain.LLMFailTransient, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.LLMFail(domain.LLMFailTransient, fmt.Errorf("đọc response: %w", err))
	}

	if resp.StatusCode >= 300 {
		return classify(resp.StatusCode, raw, retryAfterOf(resp.Header))
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return domain.LLMFail(domain.LLMFailTransient,
			fmt.Errorf("parse response: %w (%s)", err, truncate(string(raw), 300)))
	}
	return nil
}

// retryAfterOf đọc header Retry-After. Nhà cung cấp biết rõ hơn ta khi nào hạn
// mức hồi — dùng con số của họ thay vì backoff đoán mò.
func retryAfterOf(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// kindForStatus là phần phân loại CHUNG cho mọi nhà, dựa trên mã HTTP.
//
// 400 không có ở đây: mỗi nhà dùng 400 cho một nhóm lý do khác nhau (Gemini
// trả 400 cho cả key sai lẫn input quá dài), nên adapter phải tự đọc body.
func kindForStatus(status int) domain.LLMFailKind {
	switch {
	case status == http.StatusTooManyRequests:
		return domain.LLMFailQuota
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return domain.LLMFailAuth
	case status >= 500:
		return domain.LLMFailTransient
	default:
		return domain.LLMFailTransient
	}
}

// apiErrorBody là phần lỗi chung của cả Gemini lẫn OpenAI: cả hai đều bọc
// trong khoá `error`.
type apiErrorBody struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
		Type    string `json:"type"`
	} `json:"error"`
}

// parseAPIError cố đọc phần `error` của body. Body không phải JSON (HTML của
// một proxy chen ngang, chẳng hạn) thì trả struct rỗng — người gọi rơi về
// nguyên văn body đã cắt ngắn, vẫn hơn là mất sạch thông tin.
func parseAPIError(raw []byte) apiErrorBody {
	var body apiErrorBody
	_ = json.Unmarshal(raw, &body)
	return body
}

func (b apiErrorBody) text(raw []byte) string {
	if msg := strings.TrimSpace(b.Error.Message); msg != "" {
		return msg
	}
	return truncate(string(raw), 300)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// looksLikeQuota nhận ra "hết hạn mức" khi nhà cung cấp báo bằng 400/403 thay
// vì 429 — chuyện xảy ra với cả Gemini (free tier cạn) lẫn OpenAI
// (insufficient_quota, billing hard limit).
func looksLikeQuota(s string) bool {
	s = strings.ToLower(s)
	for _, needle := range []string{
		"quota", "rate limit", "rate_limit", "resource_exhausted",
		"insufficient_quota", "billing", "credit balance", "exceeded your current",
	} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

// looksLikeAuth nhận ra key sai/bị thu hồi khi nó tới dưới dạng 400.
func looksLikeAuth(s string) bool {
	s = strings.ToLower(s)
	for _, needle := range []string{
		"api key not valid", "invalid api key", "invalid_api_key",
		"api key expired", "unauthenticated", "permission denied",
		"authentication", "invalid x-api-key",
	} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

// looksLikeContent nhận ra "lỗi do chính nội dung này" — nhóm mà đổi key là vô
// nghĩa và gây hại (xem domain.LLMFailContent).
func looksLikeContent(s string) bool {
	s = strings.ToLower(s)
	for _, needle := range []string{
		"safety", "blocked", "content policy", "content_policy", "content_filter",
		"prohibited", "too long", "too many tokens", "maximum context",
		"context_length_exceeded", "string too long", "invalid_prompt",
	} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
