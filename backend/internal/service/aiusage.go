package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// AIUsage ghi lại MỖI lần gọi nhà cung cấp AI, và quy ra tiền khi đọc.
//
// Lý do tồn tại: màn Cài đặt cho đổi chuỗi dự phòng LLM và bật batch — hai cần
// gạt ảnh hưởng thẳng tới hoá đơn — nhưng cho tới giờ không có con số nào nói
// đổi xong rẻ hơn hay đắt hơn. Tối ưu mà không đo thì chỉ là đổi.
//
// Cùng khuôn mẫu với FetchStats: đây là ĐO ĐẠC, không phải nghiệp vụ. Không có
// hàm nào ở đây được phép làm hỏng một voice vì ghi số liệu thất bại.
type AIUsage struct {
	q        *repository.Queries
	settings *Settings
	log      *slog.Logger
}

func NewAIUsage(q *repository.Queries, settings *Settings, log *slog.Logger) *AIUsage {
	return &AIUsage{q: q, settings: settings, log: log}
}

// Record ghi 1 lần gọi. KHÔNG trả lỗi — xem ghi chú ở đầu file.
//
// Nhận nil receiver để chỗ gọi không phải kiểm tra: các binary chưa nối AIUsage
// (hoặc test) vẫn chạy được đường chính.
func (u *AIUsage) Record(ctx context.Context, owner uuid.UUID, ev domain.AIUsageEvent) {
	if u == nil {
		return
	}
	if !domain.ValidAIKind(ev.Kind) {
		u.log.WarnContext(ctx, "bỏ qua bản ghi ai_usage sai loại", "kind", ev.Kind)
		return
	}

	var userID *uuid.UUID
	if owner != uuid.Nil {
		userID = &owner
	}
	if err := u.q.RecordAIUsage(ctx, repository.RecordAIUsageParams{
		Kind:         ev.Kind,
		Provider:     ev.Provider,
		Model:        ev.Model,
		UserID:       userID,
		InputTokens:  ev.InputTokens,
		OutputTokens: ev.OutputTokens,
		Characters:   ev.Characters,
		AudioSeconds: ev.AudioSeconds,
		Ok:           ev.OK,
	}); err != nil {
		u.log.WarnContext(ctx, "không ghi được ai_usage",
			"error", err, "kind", ev.Kind, "provider", ev.Provider, "model", ev.Model)
	}
}

// ---------------------------------------------------------------------------
// Đọc
// ---------------------------------------------------------------------------

// AIUsageRow là 1 dòng thống kê: 1 ngày × 1 model.
type AIUsageRow struct {
	Day          string  `json:"day"`
	Kind         string  `json:"kind"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Calls        int64   `json:"calls"`
	Failed       int64   `json:"failed"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	Characters   int64   `json:"characters"`
	AudioSeconds float64 `json:"audio_seconds"`
	CostUSD      float64 `json:"cost_usd"`
	// HasPrice = false nghĩa là CostUSD không có ý nghĩa (chưa khai đơn giá),
	// khác hẳn với chi phí bằng 0. Giao diện phải phân biệt hai cái.
	HasPrice bool `json:"has_price"`
}

// AIUsageModel gộp toàn bộ cửa sổ theo model — phần trả lời thẳng câu "model
// nào đang tốn nhất", không bắt người đọc tự cộng các dòng theo ngày.
type AIUsageModel struct {
	Kind         string  `json:"kind"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Calls        int64   `json:"calls"`
	Failed       int64   `json:"failed"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	Characters   int64   `json:"characters"`
	AudioSeconds float64 `json:"audio_seconds"`
	CostUSD      float64 `json:"cost_usd"`
	HasPrice     bool    `json:"has_price"`
}

// AIUsageReport là cả section "Chi phí AI" của màn Cài đặt.
type AIUsageReport struct {
	Days   int32          `json:"days"`
	Daily  []AIUsageRow   `json:"daily"`
	Models []AIUsageModel `json:"models"`
	// TotalUSD chỉ cộng phần ĐÃ CÓ đơn giá. Cộng cả phần chưa khai giá vào (như
	// 0) sẽ cho ra một tổng trông có vẻ đầy đủ mà thiếu, và đó là con số người
	// ta mang đi báo cáo.
	TotalUSD float64 `json:"total_usd"`
	// MissingPrices: những (loại, nhà, model) đã dùng thật nhưng chưa khai giá.
	// Giao diện dùng chính danh sách này để dựng form khai giá — admin không
	// phải tự gõ tên model.
	MissingPrices []domain.AIPrice `json:"missing_prices"`
	Prices        []domain.AIPrice `json:"prices"`
}

// Report gộp lượng dùng `days` ngày gần nhất và quy ra tiền theo bảng giá HIỆN
// TẠI.
//
// Tính lúc đọc chứ không lưu tiền vào từng dòng: khai nhầm một đơn giá thì sửa
// lại là mọi con số đúng ngay, thay vì sai vĩnh viễn trong những dòng đã ghi.
func (u *AIUsage) Report(ctx context.Context, days int32) (AIUsageReport, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := u.q.ListAIUsageDaily(ctx, days)
	if err != nil {
		return AIUsageReport{}, fmt.Errorf("đọc ai_usage: %w", err)
	}

	table := u.settings.AIPrices(ctx)
	priceOf := make(map[string]domain.AIPrice, len(table.Prices))
	for _, p := range table.Prices {
		priceOf[p.Key()] = p
	}

	out := AIUsageReport{Days: days, Daily: make([]AIUsageRow, 0, len(rows)), Prices: table.Prices}
	grouped := map[string]*AIUsageModel{}
	missing := map[string]domain.AIPrice{}

	for _, r := range rows {
		key := domain.PriceKey(r.Kind, r.Provider, r.Model)
		price, priced := priceOf[key]
		cost := price.Cost(r.InputTokens, r.OutputTokens, r.Characters, r.AudioSeconds)

		out.Daily = append(out.Daily, AIUsageRow{
			Day:          r.Day.Time.Format(time.DateOnly),
			Kind:         r.Kind,
			Provider:     r.Provider,
			Model:        r.Model,
			Calls:        r.Calls,
			Failed:       r.Failed,
			InputTokens:  r.InputTokens,
			OutputTokens: r.OutputTokens,
			Characters:   r.Characters,
			AudioSeconds: r.AudioSeconds,
			CostUSD:      cost,
			HasPrice:     priced,
		})

		agg, ok := grouped[key]
		if !ok {
			agg = &AIUsageModel{
				Kind: r.Kind, Provider: r.Provider, Model: r.Model, HasPrice: priced,
			}
			grouped[key] = agg
		}
		agg.Calls += r.Calls
		agg.Failed += r.Failed
		agg.InputTokens += r.InputTokens
		agg.OutputTokens += r.OutputTokens
		agg.Characters += r.Characters
		agg.AudioSeconds += r.AudioSeconds
		agg.CostUSD += cost

		if priced {
			out.TotalUSD += cost
		} else {
			missing[key] = domain.AIPrice{Kind: r.Kind, Provider: r.Provider, Model: r.Model}
		}
	}

	out.Models = make([]AIUsageModel, 0, len(grouped))
	for _, m := range grouped {
		out.Models = append(out.Models, *m)
	}
	// Tốn nhất lên đầu; chưa có giá thì xếp theo số lần gọi — vẫn là thứ tự
	// "đáng để mắt tới trước".
	sort.Slice(out.Models, func(i, j int) bool {
		a, b := out.Models[i], out.Models[j]
		if a.CostUSD != b.CostUSD {
			return a.CostUSD > b.CostUSD
		}
		return a.Calls > b.Calls
	})

	out.MissingPrices = make([]domain.AIPrice, 0, len(missing))
	for _, p := range missing {
		out.MissingPrices = append(out.MissingPrices, p)
	}
	sort.Slice(out.MissingPrices, func(i, j int) bool {
		return out.MissingPrices[i].Key() < out.MissingPrices[j].Key()
	})

	return out, nil
}
