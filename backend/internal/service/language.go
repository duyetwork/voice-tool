package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// noLLMReason là lý do DUY NHẤT khiến hình thức C bị tắt ngoài cấu hình.
//
// Không nhắc "khởi động lại" nữa: điều kiện của mode C được đọc lại từ DB
// trong lúc chạy, nên thêm Bộ API key xong là hình thức C bật ngay.
const noLLMReason = "chưa có Bộ API key LLM"

// ModeGate quyết định hình thức thu thập nào đang dùng được, và VÌ SAO cái
// còn lại thì không.
//
// Có 2 nguồn khoá khác nhau nên phải kèm lý do, không thể chỉ trả "chưa hỗ
// trợ": người vận hành tự tắt trong ENABLED_COLLECT_MODES, hoặc hệ thống chưa
// có LLM thật (mode C không có gì để viết lại nội dung).
type ModeGate struct {
	// Enabled: nil = chưa cấu hình gì, cho qua tất cả (zero value dùng trong
	// test). Slice RỖNG khác nil: nó nghĩa là "không mode nào dùng được".
	Enabled []domain.CollectMode
	// Reason: mode -> câu giải thích vì sao bị tắt. Không có trong map nghĩa
	// là do cấu hình ENABLED_COLLECT_MODES.
	Reason map[domain.CollectMode]string
	// LLM trả lời "hệ thống có LLM thật không", ĐỌC LẠI TRONG LÚC CHẠY.
	//
	// nil = không phải hỏi (LLM_PROVIDER trong .env đã là provider thật, hoặc
	// đang trong test). Khác nil thì điều kiện của mode C nằm ở DB: người dùng
	// thêm Bộ API key qua giao diện lúc nào cũng được, và cái gate tính một lần
	// lúc khởi động sẽ vẫn báo "đang tắt" cho tới lần restart kế tiếp — tức là
	// nói sai về một hệ thống đã cấu hình đủ.
	LLM LLMReadiness
}

// LLMReadiness cho biết hiện có đường nào chạy được mode C hay không.
type LLMReadiness interface {
	HasRealLLM(ctx context.Context) bool
}

// Allows cho biết mode có đang bật không.
func (g ModeGate) Allows(ctx context.Context, mode domain.CollectMode) bool {
	if g.Enabled != nil && !slices.Contains(g.Enabled, mode) {
		return false
	}
	if mode == domain.ModePromptToVoice && g.LLM != nil {
		return g.LLM.HasRealLLM(ctx)
	}
	return true
}

// Why trả lý do mode bị tắt; rỗng nghĩa là mode đang bật.
func (g ModeGate) Why(ctx context.Context, mode domain.CollectMode) string {
	if g.Allows(ctx, mode) {
		return ""
	}
	if r := g.Reason[mode]; r != "" {
		return r
	}
	// Thứ tự quan trọng: cấu hình tắt hẳn mode thì nói về cấu hình, đừng đổ cho
	// việc thiếu key — người vận hành sẽ đi thêm key và không hiểu vì sao vô ích.
	if g.Enabled != nil && !slices.Contains(g.Enabled, mode) {
		return "chưa được bật trong cấu hình ENABLED_COLLECT_MODES của server"
	}
	if mode == domain.ModePromptToVoice && g.LLM != nil {
		return noLLMReason
	}
	return "chưa được bật trong cấu hình ENABLED_COLLECT_MODES của server"
}

// Check chặn hình thức chưa dùng được, kèm lý do cụ thể — API từ chối sớm thay
// vì để job chạy rồi mới fail trong worker.
func (g ModeGate) Check(ctx context.Context, mode domain.CollectMode) error {
	if g.Allows(ctx, mode) {
		return nil
	}
	return fmt.Errorf("%w: hình thức thu thập %s %s",
		domain.ErrInvalidInput, mode, g.Why(ctx, mode))
}

// llmProbeTTL — mode C được hỏi vài lần cho mỗi lần tải trang (mỗi hình thức 1
// lần ở /meta/collect-modes), nên câu trả lời phải được nhớ lại. 30 giây đủ
// ngắn để người vừa thêm key thấy hình thức C bật lên gần như ngay, và đủ dài
// để không biến mỗi lần mở trang thành một loạt query.
const llmProbeTTL = 30 * time.Second

// LLMKeyProbe trả lời "trong DB có Bộ API key LLM nào không", có nhớ tạm.
type LLMKeyProbe struct {
	q   *repository.Queries
	log *slog.Logger

	mu      sync.Mutex
	ok      bool
	checked time.Time
}

var _ LLMReadiness = (*LLMKeyProbe)(nil)

func NewLLMKeyProbe(q *repository.Queries, log *slog.Logger) *LLMKeyProbe {
	return &LLMKeyProbe{q: q, log: log}
}

func (p *LLMKeyProbe) HasRealLLM(ctx context.Context) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.checked.IsZero() && time.Since(p.checked) < llmProbeTTL {
		return p.ok
	}
	n, err := p.q.CountLLMAPIKeys(ctx)
	if err != nil {
		// Giữ nguyên câu trả lời cũ và thử lại ở lần sau: một cú DB hỏng thoáng
		// qua không phải là bằng chứng rằng người dùng đã xoá hết key.
		p.log.WarnContext(ctx, "không đếm được key LLM, giữ trạng thái mode C như cũ",
			"error", err, "mode_c_đang", p.ok)
		return p.ok
	}
	p.ok = n > 0
	p.checked = time.Now()
	return p.ok
}

// resolveLanguage cài đặt cascade ngôn ngữ theo tầng (business rule #9):
//
//	SourcePost.language (override) > List.language_default > mặc định hệ thống
//
// Mode A không dùng ngôn ngữ để xử lý voice, chỉ dùng để gắn nhãn — nên vẫn
// resolve bình thường, chỉ khác là không truyền vào TTS.
func resolveLanguage(postLevel, listLevel, systemDefault string) string {
	for _, candidate := range []string{postLevel, listLevel, systemDefault} {
		if v := strings.TrimSpace(candidate); v != "" {
			return strings.ToLower(v)
		}
	}
	// Không ai chốt ngôn ngữ -> để nền tảng/multime tự nhận diện.
	return domain.LanguageAuto
}

// languageSupported kiểm tra engine có hỗ trợ ngôn ngữ đã chọn (specs 3.3).
// Danh sách rỗng nghĩa là engine không khai báo giới hạn -> coi như hỗ trợ.
func languageSupported(lang string, supported []string) bool {
	if len(supported) == 0 {
		return true
	}
	base := baseLanguage(lang)
	for _, s := range supported {
		if baseLanguage(s) == base {
			return true
		}
	}
	return false
}

// baseLanguage bỏ phần region: "vi-VN" -> "vi".
func baseLanguage(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		return lang[:i]
	}
	return lang
}
