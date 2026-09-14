package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Bộ API key LLM — credential và factory
// ---------------------------------------------------------------------------

// LLMProviderName là mã nhà cung cấp LLM. Đúng 3 giá trị mà CHECK constraint
// của llm_api_key chấp nhận — thêm nhà mới phải đi kèm migration.
type LLMProviderName string

const (
	LLMGemini    LLMProviderName = "gemini"
	LLMOpenAI    LLMProviderName = "openai"
	LLMAnthropic LLMProviderName = "anthropic"
)

func (p LLMProviderName) Valid() bool {
	switch p {
	case LLMGemini, LLMOpenAI, LLMAnthropic:
		return true
	}
	return false
}

// AllLLMProviders là danh sách để UI dựng ô chọn nhà cung cấp.
var AllLLMProviders = []LLMProviderName{LLMGemini, LLMOpenAI, LLMAnthropic}

// LLMCredential đủ để dựng 1 provider LLM cụ thể: key của ai, model nào.
//
// Cùng lý do tồn tại với TTSCredential: key không còn là hằng số trong .env mà
// là dữ liệu theo từng bản ghi (bảng llm_api_key), và model thì đổi theo chuỗi
// dự phòng đang cấu hình — nên không thể dựng sẵn 1 provider lúc khởi động.
type LLMCredential struct {
	Provider LLMProviderName
	APIKey   string
	Model    string
}

// LLMFactory dựng provider LLM từ credential, lặp lại đúng khuôn TTSFactory.
type LLMFactory interface {
	For(cred LLMCredential) (LLMProvider, error)
}

// ---------------------------------------------------------------------------
// Phân loại lỗi — phần dễ làm sai nhất của router
// ---------------------------------------------------------------------------

// LLMFailKind nói vì sao một lần gọi LLM thất bại, và qua đó nói router phải
// làm gì tiếp.
//
// Điểm mấu chốt là tách LLMFailContent ra khỏi phần còn lại. Lỗi do NỘI DUNG
// (bị chặn vì chính sách, input quá dài) mà cũng xoay key thì hệ thống sẽ đốt
// sạch mọi key của mọi nhà trên cùng một input chắc chắn thất bại, rồi báo
// "hết quota" — trong khi quota vẫn còn nguyên, và người dùng đi sửa sai chỗ.
type LLMFailKind int

const (
	// LLMFailTransient — 5xx, đứt mạng, timeout. Key vẫn tốt: thử lại tại chỗ.
	LLMFailTransient LLMFailKind = iota
	// LLMFailQuota — 429 hoặc hết hạn mức. Key vẫn đúng, chỉ cần NGHỈ.
	LLMFailQuota
	// LLMFailAuth — 401/403, key sai hoặc bị thu hồi. Chờ bao lâu cũng không
	// tự khỏi, phải có người dán key mới.
	LLMFailAuth
	// LLMFailContent — nhà cung cấp từ chối chính nội dung này. Đổi key là vô
	// nghĩa; chỉ đổi sang MODEL khác mới có cơ may.
	LLMFailContent
)

func (k LLMFailKind) String() string {
	switch k {
	case LLMFailQuota:
		return "quota"
	case LLMFailAuth:
		return "auth"
	case LLMFailContent:
		return "content"
	default:
		return "transient"
	}
}

// LLMError là lỗi đã được adapter phân loại. Adapter là nơi DUY NHẤT biết cách
// đọc mã lỗi của nhà mình, nên việc phân loại nằm ở adapter chứ không ở router.
type LLMError struct {
	Kind LLMFailKind
	// RetryAfter là khoảng nghỉ nhà cung cấp yêu cầu (header Retry-After hoặc
	// mốc reset hạn mức ngày). 0 = nhà cung cấp không nói gì, router tự tính.
	RetryAfter time.Duration
	Err        error
}

func (e *LLMError) Error() string {
	if e.Err == nil {
		return "lỗi LLM (" + e.Kind.String() + ")"
	}
	return e.Err.Error()
}

func (e *LLMError) Unwrap() error { return e.Err }

// LLMFail bọc 1 lỗi kèm phân loại.
func LLMFail(kind LLMFailKind, err error) error {
	return &LLMError{Kind: kind, Err: err}
}

// LLMFailAfter bọc lỗi kèm khoảng nghỉ nhà cung cấp yêu cầu.
func LLMFailAfter(kind LLMFailKind, retryAfter time.Duration, err error) error {
	return &LLMError{Kind: kind, RetryAfter: retryAfter, Err: err}
}

// LLMFailureOf đọc phân loại ra khỏi chuỗi lỗi. Lỗi chưa được phân loại (bug
// của adapter, hoặc lỗi từ chỗ khác lọt vào) tính là TẠM THỜI: đoán sai theo
// hướng đó chỉ tốn thêm một lần thử, còn đoán sai theo hướng "quota" thì bắt
// một key còn tốt đi nghỉ.
func LLMFailureOf(err error) *LLMError {
	var le *LLMError
	if errors.As(err, &le) {
		return le
	}
	return &LLMError{Kind: LLMFailTransient, Err: err}
}

// ---------------------------------------------------------------------------
// Chuỗi dự phòng
// ---------------------------------------------------------------------------

// LLMChainStep là 1 mắt xích: gọi model này, của nhà này.
type LLMChainStep struct {
	Provider LLMProviderName `json:"provider"`
	Model    string          `json:"model"`
}

// DefaultLLMChain là chuỗi dự phòng mặc định, rẻ trước đắt sau.
//
// GHIM ID ĐẦY ĐỦ, KHÔNG DÙNG ALIAS. Alias `gpt-5.6` không trỏ về Luna mà trỏ
// về Sol ($5.00/$30.00 cho 1M token) — đắt hơn khoảng 25 lần. Viết thiếu hậu
// tố `-luna` thì hệ thống vẫn chạy đúng, không có lỗi nào hiện ra, chỉ có hoá
// đơn đội lên; và vì đây là mắt xích chỉ chạy khi Gemini đã cạn quota nên sẽ
// rất lâu mới có ai nhận ra. Cùng lý do, model Anthropic ghim bản có ngày.
var DefaultLLMChain = []LLMChainStep{
	{LLMGemini, "gemini-2.5-flash-lite"},
	{LLMGemini, "gemini-2.5-flash"},
	{LLMOpenAI, "gpt-5.6-luna"},
	{LLMAnthropic, "claude-haiku-4-5-20251001"},
}

// AllowedLLMModels là danh sách model được phép đặt vào chuỗi.
//
// Tồn tại để chặn đúng cái bẫy alias ở trên: admin sửa chuỗi trong Cài đặt mà
// gõ `gpt-5.6` thì bị từ chối ngay tại chỗ, thay vì phát hiện qua hoá đơn.
var AllowedLLMModels = map[LLMProviderName][]string{
	LLMGemini: {
		"gemini-2.5-flash-lite",
		"gemini-2.5-flash",
		"gemini-2.5-pro",
	},
	LLMOpenAI: {
		"gpt-5.6-luna",
		"gpt-5.6-sol",
	},
	LLMAnthropic: {
		"claude-haiku-4-5-20251001",
		"claude-sonnet-5",
		"claude-opus-5",
	},
}

// ValidateLLMChain kiểm tra chuỗi trước khi lưu.
func ValidateLLMChain(steps []LLMChainStep) error {
	if len(steps) == 0 {
		return fmt.Errorf("%w: chuỗi dự phòng phải có ít nhất 1 mắt xích", ErrInvalidInput)
	}
	for i, s := range steps {
		if !s.Provider.Valid() {
			return fmt.Errorf("%w: mắt xích %d có nhà cung cấp %q không hợp lệ",
				ErrInvalidInput, i+1, s.Provider)
		}
		allowed := AllowedLLMModels[s.Provider]
		if !slices.Contains(allowed, s.Model) {
			return fmt.Errorf(
				"%w: mắt xích %d dùng model %q không nằm trong danh sách cho phép của %s (%s)",
				ErrInvalidInput, i+1, s.Model, s.Provider, strings.Join(allowed, ", "))
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Batch
// ---------------------------------------------------------------------------

// LLMBatchConfig gom N mẩu text vào 1 request để giảm chi phí và số lần gọi.
//
// Hai con số mặc định dưới đây là ĐIỂM KHỞI ĐẦU PHẢI ĐO LẠI, không phải hằng
// số đúng sẵn: chốt bằng cách chạy thử trên text thật rồi so chất lượng + chi
// phí. Chúng nằm trong app_setting đúng vì lý do đó.
type LLMBatchConfig struct {
	Enabled bool `json:"enabled"`
	// Size — số mẩu mỗi request. 5 đủ để cắt ~5 lần số request mà mỗi mẩu vẫn
	// còn ngắn để model giữ chất lượng.
	Size int `json:"size"`
	// MaxChars — chặn trên theo ký tự: 5 bài dài vẫn có thể vượt ngưỡng chất
	// lượng dù đúng Size.
	MaxChars int `json:"max_chars"`
	// WaitMS — chờ bao lâu để gom đủ mẩu trước khi gửi đi.
	//
	// Batch chỉ gom được những mẩu ĐANG CÙNG CHỜ, mà worker thì nhận task rải
	// rác chứ không nhận cả lô một lúc. Không chờ chút nào thì mọi lô đều có
	// đúng 1 mẩu và batch vô tác dụng; chờ quá lâu thì người tạo voice lẻ trên
	// UI phải đợi thêm chừng ấy giây mà chẳng gom được ai.
	//
	// 2 giây: đủ để các task của cùng một vòng quét gặp nhau, và nhỏ so với
	// thời gian tạo một voice (hàng chục giây).
	WaitMS int `json:"wait_ms"`
}

var DefaultLLMBatch = LLMBatchConfig{Enabled: true, Size: 5, MaxChars: 12000, WaitMS: 2000}

const (
	maxBatchSize   = 50
	maxBatchChars  = 200000
	maxBatchWaitMS = 30000
)

func ValidateLLMBatch(c LLMBatchConfig) error {
	if c.Size < 1 || c.Size > maxBatchSize {
		return fmt.Errorf("%w: batch size phải trong khoảng 1–%d", ErrInvalidInput, maxBatchSize)
	}
	if c.MaxChars < 500 || c.MaxChars > maxBatchChars {
		return fmt.Errorf("%w: batch max_chars phải trong khoảng 500–%d",
			ErrInvalidInput, maxBatchChars)
	}
	if c.WaitMS < 0 || c.WaitMS > maxBatchWaitMS {
		return fmt.Errorf("%w: batch wait_ms phải trong khoảng 0–%d",
			ErrInvalidInput, maxBatchWaitMS)
	}
	return nil
}

// Wait là WaitMS ở dạng time.Duration.
func (c LLMBatchConfig) Wait() time.Duration {
	return time.Duration(c.WaitMS) * time.Millisecond
}

// ---------------------------------------------------------------------------
// Khoá app_setting
// ---------------------------------------------------------------------------

const (
	SettingLLMChain = "llm.chain"
	SettingLLMBatch = "llm.batch"
)
