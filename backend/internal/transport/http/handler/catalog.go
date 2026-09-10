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

// ---------------------------------------------------------------------------
// AI Engine
// ---------------------------------------------------------------------------

type aiEngineRequest struct {
	Name               string   `json:"name"`
	Provider           string   `json:"provider"`
	SupportedLanguages []string `json:"supported_languages"`
	IsActive           *bool    `json:"is_active"`
}

func (h *Catalog) CreateEngine(c *gin.Context) {
	var req aiEngineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	engine, err := h.svc.CreateAIEngine(c.Request.Context(), service.AIEngineInput{
		Name:               req.Name,
		Provider:           req.Provider,
		SupportedLanguages: req.SupportedLanguages,
		IsActive:           req.IsActive,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, engine)
}

func (h *Catalog) ListEngines(c *gin.Context) {
	items, err := h.svc.ListAIEngines(c.Request.Context(), queryBool(c, "only_active"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Catalog) GetEngine(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	engine, err := h.svc.GetAIEngine(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, engine)
}

func (h *Catalog) UpdateEngine(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req aiEngineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	engine, err := h.svc.UpdateAIEngine(c.Request.Context(), id, service.AIEngineInput{
		Name:               req.Name,
		Provider:           req.Provider,
		SupportedLanguages: req.SupportedLanguages,
		IsActive:           req.IsActive,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, engine)
}

func (h *Catalog) DeleteEngine(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteAIEngine(c.Request.Context(), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}
