package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/service"
)

// LLMAPISet — Bộ API key LLM (tab "LLM Model" của màn AI Engine).
//
// Cùng khuôn với AIEngine: mọi endpoint truyền Actor xuống service, vì quyền
// xem/sửa phụ thuộc chủ sở hữu bản ghi chứ không chỉ phụ thuộc route — không
// chặn bằng middleware được.
type LLMAPISet struct {
	svc *service.LLMAPISetService
}

func NewLLMAPISet(svc *service.LLMAPISetService) *LLMAPISet { return &LLMAPISet{svc: svc} }

// llmAPIKeyRequest — 1 key trong bộ. Key thật chỉ đi VÀO, không bao giờ đi ra.
type llmAPIKeyRequest struct {
	Provider string `json:"provider" binding:"required,oneof=gemini openai anthropic"`
	APIKey   string `json:"api_key" binding:"required"`
	// Label phân biệt 2 key cùng nhà ("gemini cá nhân" / "gemini công ty").
	Label string `json:"label"`
	// Priority nhỏ hơn = router thử trước, trong cùng 1 nhà.
	Priority int32 `json:"priority"`
}

type createLLMAPISetRequest struct {
	Name string `json:"name" binding:"required"`
	Note string `json:"note"`
	// VisibleToUsers chỉ admin đặt được — service ép người khác về false.
	VisibleToUsers bool `json:"visible_to_users"`
	// UserIDs: chia sẻ bộ cho ai.
	UserIDs []uuid.UUID `json:"user_ids"`
	// Keys: thêm luôn key ngay lúc tạo, để không phải đi qua 2 hộp thoại cho
	// một việc.
	Keys []llmAPIKeyRequest `json:"keys"`
}

func (h *LLMAPISet) Create(c *gin.Context) {
	var req createLLMAPISetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	keys := make([]service.LLMAPIKeyInput, 0, len(req.Keys))
	for _, k := range req.Keys {
		keys = append(keys, service.LLMAPIKeyInput{
			Provider: k.Provider, APIKey: k.APIKey, Label: k.Label, Priority: k.Priority,
		})
	}

	set, err := h.svc.Create(c.Request.Context(), actorOf(c), service.LLMAPISetCreate{
		Name:           req.Name,
		Note:           req.Note,
		VisibleToUsers: req.VisibleToUsers,
		Users:          req.UserIDs,
		Keys:           keys,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, set)
}

func (h *LLMAPISet) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context(), actorOf(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *LLMAPISet) Get(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	set, err := h.svc.Get(c.Request.Context(), actorOf(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, set)
}

type updateLLMAPISetRequest struct {
	Name           *string `json:"name"`
	Note           *string `json:"note"`
	VisibleToUsers *bool   `json:"visible_to_users"`
	// UserIDs khác nil = thay TOÀN BỘ danh sách chia sẻ: form gửi lên đúng
	// những ai được tick, nên bỏ tick phải là gỡ quyền.
	UserIDs *[]uuid.UUID `json:"user_ids"`
}

func (h *LLMAPISet) Update(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req updateLLMAPISetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	set, err := h.svc.Update(c.Request.Context(), actorOf(c), id, service.LLMAPISetUpdate{
		Name:           req.Name,
		Note:           req.Note,
		VisibleToUsers: req.VisibleToUsers,
		Users:          req.UserIDs,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, set)
}

func (h *LLMAPISet) Delete(c *gin.Context) {
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

// ---------------------------------------------------------------------------
// Key trong bộ
// ---------------------------------------------------------------------------

func (h *LLMAPISet) AddKey(c *gin.Context) {
	setID, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req llmAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	key, err := h.svc.AddKey(c.Request.Context(), actorOf(c), setID, service.LLMAPIKeyInput{
		Provider: req.Provider, APIKey: req.APIKey, Label: req.Label, Priority: req.Priority,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, key)
}

type updateLLMAPIKeyRequest struct {
	Provider *string `json:"provider" binding:"omitempty,oneof=gemini openai anthropic"`
	// APIKey bỏ trống = giữ key cũ. Dán key mới cũng RESET sức khoẻ key: người
	// dùng vào đây vì key hỏng, dán key mới mà vẫn còn cờ "đã tắt" thì router
	// tiếp tục bỏ qua nó và họ không hiểu vì sao.
	APIKey   string  `json:"api_key"`
	Label    *string `json:"label"`
	Priority *int32  `json:"priority"`
}

func (h *LLMAPISet) UpdateKey(c *gin.Context) {
	keyID, err := pathUUID(c, "key_id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req updateLLMAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	key, err := h.svc.UpdateKey(c.Request.Context(), actorOf(c), keyID, service.LLMAPIKeyUpdate{
		Provider: req.Provider, APIKey: req.APIKey, Label: req.Label, Priority: req.Priority,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, key)
}

func (h *LLMAPISet) DeleteKey(c *gin.Context) {
	keyID, err := pathUUID(c, "key_id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteKey(c.Request.Context(), actorOf(c), keyID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}
