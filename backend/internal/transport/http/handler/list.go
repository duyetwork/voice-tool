package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/repository"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/middleware"
)

// scheduleRequest là phần lịch quét dùng chung cho cả 2 loại kênh.
//
// Giờ đi qua API dưới dạng SỐ PHÚT TÍNH TỪ NỬA ĐÊM (0–1439), khớp thẳng với
// giá trị của <input type="time"> sau khi tách, và khớp thẳng với cột trong DB
// — không có chỗ nào phải parse chuỗi giờ, nên cũng không có chỗ nào parse sai.
type scheduleRequest struct {
	// Timezone là tên IANA, vd "Asia/Ho_Chi_Minh". Rỗng = giữ/mặc định.
	Timezone string `json:"timezone"`
	// ActiveFrom/ActiveTo: cả hai hoặc không cái nào. Bỏ trống = quét 24/7.
	ActiveFromMin *int16 `json:"active_from_min" binding:"omitempty,min=0,max=1439"`
	ActiveToMin   *int16 `json:"active_to_min" binding:"omitempty,min=0,max=1439"`
	// ActiveWeekdays: 0 = Chủ nhật … 6 = Thứ bảy. Rỗng = mọi ngày.
	ActiveWeekdays []int16 `json:"active_weekdays" binding:"omitempty,dive,min=0,max=6"`
	// FixedTimesMin: giờ chạy cố định — chỉ Danh sách Định kỳ dùng.
	FixedTimesMin []int16 `json:"fixed_times_min" binding:"omitempty,dive,min=0,max=1439"`
	// ClearWindow: xoá khung giờ đã đặt, quay lại 24/7. Cần cờ riêng vì trong
	// JSON, thiếu trường vừa có nghĩa "không sửa" vừa có nghĩa "xoá".
	ClearWindow bool `json:"clear_window"`
}

func (r *scheduleRequest) schedule() domain.ChannelSchedule {
	if r == nil {
		return domain.ChannelSchedule{}
	}
	return domain.ChannelSchedule{
		Timezone:   r.Timezone,
		FromMin:    r.ActiveFromMin,
		ToMin:      r.ActiveToMin,
		Weekdays:   r.ActiveWeekdays,
		FixedTimes: r.FixedTimesMin,
	}
}

// scheduleUpdate: nil = không đụng tới lịch.
func (r *scheduleRequest) scheduleUpdate() (*domain.ChannelSchedule, bool) {
	if r == nil {
		return nil, false
	}
	sched := r.schedule()
	return &sched, r.ClearWindow
}

// clearMaxPosts dịch giá trị max_posts_per_run nhận từ client thành cặp
// (giá trị, cờ xoá).
//
// 0 nghĩa là "không giới hạn", mà cột trong DB diễn đạt điều đó bằng NULL —
// CHECK constraint không cho phép lưu số 0. Không dịch ở đây thì client gửi 0
// sẽ nhận về lỗi ràng buộc của Postgres thay vì bỏ được cái trần.
func clearMaxPosts(v *int32) (*int32, bool) {
	if v == nil {
		return nil, false
	}
	if *v <= 0 {
		return nil, true
	}
	return v, false
}

type List struct {
	svc *service.List
}

func NewList(svc *service.List) *List { return &List{svc: svc} }

// ---------------------------------------------------------------------------
// Breaking
// ---------------------------------------------------------------------------

type createBreakingRequest struct {
	SourceURL string `json:"source_url" binding:"required,url"`
	// Nhận cả từ khoá/hashtag thô, backend chuẩn hoá từng phần tử về regex.
	// Nhiều pattern kết hợp OR: khớp 1 pattern là bắt bài.
	RegexPatterns []string   `json:"regex_patterns" binding:"required,min=1,dive,required"`
	CollectMode   string     `json:"collect_mode" binding:"required,oneof=A B C"`
	PromptID      *uuid.UUID `json:"prompt_id"`
	Language      string     `json:"language_default"`
	AutoProcess   *bool      `json:"auto_process"`
	AutoPublish   *bool      `json:"auto_publish"`
	Status        string     `json:"status" binding:"omitempty,oneof=active paused"`
	// Tham số quét — bỏ trống thì dùng chỉ số tối ưu của hệ thống.
	ScanLimit    *int32  `json:"scan_limit" binding:"omitempty,min=1,max=200"`
	ScanInterval *string `json:"scan_interval"`
	// BackfillLimit: số bài CŨ lấy về ở vòng quét đầu. Bỏ trống = 0 = chỉ lấy
	// bài đăng sau khi thêm kênh.
	BackfillLimit *int32 `json:"backfill_limit" binding:"omitempty,min=0,max=200"`
	// MaxPostsPerRun: trần Bài Post tạo ra mỗi vòng quét. 0 = không giới hạn.
	MaxPostsPerRun *int32 `json:"max_posts_per_run" binding:"omitempty,min=0"`
	// LLMAPISetID: bộ API key cho mode C. Quét tự động không có ai bấm nút để
	// chọn bộ, nên bộ phải nằm sẵn trên kênh.
	LLMAPISetID *uuid.UUID       `json:"llm_api_set_id"`
	Schedule    *scheduleRequest `json:"schedule"`
}

func (h *List) CreateBreaking(c *gin.Context) {
	var req createBreakingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	in := service.BreakingInput{
		SourceURL:      req.SourceURL,
		RegexPatterns:  req.RegexPatterns,
		CollectMode:    domain.CollectMode(req.CollectMode),
		PromptID:       req.PromptID,
		Language:       req.Language,
		AutoProcess:    req.AutoProcess,
		AutoPublish:    req.AutoPublish,
		Status:         req.Status,
		ScanLimit:      req.ScanLimit,
		BackfillLimit:  req.BackfillLimit,
		MaxPostsPerRun: req.MaxPostsPerRun,
		LLMAPISetID:    req.LLMAPISetID,
		Schedule:       req.Schedule.schedule(),
	}
	if req.ScanInterval != nil {
		d, err := parseDuration(*req.ScanInterval)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		in.ScanInterval = &d
	}

	list, err := h.svc.CreateBreaking(c.Request.Context(), middleware.ActorID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, list)
}

func (h *List) ListBreaking(c *gin.Context) {
	limit, offset := pagination(c)
	createdBy, err := queryUUID(c, "created_by")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	sortCol, dir := sorting(c)
	items, total, err := h.svc.ListBreaking(c.Request.Context(), service.ChannelFilter{
		Status:    queryString(c, "status"),
		Search:    queryString(c, "search"),
		Platform:  queryString(c, "platform"),
		CreatedBy: createdBy,
		Sort:      sortCol,
		Dir:       dir,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.Page[repository.ListListBreakingsRow]{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

func (h *List) GetBreaking(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	list, err := h.svc.GetBreaking(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, list)
}

type updateBreakingRequest struct {
	SourceURL     *string    `json:"source_url" binding:"omitempty,url"`
	RegexPatterns []string   `json:"regex_patterns" binding:"omitempty,dive,required"`
	CollectMode   *string    `json:"collect_mode" binding:"omitempty,oneof=A B C"`
	PromptID      *uuid.UUID `json:"prompt_id"`
	Language      *string    `json:"language_default"`
	AutoProcess   *bool      `json:"auto_process"`
	AutoPublish   *bool      `json:"auto_publish"`
	Status        *string    `json:"status" binding:"omitempty,oneof=active paused"`
	ScanLimit     *int32     `json:"scan_limit" binding:"omitempty,min=1,max=200"`
	ScanInterval  *string    `json:"scan_interval"`
	BackfillLimit *int32     `json:"backfill_limit" binding:"omitempty,min=0,max=200"`
	// MaxPostsPerRun = 0 nghĩa là BỎ trần (không giới hạn) — xem clearMaxPosts.
	MaxPostsPerRun *int32           `json:"max_posts_per_run" binding:"omitempty,min=0"`
	LLMAPISetID    *uuid.UUID       `json:"llm_api_set_id"`
	Schedule       *scheduleRequest `json:"schedule"`
}

func (h *List) UpdateBreaking(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req updateBreakingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	in := service.BreakingUpdate{
		SourceURL:     req.SourceURL,
		RegexPatterns: req.RegexPatterns,
		CollectMode:   req.CollectMode,
		PromptID:      req.PromptID,
		Language:      req.Language,
		AutoProcess:   req.AutoProcess,
		AutoPublish:   req.AutoPublish,
		Status:        req.Status,
		ScanLimit:     req.ScanLimit,
		BackfillLimit: req.BackfillLimit,
		LLMAPISetID:   req.LLMAPISetID,
	}
	in.MaxPostsPerRun, in.ClearMaxPosts = clearMaxPosts(req.MaxPostsPerRun)
	in.Schedule, in.ClearWindow = req.Schedule.scheduleUpdate()
	if req.ScanInterval != nil {
		d, err := parseDuration(*req.ScanInterval)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		in.ScanInterval = &d
	}

	list, err := h.svc.UpdateBreaking(c.Request.Context(), middleware.ActorID(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, list)
}

func (h *List) DeleteBreaking(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteBreaking(c.Request.Context(), middleware.ActorID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// runBreaking trigger 1 vòng quét thủ công để test cấu hình regex.
func (h *List) RunBreaking(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.RunBreaking(c.Request.Context(), middleware.ActorID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued", "list_breaking_id": id})
}

// ---------------------------------------------------------------------------
// Scheduled
// ---------------------------------------------------------------------------

type createScheduledRequest struct {
	SourceURL   string     `json:"source_url" binding:"required,url"`
	CollectMode string     `json:"collect_mode" binding:"required,oneof=A B C"`
	PromptID    *uuid.UUID `json:"prompt_id"`
	// Dạng Go duration ("30m", "6h") hoặc số giây.
	ScanFrequency string `json:"scan_frequency" binding:"required"`
	Language      string `json:"language_default"`
	AutoProcess   *bool  `json:"auto_process"`
	AutoPublish   *bool  `json:"auto_publish"`
	Status        string `json:"status" binding:"omitempty,oneof=active paused"`
	// Tham số quét — bỏ trống thì dùng chỉ số tối ưu của hệ thống.
	ScanLimit      *int32 `json:"scan_limit" binding:"omitempty,min=1,max=200"`
	MaxPostsPerRun *int32 `json:"max_posts_per_run" binding:"omitempty,min=0"`
	// BackfillLimit: số bài CŨ lấy về ở vòng quét đầu. Bỏ trống = 0 = chỉ lấy
	// bài đăng sau khi thêm kênh.
	BackfillLimit *int32           `json:"backfill_limit" binding:"omitempty,min=0,max=200"`
	LLMAPISetID   *uuid.UUID       `json:"llm_api_set_id"`
	Schedule      *scheduleRequest `json:"schedule"`
}

func (h *List) CreateScheduled(c *gin.Context) {
	var req createScheduledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	freq, err := parseDuration(req.ScanFrequency)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	list, err := h.svc.CreateScheduled(c.Request.Context(), middleware.ActorID(c), service.ScheduledInput{
		SourceURL:      req.SourceURL,
		CollectMode:    domain.CollectMode(req.CollectMode),
		PromptID:       req.PromptID,
		ScanFrequency:  freq,
		Language:       req.Language,
		AutoProcess:    req.AutoProcess,
		AutoPublish:    req.AutoPublish,
		Status:         req.Status,
		ScanLimit:      req.ScanLimit,
		MaxPostsPerRun: req.MaxPostsPerRun,
		BackfillLimit:  req.BackfillLimit,
		LLMAPISetID:    req.LLMAPISetID,
		Schedule:       req.Schedule.schedule(),
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, list)
}

func (h *List) ListScheduled(c *gin.Context) {
	limit, offset := pagination(c)
	createdBy, err := queryUUID(c, "created_by")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	sortCol, dir := sorting(c)
	items, total, err := h.svc.ListScheduled(c.Request.Context(), service.ChannelFilter{
		Status:    queryString(c, "status"),
		Search:    queryString(c, "search"),
		Platform:  queryString(c, "platform"),
		CreatedBy: createdBy,
		Sort:      sortCol,
		Dir:       dir,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.Page[repository.ListListScheduledsRow]{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

func (h *List) GetScheduled(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	list, err := h.svc.GetScheduled(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, list)
}

type updateScheduledRequest struct {
	SourceURL     *string    `json:"source_url" binding:"omitempty,url"`
	CollectMode   *string    `json:"collect_mode" binding:"omitempty,oneof=A B C"`
	PromptID      *uuid.UUID `json:"prompt_id"`
	ScanFrequency *string    `json:"scan_frequency"`
	Language      *string    `json:"language_default"`
	AutoProcess   *bool      `json:"auto_process"`
	AutoPublish   *bool      `json:"auto_publish"`
	Status        *string    `json:"status" binding:"omitempty,oneof=active paused"`

	ScanLimit *int32 `json:"scan_limit" binding:"omitempty,min=1,max=200"`
	// MaxPostsPerRun = 0 nghĩa là BỎ trần (không giới hạn) — xem clearMaxPosts.
	MaxPostsPerRun *int32           `json:"max_posts_per_run" binding:"omitempty,min=0"`
	BackfillLimit  *int32           `json:"backfill_limit" binding:"omitempty,min=0,max=200"`
	LLMAPISetID    *uuid.UUID       `json:"llm_api_set_id"`
	Schedule       *scheduleRequest `json:"schedule"`
}

func (h *List) UpdateScheduled(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req updateScheduledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	in := service.ScheduledUpdate{
		SourceURL:     req.SourceURL,
		CollectMode:   req.CollectMode,
		PromptID:      req.PromptID,
		Language:      req.Language,
		AutoProcess:   req.AutoProcess,
		AutoPublish:   req.AutoPublish,
		Status:        req.Status,
		ScanLimit:     req.ScanLimit,
		BackfillLimit: req.BackfillLimit,
		LLMAPISetID:   req.LLMAPISetID,
	}
	in.MaxPostsPerRun, in.ClearMaxPosts = clearMaxPosts(req.MaxPostsPerRun)
	in.Schedule, in.ClearWindow = req.Schedule.scheduleUpdate()
	if req.ScanFrequency != nil {
		freq, err := parseDuration(*req.ScanFrequency)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		in.ScanFrequency = &freq
	}

	list, err := h.svc.UpdateScheduled(c.Request.Context(), middleware.ActorID(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, list)
}

func (h *List) DeleteScheduled(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteScheduled(c.Request.Context(), middleware.ActorID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}
