package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/middleware"
)

// AIEngine — API key TTS của từng người (A2).
//
// Mọi endpoint đều truyền Actor xuống service: quyền xem/sửa phụ thuộc chủ sở
// hữu bản ghi, không chỉ phụ thuộc route, nên không thể chặn bằng middleware.
type AIEngine struct {
	svc *service.AIEngineService
}

func NewAIEngine(svc *service.AIEngineService) *AIEngine { return &AIEngine{svc: svc} }

func actorOf(c *gin.Context) service.Actor {
	return service.Actor{ID: middleware.ActorID(c), Role: middleware.ActorRole(c)}
}

type createAIEngineRequest struct {
	// APIKey không bao giờ được trả lại trong response — xem service.AIEngineView.
	APIKey string `json:"api_key" binding:"required"`
	// UserIDs: gán key cho ai. Chỉ admin dùng được (service ép người khác về
	// chính họ); rỗng = gán cho chính người đang thao tác.
	UserIDs []uuid.UUID `json:"user_ids"`
}

type updateAIEngineRequest struct {
	// APIKey bỏ trống = giữ key cũ.
	APIKey string `json:"api_key"`
	// UserID: chuyển key sang cho người khác — chỉ admin.
	UserID *uuid.UUID `json:"user_id"`
	// IsActive: bật/tắt key. Bỏ trống = giữ nguyên trạng thái, nên công tắc ở
	// UI gửi đúng một trường này mà không cần gửi kèm key hay chủ sở hữu.
	IsActive *bool `json:"is_active"`
}

// Create trả về DANH SÁCH: admin gán 1 key cho nhiều người thì mỗi người là
// một bản ghi riêng, nên response luôn là mảng cho cả 2 trường hợp.
func (h *AIEngine) Create(c *gin.Context) {
	var req createAIEngineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	items, err := h.svc.Create(c.Request.Context(), actorOf(c), service.AIEngineCreate{
		APIKey: req.APIKey,
		Owners: req.UserIDs,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, gin.H{"items": items})
}

// List: admin thấy key của mọi người (lọc thêm bằng `user_id`), các vai trò
// khác luôn chỉ thấy key của chính mình — service ép, không tin query string.
func (h *AIEngine) List(c *gin.Context) {
	owner, err := queryUUID(c, "user_id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, err := h.svc.List(c.Request.Context(), actorOf(c), owner)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *AIEngine) Get(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	engine, err := h.svc.Get(c.Request.Context(), actorOf(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, engine)
}

func (h *AIEngine) Update(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req updateAIEngineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	engine, err := h.svc.Update(c.Request.Context(), actorOf(c), id, service.AIEngineUpdate{
		APIKey: req.APIKey,
		Owner:  req.UserID,
		Active: req.IsActive,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, engine)
}

func (h *AIEngine) Delete(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), actorOf(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}
