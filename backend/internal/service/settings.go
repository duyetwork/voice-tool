package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// settingsCacheTTL — bao lâu thì đọc lại app_setting từ DB.
//
// Chuỗi dự phòng được đọc ở MỖI lần gọi LLM, kể cả trong vòng lặp batch, nên
// đọc thẳng DB mỗi lần là thêm một round-trip vào đường nóng để lấy một giá trị
// thay đổi vài tháng một lần. 30 giây là mức mà admin sửa xong, chờ chưa hết
// một hớp cà phê đã thấy hiệu lực, nhưng worker không phải hỏi DB liên tục.
const settingsCacheTTL = 30 * time.Second

// Settings đọc/ghi bảng app_setting — cấu hình chung của cả hệ thống.
//
// Luật quyền nằm ở handler (chỉ admin vào được route): đây là cấu hình ảnh
// hưởng hạn mức và chi phí của mọi người, không phải dữ liệu của riêng ai, nên
// không có khái niệm "chủ sở hữu" để kiểm tra như ai_engine.
type Settings struct {
	q   *repository.Queries
	log *slog.Logger

	mu     sync.RWMutex
	cached map[string]cachedSetting
}

type cachedSetting struct {
	raw     []byte
	expires time.Time
}

func NewSettings(q *repository.Queries, log *slog.Logger) *Settings {
	return &Settings{q: q, log: log, cached: map[string]cachedSetting{}}
}

// LLMChain trả về chuỗi dự phòng đang cấu hình.
//
// KHÔNG trả lỗi: đây nằm trên đường tạo voice, và "không đọc được app_setting"
// không phải lý do để voice thất bại khi đã có sẵn một chuỗi mặc định đúng.
// Đọc hỏng thì ghi log rồi chạy bằng mặc định.
func (s *Settings) LLMChain(ctx context.Context) []domain.LLMChainStep {
	var wrapper struct {
		Steps []domain.LLMChainStep `json:"steps"`
	}
	if !s.read(ctx, domain.SettingLLMChain, &wrapper) || len(wrapper.Steps) == 0 {
		return domain.DefaultLLMChain
	}
	// Chuỗi đã lưu vẫn phải qua kiểm tra: model có thể bị gỡ khỏi danh sách cho
	// phép sau một lần nâng cấp, và chạy tiếp bằng một ID không còn tồn tại thì
	// mọi key đều "hỏng" mà không ai hiểu vì sao.
	if err := domain.ValidateLLMChain(wrapper.Steps); err != nil {
		s.log.WarnContext(ctx, "chuỗi dự phòng LLM đã lưu không hợp lệ, dùng mặc định",
			"error", err)
		return domain.DefaultLLMChain
	}
	return wrapper.Steps
}

func (s *Settings) LLMBatch(ctx context.Context) domain.LLMBatchConfig {
	cfg := domain.DefaultLLMBatch
	if !s.read(ctx, domain.SettingLLMBatch, &cfg) {
		return domain.DefaultLLMBatch
	}
	if err := domain.ValidateLLMBatch(cfg); err != nil {
		s.log.WarnContext(ctx, "cấu hình batch đã lưu không hợp lệ, dùng mặc định", "error", err)
		return domain.DefaultLLMBatch
	}
	return cfg
}

// ---------------------------------------------------------------------------
// Ghi
// ---------------------------------------------------------------------------

func (s *Settings) SetLLMChain(ctx context.Context, actor uuid.UUID, steps []domain.LLMChainStep) error {
	if err := domain.ValidateLLMChain(steps); err != nil {
		return err
	}
	return s.write(ctx, actor, domain.SettingLLMChain,
		map[string]any{"steps": steps})
}

func (s *Settings) SetLLMBatch(ctx context.Context, actor uuid.UUID, cfg domain.LLMBatchConfig) error {
	if err := domain.ValidateLLMBatch(cfg); err != nil {
		return err
	}
	return s.write(ctx, actor, domain.SettingLLMBatch, cfg)
}

// SettingsView là toàn bộ cấu hình chung, cho màn Cài đặt.
//
// Trả kèm danh sách model cho phép để UI dựng được dropdown mà không phải giữ
// một bản sao của danh sách đó trong code frontend — bản sao ấy chắc chắn sẽ
// lệch với backend ở lần thêm model tiếp theo.
type SettingsView struct {
	LLMChain      []domain.LLMChainStep               `json:"llm_chain"`
	LLMBatch      domain.LLMBatchConfig               `json:"llm_batch"`
	AllowedModels map[domain.LLMProviderName][]string `json:"allowed_models"`
	Providers     []domain.LLMProviderName            `json:"providers"`
}

func (s *Settings) View(ctx context.Context) SettingsView {
	return SettingsView{
		LLMChain:      s.LLMChain(ctx),
		LLMBatch:      s.LLMBatch(ctx),
		AllowedModels: domain.AllowedLLMModels,
		Providers:     domain.AllLLMProviders,
	}
}

// ---------------------------------------------------------------------------

// read nạp 1 khoá vào `out`. false = không có giá trị đã lưu (hoặc đọc hỏng),
// người gọi dùng mặc định của mình.
func (s *Settings) read(ctx context.Context, key string, out any) bool {
	raw, ok := s.rawValue(ctx, key)
	if !ok {
		return false
	}
	if err := json.Unmarshal(raw, out); err != nil {
		s.log.WarnContext(ctx, "app_setting không parse được", "error", err, "key", key)
		return false
	}
	return true
}

func (s *Settings) rawValue(ctx context.Context, key string) ([]byte, bool) {
	s.mu.RLock()
	entry, hit := s.cached[key]
	s.mu.RUnlock()
	if hit && time.Now().Before(entry.expires) {
		return entry.raw, entry.raw != nil
	}

	row, err := s.q.GetAppSetting(ctx, key)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Cache cả trường hợp KHÔNG có giá trị: chưa ai vào Cài đặt là trạng
		// thái bình thường và lâu dài, không có lý do gì hỏi DB mỗi lần.
		s.store(key, nil)
		return nil, false
	case err != nil:
		s.log.WarnContext(ctx, "không đọc được app_setting", "error", err, "key", key)
		return nil, false
	}

	s.store(key, row.Value)
	return row.Value, true
}

func (s *Settings) store(key string, raw []byte) {
	s.mu.Lock()
	s.cached[key] = cachedSetting{raw: raw, expires: time.Now().Add(settingsCacheTTL)}
	s.mu.Unlock()
}

func (s *Settings) write(ctx context.Context, actor uuid.UUID, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("mã hoá app_setting %s: %w", key, err)
	}
	if _, err := s.q.UpsertAppSetting(ctx, repository.UpsertAppSettingParams{
		Key: key, Value: raw, UpdatedBy: &actor,
	}); err != nil {
		return fmt.Errorf("ghi app_setting %s: %w", key, err)
	}

	// Ghi xong là cache của CHÍNH process này hết hiệu lực ngay. Các process
	// khác (worker, scheduler) vẫn chờ hết TTL — chấp nhận được, vì cấu hình
	// này không có ràng buộc phải đồng bộ tức thì giữa các process.
	s.store(key, raw)
	return nil
}
