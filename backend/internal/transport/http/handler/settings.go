package handler

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/middleware"
)

// Settings — cấu hình chung của hệ thống (chuỗi dự phòng LLM, batch) và thống
// kê lỗi bị nền tảng chặn.
//
// Route nằm trong nhóm admin: đây là cấu hình ảnh hưởng HẠN MỨC VÀ CHI PHÍ của
// cả hệ thống, không phải dữ liệu của riêng ai — nên quyền chặn được bằng
// middleware, khác với Bộ API key (phải kiểm tra theo chủ sở hữu từng bản ghi).
type Settings struct {
	svc   *service.Settings
	stats *service.FetchStats
	usage *service.AIUsage
}

func NewSettings(svc *service.Settings, stats *service.FetchStats, usage *service.AIUsage) *Settings {
	return &Settings{svc: svc, stats: stats, usage: usage}
}

// Get trả cấu hình đang dùng KÈM danh sách model cho phép, để UI dựng dropdown
// mà không phải giữ một bản sao của danh sách đó trong code frontend — bản sao
// ấy chắc chắn sẽ lệch với backend ở lần thêm model tiếp theo.
func (h *Settings) Get(c *gin.Context) {
	httpx.OK(c, h.svc.View(c.Request.Context()))
}

type updateSettingsRequest struct {
	// LLMChain khác nil = thay toàn bộ chuỗi dự phòng.
	LLMChain *[]domain.LLMChainStep `json:"llm_chain"`
	LLMBatch *domain.LLMBatchConfig `json:"llm_batch"`
	// AIPrices khác nil = thay TOÀN BỘ bảng giá. Không sửa từng dòng: bảng giá
	// chỉ vài dòng, và gửi cả bảng thì không có trạng thái nửa vời khi hai
	// admin sửa cùng lúc.
	AIPrices *[]domain.AIPrice `json:"ai_prices"`
}

func (h *Settings) Update(c *gin.Context) {
	var req updateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	actor := middleware.ActorID(c)

	if req.LLMChain != nil {
		if err := h.svc.SetLLMChain(c.Request.Context(), actor, *req.LLMChain); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	if req.LLMBatch != nil {
		if err := h.svc.SetLLMBatch(c.Request.Context(), actor, *req.LLMBatch); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	if req.AIPrices != nil {
		table := domain.AIPriceTable{Prices: *req.AIPrices}
		if err := h.svc.SetAIPrices(c.Request.Context(), actor, table); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	httpx.OK(c, h.svc.View(c.Request.Context()))
}

// FetchStats trả số lần từng nền tảng chặn ta, gộp theo ngày.
//
// Đây là dữ liệu để trả lời một câu hỏi cụ thể: có đáng mua proxy không. Nhìn
// riêng `bot_block` và `rate_limit` — hai loại proxy giải quyết được; còn
// `login_required` thì proxy không giúp gì, phải có cookies.
func (h *Settings) FetchStats(c *gin.Context) {
	days, err := daysParam(c, 7)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	rows, err := h.stats.List(c.Request.Context(), days)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": rows, "days": days})
}

// AIUsage trả lượng dùng AI đã đo được, quy ra tiền theo bảng giá admin khai.
//
// Đây là nửa còn thiếu của mục "Tối ưu chi phí AI": màn Cài đặt vốn cho đổi
// chuỗi dự phòng và bật batch — hai cần gạt ảnh hưởng thẳng tới hoá đơn —
// nhưng không có con số nào để biết đổi xong rẻ hơn hay đắt hơn.
func (h *Settings) AIUsage(c *gin.Context) {
	days, err := daysParam(c, 30)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	report, err := h.usage.Report(c.Request.Context(), days)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, report)
}

// daysParam đọc ?days= với trần 365 — cửa sổ dài hơn thế thì bảng đã bị job dọn
// dẹp cắt mất phần đuôi, trả về một khoảng trống trông như "tháng đó không ai
// dùng".
func daysParam(c *gin.Context, def int32) (int32, error) {
	raw := c.Query("days")
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > 365 {
		return 0, fmt.Errorf("%w: days phải là số nguyên trong khoảng 1–365",
			domain.ErrInvalidInput)
	}
	return int32(n), nil
}
