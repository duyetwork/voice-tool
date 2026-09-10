package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/repository"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/middleware"
)

type SourcePost struct {
	svc *service.SourcePost
}

func NewSourcePost(svc *service.SourcePost) *SourcePost { return &SourcePost{svc: svc} }

type createSourcePostRequest struct {
	// Text gõ tay KHÔNG đi qua đây mà đi thẳng `POST /voices`: nó không có bài
	// gốc nào để Bài Post truy vết về.
	SourceURL   string     `json:"source_url" binding:"required,url"`
	CollectMode string     `json:"collect_mode" binding:"required,oneof=A B C"`
	PromptID    *uuid.UUID `json:"prompt_id"`
	Language    string     `json:"language"`
	// Platform tuỳ chọn: để trống thì hệ thống tự nhận diện từ URL.
	Platform string `json:"platform"`
	// AutoProcess mặc định bật cho F1 (specs -1): nhập URL là có voice ngay.
	AutoProcess *bool `json:"auto_process"`
	// AllowDuplicate: người dùng đã thấy thông báo trùng và chọn vẫn tạo mới.
	AllowDuplicate bool `json:"allow_duplicate"`
}

// duplicateBody trả kèm 409 để UI hiện được bài đã có, thay vì chỉ 1 câu báo
// lỗi mà người dùng không biết bài cũ nằm đâu.
type duplicateBody struct {
	Error    string       `json:"error"`
	Message  string       `json:"message"`
	Existing existingPost `json:"existing"`
}

type existingPost struct {
	ID          string `json:"id"`
	SourceURL   string `json:"source_url"`
	Title       string `json:"title,omitempty"`
	Status      string `json:"status"`
	CollectMode string `json:"collect_mode"`
	CreatedAt   string `json:"created_at"`
	PostID      string `json:"post_id_extracted,omitempty"`
}

func (h *SourcePost) Create(c *gin.Context) {
	var req createSourcePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	autoProcess := true
	if req.AutoProcess != nil {
		autoProcess = *req.AutoProcess
	}

	post, err := h.svc.Create(c.Request.Context(), middleware.ActorID(c), service.CreateInput{
		SourceURL:      req.SourceURL,
		CollectMode:    domain.CollectMode(req.CollectMode),
		PromptID:       req.PromptID,
		Language:       req.Language,
		AutoProcess:    autoProcess,
		Platform:       req.Platform,
		AllowDuplicate: req.AllowDuplicate,
	})
	if err != nil {
		var dup *service.DuplicatePostError
		if errors.As(err, &dup) {
			e := dup.Existing
			c.AbortWithStatusJSON(http.StatusConflict, duplicateBody{
				Error: "duplicate_post",
				Message: "Bài đăng này đã có trong hệ thống — chọn bỏ qua, " +
					"hoặc vẫn tạo Bài Post mới từ cùng nội dung.",
				Existing: existingPost{
					ID:          e.ID.String(),
					SourceURL:   e.SourceUrl,
					Title:       derefOr(e.Title),
					Status:      e.Status,
					CollectMode: e.CollectMode,
					CreatedAt:   e.CreatedAt.Format(time.RFC3339),
					PostID:      derefOr(e.PostIDExtracted),
				},
			})
			return
		}
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, post)
}

func (h *SourcePost) List(c *gin.Context) {
	breakingID, err := queryUUID(c, "list_breaking_id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	scheduledID, err := queryUUID(c, "list_scheduled_id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	createdBy, err := queryUUID(c, "created_by")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	createdFrom, err := queryTime(c, "created_from", false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	createdTo, err := queryTime(c, "created_to", true)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	limit, offset := pagination(c)
	_, dir := sorting(c)

	items, total, err := h.svc.List(c.Request.Context(), service.ListFilter{
		SourceType:      queryString(c, "source_type"),
		Status:          queryString(c, "status"),
		Platform:        queryString(c, "platform"),
		CollectMode:     queryString(c, "collect_mode"),
		Language:        queryString(c, "language"),
		Dir:             dir,
		CreatedBy:       createdBy,
		ListBreakingID:  breakingID,
		ListScheduledID: scheduledID,
		CreatedFrom:     createdFrom,
		CreatedTo:       createdTo,
		Limit:           limit,
		Offset:          offset,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.Page[repository.ListSourcePostsRow]{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

func (h *SourcePost) Get(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	post, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, post)
}

type updateSourcePostRequest struct {
	CollectMode *string    `json:"collect_mode" binding:"omitempty,oneof=A B C"`
	PromptID    *uuid.UUID `json:"prompt_id"`
	Language    *string    `json:"language"`
}

func (h *SourcePost) Update(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req updateSourcePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	post, err := h.svc.Update(c.Request.Context(), middleware.ActorID(c), id, service.UpdateInput{
		CollectMode: req.CollectMode,
		PromptID:    req.PromptID,
		Language:    req.Language,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, post)
}

func (h *SourcePost) Delete(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), middleware.ActorID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// run enqueue job voice:process (không chạy Core Engine trong handler).
func (h *SourcePost) Run(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.Run(c.Request.Context(), middleware.ActorID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(202, gin.H{"status": "queued", "source_post_id": id})
}
