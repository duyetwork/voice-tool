package llm

import (
	"net/http"
	"testing"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// Phân biệt LỖI DO KEY với LỖI DO NỘI DUNG là chỗ dễ làm sai nhất của cả router.
//
// Xếp nhầm lỗi nội dung thành lỗi key thì hệ thống đốt sạch mọi key của mọi nhà
// trên cùng một input chắc chắn thất bại, rồi báo "hết quota" — trong khi quota
// vẫn còn nguyên, và người dùng đi sửa sai chỗ.
//
// Xếp nhầm theo chiều ngược lại (key sai thành lỗi nội dung) thì một key đã bị
// thu hồi không bao giờ bị tắt, và mọi voice đi qua nó đều hỏng.
func TestGeminiClassify(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   domain.LLMFailKind
	}{
		{
			"key sai tới dưới dạng 400",
			http.StatusBadRequest,
			`{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}`,
			domain.LLMFailAuth,
		},
		{
			"prompt quá dài cũng là 400 — nhưng KHÔNG phải lỗi key",
			http.StatusBadRequest,
			`{"error":{"code":400,"message":"The input token count exceeds the maximum context length","status":"INVALID_ARGUMENT"}}`,
			domain.LLMFailContent,
		},
		{
			"hết hạn mức",
			http.StatusTooManyRequests,
			`{"error":{"code":429,"message":"Quota exceeded for quota metric","status":"RESOURCE_EXHAUSTED"}}`,
			domain.LLMFailQuota,
		},
		{
			"RESOURCE_EXHAUSTED báo bằng 403",
			http.StatusForbidden,
			`{"error":{"code":403,"message":"Resource has been exhausted","status":"RESOURCE_EXHAUSTED"}}`,
			domain.LLMFailQuota,
		},
		{
			"nhà cung cấp sập",
			http.StatusServiceUnavailable,
			`{"error":{"code":503,"message":"The model is overloaded"}}`,
			domain.LLMFailTransient,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := geminiClassify(tc.status, []byte(tc.body), 0)
			if got := domain.LLMFailureOf(err).Kind; got != tc.want {
				t.Errorf("Kind = %v, muốn %v (lỗi: %v)", got, tc.want, err)
			}
		})
	}
}

func TestOpenAIClassify(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   domain.LLMFailKind
	}{
		{
			// 429 nhưng không tự hồi theo thời gian — vẫn là quota (key không
			// sai), chỉ là khoảng nghỉ phải dài hơn rate-limit thường.
			"hết tiền",
			http.StatusTooManyRequests,
			`{"error":{"message":"You exceeded your current quota","type":"insufficient_quota"}}`,
			domain.LLMFailQuota,
		},
		{
			"rate-limit theo phút",
			http.StatusTooManyRequests,
			`{"error":{"message":"Rate limit reached","type":"rate_limit_error"}}`,
			domain.LLMFailQuota,
		},
		{
			"key bị thu hồi",
			http.StatusUnauthorized,
			`{"error":{"message":"Incorrect API key provided","type":"invalid_request_error"}}`,
			domain.LLMFailAuth,
		},
		{
			"nội dung bị chặn",
			http.StatusBadRequest,
			`{"error":{"message":"Your request was rejected as a result of our safety system","type":"invalid_request_error"}}`,
			domain.LLMFailContent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := openAIClassify(tc.status, []byte(tc.body), 0)
			if got := domain.LLMFailureOf(err).Kind; got != tc.want {
				t.Errorf("Kind = %v, muốn %v (lỗi: %v)", got, tc.want, err)
			}
		})
	}
}

// Hết tiền thì thử lại mỗi phút chỉ tạo rác trong log — phải có khoảng nghỉ dài
// ngay cả khi nhà cung cấp không gửi Retry-After.
func TestOpenAIHetTienNghiDai(t *testing.T) {
	err := openAIClassify(http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"quota","type":"insufficient_quota"}}`), 0)

	if got := domain.LLMFailureOf(err).RetryAfter; got < time.Minute {
		t.Errorf("RetryAfter = %v, muốn ít nhất vài phút", got)
	}
}

// Con số của nhà cung cấp luôn thắng suy đoán của ta.
func TestRetryAfterCuaNhaCungCapThang(t *testing.T) {
	err := geminiClassify(http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"quota"}}`), 42*time.Second)

	if got := domain.LLMFailureOf(err).RetryAfter; got != 42*time.Second {
		t.Errorf("RetryAfter = %v, muốn 42s", got)
	}
}

// Body không phải JSON (HTML của một proxy chen ngang) vẫn phải ra lỗi đọc được,
// không được panic và không được mất sạch thông tin.
func TestClassifyBodyKhongPhaiJSON(t *testing.T) {
	err := geminiClassify(http.StatusBadGateway, []byte("<html>502 Bad Gateway</html>"), 0)
	if err == nil {
		t.Fatal("muốn lỗi")
	}
	if domain.LLMFailureOf(err).Kind != domain.LLMFailTransient {
		t.Errorf("502 phải là lỗi tạm thời, được %v", domain.LLMFailureOf(err).Kind)
	}
}
