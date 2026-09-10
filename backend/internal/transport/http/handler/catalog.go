package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/repository"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/middleware"
)

type Catalog struct {
	svc *service.Catalog
}

func NewCatalog(svc *service.Catalog) *Catalog { return &Catalog{svc: svc} }

// ---------------------------------------------------------------------------
// Prompt mẫu
// ---------------------------------------------------------------------------

type createPromptRequest struct {
	Name    string `json:"name" binding:"required"`
	Content string `json:"content" binding:"required"`
}

func (h *Catalog) CreatePrompt(c *gin.Context) {
	var req createPromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	prompt, err := h.svc.CreatePrompt(c.Request.Context(), middleware.ActorID(c), req.Name, req.Content)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, prompt)
}

func (h *Catalog) ListPrompts(c *gin.Context) {
	limit, offset := pagination(c)
	_, dir := sorting(c)
	items, total, err := h.svc.ListPrompts(c.Request.Context(), limit, offset, dir)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.Page[repository.Prompt]{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

func (h *Catalog) GetPrompt(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	prompt, err := h.svc.GetPrompt(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, prompt)
}

type updatePromptRequest struct {
	Name    *string `json:"name"`
	Content *string `json:"content"`
}

func (h *Catalog) UpdatePrompt(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req updatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	prompt, err := h.svc.UpdatePrompt(c.Request.Context(), id, req.Name, req.Content)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, prompt)
}

func (h *Catalog) DeletePrompt(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeletePrompt(c.Request.Context(), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}
