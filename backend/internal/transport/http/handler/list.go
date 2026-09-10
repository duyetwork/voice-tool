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
}

func (h *List) CreateBreaking(c *gin.Context) {
	var req createBreakingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	in := service.BreakingInput{
		SourceURL:     req.SourceURL,
		RegexPatterns: req.RegexPatterns,
		CollectMode:   domain.CollectMode(req.CollectMode),
		PromptID:      req.PromptID,
		Language:      req.Language,
		AutoProcess:   req.AutoProcess,
		AutoPublish:   req.AutoPublish,
		Status:        req.Status,
		ScanLimit:     req.ScanLimit,
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
	}
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

	ScanLimit      *int32 `json:"scan_limit" binding:"omitempty,min=1,max=200"`
	MaxPostsPerRun *int32 `json:"max_posts_per_run" binding:"omitempty,min=0"`
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
		SourceURL:      req.SourceURL,
		CollectMode:    req.CollectMode,
		PromptID:       req.PromptID,
		Language:       req.Language,
		AutoProcess:    req.AutoProcess,
		AutoPublish:    req.AutoPublish,
		Status:         req.Status,
		ScanLimit:      req.ScanLimit,
		MaxPostsPerRun: req.MaxPostsPerRun,
	}
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
